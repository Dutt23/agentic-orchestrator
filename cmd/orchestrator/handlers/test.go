package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/common/bootstrap"
	"github.com/lyzr/orchestrator/common/clients"
	rediscommon "github.com/lyzr/orchestrator/common/redis"
	"github.com/redis/go-redis/v9"
)

// TestHandler provides test endpoints for performance benchmarking
// These endpoints are NOT for production - only for measuring specific operations
type TestHandler struct {
	components *bootstrap.Components
	redis      *rediscommon.Client
	casClient  clients.CASClient
}

// NewTestHandler creates a new test handler
// Note: Creates its own CAS client based on USE_MOVER flag
func NewTestHandler(components *bootstrap.Components, redis *rediscommon.Client, redisRaw *redis.Client) *TestHandler {
	// Create CAS client (routes to mover if USE_MOVER=true)
	casClient, _ := clients.NewCASClient(redisRaw, components.Logger)

	return &TestHandler{
		components: components,
		redis:      redis,
		casClient:  casClient,
	}
}

// FetchWorkflowIR fetches and returns workflow IR from Redis
// This is the EXACT operation workflow-runner does when loading a workflow
// GET /api/v1/test/fetch-workflow/{run_id}
func (h *TestHandler) FetchWorkflowIR(c echo.Context) error {
	runID := c.Param("run_id")

	if runID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "run_id is required",
		})
	}

	// This is what workflow-runner does: Load IR from Redis
	irKey := "ir:" + runID
	irJSON, err := h.redis.Get(c.Request().Context(), irKey)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"error": "workflow IR not found",
		})
	}

	// Return the IR (measures: Redis fetch + network send)
	return c.JSONBlob(http.StatusOK, []byte(irJSON))
}

// FetchFromCAS fetches content from CAS
// This tests CAS read performance (Redis or via mover)
// GET /api/v1/test/fetch-cas/{cas_id}
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

	// Return raw data
	if bytes, ok := data.([]byte); ok {
		return c.Blob(http.StatusOK, "application/octet-stream", bytes)
	}

	return c.JSON(http.StatusOK, data)
}

// CreateTestWorkflow creates a dummy workflow IR for benchmarking
// POST /api/v1/test/create-workflow
// Body: {"run_id": "test-123", "node_count": 10}
func (h *TestHandler) CreateTestWorkflow(c echo.Context) error {
	var req struct {
		RunID     string `json:"run_id"`
		NodeCount int    `json:"node_count"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "invalid request",
		})
	}

	if req.RunID == "" {
		req.RunID = "test-" + generateID()
	}

	if req.NodeCount == 0 {
		req.NodeCount = 10
	}

	// Create dummy IR
	ir := generateDummyIR(req.NodeCount)

	// Store in Redis
	irKey := "ir:" + req.RunID
	err := h.redis.Set(c.Request().Context(), irKey, ir, 3600) // 1 hour TTL
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "failed to store IR",
		})
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"run_id":     req.RunID,
		"node_count": req.NodeCount,
		"ir_key":     irKey,
	})
}

// generateDummyIR creates a fake workflow IR for testing
func generateDummyIR(nodeCount int) string {
	// Simple JSON IR with N nodes
	return `{"nodes":[` + generateNodes(nodeCount) + `],"edges":[]}`
}

func generateNodes(count int) string {
	nodes := ""
	for i := 0; i < count; i++ {
		if i > 0 {
			nodes += ","
		}
		nodes += `{"id":"node` + string(rune('0'+i)) + `","type":"function"}`
	}
	return nodes
}

func generateID() string {
	return "test-id-placeholder"  // TODO: Use proper ID generation
}
