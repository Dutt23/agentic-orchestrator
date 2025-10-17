package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/common/bootstrap"
	"github.com/lyzr/orchestrator/common/clients"
)

// TestHandler provides test endpoints for workflow-runner benchmarking
type TestHandler struct {
	components      *bootstrap.Components
	casClient       clients.CASClient
	orchestratorURL string
}

// NewTestHandler creates a new test handler
func NewTestHandler(components *bootstrap.Components, casClient clients.CASClient, orchestratorURL string) *TestHandler {
	return &TestHandler{
		components:      components,
		casClient:       casClient,
		orchestratorURL: orchestratorURL,
	}
}

// FetchFromOrchestrator fetches workflow from orchestrator
// This is the EXACT flow that happens during workflow execution
// GET /api/v1/test/fetch-from-orchestrator/{run_id}
//
// Flow:
//   Test → workflow-runner → orchestrator (this endpoint)
//                          → Redis/CAS
//                          → response back through chain
func (h *TestHandler) FetchFromOrchestrator(c echo.Context) error {
	runID := c.Param("run_id")

	if runID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "run_id is required",
		})
	}

	h.components.Logger.Debug("Test: Fetching workflow from orchestrator", "run_id", runID)

	// Make HTTP call to orchestrator (what workflow-runner does in real flow)
	url := fmt.Sprintf("%s/api/v1/test/fetch-workflow/%s", h.orchestratorURL, runID)

	// Create request with test token (pass through from incoming request)
	req, err := http.NewRequestWithContext(c.Request().Context(), "GET", url, nil)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "failed to create request",
		})
	}

	// Forward test token
	if token := c.Request().Header.Get("X-Test-Token"); token != "" {
		req.Header.Set("X-Test-Token", token)
	}

	// Execute request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		h.components.Logger.Error("Failed to fetch from orchestrator", "error", err)
		return c.JSON(http.StatusBadGateway, map[string]interface{}{
			"error": "failed to fetch from orchestrator",
		})
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		h.components.Logger.Warn("Orchestrator returned error", "status", resp.StatusCode)
		return c.JSON(resp.StatusCode, map[string]interface{}{
			"error": "orchestrator returned error",
		})
	}

	// Read response and forward it
	// This simulates what workflow-runner does: fetch IR and use it
	return c.Stream(resp.StatusCode, resp.Header.Get("Content-Type"), resp.Body)
}

// FetchFromCAS fetches data from CAS (tests CAS client with mover routing)
// GET /api/v1/test/fetch-from-cas/{cas_id}
func (h *TestHandler) FetchFromCAS(c echo.Context) error {
	casID := c.Param("cas_id")

	if casID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "cas_id is required",
		})
	}

	// Fetch from CAS (routes through mover if USE_MOVER=true)
	data, err := h.casClient.Get(c.Request().Context(), casID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"error": "CAS entry not found",
		})
	}

	// Return data
	if bytes, ok := data.([]byte); ok {
		return c.Blob(http.StatusOK, "application/octet-stream", bytes)
	}

	return c.JSON(http.StatusOK, data)
}
