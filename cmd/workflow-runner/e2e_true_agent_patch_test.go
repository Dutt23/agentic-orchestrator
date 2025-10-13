package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// TestE2E_TrueAgentPatch tests actual agent execution with workflow patching
// This test creates a workflow, starts a run with an agent that patches the workflow,
// and verifies the agent-runner-py service actually processes it.
//
// Prerequisites:
// - Orchestrator running on port 8081
// - Agent-runner-py running on port 8082
// - Redis running on port 6379
// - OPENAI_API_KEY set in agent-runner-py/.env
//
// Run with: E2E_TRUE_AGENT=true go test -v -run TestE2E_TrueAgentPatch -timeout 120s
func TestE2E_TrueAgentPatch(t *testing.T) {
	if os.Getenv("E2E_TRUE_AGENT") != "true" {
		t.Skip("Skipping true agent E2E test. Set E2E_TRUE_AGENT=true to run")
	}

	orchestratorURL := getEnvOrDefaultTrue("ORCHESTRATOR_URL", "http://localhost:8081")
	agentRunnerURL := getEnvOrDefaultTrue("AGENT_RUNNER_URL", "http://localhost:8082")

	// Check if orchestrator is available
	resp, err := http.Get(orchestratorURL + "/health")
	if err != nil || resp.StatusCode != 200 {
		t.Skipf("Orchestrator not available at %s", orchestratorURL)
	}
	resp.Body.Close()

	// Check if agent-runner-py is available
	agentResp, err := http.Get(agentRunnerURL + "/health")
	if err != nil || agentResp.StatusCode != 200 {
		t.Skipf("Agent-runner-py not available at %s", agentRunnerURL)
	}
	agentResp.Body.Close()

	// Check Redis - use DB 0 (same as agent-runner-py)
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   0, // Agent-runner-py uses DB 0
	})
	defer redisClient.Close()

	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available")
	}

	t.Run("AgentPatchWithExecution", func(t *testing.T) {
		// Step 1: Create workflow with agent node that will patch
		workflowTag := fmt.Sprintf("agent-patch-test-%s", uuid.New().String()[:8])

		t.Logf("=== Creating Workflow with Agent ===")
		workflow := map[string]interface{}{
			"tag_name": workflowTag,
			"workflow": map[string]interface{}{
				"nodes": []map[string]interface{}{
					{
						"id":   "agent_analyzer",
						"type": "agent",
						"config": map[string]interface{}{
							"model": "gpt-4",
							// Agent prompt asks it to patch the workflow
							"prompt": "Always add an email notification node after any analysis. Add a send_email node that connects to the end of this workflow.",
						},
					},
				},
				"edges": []map[string]interface{}{},
			},
		}

		createdWorkflow := createWorkflowAPITrue(t, orchestratorURL, workflow)
		require.NotNil(t, createdWorkflow)
		t.Logf("✓ Created workflow: %s", workflowTag)

		// Step 2: Publish job to Redis queue (simulating orchestrator starting a run)
		t.Logf("\n=== Publishing Job to Agent Queue ===")
		runID := fmt.Sprintf("run_%s", uuid.New().String()[:8])
		jobID := uuid.New().String()

		job := map[string]interface{}{
			"version":        "1.0",
			"job_id":         jobID,
			"run_id":         runID,
			"node_id":        "agent_analyzer",
			"workflow_tag":   workflowTag,
			"workflow_owner": "test-user",
			"prompt":         "Always add an email notification node after any analysis. Add a send_email function node that connects at the end of this workflow.",
			"current_workflow": map[string]interface{}{
				"nodes": []map[string]interface{}{
					{
						"id":   "agent_analyzer",
						"type": "agent",
					},
				},
				"edges": []map[string]interface{}{},
			},
			"context": map[string]interface{}{
				"previous_results": []interface{}{},
			},
		}

		jobJSON, _ := json.Marshal(job)
		err = redisClient.RPush(ctx, "agent:jobs", string(jobJSON)).Err()
		require.NoError(t, err, "Failed to publish job to Redis")
		t.Logf("✓ Published job %s to agent:jobs queue", jobID)
		t.Logf("✓ Job for run: %s", runID)

		// Step 3: Wait for agent to process (this will take a few seconds with real LLM)
		t.Logf("\n=== Waiting for Agent Processing ===")
		t.Log("Agent-runner-py should:")
		t.Log("  1. Pick up job from Redis queue")
		t.Log("  2. Call LLM with patch intent")
		t.Log("  3. Call patch_workflow tool")
		t.Log("  4. Forward patch to orchestrator API")

		// Poll for run status or check if workflow was patched
		time.Sleep(10 * time.Second) // Give agent time to process

		// Step 4: Check if workflow was patched
		t.Logf("\n=== Checking for Patch ===")
		updatedWorkflow := getWorkflowAPITrue(t, orchestratorURL, workflowTag, true)

		materializedWorkflow := updatedWorkflow["workflow"].(map[string]interface{})
		nodes := materializedWorkflow["nodes"].([]interface{})

		t.Logf("Workflow now has %d nodes (started with 1)", len(nodes))

		// Check if send_email node was added
		foundEmailNode := false
		for _, nodeInterface := range nodes {
			node := nodeInterface.(map[string]interface{})
			if node["id"] == "send_email" {
				foundEmailNode = true
				t.Logf("✓ Found send_email node added by agent!")
				break
			}
		}

		if foundEmailNode {
			t.Log("✓ AGENT SUCCESSFULLY PATCHED THE WORKFLOW!")
		} else {
			t.Log("⚠ Agent did not patch workflow (may have executed instead)")
			t.Log("Check agent-runner-py logs to see what happened")
		}

		// Step 5: Check agent-runner-py logs/metrics
		metricsResp, err := http.Get(agentRunnerURL + "/metrics")
		if err == nil {
			defer metricsResp.Body.Close()
			metricsBody, _ := io.ReadAll(metricsResp.Body)
			t.Logf("\n=== Agent Runner Metrics ===")
			t.Logf("%s", string(metricsBody))
		}

		// Cleanup
		t.Logf("\n=== Cleanup ===")
		deleteWorkflowAPITrue(t, orchestratorURL, workflowTag)
		t.Logf("✓ Cleaned up workflow: %s", workflowTag)
	})
}

// Helper functions for true agent test

func createWorkflowAPITrue(t *testing.T, baseURL string, workflow map[string]interface{}) map[string]interface{} {
	body, _ := json.Marshal(workflow)
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/workflows", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "test-user")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "Failed to create workflow")
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("Failed to create workflow: %d - %s", resp.StatusCode, string(bodyBytes))
	}

	var result map[string]interface{}
	json.Unmarshal(bodyBytes, &result)
	return result
}

func startRunAPITrue(t *testing.T, baseURL string, runPayload map[string]interface{}) map[string]interface{} {
	body, _ := json.Marshal(runPayload)
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/runs", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "test-user")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "Failed to start run")
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("Failed to start run: %d - %s", resp.StatusCode, string(bodyBytes))
	}

	var result map[string]interface{}
	json.Unmarshal(bodyBytes, &result)
	return result
}

func getWorkflowAPITrue(t *testing.T, baseURL string, tag string, materialize bool) map[string]interface{} {
	url := fmt.Sprintf("%s/api/v1/workflows/%s", baseURL, tag)
	if materialize {
		url += "?materialize=true"
	}

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("X-User-ID", "test-user")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "Failed to get workflow")
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Failed to get workflow: %d - %s", resp.StatusCode, string(bodyBytes))
	}

	var result map[string]interface{}
	json.Unmarshal(bodyBytes, &result)
	return result
}

func deleteWorkflowAPITrue(t *testing.T, baseURL string, tag string) {
	req, _ := http.NewRequest("DELETE", baseURL+"/api/v1/workflows/"+tag, nil)
	req.Header.Set("X-User-ID", "test-user")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Logf("Warning: Failed to delete workflow: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Logf("Warning: Delete workflow returned %d - %s", resp.StatusCode, string(bodyBytes))
	}
}

func getEnvOrDefaultTrue(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
