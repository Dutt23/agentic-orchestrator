package routes

import (
	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/cmd/orchestrator/handlers"
	"github.com/lyzr/orchestrator/common/bootstrap"
)

// RegisterRunRoutes registers run and patch routes (planned, not yet implemented)
func RegisterRunRoutes(e *echo.Echo, components *bootstrap.Components) {
	h := handlers.NewPlaceholderHandler(components)

	// Run routes (not yet implemented)
	runs := e.Group("/api/v1/runs")
	{
		runs.POST("", h.NotImplemented)                // POST /api/v1/runs
		runs.GET("/:id", h.NotImplemented)             // GET /api/v1/runs/{run_id}
		runs.GET("", h.NotImplemented)                 // GET /api/v1/runs?status=running
		runs.POST("/:id/cancel", h.NotImplemented)     // POST /api/v1/runs/{run_id}/cancel
	}

	// Patch routes (not yet implemented)
	patches := e.Group("/api/v1/patches")
	{
		patches.POST("", h.NotImplemented)             // POST /api/v1/patches
		patches.GET("/:id", h.NotImplemented)          // GET /api/v1/patches/{patch_id}
	}
}
