package routes

import (
	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/cmd/orchestrator/handlers"
	"github.com/lyzr/orchestrator/common/bootstrap"
)

// RegisterRunRoutes registers all run-related routes
func RegisterRunRoutes(e *echo.Echo, components *bootstrap.Components) {
	// Create handler with dependencies
	h := handlers.NewRunHandler(components)

	// Run routes
	runs := e.Group("/api/v1/runs")
	{
		runs.POST("", h.SubmitRun)             // POST /api/v1/runs
		runs.GET("/:id", h.GetRun)             // GET /api/v1/runs/{run_id}
		runs.GET("", h.ListRuns)               // GET /api/v1/runs?status=running
		runs.POST("/:id/cancel", h.CancelRun)  // POST /api/v1/runs/{run_id}/cancel
	}

	// Patch routes (for creating patches on workflows)
	patches := e.Group("/api/v1/patches")
	{
		patches.POST("", h.CreatePatch)         // POST /api/v1/patches
		patches.GET("/:id", h.GetPatch)         // GET /api/v1/patches/{patch_id}
	}
}
