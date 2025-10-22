package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/common/bootstrap"
	"github.com/lyzr/orchestrator/common/clients"
)

// TestHandler provides test endpoints for workflow-runner benchmarking
type TestHandler struct {
	components          *bootstrap.Components
	casClient           clients.CASClient
	orchestratorClient  *clients.OrchestratorClient
}

// NewTestHandler creates a new test handler
func NewTestHandler(components *bootstrap.Components, casClient clients.CASClient, orchestratorURL string) *TestHandler {
	return &TestHandler{
		components:         components,
		casClient:          casClient,
		orchestratorClient: clients.NewOrchestratorClient(orchestratorURL, components.Logger),
	}
}

// FetchFromOrchestrator fetches workflow from orchestrator
// This is the EXACT flow that happens during workflow execution
// GET /api/v1/test/fetch-from-orchestrator/{run_id}
//
// Flow:
//
//	Test → workflow-runner → orchestrator (this endpoint)
//	                       → Redis/CAS (via mover if enabled)
//	                       → response back through chain
func (h *TestHandler) FetchFromOrchestrator(c echo.Context) error {
	runID := c.Param("run_id")

	if runID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "run_id is required",
		})
	}

	h.components.Logger.Debug("Test: Fetching workflow from orchestrator", "run_id", runID)

	// Get context and add test token from incoming request
	ctx := c.Request().Context()

	// Forward test token from incoming request header to context
	// The HTTPClient will automatically extract it and add to outgoing request
	if token := c.Request().Header.Get("X-Test-Token"); token != "" {
		ctx = clients.WithTestToken(ctx, token)
	}

	// Fetch workflow IR using OrchestratorClient (supports mover for HTTP)
	irData, err := h.orchestratorClient.FetchWorkflowIR(ctx, runID)
	if err != nil {
		h.components.Logger.Error("Failed to fetch workflow IR from orchestrator", "error", err, "run_id", runID)
		return c.JSON(http.StatusBadGateway, map[string]interface{}{
			"error": "failed to fetch from orchestrator",
		})
	}

	// Return the workflow IR
	// This simulates what workflow-runner does: fetch IR and use it
	return c.JSONBlob(http.StatusOK, irData)
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
