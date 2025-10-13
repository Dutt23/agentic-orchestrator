package routes

import (
	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/cmd/orchestrator/handlers"
	"github.com/lyzr/orchestrator/cmd/orchestrator/middleware"
	"github.com/lyzr/orchestrator/common/bootstrap"
)

// RegisterWorkflowRoutes registers all workflow-related routes
func RegisterWorkflowRoutes(e *echo.Echo, components *bootstrap.Components) {
	// Create handler with dependencies
	h := handlers.NewWorkflowHandler(components)

	// Workflow routes with username extraction middleware
	wf := e.Group("/api/v1/workflows")
	wf.Use(middleware.ExtractUsername()) // Extract X-User-ID into context
	{
		wf.GET("/:tag", h.GetWorkflow)                       // GET /api/v1/workflows/main
		wf.GET("/:tag/versions/:seq", h.GetWorkflowVersion) // GET /api/v1/workflows/main/versions/3
		wf.POST("", h.CreateWorkflow)                        // POST /api/v1/workflows
		wf.GET("", h.ListWorkflows)                          // GET /api/v1/workflows
		wf.DELETE("/:tag", h.DeleteWorkflow)                 // DELETE /api/v1/workflows/main
	}
}
