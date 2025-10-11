package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/cmd/orchestrator/repository"
	"github.com/lyzr/orchestrator/cmd/orchestrator/service"
	"github.com/lyzr/orchestrator/common/bootstrap"
)

// WorkflowHandler handles workflow requests using multiple focused services
type WorkflowHandler struct {
	components *bootstrap.Components

	// Individual services
	casService      *service.CASService
	artifactService *service.ArtifactService
	tagService      *service.TagService

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

	// Initialize lightweight workflow orchestrator
	workflowService := service.NewWorkflowServiceV2(casService, artifactService, tagService, components.Logger)

	return &WorkflowHandler{
		components:      components,
		casService:      casService,
		artifactService: artifactService,
		tagService:      tagService,
		workflowService: workflowService,
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

// GetWorkflow retrieves a workflow by tag name
// GET /api/v1/workflows/:tag
func (h *WorkflowHandler) GetWorkflow(c echo.Context) error {
	ctx := c.Request().Context()
	tagName := c.Param("tag")

	if tagName == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "tag name is required",
		})
	}

	// Option 1: Use workflow service
	workflow, err := h.workflowService.GetWorkflowByTag(ctx, tagName)
	if err != nil {
		h.components.Logger.Error("failed to get workflow", "tag", tagName, "error", err)
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"error": "workflow not found",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"tag":      tagName,
		"workflow": workflow,
	})
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
