package routes

import (
	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/cmd/orchestrator/container"
	"github.com/lyzr/orchestrator/cmd/orchestrator/handlers"
)

// RegisterTagRoutes registers tag-related routes (planned, not yet implemented)
// Note: Tag operations are currently handled through /api/v1/workflows endpoints
func RegisterTagRoutes(e *echo.Echo, c *container.Container) {
	h := handlers.NewPlaceholderHandler(c.Components)

	// Tag routes (not yet implemented)
	tags := e.Group("/api/v1/tags")
	{
		tags.GET("", h.NotImplemented)                  // GET /api/v1/tags
		tags.GET("/:name", h.NotImplemented)            // GET /api/v1/tags/main
		tags.POST("/:name/move", h.NotImplemented)      // POST /api/v1/tags/main/move
		tags.POST("/:name/undo", h.NotImplemented)      // POST /api/v1/tags/main/undo
		tags.POST("/:name/redo", h.NotImplemented)      // POST /api/v1/tags/main/redo
		tags.GET("/:name/history", h.NotImplemented)    // GET /api/v1/tags/main/history
	}
}
