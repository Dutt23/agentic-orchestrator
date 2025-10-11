package routes

import (
	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/cmd/orchestrator/handlers"
	"github.com/lyzr/orchestrator/common/bootstrap"
)

// RegisterTagRoutes registers all tag-related routes (Git-like branching)
func RegisterTagRoutes(e *echo.Echo, components *bootstrap.Components) {
	// Create handler with dependencies
	h := handlers.NewTagHandler(components)

	// Tag routes
	tags := e.Group("/api/v1/tags")
	{
		tags.GET("", h.ListTags)                  // GET /api/v1/tags
		tags.GET("/:name", h.GetTag)              // GET /api/v1/tags/main
		tags.POST("/:name/move", h.MoveTag)       // POST /api/v1/tags/main/move
		tags.POST("/:name/undo", h.UndoTag)       // POST /api/v1/tags/main/undo
		tags.POST("/:name/redo", h.RedoTag)       // POST /api/v1/tags/main/redo
		tags.GET("/:name/history", h.GetHistory)  // GET /api/v1/tags/main/history
	}
}
