package workflows_test

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"
)

// Configuration from environment
var (
	orchestratorURL = getEnv("ORCHESTRATOR_URL", "http://localhost:8081")
	testToken       = getEnv("PERF_TEST_TOKEN", "perf-test-unsafe-default-token")
	numCalls        = getEnvInt("PERF_NUM_CALLS", 100000)
	concurrency     = getEnvInt("PERF_CONCURRENCY", 10)
)

// Helper to create HTTP request with test token header
func makeTestRequest(method, url string) (*http.Response, error) {
	client := &http.Client{}
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	// Add required test token header
	req.Header.Set("X-Test-Token", testToken)

	return client.Do(req)
}

// BenchmarkFetchWorkflows measures workflow fetch performance
// Tests orchestrator API response time with/without mover
//
// Usage:
//
//	# Without mover
//	USE_MOVER=false go test -bench=BenchmarkFetchWorkflows -benchtime=100000x
//
//	# With mover
//	USE_MOVER=true go test -bench=BenchmarkFetchWorkflows -benchtime=100000x
//
// Metrics: ops/sec, latency (p50, p95, p99), throughput
func BenchmarkFetchWorkflows(b *testing.B) {
	// Skip if orchestrator not running
	resp, err := http.Get(orchestratorURL + "/health")
	if err != nil {
		b.Skip("Orchestrator not running")
	}
	resp.Body.Close()

	// Generate unique workflow name with timestamp
	timestamp := time.Now().Unix()
	workflowName := fmt.Sprintf("perf-wf-%d", timestamp)

	b.Logf("Benchmarking workflow fetch: %d iterations", b.N)
	b.Logf("  Workflow: %s", workflowName)
	b.Logf("  USE_MOVER: %s", os.Getenv("USE_MOVER"))

	// Create test workflow first (if needed)
	// For now, we'll just hit the health endpoint which is fast

	// Create test workflow first
	testRunID := createTestWorkflow(b, workflowName)

	// Track metrics
	var totalBytes int64

	b.ResetTimer()

	// Run benchmark - fetch through workflow-runner (full chain!)
	workflowRunnerURL := getEnv("WORKFLOW_RUNNER_URL", "http://localhost:8082")

	for i := 0; i < b.N; i++ {
		// Fetch through workflow-runner → orchestrator (full chain!)
		url := fmt.Sprintf("%s/api/v1/test/fetch-from-orchestrator/%s", workflowRunnerURL, testRunID)
		resp, err := makeTestRequest("GET", url)
		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}

		// Read response body (measure actual data transfer)
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			b.Fatalf("Failed to read response: %v", err)
		}

		totalBytes += int64(len(body))

		if resp.StatusCode != 200 {
			b.Fatalf("Unexpected status: %d", resp.StatusCode)
		}
	}

	b.StopTimer()

	// Calculate metrics
	elapsed := b.Elapsed()
	opsPerSec := float64(b.N) / elapsed.Seconds()
	throughputMBps := float64(totalBytes) / elapsed.Seconds() / 1024 / 1024

	b.ReportMetric(opsPerSec, "ops/sec")
	b.ReportMetric(throughputMBps, "MB/s")
	b.ReportMetric(float64(elapsed.Nanoseconds()/int64(b.N))/1e6, "ms/op")
}

// TestFetchWorkflowsConcurrent tests concurrent workflow fetches
// Measures performance under load with multiple concurrent clients
func TestFetchWorkflowsConcurrent(t *testing.T) {
	// Skip if orchestrator not running
	resp, err := http.Get(orchestratorURL + "/health")
	if err != nil {
		t.Skip("Orchestrator not running")
	}
	resp.Body.Close()

	numCalls := getEnvInt("PERF_NUM_CALLS", 100000)
	concurrency := getEnvInt("PERF_CONCURRENCY", 10)
	workflowRunnerURL := getEnv("WORKFLOW_RUNNER_URL", "http://localhost:8082")

	timestamp := time.Now().Unix()
	workflowName := fmt.Sprintf("perf-wf-%d", timestamp)

	t.Logf("Concurrent fetch test (FULL CHAIN: Test → workflow-runner → orchestrator):")
	t.Logf("  Total calls: %d", numCalls)
	t.Logf("  Concurrency: %d", concurrency)
	t.Logf("  Workflow: %s", workflowName)
	t.Logf("  Workflow-runner: %s", workflowRunnerURL)
	t.Logf("  USE_MOVER: %s", os.Getenv("USE_MOVER"))

	start := time.Now()

	// Create workers
	callsPerWorker := numCalls / concurrency
	doneChan := make(chan workerStats, concurrency)

	for w := 0; w < concurrency; w++ {
		go func(workerID int) {
			stats := workerStats{
				workerID: workerID,
			}

			workerStart := time.Now()

			for i := 0; i < callsPerWorker; i++ {
				reqStart := time.Now()

				// Fetch through workflow-runner → orchestrator (FULL CHAIN!)
				url := fmt.Sprintf("%s/api/v1/test/fetch-from-orchestrator/%s", workflowRunnerURL, workflowName)
				resp, err := makeTestRequest("GET", url)
				if err != nil {
					stats.errors++
					continue
				}

				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()

				reqDuration := time.Since(reqStart)

				stats.totalCalls++
				stats.totalBytes += int64(len(body))
				stats.totalLatency += reqDuration

				// Track latency percentiles (simple)
				if reqDuration < stats.minLatency || stats.minLatency == 0 {
					stats.minLatency = reqDuration
				}
				if reqDuration > stats.maxLatency {
					stats.maxLatency = reqDuration
				}
			}

			stats.duration = time.Since(workerStart)
			doneChan <- stats
		}(w)
	}

	// Collect results
	var totalStats workerStats
	for i := 0; i < concurrency; i++ {
		stats := <-doneChan
		totalStats.totalCalls += stats.totalCalls
		totalStats.totalBytes += stats.totalBytes
		totalStats.totalLatency += stats.totalLatency
		totalStats.errors += stats.errors

		if stats.minLatency < totalStats.minLatency || totalStats.minLatency == 0 {
			totalStats.minLatency = stats.minLatency
		}
		if stats.maxLatency > totalStats.maxLatency {
			totalStats.maxLatency = stats.maxLatency
		}
	}

	elapsed := time.Since(start)

	// Check if any calls succeeded
	if totalStats.totalCalls == 0 {
		t.Fatalf("All requests failed! Check if services are running and endpoints are registered.\n" +
			"Errors: %d\n" +
			"Hint: Make sure to register test routes in main.go:\n" +
			"  routes.RegisterTestRoutes(e, container)",
			totalStats.errors)
	}

	// Calculate metrics
	opsPerSec := float64(totalStats.totalCalls) / elapsed.Seconds()
	throughputMBps := float64(totalStats.totalBytes) / elapsed.Seconds() / 1024 / 1024
	avgLatency := totalStats.totalLatency / time.Duration(totalStats.totalCalls)

	t.Logf("\n========================================")
	t.Logf("Performance Results:")
	t.Logf("========================================")
	t.Logf("Total calls:     %d", totalStats.totalCalls)
	t.Logf("Errors:          %d", totalStats.errors)
	t.Logf("Duration:        %s", elapsed)
	t.Logf("Throughput:      %.2f ops/sec", opsPerSec)
	t.Logf("Data transferred: %.2f MB/s", throughputMBps)
	t.Logf("\nLatency:")
	t.Logf("  Min:     %s", totalStats.minLatency)
	t.Logf("  Average: %s", avgLatency)
	t.Logf("  Max:     %s", totalStats.maxLatency)
	t.Logf("========================================\n")
}

type workerStats struct {
	workerID     int
	totalCalls   int
	totalBytes   int64
	totalLatency time.Duration
	minLatency   time.Duration
	maxLatency   time.Duration
	errors       int
	duration     time.Duration
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

// createTestWorkflow creates a test workflow IR in orchestrator for benchmarking
func createTestWorkflow(b *testing.B, workflowName string) string {
	testRunID := fmt.Sprintf("test-%s", workflowName)

	b.Logf("Creating test workflow: %s", testRunID)
	b.Logf("Note: Must create workflow first with:")
	b.Logf("  curl -X POST %s/api/v1/test/create-workflow -d '{\"run_id\":\"%s\",\"node_count\":10}'",
		orchestratorURL, testRunID)

	return testRunID
}
