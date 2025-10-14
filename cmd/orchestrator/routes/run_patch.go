package routes

import (
	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/cmd/orchestrator/container"
	"github.com/lyzr/orchestrator/cmd/orchestrator/handlers"
	"github.com/lyzr/orchestrator/cmd/orchestrator/middleware"
)

// RegisterRunPatchRoutes registers all run-patch-related routes
func RegisterRunPatchRoutes(e *echo.Echo, c *container.Container) {
	// Create handler using services from container
	h := handlers.NewRunPatchHandler(c)

	// Run patch routes with username extraction middleware
	runs := e.Group("/api/v1/runs")
	runs.Use(middleware.ExtractUsername()) // Extract X-User-ID into context
	{
		runs.POST("/:run_id/patches", h.CreateRunPatch)                         // POST /api/v1/runs/{run_id}/patches
		runs.GET("/:run_id/patches", h.GetRunPatches)                           // GET /api/v1/runs/{run_id}/patches
		runs.GET("/:run_id/patches/:cas_id/operations", h.GetPatchOperations)   // GET /api/v1/runs/{run_id}/patches/{cas_id}/operations
	}
}
