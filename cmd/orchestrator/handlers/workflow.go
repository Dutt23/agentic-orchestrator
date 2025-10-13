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

// PatchWorkflow applies JSON Patch operations to create a new workflow version
// PATCH /api/v1/workflows/:tag/patch
func (h *WorkflowHandler) PatchWorkflow(c echo.Context) error {
	ctx := c.Request().Context()
	tagNameEncoded := c.Param("tag")

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

	// Parse request body
	var req struct {
		Operations  []map[string]interface{} `json:"operations"`
		Description string                   `json:"description"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "invalid request body",
		})
	}

	if len(req.Operations) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "operations array is required and cannot be empty",
		})
	}

	h.components.Logger.Info("patch workflow request",
		"username", username,
		"tag", tagName,
		"operation_count", len(req.Operations),
		"description", req.Description)

	// Validate tag name
	if errMsg := service.ValidateUserTagName(tagName); errMsg != "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": fmt.Sprintf("invalid tag name: %s", errMsg),
		})
	}

	// Get current workflow with full materialization
	components, err := h.workflowService.GetWorkflowComponents(ctx, username, tagName)
	if err != nil {
		h.components.Logger.Error("failed to get workflow for patching",
			"username", username,
			"tag", tagName,
			"error", err)
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"error": "workflow not found",
		})
	}

	// Materialize current workflow to apply patches
	currentWorkflow, err := h.materializerService.Materialize(ctx, components)
	if err != nil {
		h.components.Logger.Error("failed to materialize workflow for patching",
			"username", username,
			"tag", tagName,
			"error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "failed to load current workflow",
		})
	}

	// Validate patch operations by trying to apply them
	_, err = h.applyJSONPatchToWorkflow(currentWorkflow, req.Operations)
	if err != nil {
		h.components.Logger.Warn("failed to validate patch operations",
			"username", username,
			"tag", tagName,
			"error", err)
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": fmt.Sprintf("invalid patch operations: %v", err),
		})
	}

	// Create patch artifact (stores operations, not the full patched workflow)
	patchReq := &service.CreatePatchRequest{
		Username:    username,
		TagName:     tagName,
		Operations:  req.Operations,
		Description: req.Description,
		CreatedBy:   username,
	}

	resp, err := h.workflowService.CreatePatch(ctx, patchReq)
	if err != nil {
		h.components.Logger.Error("failed to create patch",
			"username", username,
			"tag", tagName,
			"error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": fmt.Sprintf("failed to save patch: %v", err),
		})
	}

	h.components.Logger.Info("patch created successfully",
		"username", username,
		"tag", tagName,
		"artifact_id", resp.ArtifactID,
		"depth", resp.Depth,
		"op_count", resp.OpCount)

	// Build response
	response := map[string]interface{}{
		"artifact_id": resp.ArtifactID,
		"cas_id":      resp.CASID,
		"depth":       resp.Depth,
		"op_count":    resp.OpCount,
		"tag":         resp.TagName,
		"owner":       resp.Username,
		"description": req.Description,
		"created_at":  resp.CreatedAt,
	}

	return c.JSON(http.StatusOK, response)
}

// applyJSONPatchToWorkflow applies JSON Patch operations to a workflow
func (h *WorkflowHandler) applyJSONPatchToWorkflow(workflow map[string]interface{}, operations []map[string]interface{}) (map[string]interface{}, error) {
	// Create a copy of the workflow to avoid modifying the original
	patchedWorkflow := make(map[string]interface{})
	for k, v := range workflow {
		patchedWorkflow[k] = v
	}

	// Apply each operation
	for i, op := range operations {
		opType, ok := op["op"].(string)
		if !ok {
			return nil, fmt.Errorf("operation %d missing 'op' field", i)
		}

		path, ok := op["path"].(string)
		if !ok {
			return nil, fmt.Errorf("operation %d missing 'path' field", i)
		}

		switch opType {
		case "add":
			if err := h.applyAddOperation(patchedWorkflow, path, op["value"]); err != nil {
				return nil, fmt.Errorf("operation %d (add) failed: %w", i, err)
			}

		case "remove":
			if err := h.applyRemoveOperation(patchedWorkflow, path); err != nil {
				return nil, fmt.Errorf("operation %d (remove) failed: %w", i, err)
			}

		case "replace":
			if err := h.applyReplaceOperation(patchedWorkflow, path, op["value"]); err != nil {
				return nil, fmt.Errorf("operation %d (replace) failed: %w", i, err)
			}

		default:
			return nil, fmt.Errorf("unsupported operation type: %s", opType)
		}
	}

	return patchedWorkflow, nil
}

// applyAddOperation handles "add" operations
func (h *WorkflowHandler) applyAddOperation(workflow map[string]interface{}, path string, value interface{}) error {
	if path == "/nodes/-" {
		// Add node to the end of nodes array
		nodes, ok := workflow["nodes"].([]interface{})
		if !ok {
			workflow["nodes"] = []interface{}{value}
			return nil
		}
		workflow["nodes"] = append(nodes, value)
		return nil
	}

	if path == "/edges/-" {
		// Add edge to the end of edges array
		edges, ok := workflow["edges"].([]interface{})
		if !ok {
			workflow["edges"] = []interface{}{value}
			return nil
		}
		workflow["edges"] = append(edges, value)
		return nil
	}

	return fmt.Errorf("unsupported add path: %s", path)
}

// applyRemoveOperation handles "remove" operations
func (h *WorkflowHandler) applyRemoveOperation(workflow map[string]interface{}, path string) error {
	// Parse path like "/nodes/2" or "/edges/1"
	var collection string
	var index int

	if n, err := fmt.Sscanf(path, "/%[^/]/%d", &collection, &index); err != nil || n != 2 {
		return fmt.Errorf("invalid remove path format: %s", path)
	}

	if collection == "nodes" {
		nodes, ok := workflow["nodes"].([]interface{})
		if !ok || index < 0 || index >= len(nodes) {
			return fmt.Errorf("invalid node index: %d", index)
		}
		workflow["nodes"] = append(nodes[:index], nodes[index+1:]...)
		return nil
	}

	if collection == "edges" {
		edges, ok := workflow["edges"].([]interface{})
		if !ok || index < 0 || index >= len(edges) {
			return fmt.Errorf("invalid edge index: %d", index)
		}
		workflow["edges"] = append(edges[:index], edges[index+1:]...)
		return nil
	}

	return fmt.Errorf("unsupported remove collection: %s", collection)
}

// applyReplaceOperation handles "replace" operations
func (h *WorkflowHandler) applyReplaceOperation(workflow map[string]interface{}, path string, value interface{}) error {
	// Parse path like "/nodes/2" or "/edges/1"
	var collection string
	var index int

	if n, err := fmt.Sscanf(path, "/%[^/]/%d", &collection, &index); err != nil || n != 2 {
		return fmt.Errorf("invalid replace path format: %s", path)
	}

	if collection == "nodes" {
		nodes, ok := workflow["nodes"].([]interface{})
		if !ok || index < 0 || index >= len(nodes) {
			return fmt.Errorf("invalid node index: %d", index)
		}
		nodes[index] = value
		return nil
	}

	if collection == "edges" {
		edges, ok := workflow["edges"].([]interface{})
		if !ok || index < 0 || index >= len(edges) {
			return fmt.Errorf("invalid edge index: %d", index)
		}
		edges[index] = value
		return nil
	}

	return fmt.Errorf("unsupported replace collection: %s", collection)
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
