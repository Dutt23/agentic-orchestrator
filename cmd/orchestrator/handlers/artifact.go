package handlers

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/cmd/orchestrator/service"
	"github.com/lyzr/orchestrator/common/bootstrap"
)

// ArtifactHandler handles artifact-related operations
type ArtifactHandler struct {
	components  *bootstrap.Components
	casService  *service.CASService
	artifactSvc *service.ArtifactService
}

// NewArtifactHandler creates a new artifact handler
func NewArtifactHandler(components *bootstrap.Components, casService *service.CASService, artifactSvc *service.ArtifactService) *ArtifactHandler {
	return &ArtifactHandler{
		components:  components,
		casService:  casService,
		artifactSvc: artifactSvc,
	}
}

// GetArtifact retrieves an artifact and its content by ID
// GET /api/v1/artifacts/:id
func (h *ArtifactHandler) GetArtifact(c echo.Context) error {
	artifactIDStr := c.Param("id")

	// Parse UUID
	artifactID, err := uuid.Parse(artifactIDStr)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid artifact_id format")
	}

	h.components.Logger.Info("fetching artifact", "artifact_id", artifactID)

	// Get artifact metadata
	artifact, err := h.artifactSvc.GetByID(c.Request().Context(), artifactID)
	if err != nil {
		h.components.Logger.Error("failed to get artifact", "artifact_id", artifactID, "error", err)
		return echo.NewHTTPError(http.StatusNotFound, "artifact not found")
	}

	// Get content from CAS
	content, err := h.casService.GetContent(c.Request().Context(), artifact.CasID)
	if err != nil {
		h.components.Logger.Error("failed to get artifact content", "artifact_id", artifactID, "cas_id", artifact.CasID, "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to retrieve artifact content")
	}

	h.components.Logger.Info("artifact fetched successfully",
		"artifact_id", artifactID,
		"kind", artifact.Kind,
		"size", len(content))

	// Return artifact metadata and content
	return c.JSON(http.StatusOK, map[string]interface{}{
		"artifact_id": artifact.ArtifactID,
		"kind":        artifact.Kind,
		"cas_id":      artifact.CasID,
		"created_by":  artifact.CreatedBy,
		"created_at":  artifact.CreatedAt,
		"content":     content,
	})
}
