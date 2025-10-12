package handlers

import (
	"context"
	"fmt"
	"net/http"

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

	// Validate tag name (no "/" or "_global_" prefix allowed)
	if errMsg := service.ValidateUserTagName(req.TagName); errMsg != "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": fmt.Sprintf("invalid tag_name: %s", errMsg),
		})
	}

	// Set created_by from username
	req.CreatedBy = username

	// Build internal tag name with namespace (user provides "main" → stored as "alice/main")
	userProvidedTag := req.TagName
	req.TagName = service.BuildInternalTagName(username, userProvidedTag)

	// Use workflow service orchestrator with internal tag name
	resp, err := h.workflowService.CreateWorkflow(ctx, &req)
	if err != nil {
		h.components.Logger.Error("failed to create workflow", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": fmt.Sprintf("failed to create workflow: %v", err),
		})
	}

	// Strip prefix from response (return "main" instead of "alice/main")
	resp.TagName = service.ExtractUserTagName(resp.TagName)

	// Add owner information
	response := map[string]interface{}{
		"artifact_id":  resp.ArtifactID,
		"cas_id":       resp.CASID,
		"version_hash": resp.VersionHash,
		"tag":          resp.TagName, // Clean tag name
		"owner":        username,     // Show ownership
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
	userTag := c.Param("tag") // User provides: "main"

	// Extract username from context (set by middleware)
	username, err := middleware.RequireUsername(c)
	if err != nil {
		return err
	}

	// Parse materialize flag (default: false)
	materializeParam := c.QueryParam("materialize")
	materialize := materializeParam == "true"

	if userTag == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "tag name is required",
		})
	}

	// Validate tag name
	if errMsg := service.ValidateUserTagName(userTag); errMsg != "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": fmt.Sprintf("invalid tag name: %s", errMsg),
		})
	}

	// Build internal tag name (user provides "main" → lookup "alice/main")
	internalTag := service.BuildInternalTagName(username, userTag)

	// Fetch workflow components with internal tag name
	components, err := h.workflowService.GetWorkflowComponents(ctx, internalTag)
	if err != nil {
		h.components.Logger.Error("failed to get workflow components", "tag", userTag, "error", err)
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"error": "workflow not found",
		})
	}

	// Build response with clean tag name
	response := h.buildWorkflowResponse(userTag, username, components)

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
func (h *WorkflowHandler) buildWorkflowResponse(userTag, owner string, components *models.WorkflowComponents) map[string]interface{} {
	response := map[string]interface{}{
		"tag":         userTag, // Clean tag name (e.g., "main")
		"owner":       owner,   // Show ownership (e.g., "alice")
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

	// Strip prefixes from response
	workflows := make([]map[string]interface{}, len(tags))
	for i, tag := range tags {
		workflows[i] = map[string]interface{}{
			"tag":         service.ExtractUserTagName(tag.TagName), // Clean name
			"owner":       service.ExtractUsername(tag.TagName),     // Owner (empty for global)
			"target_id":   tag.TargetID,
			"target_kind": tag.TargetKind,
			"version":     tag.Version,
			"moved_at":    tag.MovedAt,
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
	userTag := c.Param("tag") // User provides: "main"

	// Extract username from context
	username, err := middleware.RequireUsername(c)
	if err != nil {
		return err
	}

	if userTag == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "tag name is required",
		})
	}

	// Validate tag name
	if errMsg := service.ValidateUserTagName(userTag); errMsg != "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": fmt.Sprintf("invalid tag name: %s", errMsg),
		})
	}

	// Use DeleteTagWithNamespace for ownership verification
	if err := h.tagService.DeleteTagWithNamespace(ctx, userTag, username); err != nil {
		h.components.Logger.Error("failed to delete workflow", "tag", userTag, "username", username, "error", err)

		// Check if it's an access denied error
		if err.Error() == "access denied: cannot delete tag owned by another user" {
			return c.JSON(http.StatusForbidden, map[string]interface{}{
				"error": "you cannot delete tags owned by other users",
			})
		}

		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "failed to delete workflow",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "workflow tag deleted successfully",
		"tag":     userTag,  // Return clean name
		"owner":   username,
	})
}
