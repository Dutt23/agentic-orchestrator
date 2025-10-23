package handlers

import (
	"net/http"
	"sync"
	"time"

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
	// In-memory cache for test workflows (eliminates Redis bottleneck during perf tests)
	workflowCache   map[string]string
	workflowCacheMu sync.RWMutex
}

// NewTestHandler creates a new test handler
// Note: Creates its own CAS client based on USE_MOVER flag
func NewTestHandler(components *bootstrap.Components, redis *rediscommon.Client, redisRaw *redis.Client) *TestHandler {
	// Create CAS client (routes to mover if USE_MOVER=true)
	casClient, _ := clients.NewCASClient(redisRaw, components.Logger)

	return &TestHandler{
		components:    components,
		redis:         redis,
		casClient:     casClient,
		workflowCache: make(map[string]string), // Initialize in-memory cache
	}
}

// FetchWorkflowIR fetches and returns workflow IR from cache (first) or Redis
// This is the EXACT operation workflow-runner does when loading a workflow
// GET /api/v1/test/fetch-workflow/{run_id}
func (h *TestHandler) FetchWorkflowIR(c echo.Context) error {
	runID := c.Param("run_id")

	if runID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "run_id is required",
		})
	}

	irKey := "ir:" + runID

	// Check in-memory cache first (for perf tests - avoid Redis bottleneck)
	// h.workflowCacheMu.RLock()
	if cachedIR, found := h.workflowCache[irKey]; found {
		// h.workflowCacheMu.RUnlock()
		// Cache hit - instant response!
		return c.JSONBlob(http.StatusOK, []byte(cachedIR))
	}
	// h.workflowCacheMu.RUnlock()

	// Cache miss - fetch from Redis
	h.components.Logger.Info("Fetching from Redis (cache miss)", "id_key", irKey)
	irJSON, err := h.redis.Get(c.Request().Context(), irKey)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"error": "workflow IR not found",
		})
	}

	// Store in cache for future requests
	h.workflowCacheMu.Lock()
	h.workflowCache[irKey] = irJSON
	h.workflowCacheMu.Unlock()

	// Return the IR
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
	h.components.Logger.Info("Cache key stores", "run_id", irKey)
	err := h.redis.Set(c.Request().Context(), irKey, ir, 3600*time.Second) // 1 hour TTL
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "failed to store IR",
		})
	}

	// Also store in in-memory cache for fast perf test access
	h.workflowCacheMu.Lock()
	h.workflowCache[irKey] = ir
	h.workflowCacheMu.Unlock()

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
	return "test-id-placeholder" // TODO: Use proper ID generation
}
