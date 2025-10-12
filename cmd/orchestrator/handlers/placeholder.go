package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/common/bootstrap"
)

// PlaceholderHandler returns "not implemented" responses for endpoints that are planned but not yet built
type PlaceholderHandler struct {
	components *bootstrap.Components
}

// NewPlaceholderHandler creates a new placeholder handler
func NewPlaceholderHandler(components *bootstrap.Components) *PlaceholderHandler {
	return &PlaceholderHandler{
		components: components,
	}
}

// NotImplemented returns a standard "not implemented" response
func (h *PlaceholderHandler) NotImplemented(c echo.Context) error {
	return c.JSON(http.StatusNotImplemented, map[string]interface{}{
		"error": "This endpoint is not yet implemented",
	})
}
