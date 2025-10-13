package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/cmd/orchestrator/middleware"
	"github.com/lyzr/orchestrator/cmd/orchestrator/models"
	"github.com/lyzr/orchestrator/cmd/orchestrator/repository"
	"github.com/lyzr/orchestrator/cmd/orchestrator/service"
	"github.com/lyzr/orchestrator/common/bootstrap"
)

// WorkflowHandler handles workflow requests
type WorkflowHandler struct {
	components          *bootstrap.Components
	tagService          *service.TagService
	materializerService *service.MaterializerService
	workflowService     *service.WorkflowServiceV2
}

// NewWorkflowHandler creates a new workflow handler
func NewWorkflowHandler(components *bootstrap.Components) *WorkflowHandler {
	// Initialize repositories
	casBlobRepo := repository.NewCASBlobRepository(components.DB)
	artifactRepo := repository.NewArtifactRepository(components.DB)
	tagRepo := repository.NewTagRepository(components.DB)

	// Initialize services
	casService := service.NewCASService(casBlobRepo, components.Logger)
	artifactService := service.NewArtifactService(artifactRepo, components.Logger)
	tagService := service.NewTagService(tagRepo, components.Logger)
	materializerService := service.NewMaterializerService(components.Logger)
	workflowService := service.NewWorkflowServiceV2(casService, artifactService, tagService, components.Logger)

	return &WorkflowHandler{
		components:          components,
		tagService:          tagService,
		materializerService: materializerService,
		workflowService:     workflowService,
	}
}

// CreateWorkflow creates a new workflow (DAG version)
// POST /api/v1/workflows
// This handler can either:
// 1. Use the lightweight workflowService orchestrator (current implementation)
// 2. Orchestrate services directly in the controller (alternative shown below)
func (h *WorkflowHandler) CreateWorkflow(c echo.Context) error {
	ctx := c.Request().Context()

	// Extract username from context (set by middleware)
	username, err := middleware.RequireUsername(c)
	if err != nil {
		return err // Already returns JSON error response
	}

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

	// Validate tag name
	if errMsg := service.ValidateUserTagName(req.TagName); errMsg != "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": fmt.Sprintf("invalid tag_name: %s", errMsg),
		})
	}

	// Set created_by from username
	req.CreatedBy = username
	// Set username for tag namespace
	req.Username = username

	// Use workflow service orchestrator
	resp, err := h.workflowService.CreateWorkflow(ctx, &req)
	if err != nil {
		h.components.Logger.Error("failed to create workflow", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": fmt.Sprintf("failed to create workflow: %v", err),
		})
	}

	// Build response
	response := map[string]interface{}{
		"artifact_id":  resp.ArtifactID,
		"cas_id":       resp.CASID,
		"version_hash": resp.VersionHash,
		"tag":          resp.TagName,    // Tag name (e.g., "main")
		"owner":        resp.Username,   // Owner (e.g., "sdutt")
		"nodes_count":  resp.NodesCount,
		"edges_count":  resp.EdgesCount,
		"created_at":   resp.CreatedAt,
	}

	return c.JSON(http.StatusCreated, response)
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
	tagNameEncoded := c.Param("tag") // User provides: "main" or "release%2Fv1.0"

	// URL-decode the tag name (Echo doesn't decode path parameters automatically)
	tagName, err := url.QueryUnescape(tagNameEncoded)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "invalid tag name encoding",
		})
	}

	// Extract username from context (set by middleware)
	username, err := middleware.RequireUsername(c)
	if err != nil {
		return err
	}

	h.components.Logger.Info("GetWorkflow called", "username", username, "tag", tagName)

	// Parse materialize flag (default: false)
	materializeParam := c.QueryParam("materialize")
	materialize := materializeParam == "true"

	if tagName == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "tag name is required",
		})
	}

	// Validate tag name
	if errMsg := service.ValidateUserTagName(tagName); errMsg != "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": fmt.Sprintf("invalid tag name: %s", errMsg),
		})
	}

	// Fetch workflow components (pass username and tagName separately)
	components, err := h.workflowService.GetWorkflowComponents(ctx, username, tagName)
	if err != nil {
		h.components.Logger.Error("failed to get workflow components", "username", username, "tag", tagName, "error", err)
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"error": "workflow not found",
		})
	}

	// Build response
	response := h.buildWorkflowResponse(tagName, username, components)

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
func (h *WorkflowHandler) buildWorkflowResponse(tagName, owner string, components *models.WorkflowComponents) map[string]interface{} {
	response := map[string]interface{}{
		"tag":         tagName,            // Tag name (e.g., "main")
		"owner":       owner,              // Owner username (e.g., "sdutt")
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

// ListWorkflows lists workflows (tags) for the authenticated user
// GET /api/v1/workflows?scope=user|global|all
//
// Query parameters:
//   - scope: "user" (default), "global", or "all"
//     - user: List only the user's tags
//     - global: List only global (system-wide) tags
//     - all: List user's tags + global tags
func (h *WorkflowHandler) ListWorkflows(c echo.Context) error {
	ctx := c.Request().Context()

	// Extract username from context
	username, err := middleware.RequireUsername(c)
	if err != nil {
		return err
	}

	// Parse scope parameter (default: user)
	scope := c.QueryParam("scope")
	if scope == "" {
		scope = "user"
	}

	var tags []*models.Tag

	// List tags based on scope
	switch scope {
	case "user":
		tags, err = h.tagService.ListUserTags(ctx, username)
	case "global":
		tags, err = h.tagService.ListGlobalTags(ctx)
	case "all":
		tags, err = h.tagService.ListAllAccessibleTags(ctx, username)
	default:
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "invalid scope parameter (must be 'user', 'global', or 'all')",
		})
	}

	if err != nil {
		h.components.Logger.Error("failed to list workflows", "scope", scope, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "failed to list workflows",
		})
	}

	// Build response
	workflows := make([]map[string]interface{}, len(tags))
	for i, tag := range tags {
		workflows[i] = map[string]interface{}{
			"tag":         tag.TagName,    // Tag name (e.g., "main")
			"owner":       tag.Username,   // Owner (e.g., "sdutt" or "_global_")
			"target_id":   tag.TargetID,
			"target_kind": tag.TargetKind,
			"version":     tag.Version,
			"moved_at":    tag.MovedAt,
		}
		if tag.CreatedBy != nil {
			workflows[i]["created_by"] = *tag.CreatedBy
		}
		if tag.MovedBy != nil {
			workflows[i]["moved_by"] = *tag.MovedBy
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"workflows": workflows,
		"count":     len(workflows),
		"scope":     scope,
	})
}

// DeleteWorkflow deletes a workflow tag
// DELETE /api/v1/workflows/:tag
func (h *WorkflowHandler) DeleteWorkflow(c echo.Context) error {
	ctx := c.Request().Context()
	tagNameEncoded := c.Param("tag") // User provides: "main" or "release%2Fv1.0"

	// URL-decode the tag name
	tagName, err := url.QueryUnescape(tagNameEncoded)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "invalid tag name encoding",
		})
	}

	// Extract username from context
	username, err := middleware.RequireUsername(c)
	if err != nil {
		return err
	}

	if tagName == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "tag name is required",
		})
	}

	// Validate tag name
	if errMsg := service.ValidateUserTagName(tagName); errMsg != "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": fmt.Sprintf("invalid tag name: %s", errMsg),
		})
	}

	// Delete tag (ownership is implicit - username is primary key)
	if err := h.tagService.DeleteTag(ctx, username, tagName); err != nil {
		h.components.Logger.Error("failed to delete workflow", "username", username, "tag", tagName, "error", err)

		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "failed to delete workflow",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "workflow tag deleted successfully",
		"tag":     tagName,
		"owner":   username,
	})
}

// GetWorkflowVersion retrieves a workflow at a specific version/sequence number
// GET /api/v1/workflows/:tag/versions/:seq?materialize=false
//
// Path parameters:
//   - tag: Tag name (e.g., "main")
//   - seq: Sequence number (1-indexed, e.g., 3 means apply patches 1,2,3)
//
// Query parameters:
//   - materialize: "true" or "false" (default: "false")
func (h *WorkflowHandler) GetWorkflowVersion(c echo.Context) error {
	ctx := c.Request().Context()
	tagNameEncoded := c.Param("tag")
	seqStr := c.Param("seq")

	// URL-decode the tag name
	tagName, err := url.QueryUnescape(tagNameEncoded)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "invalid tag name encoding",
		})
	}

	// Extract username from context (set by middleware)
	username, err := middleware.RequireUsername(c)
	if err != nil {
		return err
	}

	// Parse materialize flag (default: false)
	materializeParam := c.QueryParam("materialize")
	materialize := materializeParam == "true"

	if tagName == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "tag name is required",
		})
	}

	if seqStr == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "seq is required",
		})
	}

	// Parse seq as integer
	var seq int
	if _, err := fmt.Sscanf(seqStr, "%d", &seq); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "seq must be a valid integer",
		})
	}

	// Validate tag name
	if errMsg := service.ValidateUserTagName(tagName); errMsg != "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": fmt.Sprintf("invalid tag name: %s", errMsg),
		})
	}

	// Fetch workflow components at specific version
	components, err := h.workflowService.GetWorkflowComponentsAtVersion(ctx, username, tagName, seq)
	if err != nil {
		h.components.Logger.Error("failed to get workflow components at version",
			"username", username,
			"tag", tagName,
			"seq", seq,
			"error", err)
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"error": fmt.Sprintf("workflow version not found: %v", err),
		})
	}

	// Build response
	response := h.buildWorkflowResponse(tagName, username, components)
	response["seq"] = seq

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
