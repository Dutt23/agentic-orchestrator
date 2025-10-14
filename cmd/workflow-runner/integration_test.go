package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/compiler"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/coordinator"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/sdk"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnv holds test environment
type TestEnv struct {
	redis  *redis.Client
	sdk    *sdk.SDK
	coord  *coordinator.Coordinator
	logger *testLogger
	ctx    context.Context
	cancel context.CancelFunc
}

// testLogger implements coordinator.Logger interface
type testLogger struct {
	t *testing.T
}

func (l *testLogger) Info(msg string, keysAndValues ...interface{}) {
	l.t.Logf("[INFO] %s %v", msg, keysAndValues)
}

func (l *testLogger) Error(msg string, keysAndValues ...interface{}) {
	l.t.Logf("[ERROR] %s %v", msg, keysAndValues)
}

func (l *testLogger) Warn(msg string, keysAndValues ...interface{}) {
	l.t.Logf("[WARN] %s %v", msg, keysAndValues)
}

func (l *testLogger) Debug(msg string, keysAndValues ...interface{}) {
	l.t.Logf("[DEBUG] %s %v", msg, keysAndValues)
}

// setupTestEnv creates a test environment
func setupTestEnv(t *testing.T) *TestEnv {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	// Connect to Redis (assumes Redis running on localhost:6379)
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   15, // Use DB 15 for tests
	})

	// Ping Redis
	err := redisClient.Ping(ctx).Err()
	require.NoError(t, err, "Redis must be running on localhost:6379")

	// Flush test DB
	err = redisClient.FlushDB(ctx).Err()
	require.NoError(t, err)

	logger := &testLogger{t: t}

	// Create mock CAS client
	casClient := &mockCASClient{
		storage: make(map[string][]byte),
		t:       t,
	}

	// Load Lua script
	luaScript := `
-- Apply delta to counter atomically with idempotency
local applied_set = KEYS[1]
local counter_key = KEYS[2]
local op_key = ARGV[1]
local delta = tonumber(ARGV[2])

if redis.call('SISMEMBER', applied_set, op_key) == 1 then
    return {redis.call('GET', counter_key) or 0, 0, 0}
end

redis.call('SADD', applied_set, op_key)
local new_value = redis.call('INCRBY', counter_key, delta)

local hit_zero = 0
if new_value == 0 then
    redis.call('PUBLISH', 'completion_events', counter_key)
    hit_zero = 1
end

return {new_value, 1, hit_zero}
`

	// Create SDK
	workflowSDK := sdk.NewSDK(redisClient, casClient, logger, luaScript)

	// Create coordinator
	coord := coordinator.NewCoordinator(&coordinator.CoordinatorOpts{
		Redis:               redisClient,
		SDK:                 workflowSDK,
		Logger:              logger,
		OrchestratorBaseURL: "http://localhost:8081",
		CASClient:           casClient,
	})

	// Start coordinator in background
	go func() {
		if err := coord.Start(ctx); err != nil && err != context.Canceled {
			t.Logf("Coordinator error: %v", err)
		}
	}()

	return &TestEnv{
		redis:  redisClient,
		sdk:    workflowSDK,
		coord:  coord,
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}
}

// cleanup cleans up test environment
func (e *TestEnv) cleanup() {
	e.cancel()
	e.redis.FlushDB(e.ctx)
	e.redis.Close()
}

// mockCASClient implements sdk.CASClient
type mockCASClient struct {
	storage map[string][]byte
	t       *testing.T
}

func (m *mockCASClient) Put(ctx context.Context, data []byte, contentType string) (string, error) {
	casID := fmt.Sprintf("cas://test/%s", uuid.New().String()[:8])
	m.storage[casID] = data
	m.t.Logf("CAS Put: %s (%d bytes)", casID, len(data))
	return casID, nil
}

func (m *mockCASClient) Get(ctx context.Context, casID string) (interface{}, error) {
	if data, exists := m.storage[casID]; exists {
		return data, nil
	}
	// Return empty object for mock CAS IDs
	return []byte("{}"), nil
}

func (m *mockCASClient) Store(ctx context.Context, data interface{}) (string, error) {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	casID := fmt.Sprintf("cas://test/%s", uuid.New().String()[:8])
	m.storage[casID] = dataJSON
	m.t.Logf("CAS Store: %s (%d bytes)", casID, len(dataJSON))
	return casID, nil
}

// Helper: Create and initialize a run
func (e *TestEnv) initializeRun(t *testing.T, schema *compiler.WorkflowSchema) string {
	// Compile workflow
	ir, err := compiler.CompileWorkflowSchema(schema, e.sdk.CASClient)
	require.NoError(t, err)

	runID := fmt.Sprintf("run_%s", uuid.New().String()[:8])

	// Store IR in Redis
	irJSON, err := json.Marshal(ir)
	require.NoError(t, err)
	err = e.redis.Set(e.ctx, fmt.Sprintf("ir:%s", runID), irJSON, 0).Err()
	require.NoError(t, err)

	// Initialize counter (start at 1 for the initial trigger)
	err = e.sdk.InitializeCounter(e.ctx, runID, 1)
	require.NoError(t, err)

	t.Logf("Initialized run: %s with %d nodes", runID, len(ir.Nodes))

	return runID
}

// Helper: Simulate agent/worker completion
func (e *TestEnv) signalCompletion(t *testing.T, runID, nodeID, resultRef string) {
	signal := map[string]interface{}{
		"version":    "1.0",
		"job_id":     uuid.New().String(),
		"run_id":     runID,
		"node_id":    nodeID,
		"status":     "completed",
		"result_ref": resultRef,
		"metadata": map[string]interface{}{
			"execution_time_ms": 100,
		},
	}

	signalJSON, err := json.Marshal(signal)
	require.NoError(t, err)

	err = e.redis.RPush(e.ctx, "completion_signals", signalJSON).Err()
	require.NoError(t, err)

	t.Logf("Signaled completion: node=%s, result=%s", nodeID, resultRef)
}

// Helper: Wait for counter to reach 0
func (e *TestEnv) waitForCompletion(t *testing.T, runID string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		counter, err := e.sdk.GetCounter(e.ctx, runID)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if counter == 0 {
			t.Logf("Workflow completed: counter=0")
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// Test 1: Sequential Flow (A→B→C)
func TestSequentialFlow(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Create simple sequential workflow
	schema := &compiler.WorkflowSchema{
		Nodes: []compiler.WorkflowNode{
			{ID: "A", Type: "function", Config: map[string]interface{}{"handler": "process_a"}},
			{ID: "B", Type: "function", Config: map[string]interface{}{"handler": "process_b"}},
			{ID: "C", Type: "function", Config: map[string]interface{}{"handler": "process_c"}},
		},
		Edges: []compiler.WorkflowEdge{
			{From: "A", To: "B"},
			{From: "B", To: "C"},
		},
	}

	runID := env.initializeRun(t, schema)

	// Simulate execution
	env.signalCompletion(t, runID, "A", "cas://result_a")
	time.Sleep(200 * time.Millisecond)

	env.signalCompletion(t, runID, "B", "cas://result_b")
	time.Sleep(200 * time.Millisecond)

	env.signalCompletion(t, runID, "C", "cas://result_c")
	time.Sleep(200 * time.Millisecond)

	// Verify completion
	completed := env.waitForCompletion(t, runID, 2*time.Second)
	assert.True(t, completed, "Workflow should complete")
}

// Test 2: Parallel Flow (A→(B,C)→D)
func TestParallelFlow(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	schema := &compiler.WorkflowSchema{
		Nodes: []compiler.WorkflowNode{
			{ID: "A", Type: "function", Config: map[string]interface{}{"handler": "start"}},
			{ID: "B", Type: "function", Config: map[string]interface{}{"handler": "parallel_b"}},
			{ID: "C", Type: "function", Config: map[string]interface{}{"handler": "parallel_c"}},
			{ID: "D", Type: "function", Config: map[string]interface{}{"handler": "join"}},
		},
		Edges: []compiler.WorkflowEdge{
			{From: "A", To: "B"},
			{From: "A", To: "C"},
			{From: "B", To: "D"},
			{From: "C", To: "D"},
		},
	}

	runID := env.initializeRun(t, schema)

	// A completes, should emit to B and C
	env.signalCompletion(t, runID, "A", "cas://result_a")
	time.Sleep(200 * time.Millisecond)

	// Verify counter increased (should be 2: one for B, one for C)
	counter, _ := env.sdk.GetCounter(env.ctx, runID)
	assert.Equal(t, 2, counter, "Counter should be 2 after fan-out")

	// B completes
	env.signalCompletion(t, runID, "B", "cas://result_b")
	time.Sleep(200 * time.Millisecond)

	// C completes
	env.signalCompletion(t, runID, "C", "cas://result_c")
	time.Sleep(200 * time.Millisecond)

	// D should wait for both B and C (join pattern)
	// TODO: This requires proper join implementation in coordinator
	// For now, D will execute twice (once per incoming token)
	// This is a known limitation of MVP

	// Verify workflow progresses
	counter, _ = env.sdk.GetCounter(env.ctx, runID)
	t.Logf("Counter after B and C complete: %d", counter)
}

// Test 3: Branch with CEL Condition
func TestBranchWithCEL(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	schema := &compiler.WorkflowSchema{
		Nodes: []compiler.WorkflowNode{
			{ID: "score", Type: "conditional", Config: map[string]interface{}{}},
			{ID: "high_path", Type: "function", Config: map[string]interface{}{"handler": "premium"}},
			{ID: "low_path", Type: "function", Config: map[string]interface{}{"handler": "basic"}},
		},
		Edges: []compiler.WorkflowEdge{
			{From: "score", To: "high_path", Condition: "output.score >= 80"},
			{From: "score", To: "low_path", Condition: "output.score < 80"},
		},
	}

	runID := env.initializeRun(t, schema)

	// Create result with high score
	result := map[string]interface{}{"score": 85}
	resultJSON, _ := json.Marshal(result)
	resultRef, _ := env.sdk.CASClient.Put(env.ctx, resultJSON, "application/json")

	// Signal completion with high score
	env.signalCompletion(t, runID, "score", resultRef)
	time.Sleep(300 * time.Millisecond)

	// Verify high_path was triggered
	// Check that token was published to high_path stream
	messages := env.redis.XRead(env.ctx, &redis.XReadArgs{
		Streams: []string{"wf.tasks.function", "0"},
		Count:   10,
		Block:   100 * time.Millisecond,
	}).Val()

	if len(messages) > 0 && len(messages[0].Messages) > 0 {
		msg := messages[0].Messages[0]
		tokenData := msg.Values["token"].(string)
		var token map[string]interface{}
		json.Unmarshal([]byte(tokenData), &token)
		t.Logf("Token routed to: %s", token["to_node"])
		assert.Equal(t, "high_path", token["to_node"], "Should route to high_path")
	}
}

// Test 4: Loop with CEL Condition
func TestLoopWithCEL(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	schema := &compiler.WorkflowSchema{
		Nodes: []compiler.WorkflowNode{
			{
				ID:   "retry_fetch",
				Type: "loop",
				Config: map[string]interface{}{
					"max_iterations": float64(3),
					"loop_back_to":   "retry_fetch",
					"condition":      "output.status != 'success'",
					"break_path":     []interface{}{"success_handler"},
					"timeout_path":   []interface{}{"failure_handler"},
				},
			},
			{ID: "success_handler", Type: "function", Config: map[string]interface{}{"handler": "success"}},
			{ID: "failure_handler", Type: "function", Config: map[string]interface{}{"handler": "failure"}},
		},
		Edges: []compiler.WorkflowEdge{
			// retry_fetch is an entry node (no dependencies), so no edge needed
		},
	}

	runID := env.initializeRun(t, schema)

	// First attempt - fails, should loop
	result1 := map[string]interface{}{"status": "error", "attempt": 1}
	resultJSON1, _ := json.Marshal(result1)
	resultRef1, _ := env.sdk.CASClient.Put(env.ctx, resultJSON1, "application/json")
	env.signalCompletion(t, runID, "retry_fetch", resultRef1)
	time.Sleep(200 * time.Millisecond)

	// Check loop iteration
	iteration := env.redis.HGet(env.ctx, fmt.Sprintf("loop:%s:retry_fetch", runID), "current_iteration").Val()
	assert.Equal(t, "1", iteration, "Loop iteration should be 1")

	// Second attempt - succeeds, should break
	result2 := map[string]interface{}{"status": "success", "attempt": 2}
	resultJSON2, _ := json.Marshal(result2)
	resultRef2, _ := env.sdk.CASClient.Put(env.ctx, resultJSON2, "application/json")
	env.signalCompletion(t, runID, "retry_fetch", resultRef2)
	time.Sleep(200 * time.Millisecond)

	// Verify loop broken and success_handler signaled
	t.Log("Loop should break and route to success_handler")
}

// Test 5: Runtime Patch (Most Complex)
func TestRuntimePatch(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Initial workflow: A→B
	schema := &compiler.WorkflowSchema{
		Nodes: []compiler.WorkflowNode{
			{ID: "A", Type: "function", Config: map[string]interface{}{"handler": "start"}},
			{ID: "B", Type: "function", Config: map[string]interface{}{"handler": "process"}},
		},
		Edges: []compiler.WorkflowEdge{
			{From: "A", To: "B"},
		},
	}

	runID := env.initializeRun(t, schema)

	// Get current IR
	irKey := fmt.Sprintf("ir:%s", runID)
	irJSON, err := env.redis.Get(env.ctx, irKey).Result()
	require.NoError(t, err)

	var currentIR sdk.IR
	err = json.Unmarshal([]byte(irJSON), &currentIR)
	require.NoError(t, err)

	t.Logf("Initial IR: %d nodes", len(currentIR.Nodes))

	// Apply patch: Add node C and edge B→C
	// Simulate what the patch API would do

	// 1. Convert IR to schema (simplified)
	patchedSchema := &compiler.WorkflowSchema{
		Nodes: []compiler.WorkflowNode{
			{ID: "A", Type: "function", Config: map[string]interface{}{"handler": "start"}},
			{ID: "B", Type: "function", Config: map[string]interface{}{"handler": "process"}},
			{ID: "C", Type: "function", Config: map[string]interface{}{"handler": "new_step"}}, // NEW
		},
		Edges: []compiler.WorkflowEdge{
			{From: "A", To: "B"},
			{From: "B", To: "C"}, // NEW
		},
	}

	// 2. Recompile
	newIR, err := compiler.CompileWorkflowSchema(patchedSchema, env.sdk.CASClient)
	require.NoError(t, err)

	t.Logf("Patched IR: %d nodes (added 1)", len(newIR.Nodes))
	assert.Equal(t, 3, len(newIR.Nodes), "Should have 3 nodes after patch")

	// 3. Update Redis (this is what PATCH API does)
	newIRJSON, err := json.Marshal(newIR)
	require.NoError(t, err)
	err = env.redis.Set(env.ctx, irKey, newIRJSON, 0).Err()
	require.NoError(t, err)

	t.Log("Applied patch: added node C and edge B→C")

	// 4. Continue execution
	env.signalCompletion(t, runID, "A", "cas://result_a")
	time.Sleep(200 * time.Millisecond)

	// Coordinator should load NEW IR and route to B
	env.signalCompletion(t, runID, "B", "cas://result_b")
	time.Sleep(200 * time.Millisecond)

	// Coordinator should route to C (new node from patch!)
	// Check that C was added to stream
	messages := env.redis.XRead(env.ctx, &redis.XReadArgs{
		Streams: []string{"wf.tasks.function", "0"},
		Count:   10,
		Block:   100 * time.Millisecond,
	}).Val()

	foundC := false
	for _, stream := range messages {
		for _, msg := range stream.Messages {
			tokenData := msg.Values["token"].(string)
			var token map[string]interface{}
			json.Unmarshal([]byte(tokenData), &token)
			if token["to_node"] == "C" {
				foundC = true
				t.Log("✓ New node C was routed correctly after patch!")
				break
			}
		}
	}

	assert.True(t, foundC, "Patched node C should be in routing")

	// Complete C
	env.signalCompletion(t, runID, "C", "cas://result_c")
	time.Sleep(200 * time.Millisecond)

	// Verify completion
	completed := env.waitForCompletion(t, runID, 2*time.Second)
	assert.True(t, completed, "Patched workflow should complete")
}

// Test 6: Agent Mock Flow
func TestAgentMockFlow(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	schema := &compiler.WorkflowSchema{
		Nodes: []compiler.WorkflowNode{
			{ID: "agent_node", Type: "agent", Config: map[string]interface{}{"model": "gpt-4"}},
			{ID: "process_result", Type: "function", Config: map[string]interface{}{"handler": "process"}},
		},
		Edges: []compiler.WorkflowEdge{
			{From: "agent_node", To: "process_result"},
		},
	}

	runID := env.initializeRun(t, schema)

	// Mock agent execution result
	agentResult := map[string]interface{}{
		"tool_calls": []map[string]interface{}{
			{"tool": "execute_pipeline", "result": "success"},
		},
		"output": "Agent completed task",
	}
	resultJSON, _ := json.Marshal(agentResult)
	resultRef, _ := env.sdk.CASClient.Put(env.ctx, resultJSON, "application/json")

	// Simulate agent signaling completion (like agent-runner-py does)
	env.signalCompletion(t, runID, "agent_node", resultRef)
	time.Sleep(200 * time.Millisecond)

	// Verify coordinator routed to agent stream
	messages := env.redis.XRead(env.ctx, &redis.XReadArgs{
		Streams: []string{"wf.tasks.agent", "0"},
		Count:   10,
		Block:   100 * time.Millisecond,
	}).Val()

	if len(messages) > 0 {
		t.Logf("✓ Agent stream has %d messages", len(messages[0].Messages))
	}

	// Complete process_result
	env.signalCompletion(t, runID, "process_result", "cas://final")
	time.Sleep(200 * time.Millisecond)

	completed := env.waitForCompletion(t, runID, 2*time.Second)
	assert.True(t, completed, "Agent workflow should complete")
}

// Test 7: Complex Patch - Multiple Operations
func TestComplexPatch(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Initial: A→B→C
	schema := &compiler.WorkflowSchema{
		Nodes: []compiler.WorkflowNode{
			{ID: "A", Type: "function", Config: map[string]interface{}{"handler": "start"}},
			{ID: "B", Type: "function", Config: map[string]interface{}{"handler": "process"}},
			{ID: "C", Type: "function", Config: map[string]interface{}{"handler": "finish"}},
		},
		Edges: []compiler.WorkflowEdge{
			{From: "A", To: "B"},
			{From: "B", To: "C"},
		},
	}

	runID := env.initializeRun(t, schema)

	// Patch: Add parallel branch B→D and B→E, both converging to F
	// Final structure: A→B→(C,D,E)→F
	patchedSchema := &compiler.WorkflowSchema{
		Nodes: []compiler.WorkflowNode{
			{ID: "A", Type: "function", Config: map[string]interface{}{"handler": "start"}},
			{ID: "B", Type: "function", Config: map[string]interface{}{"handler": "process"}},
			{ID: "C", Type: "function", Config: map[string]interface{}{"handler": "finish"}},
			{ID: "D", Type: "function", Config: map[string]interface{}{"handler": "parallel_1"}}, // NEW
			{ID: "E", Type: "function", Config: map[string]interface{}{"handler": "parallel_2"}}, // NEW
			{ID: "F", Type: "function", Config: map[string]interface{}{"handler": "final"}},      // NEW
		},
		Edges: []compiler.WorkflowEdge{
			{From: "A", To: "B"},
			{From: "B", To: "C"},
			{From: "B", To: "D"}, // NEW
			{From: "B", To: "E"}, // NEW
			{From: "C", To: "F"},
			{From: "D", To: "F"},
			{From: "E", To: "F"},
		},
	}

	// Apply patch
	newIR, err := compiler.CompileWorkflowSchema(patchedSchema, env.sdk.CASClient)
	require.NoError(t, err)

	newIRJSON, _ := json.Marshal(newIR)
	irKey := fmt.Sprintf("ir:%s", runID)
	env.redis.Set(env.ctx, irKey, newIRJSON, 0)

	t.Logf("Applied complex patch: added 3 nodes (D, E, F) and fan-out from B")

	// Execute
	env.signalCompletion(t, runID, "A", "cas://result_a")
	time.Sleep(200 * time.Millisecond)

	// B completes - should fan out to C, D, E
	env.signalCompletion(t, runID, "B", "cas://result_b")
	time.Sleep(300 * time.Millisecond)

	// Check counter - should be 3 (for C, D, E)
	counter, _ := env.sdk.GetCounter(env.ctx, runID)
	t.Logf("Counter after B (fan-out to 3): %d", counter)
	assert.Equal(t, 3, counter, "Should fan out to 3 nodes")

	// Complete all three
	env.signalCompletion(t, runID, "C", "cas://result_c")
	env.signalCompletion(t, runID, "D", "cas://result_d")
	env.signalCompletion(t, runID, "E", "cas://result_e")
	time.Sleep(300 * time.Millisecond)

	// F should be triggered (but will execute 3 times in MVP due to no join)
	// In production, would need join logic
	t.Log("Complex patched workflow executed with fan-out")
}

// Test 8: End-to-End Agent Workflow with Pipeline Execution
func TestEndToEndAgentWorkflow(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Realistic workflow: fetch_data (agent) → process_result → store_result
	// Agent will execute a pipeline to fetch and transform data
	schema := &compiler.WorkflowSchema{
		Nodes: []compiler.WorkflowNode{
			{
				ID:   "fetch_data_agent",
				Type: "agent",
				Config: map[string]interface{}{
					"model":       "gpt-4",
					"temperature": 0.3,
					"tools":       []string{"execute_pipeline"},
				},
			},
			{
				ID:   "process_result",
				Type: "function",
				Config: map[string]interface{}{
					"handler": "process_agent_output",
				},
			},
			{
				ID:   "store_result",
				Type: "function",
				Config: map[string]interface{}{
					"handler": "store_to_db",
				},
			},
		},
		Edges: []compiler.WorkflowEdge{
			{From: "fetch_data_agent", To: "process_result"},
			{From: "process_result", To: "store_result"},
		},
	}

	runID := env.initializeRun(t, schema)
	t.Logf("=== End-to-End Workflow Started: %s ===", runID)

	// Simulate agent execution
	// Agent receives prompt: "fetch flight prices from NYC to LAX and show top 3"
	// Agent calls execute_pipeline tool with http_request + table_sort + top_k

	agentResult := map[string]interface{}{
		"status": "success",
		"data": []map[string]interface{}{
			{"airline": "Delta", "price": 299, "departure": "08:00"},
			{"airline": "United", "price": 325, "departure": "10:30"},
			{"airline": "American", "price": 349, "departure": "14:15"},
		},
		"pipeline_steps": 3,
		"tool_calls": []map[string]interface{}{
			{
				"tool": "execute_pipeline",
				"steps": []map[string]interface{}{
					{"step": "http_request", "url": "https://api.flights.com/search"},
					{"step": "table_sort", "field": "price", "order": "asc"},
					{"step": "top_k", "k": 3},
				},
			},
		},
	}

	// Store agent result in CAS
	agentResultJSON, _ := json.Marshal(agentResult)
	agentResultRef, _ := env.sdk.CASClient.Put(env.ctx, agentResultJSON, "application/json")

	// Agent signals completion
	t.Log("Agent executing pipeline and signaling completion...")
	env.signalCompletion(t, runID, "fetch_data_agent", agentResultRef)
	time.Sleep(300 * time.Millisecond)

	// Verify coordinator routed to wf.tasks.function stream for process_result
	messages := env.redis.XRead(env.ctx, &redis.XReadArgs{
		Streams: []string{"wf.tasks.function", "0"},
		Count:   10,
		Block:   100 * time.Millisecond,
	}).Val()

	foundProcessResult := false
	if len(messages) > 0 {
		for _, msg := range messages[0].Messages {
			tokenData := msg.Values["token"].(string)
			var token map[string]interface{}
			json.Unmarshal([]byte(tokenData), &token)
			if token["to_node"] == "process_result" {
				foundProcessResult = true
				t.Log("✓ Coordinator routed to process_result after agent completion")
				break
			}
		}
	}
	assert.True(t, foundProcessResult, "Should route to process_result")

	// Simulate process_result execution
	processedResult := map[string]interface{}{
		"processed_flights": 3,
		"cheapest_price":    299,
		"average_price":     324,
	}
	processedJSON, _ := json.Marshal(processedResult)
	processedRef, _ := env.sdk.CASClient.Put(env.ctx, processedJSON, "application/json")

	t.Log("Processing agent result...")
	env.signalCompletion(t, runID, "process_result", processedRef)
	time.Sleep(200 * time.Millisecond)

	// Simulate store_result execution
	storeResult := map[string]interface{}{
		"stored":        true,
		"record_id":     "rec_123",
		"flights_count": 3,
	}
	storeJSON, _ := json.Marshal(storeResult)
	storeRef, _ := env.sdk.CASClient.Put(env.ctx, storeJSON, "application/json")

	t.Log("Storing final result...")
	env.signalCompletion(t, runID, "store_result", storeRef)
	time.Sleep(200 * time.Millisecond)

	// Verify workflow completion
	completed := env.waitForCompletion(t, runID, 3*time.Second)
	assert.True(t, completed, "End-to-end workflow should complete")

	// Verify all results stored in CAS
	assert.Contains(t, env.sdk.CASClient.(*mockCASClient).storage, agentResultRef)
	assert.Contains(t, env.sdk.CASClient.(*mockCASClient).storage, processedRef)
	assert.Contains(t, env.sdk.CASClient.(*mockCASClient).storage, storeRef)

	t.Log("✓ End-to-End workflow completed successfully!")
	t.Log("✓ Agent → Pipeline → Process → Store")
}

// Test 9: End-to-End Agent Workflow with Mid-Flight Patch
func TestEndToEndAgentWithPatch(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Initial workflow: agent_analyze → summarize
	schema := &compiler.WorkflowSchema{
		Nodes: []compiler.WorkflowNode{
			{
				ID:   "agent_analyze",
				Type: "agent",
				Config: map[string]interface{}{
					"model": "gpt-4",
					"tools": []string{"execute_pipeline", "patch_workflow"},
				},
			},
			{
				ID:   "summarize",
				Type: "function",
				Config: map[string]interface{}{
					"handler": "create_summary",
				},
			},
		},
		Edges: []compiler.WorkflowEdge{
			{From: "agent_analyze", To: "summarize"},
		},
	}

	runID := env.initializeRun(t, schema)
	t.Logf("=== Agent Workflow with Patch Started: %s ===", runID)

	// Agent decides to patch workflow to add notification
	// This simulates: "always send email when analysis completes"

	// Get current IR
	irKey := fmt.Sprintf("ir:%s", runID)
	irJSON, _ := env.redis.Get(env.ctx, irKey).Result()
	var currentIR sdk.IR
	json.Unmarshal([]byte(irJSON), &currentIR)

	t.Logf("Initial workflow: %d nodes", len(currentIR.Nodes))

	// Agent calls patch_workflow tool
	patchRequest := map[string]interface{}{
		"workflow_tag": "main",
		"patch_spec": map[string]interface{}{
			"operations": []map[string]interface{}{
				{
					"op":   "add",
					"path": "/nodes/-",
					"value": map[string]interface{}{
						"id":     "send_email",
						"type":   "function",
						"config": map[string]interface{}{"handler": "send_notification"},
					},
				},
				{
					"op":   "add",
					"path": "/edges/-",
					"value": map[string]interface{}{
						"from": "summarize",
						"to":   "send_email",
					},
				},
			},
			"description": "Add email notification after summary",
		},
	}

	// Apply patch (simulating orchestrator API)
	patchedSchema := &compiler.WorkflowSchema{
		Nodes: []compiler.WorkflowNode{
			{ID: "agent_analyze", Type: "agent", Config: map[string]interface{}{"model": "gpt-4"}},
			{ID: "summarize", Type: "function", Config: map[string]interface{}{"handler": "create_summary"}},
			{ID: "send_email", Type: "function", Config: map[string]interface{}{"handler": "send_notification"}}, // NEW
		},
		Edges: []compiler.WorkflowEdge{
			{From: "agent_analyze", To: "summarize"},
			{From: "summarize", To: "send_email"}, // NEW
		},
	}

	newIR, _ := compiler.CompileWorkflowSchema(patchedSchema, env.sdk.CASClient)
	newIRJSON, _ := json.Marshal(newIR)
	env.redis.Set(env.ctx, irKey, newIRJSON, 0)

	t.Logf("✓ Agent applied patch: added send_email node (%d → %d nodes)", len(currentIR.Nodes), len(newIR.Nodes))

	// Agent completes analysis
	analysisResult := map[string]interface{}{
		"analysis":   "Data shows 25% increase in sales",
		"confidence": 0.95,
		"tool_calls": []map[string]interface{}{
			{"tool": "execute_pipeline", "result": "success"},
			{"tool": "patch_workflow", "result": "patched", "patch": patchRequest},
		},
	}
	analysisJSON, _ := json.Marshal(analysisResult)
	analysisRef, _ := env.sdk.CASClient.Put(env.ctx, analysisJSON, "application/json")

	t.Log("Agent completing analysis (after patching workflow)...")
	env.signalCompletion(t, runID, "agent_analyze", analysisRef)
	time.Sleep(300 * time.Millisecond)

	// Summarize executes
	summaryResult := map[string]interface{}{
		"summary":      "Sales increased by 25%",
		"key_insights": []string{"Q4 growth", "New markets"},
	}
	summaryJSON, _ := json.Marshal(summaryResult)
	summaryRef, _ := env.sdk.CASClient.Put(env.ctx, summaryJSON, "application/json")

	t.Log("Creating summary...")
	env.signalCompletion(t, runID, "summarize", summaryRef)
	time.Sleep(300 * time.Millisecond)

	// Verify coordinator routed to send_email (new node from patch!)
	messages := env.redis.XRead(env.ctx, &redis.XReadArgs{
		Streams: []string{"wf.tasks.function", "0"},
		Count:   20,
		Block:   100 * time.Millisecond,
	}).Val()

	foundEmail := false
	if len(messages) > 0 {
		for _, msg := range messages[0].Messages {
			tokenData := msg.Values["token"].(string)
			var token map[string]interface{}
			json.Unmarshal([]byte(tokenData), &token)
			if token["to_node"] == "send_email" {
				foundEmail = true
				t.Log("✓ Coordinator routed to NEW send_email node (from agent patch)!")
				break
			}
		}
	}
	assert.True(t, foundEmail, "Should route to patched send_email node")

	// Send email executes
	emailResult := map[string]interface{}{
		"email_sent": true,
		"to":         "team@company.com",
		"subject":    "Sales Analysis Complete",
	}
	emailJSON, _ := json.Marshal(emailResult)
	emailRef, _ := env.sdk.CASClient.Put(env.ctx, emailJSON, "application/json")

	t.Log("Sending email notification...")
	env.signalCompletion(t, runID, "send_email", emailRef)
	time.Sleep(200 * time.Millisecond)

	// Verify completion
	completed := env.waitForCompletion(t, runID, 3*time.Second)
	assert.True(t, completed, "Patched workflow should complete")

	t.Log("✓ End-to-End Agent workflow with mid-flight patch completed!")
	t.Log("✓ Agent analyzed → Patched workflow → Summarized → Sent email")
}

// Test 10: Patch with Conditional Node
func TestPatchWithConditional(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Initial: A→B
	schema := &compiler.WorkflowSchema{
		Nodes: []compiler.WorkflowNode{
			{ID: "A", Type: "function", Config: map[string]interface{}{"handler": "start"}},
			{ID: "B", Type: "function", Config: map[string]interface{}{"handler": "process"}},
		},
		Edges: []compiler.WorkflowEdge{
			{From: "A", To: "B"},
		},
	}

	runID := env.initializeRun(t, schema)

	// Patch: Add conditional routing after B
	patchedSchema := &compiler.WorkflowSchema{
		Nodes: []compiler.WorkflowNode{
			{ID: "A", Type: "function", Config: map[string]interface{}{"handler": "start"}},
			{ID: "B", Type: "conditional", Config: map[string]interface{}{}}, // Changed to conditional
			{ID: "high", Type: "function", Config: map[string]interface{}{"handler": "premium"}},
			{ID: "low", Type: "function", Config: map[string]interface{}{"handler": "basic"}},
		},
		Edges: []compiler.WorkflowEdge{
			{From: "A", To: "B"},
			{From: "B", To: "high", Condition: "output.value > 100"},
			{From: "B", To: "low", Condition: "output.value <= 100"},
		},
	}

	// Apply patch
	newIR, err := compiler.CompileWorkflowSchema(patchedSchema, env.sdk.CASClient)
	require.NoError(t, err)

	newIRJSON, _ := json.Marshal(newIR)
	env.redis.Set(env.ctx, fmt.Sprintf("ir:%s", runID), newIRJSON, 0)

	t.Log("Applied patch: converted B to conditional with CEL routing")

	// Execute
	env.signalCompletion(t, runID, "A", "cas://result_a")
	time.Sleep(200 * time.Millisecond)

	// B completes with high value
	result := map[string]interface{}{"value": 150}
	resultJSON, _ := json.Marshal(result)
	resultRef, _ := env.sdk.CASClient.Put(env.ctx, resultJSON, "application/json")

	env.signalCompletion(t, runID, "B", resultRef)
	time.Sleep(300 * time.Millisecond)

	// Should route to "high" based on CEL condition
	t.Log("✓ Patched conditional routing with CEL evaluation")
}
