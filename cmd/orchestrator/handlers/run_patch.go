package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/cmd/orchestrator/container"
	"github.com/lyzr/orchestrator/cmd/orchestrator/middleware"
	"github.com/lyzr/orchestrator/cmd/orchestrator/repository"
	"github.com/lyzr/orchestrator/cmd/orchestrator/service"
	"github.com/lyzr/orchestrator/common/bootstrap"
)

// RunPatchHandler handles HTTP requests for run-specific patches
type RunPatchHandler struct {
	components      *bootstrap.Components
	runPatchService *service.RunPatchService
}

// NewRunPatchHandler creates a new run patch handler
func NewRunPatchHandler(c *container.Container) *RunPatchHandler {
	// Initialize run patch repository (not in container yet)
	runPatchRepo := repository.NewRunPatchRepository(c.Components.DB)

	// Initialize run patch service (uses other services from container)
	runPatchService := service.NewRunPatchService(
		runPatchRepo,
		c.RunRepo, // Use RunRepo from container to get run details
		c.CASService,
		c.ArtifactRepo,
		c.Components,
	)

	return &RunPatchHandler{
		components:      c.Components,
		runPatchService: runPatchService,
	}
}

// CreateRunPatch creates a new run-specific patch
// POST /api/v1/runs/:run_id/patches
func (h *RunPatchHandler) CreateRunPatch(c echo.Context) error {
	ctx := c.Request().Context()
	runID := c.Param("run_id")

	if runID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "run_id is required",
		})
	}

	// Extract username from context (set by middleware)
	username, err := middleware.RequireUsername(c)
	if err != nil {
		return err
	}

	// Parse request
	var req struct {
		NodeID      string                   `json:"node_id"` // Optional: which node generated this patch
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

	h.components.Logger.Info("creating run patch",
		"run_id", runID,
		"username", username,
		"node_id", req.NodeID,
		"operations", len(req.Operations))

	// Create run patch
	createReq := &service.CreateRunPatchRequest{
		RunID:       runID,
		NodeID:      req.NodeID,
		Operations:  req.Operations,
		Description: req.Description,
		CreatedBy:   username,
	}

	resp, err := h.runPatchService.CreateRunPatch(ctx, createReq)
	if err != nil {
		h.components.Logger.Error("failed to create run patch", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "failed to create run patch",
		})
	}

	h.components.Logger.Info("run patch created",
		"run_id", runID,
		"artifact_id", resp.ArtifactID,
		"seq", resp.Seq)

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"id":          resp.ID,
		"run_id":      resp.RunID,
		"artifact_id": resp.ArtifactID,
		"cas_id":      resp.CASID,
		"seq":         resp.Seq,
		"op_count":    resp.OpCount,
		"description": resp.Description,
		"created_by":  resp.CreatedBy,
	})
}

// GetRunPatches retrieves all patches for a specific run
// GET /api/v1/runs/:run_id/patches
func (h *RunPatchHandler) GetRunPatches(c echo.Context) error {
	ctx := c.Request().Context()
	runID := c.Param("run_id")

	if runID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "run_id is required",
		})
	}

	// Extract username from context (set by middleware)
	username, err := middleware.RequireUsername(c)
	if err != nil {
		return err
	}

	h.components.Logger.Info("fetching run patches",
		"run_id", runID,
		"username", username)

	// Get patches
	patches, err := h.runPatchService.GetRunPatches(ctx, runID)
	if err != nil {
		h.components.Logger.Error("failed to get run patches", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "failed to get run patches",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"run_id":  runID,
		"patches": patches,
		"count":   len(patches),
	})
}

// GetPatchOperations retrieves operations for a specific patch
// GET /api/v1/runs/:run_id/patches/:cas_id/operations
func (h *RunPatchHandler) GetPatchOperations(c echo.Context) error {
	ctx := c.Request().Context()
	runID := c.Param("run_id")
	casID := c.Param("cas_id")

	if runID == "" || casID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "run_id and cas_id are required",
		})
	}

	// Extract username from context
	username, err := middleware.RequireUsername(c)
	if err != nil {
		return err
	}

	h.components.Logger.Info("fetching patch operations",
		"run_id", runID,
		"cas_id", casID,
		"username", username)

	// Get operations
	operations, err := h.runPatchService.GetPatchOperations(ctx, casID)
	if err != nil {
		h.components.Logger.Error("failed to get patch operations", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "failed to get patch operations",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"run_id":     runID,
		"cas_id":     casID,
		"operations": operations,
	})
}
