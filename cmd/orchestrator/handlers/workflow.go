package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/cmd/orchestrator/models"
	"github.com/lyzr/orchestrator/cmd/orchestrator/repository"
	"github.com/lyzr/orchestrator/cmd/orchestrator/service"
	"github.com/lyzr/orchestrator/common/bootstrap"
)

// WorkflowHandler handles workflow requests using multiple focused services
type WorkflowHandler struct {
	components *bootstrap.Components

	// Individual services
	casService         *service.CASService
	artifactService    *service.ArtifactService
	tagService         *service.TagService
	materializerService *service.MaterializerService

	// Optional: lightweight orchestrator
	workflowService *service.WorkflowServiceV2
}

// NewWorkflowHandler creates a new workflow handler with focused services
func NewWorkflowHandler(components *bootstrap.Components) *WorkflowHandler {
	// Initialize repositories
	casBlobRepo := repository.NewCASBlobRepository(components.DB)
	artifactRepo := repository.NewArtifactRepository(components.DB)
	tagRepo := repository.NewTagRepository(components.DB)

	// Initialize focused services
	casService := service.NewCASService(casBlobRepo, components.Logger)
	artifactService := service.NewArtifactService(artifactRepo, components.Logger)
	tagService := service.NewTagService(tagRepo, components.Logger)
	materializerService := service.NewMaterializerService(components.Logger)

	// Initialize lightweight workflow orchestrator
	workflowService := service.NewWorkflowServiceV2(casService, artifactService, tagService, components.Logger)

	return &WorkflowHandler{
		components:         components,
		casService:         casService,
		artifactService:    artifactService,
		tagService:         tagService,
		materializerService: materializerService,
		workflowService:    workflowService,
	}
}

// CreateWorkflow creates a new workflow (DAG version)
// POST /api/v1/workflows
// This handler can either:
// 1. Use the lightweight workflowService orchestrator (current implementation)
// 2. Orchestrate services directly in the controller (alternative shown below)
func (h *WorkflowHandler) CreateWorkflow(c echo.Context) error {
	ctx := c.Request().Context()

	// Parse and validate request
	var req service.CreateWorkflowRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "invalid request body",
		})
	}

	if req.TagName == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "tag_name is required",
		})
	}

	if req.Workflow == nil || len(req.Workflow) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "workflow is required",
		})
	}

	// Set created_by from header
	if req.CreatedBy == "" {
		req.CreatedBy = c.Request().Header.Get("X-User-ID")
		if req.CreatedBy == "" {
			req.CreatedBy = "anonymous"
		}
	}

	// Option 1: Use workflow service orchestrator (cleaner)
	resp, err := h.workflowService.CreateWorkflow(ctx, &req)
	if err != nil {
		h.components.Logger.Error("failed to create workflow", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": fmt.Sprintf("failed to create workflow: %v", err),
		})
	}

	return c.JSON(http.StatusCreated, resp)
}

// CreateWorkflowDirectOrchestration is an alternative implementation
// that orchestrates services directly in the controller
func (h *WorkflowHandler) CreateWorkflowDirectOrchestration(c echo.Context) error {
	ctx := c.Request().Context()

	// Parse request (same as above)
	var req service.CreateWorkflowRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "invalid request body",
		})
	}

	// Validation
	if req.TagName == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "tag_name is required",
		})
	}

	// Set defaults
	if req.CreatedBy == "" {
		req.CreatedBy = c.Request().Header.Get("X-User-ID")
		if req.CreatedBy == "" {
			req.CreatedBy = "anonymous"
		}
	}

	// Step 1: Store content in CAS
	workflowJSON, err := json.Marshal(req.Workflow)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "invalid workflow structure",
		})
	}

	casID, err := h.casService.StoreContent(ctx, workflowJSON, "application/json;type=dag")
	if err != nil {
		h.components.Logger.Error("failed to store content", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "failed to store workflow content",
		})
	}

	// Step 2: Create or find artifact
	versionHash := casID
	artifact, err := h.artifactService.GetByVersionHash(ctx, versionHash)

	var artifactID string
	if err != nil {
		// Artifact doesn't exist, create it
		nodesCount, edgesCount := service.CountWorkflowElements(req.Workflow) // Would need to export this
		id, err := h.artifactService.CreateDAGVersion(
			ctx,
			casID,
			versionHash,
			req.TagName,
			req.CreatedBy,
			nodesCount,
			edgesCount,
		)
		if err != nil {
			h.components.Logger.Error("failed to create artifact", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]interface{}{
				"error": "failed to create artifact",
			})
		}
		artifactID = id.String()
	} else {
		// Reuse existing artifact
		artifactID = artifact.ArtifactID.String()
	}

	// Step 3: Create or move tag
	if err := h.tagService.CreateOrMoveTag(ctx, req.TagName, "dag_version", artifact.ArtifactID, versionHash, req.CreatedBy); err != nil {
		h.components.Logger.Error("failed to create/move tag", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "failed to create/move tag",
		})
	}

	// Return response
	return c.JSON(http.StatusCreated, map[string]interface{}{
		"artifact_id":  artifactID,
		"cas_id":       casID,
		"version_hash": versionHash,
		"tag_name":     req.TagName,
		"message":      "workflow created successfully",
	})
}

// GetWorkflow retrieves a workflow by tag name with optional materialization
// GET /api/v1/workflows/:tag?materialize=false
//
// Query parameters:
//   - materialize: "true" or "false" (default: "false")
//     If true, returns the fully materialized workflow (base + patches applied)
//     If false, returns components only (base + patch chain metadata)
func (h *WorkflowHandler) GetWorkflow(c echo.Context) error {
	ctx := c.Request().Context()
	tagName := c.Param("tag")

	// Parse materialize flag (default: false)
	materializeParam := c.QueryParam("materialize")
	materialize := materializeParam == "true"

	if tagName == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "tag name is required",
		})
	}

	// Fetch workflow components (always does 4 queries)
	components, err := h.workflowService.GetWorkflowComponents(ctx, tagName)
	if err != nil {
		h.components.Logger.Error("failed to get workflow components", "tag", tagName, "error", err)
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"error": "workflow not found",
		})
	}

	// Build response
	response := h.buildWorkflowResponse(tagName, components)

	// Optionally materialize the workflow
	if materialize {
		if err := h.addMaterializedWorkflow(response, components); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]interface{}{
				"error": fmt.Sprintf("failed to materialize workflow: %v", err),
			})
		}
	} else {
		response["workflow"] = nil
	}

	return c.JSON(http.StatusOK, response)
}

// buildWorkflowResponse constructs the response with metadata and components
func (h *WorkflowHandler) buildWorkflowResponse(tagName string, components *models.WorkflowComponents) map[string]interface{} {
	response := map[string]interface{}{
		"tag":         tagName,
		"artifact_id": components.ArtifactID,
		"kind":        components.Kind,
		"depth":       components.Depth,
		"patch_count": components.PatchCount,
		"created_at":  components.CreatedAt,
	}

	if components.CreatedBy != nil {
		response["created_by"] = *components.CreatedBy
	}

	response["components"] = h.buildComponentDetails(components)

	return response
}

// buildComponentDetails constructs the component details section of the response
func (h *WorkflowHandler) buildComponentDetails(components *models.WorkflowComponents) map[string]interface{} {
	details := map[string]interface{}{
		"base_cas_id":       components.BaseCASID,
		"base_version_hash": components.BaseVersionHash,
	}

	if components.BaseVersion != nil {
		details["base_version"] = *components.BaseVersion
	}

	if len(components.PatchChain) > 0 {
		details["patches"] = h.buildPatchChainMetadata(components.PatchChain)
	}

	return details
}

// buildPatchChainMetadata converts patch chain to metadata format (without content)
func (h *WorkflowHandler) buildPatchChainMetadata(patchChain []models.PatchInfo) []map[string]interface{} {
	patches := make([]map[string]interface{}, 0, len(patchChain))

	for _, patch := range patchChain {
		patchInfo := map[string]interface{}{
			"seq":         patch.Seq,
			"artifact_id": patch.ArtifactID,
			"cas_id":      patch.CASID,
			"depth":       patch.Depth,
			"created_at":  patch.CreatedAt,
		}

		if patch.OpCount != nil {
			patchInfo["op_count"] = *patch.OpCount
		}
		if patch.CreatedBy != nil {
			patchInfo["created_by"] = *patch.CreatedBy
		}

		patches = append(patches, patchInfo)
	}

	return patches
}

// addMaterializedWorkflow materializes the workflow and adds it to the response
func (h *WorkflowHandler) addMaterializedWorkflow(response map[string]interface{}, components *models.WorkflowComponents) error {
	h.components.Logger.Info("materialization requested",
		"tag", components.TagName,
		"kind", components.Kind,
		"depth", components.Depth,
		"patch_count", components.PatchCount,
	)

	// Use MaterializerService to apply patches
	workflow, err := h.materializerService.Materialize(context.Background(), components)
	if err != nil {
		h.components.Logger.Error("materialization failed",
			"tag", components.TagName,
			"error", err,
		)
		return fmt.Errorf("failed to materialize workflow: %w", err)
	}

	response["workflow"] = workflow
	return nil
}

// GetWorkflowDirectOrchestration is an alternative that orchestrates directly
func (h *WorkflowHandler) GetWorkflowDirectOrchestration(c echo.Context) error {
	ctx := c.Request().Context()
	tagName := c.Param("tag")

	// Step 1: Get tag
	tag, err := h.tagService.GetTag(ctx, tagName)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"error": "tag not found",
		})
	}

	// Step 2: Get artifact
	artifact, err := h.artifactService.GetByID(ctx, tag.TargetID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"error": "artifact not found",
		})
	}

	// Step 3: Get content from CAS
	content, err := h.casService.GetContent(ctx, artifact.CasID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "failed to retrieve workflow content",
		})
	}

	// Step 4: Parse and return
	var workflow map[string]interface{}
	if err := json.Unmarshal(content, &workflow); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "failed to parse workflow",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"tag":      tagName,
		"workflow": workflow,
	})
}

// ListWorkflows lists all workflows (tags)
// GET /api/v1/workflows
func (h *WorkflowHandler) ListWorkflows(c echo.Context) error {
	ctx := c.Request().Context()

	// Direct service call - no orchestration needed
	tags, err := h.tagService.ListTags(ctx)
	if err != nil {
		h.components.Logger.Error("failed to list workflows", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "failed to list workflows",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"workflows": tags,
		"count":     len(tags),
	})
}

// DeleteWorkflow deletes a workflow tag
// DELETE /api/v1/workflows/:tag
func (h *WorkflowHandler) DeleteWorkflow(c echo.Context) error {
	ctx := c.Request().Context()
	tagName := c.Param("tag")

	if tagName == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "tag name is required",
		})
	}

	// Direct service call - no orchestration needed
	if err := h.tagService.DeleteTag(ctx, tagName); err != nil {
		h.components.Logger.Error("failed to delete workflow", "tag", tagName, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "failed to delete workflow",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "workflow tag deleted successfully",
		"tag":     tagName,
	})
}
