package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// CreateLargeWorkflow creates a large workflow IR for testing splice performance
// GET /api/v1/test/create-large-workflow/{size_kb}
//
// Creates a workflow with specified size in KB (default 1024 = 1MB)
// This tests mover's splice performance with large responses
//
// Example:
//
//	curl -H "X-Test-Token: my-secret-token" \
//	  http://localhost:8081/api/v1/test/create-large-workflow/1024
//
// Returns: Workflow IR of ~1MB size
func (h *TestHandler) CreateLargeWorkflow(c echo.Context) error {
	var req struct {
		Size_kb int    `json:"size_kb"`
		RunID   string `json:"run_id"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "invalid request",
		})
	}

	targetSizeKB := req.Size_kb
	h.components.Logger.Info("here it is ", "size_kb", targetSizeKB)
	if targetSizeKB <= 0 || targetSizeKB > 10240 {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "size_kb must be between 1 and 10240 (10MB max)",
		})
	}

	// Generate a large workflow IR
	workflow := generateLargeWorkflowIR(targetSizeKB)

	h.components.Logger.Info("Generated large workflow IR",
		"size_kb", targetSizeKB,
		"actual_bytes", len(workflow))

	irKey := "ir:" + req.RunID
	h.components.Logger.Info("Cache key stores", "run_id", irKey)
	// err := h.redis.Set(c.Request().Context(), irKey, string(workflow), 3600*time.Second) // 1 hour TTL
	// if err != nil {
	// 	return c.JSON(http.StatusInternalServerError, map[string]interface{}{
	// 		"error": "failed to store IR",
	// 	})
	// }

	// Also store in in-memory cache for fast perf test access (eliminates Redis bottleneck)
	// h.workflowCacheMu.Lock()
	h.workflowCache[irKey] = string(workflow)
	// h.workflowCacheMu.Unlock()
	h.components.Logger.Info("✅ Stored in cache for perf testing", "run_id", req.RunID, "size_kb", len(workflow)/1024)

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"run_id":     req.RunID,
		"node_count": 10,
		"ir_key":     irKey,
	})
}

// generateLargeWorkflowIR creates a workflow IR of approximately the specified size
func generateLargeWorkflowIR(sizeKB int) []byte {
	targetBytes := sizeKB * 1024

	// Base workflow structure
	type Node struct {
		ID          string                 `json:"id"`
		Type        string                 `json:"type"`
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Config      map[string]interface{} `json:"config"`
		Children    []string               `json:"children"`
		Data        string                 `json:"data"` // Padding
	}

	type WorkflowIR struct {
		Version     string                 `json:"version"`
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Nodes       map[string]Node        `json:"nodes"`
		Entry       string                 `json:"entry"`
		Metadata    map[string]interface{} `json:"metadata"`
	}

	// Create workflow with many nodes
	nodes := make(map[string]Node)

	// Calculate how many nodes we need
	// Each node is ~500 bytes, so for 1MB we need ~2000 nodes
	estimatedNodeSize := 500
	numNodes := (targetBytes / estimatedNodeSize) + 1

	// Generate nodes
	for i := 0; i < numNodes; i++ {
		nodeID := fmt.Sprintf("node_%d", i)

		// Add padding data to reach target size
		paddingSize := 200
		padding := strings.Repeat("x", paddingSize)

		nodes[nodeID] = Node{
			ID:          nodeID,
			Type:        "task",
			Name:        fmt.Sprintf("Task %d", i),
			Description: fmt.Sprintf("This is task number %d in a large workflow for performance testing", i),
			Config: map[string]interface{}{
				"timeout":  300,
				"retries":  3,
				"priority": i % 10,
				"parameters": map[string]interface{}{
					"param1": fmt.Sprintf("value_%d", i),
					"param2": i * 100,
					"param3": true,
				},
			},
			Children: []string{fmt.Sprintf("node_%d", (i+1)%numNodes)},
			Data:     padding,
		}
	}

	workflow := WorkflowIR{
		Version:     "1.0",
		Name:        fmt.Sprintf("large-workflow-%dkb", sizeKB),
		Description: fmt.Sprintf("Large workflow for testing splice performance (%d KB)", sizeKB),
		Nodes:       nodes,
		Entry:       "node_0",
		Metadata: map[string]interface{}{
			"created_for":    "splice_performance_testing",
			"target_size_kb": sizeKB,
			"num_nodes":      numNodes,
		},
	}

	// Serialize to JSON
	data, err := json.Marshal(workflow)
	if err != nil {
		// Fallback to simple response
		return []byte(`{"error":"failed to generate workflow"}`)
	}

	return data
}

// StoreAndFetchLargeWorkflow creates a large workflow in Redis and returns its ID
// POST /api/v1/test/store-large-workflow/{size_kb}
//
// This stores the workflow in Redis so it can be fetched via the normal API
// Returns: {"run_id": "test-large-1024", "size_bytes": 1048576}
func (h *TestHandler) StoreAndFetchLargeWorkflow(c echo.Context) error {
	sizeKB := c.Param("size_kb")
	if sizeKB == "" {
		sizeKB = "1024"
	}

	targetSizeKB, err := strconv.Atoi(sizeKB)
	if err != nil || targetSizeKB <= 0 || targetSizeKB > 10240 {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "size_kb must be between 1 and 10240",
		})
	}

	// Generate workflow
	workflowData := generateLargeWorkflowIR(targetSizeKB)
	runID := fmt.Sprintf("test-large-%d", targetSizeKB)

	// Store in Redis
	irKey := "ir:" + runID
	err = h.redis.Set(c.Request().Context(), irKey, string(workflowData), 300) // 5 min TTL
	if err != nil {
		h.components.Logger.Error("Failed to store large workflow in Redis", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "failed to store workflow",
		})
	}

	h.components.Logger.Info("Stored large workflow in Redis",
		"run_id", runID,
		"size_bytes", len(workflowData),
		"size_kb", len(workflowData)/1024)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"run_id":     runID,
		"size_bytes": len(workflowData),
		"size_kb":    len(workflowData) / 1024,
		"fetch_url":  fmt.Sprintf("/api/v1/test/fetch-workflow/%s", runID),
	})
}
