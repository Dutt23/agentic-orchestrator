package routes

import (
	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/cmd/orchestrator/container"
	"github.com/lyzr/orchestrator/cmd/orchestrator/handlers"
	"github.com/lyzr/orchestrator/cmd/orchestrator/middleware"
)

// RegisterWorkflowRoutes registers all workflow-related routes
func RegisterWorkflowRoutes(e *echo.Echo, c *container.Container) {
	// Create handler using services from container
	h := handlers.NewWorkflowHandler(c)

	// Workflow routes with username extraction middleware
	wf := e.Group("/api/v1/workflows")
	wf.Use(middleware.ExtractUsername()) // Extract X-User-ID into context
	{
		wf.GET("/:tag", h.GetWorkflow)                       // GET /api/v1/workflows/main
		wf.GET("/:tag/versions/:seq", h.GetWorkflowVersion) // GET /api/v1/workflows/main/versions/3
		wf.POST("", h.CreateWorkflow)                        // POST /api/v1/workflows
		wf.PATCH("/:tag/patch", h.PatchWorkflow)             // PATCH /api/v1/workflows/main/patch
		wf.GET("", h.ListWorkflows)                          // GET /api/v1/workflows
		wf.DELETE("/:tag", h.DeleteWorkflow)                 // DELETE /api/v1/workflows/main
	}
}
