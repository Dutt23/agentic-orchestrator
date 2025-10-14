package routes

import (
	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/cmd/orchestrator/handlers"
)

// RegisterArtifactRoutes registers artifact-related routes
func RegisterArtifactRoutes(e *echo.Group, handler *handlers.ArtifactHandler) {
	// GET /api/v1/artifacts/:id - Get artifact by ID with content
	e.GET("/artifacts/:id", handler.GetArtifact)
}
