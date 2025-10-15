package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/lyzr/orchestrator/common/metrics"
	"github.com/lyzr/orchestrator/common/sdk"
	"github.com/redis/go-redis/v9"
)

// HTTPWorker processes HTTP tasks from Redis stream
type HTTPWorker struct {
	redis       *redis.Client
	sdk         *sdk.SDK
	logger      sdk.Logger
	stream      string
	consumerGroup string
	consumerName  string
	httpClient    *http.Client
}

// NewHTTPWorker creates a new HTTP worker
func NewHTTPWorker(redisClient *redis.Client, workflowSDK *sdk.SDK, logger sdk.Logger) *HTTPWorker {
	return &HTTPWorker{
		redis:         redisClient,
		sdk:           workflowSDK,
		logger:        logger,
		stream:        "wf.tasks.http",
		consumerGroup: "http_workers",
		consumerName:  fmt.Sprintf("http_worker_%s", uuid.New().String()[:8]),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Start begins processing HTTP tasks
func (w *HTTPWorker) Start(ctx context.Context) error {
	w.logger.Info("starting HTTP worker",
		"stream", w.stream,
		"consumer_group", w.consumerGroup,
		"consumer_name", w.consumerName)

	// Create consumer group if it doesn't exist
	if err := w.redis.XGroupCreateMkStream(ctx, w.stream, w.consumerGroup, "0").Err(); err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("failed to create consumer group: %w", err)
	}

	// Process messages in a loop
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("HTTP worker stopping")
			return nil
		default:
			if err := w.processNextMessage(ctx); err != nil {
				w.logger.Error("failed to process message", "error", err)
				time.Sleep(1 * time.Second) // Back off on error
			}
		}
	}
}

// processNextMessage reads and processes one message from the stream
func (w *HTTPWorker) processNextMessage(ctx context.Context) error {
	// Read message from stream (XREADGROUP)
	streams, err := w.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    w.consumerGroup,
		Consumer: w.consumerName,
		Streams:  []string{w.stream, ">"},
		Count:    1,
		Block:    5 * time.Second,
	}).Result()

	if err == redis.Nil {
		// No messages, continue
		return nil
	}
	if err != nil {
		return fmt.Errorf("XREADGROUP error: %w", err)
	}

	// Process each message
	for _, stream := range streams {
		for _, message := range stream.Messages {
			if err := w.handleMessage(ctx, message); err != nil {
				w.logger.Error("failed to handle message", "message_id", message.ID, "error", err)
				// Continue to next message even if this one fails
			}

			// Acknowledge message
			if err := w.redis.XAck(ctx, w.stream, w.consumerGroup, message.ID).Err(); err != nil {
				w.logger.Error("failed to ACK message", "message_id", message.ID, "error", err)
			}
		}
	}

	return nil
}

// handleMessage processes a single message
func (w *HTTPWorker) handleMessage(ctx context.Context, message redis.XMessage) error {
	// Parse token from message
	tokenJSON, ok := message.Values["token"].(string)
	if !ok {
		return fmt.Errorf("message missing token field")
	}

	var token sdk.Token
	if err := json.Unmarshal([]byte(tokenJSON), &token); err != nil {
		return fmt.Errorf("failed to unmarshal token: %w", err)
	}

	// Also parse as map to get sent_at timestamp
	var tokenMap map[string]interface{}
	if err := json.Unmarshal([]byte(tokenJSON), &tokenMap); err != nil {
		return fmt.Errorf("failed to unmarshal token map: %w", err)
	}

	w.logger.Info("processing HTTP task",
		"run_id", token.RunID,
		"node_id", token.ToNode,
		"token_id", token.ID)

	// Use pre-resolved config from token (coordinator has already resolved variables)
	var config map[string]interface{}
	if token.Config != nil {
		config = token.Config
		w.logger.Debug("using pre-resolved config from token",
			"run_id", token.RunID,
			"node_id", token.ToNode)
	} else {
		// Fallback: Load config from IR (backward compatibility)
		w.logger.Warn("token missing config, falling back to IR",
			"run_id", token.RunID,
			"node_id", token.ToNode)

		irKey := fmt.Sprintf("ir:%s", token.RunID)
		irJSON, err := w.redis.Get(ctx, irKey).Result()
		if err != nil {
			return fmt.Errorf("failed to load IR: %w", err)
		}

		var ir sdk.IR
		if err := json.Unmarshal([]byte(irJSON), &ir); err != nil {
			return fmt.Errorf("failed to unmarshal IR: %w", err)
		}

		node, exists := ir.Nodes[token.ToNode]
		if !exists {
			return fmt.Errorf("node not found: %s", token.ToNode)
		}

		// Load config (check inline first, then CAS)
		if len(node.Config) > 0 {
			config = node.Config
		} else if node.ConfigRef != "" {
			var err error
			config, err = w.loadConfig(ctx, node.ConfigRef)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
		} else {
			config = make(map[string]interface{})
		}
	}

	// Capture runtime metrics before execution
	runtimeMetrics := metrics.CaptureStart(ctx)

	// Track timing for metrics
	startTime := time.Now()

	// Calculate queue time (time from token sent_at to now)
	var queueTimeMs int64 = 0
	var sentAtStr string
	if sentAt, ok := tokenMap["sent_at"].(string); ok && sentAt != "" {
		sentAtStr = sentAt
		if sentTime, err := time.Parse(time.RFC3339Nano, sentAt); err == nil {
			queueTimeMs = startTime.Sub(sentTime).Milliseconds()
		}
	}

	// Execute HTTP request
	result, err := w.executeHTTPRequest(ctx, config)
	endTime := time.Now()

	// Finalize runtime metrics after execution
	runtimeMetrics.Finalize(ctx)

	// Calculate metrics (even on failure)
	executionTimeMs := endTime.Sub(startTime).Milliseconds()
	totalDurationMs := queueTimeMs + executionTimeMs

	// Build metrics map with timing and runtime metrics
	metricsMap := map[string]interface{}{
		"sent_at":           sentAtStr,
		"start_time":        startTime.Format(time.RFC3339Nano),
		"end_time":          endTime.Format(time.RFC3339Nano),
		"queue_time_ms":     queueTimeMs,
		"execution_time_ms": executionTimeMs,
		"total_duration_ms": totalDurationMs,
	}

	// Merge runtime metrics into metrics map
	for k, v := range runtimeMetrics.ToMap() {
		metricsMap[k] = v
	}

	// Add system information (captured once at startup)
	systemInfo := metrics.GetSystemInfo()
	metricsMap["system"] = systemInfo.ToMap()

	if err != nil {
		w.logger.Error("HTTP request failed", "error", err)
		// Signal failure with error metadata AND metrics
		failureResult := map[string]interface{}{
			"status":  "failed",
			"error":   err.Error(),
			"metrics": metricsMap,
		}
		return SignalCompletion(ctx, w.redis, w.logger, &CompletionOpts{
			Token:      &token,
			Status:     "failed",
			ResultData: failureResult, // Send result data even on failure
			Metadata: map[string]interface{}{
				"error_type":    "HTTPRequestError",
				"error_message": err.Error(),
			},
		})
	}

	// Add metrics to successful result
	result["metrics"] = metricsMap

	// Signal completion with result data (Option B: coordinator stores in CAS)
	return SignalCompletion(ctx, w.redis, w.logger, &CompletionOpts{
		Token:      &token,
		Status:     "completed",
		ResultData: result,
		Metadata: map[string]interface{}{
			"status_code": result["status_code"],
			"duration_ms": result["duration_ms"],
		},
	})
}

// loadConfig loads node config from CAS
func (w *HTTPWorker) loadConfig(ctx context.Context, configRef string) (map[string]interface{}, error) {
	if configRef == "" {
		return make(map[string]interface{}), nil
	}

	data, err := w.sdk.CASClient.Get(ctx, configRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get config from CAS: %w", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data.([]byte), &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return config, nil
}

// executeHTTPRequest executes an HTTP request based on config
func (w *HTTPWorker) executeHTTPRequest(ctx context.Context, config map[string]interface{}) (map[string]interface{}, error) {
	// Extract config fields
	url, ok := config["url"].(string)
	if !ok || url == "" {
		return nil, fmt.Errorf("missing or invalid url in config")
	}

	method, ok := config["method"].(string)
	if !ok || method == "" {
		method = "GET"
	}

	var body []byte
	if payload, ok := config["payload"].(string); ok && payload != "" {
		body = []byte(payload)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "workflow-runner/1.0")

	// Execute request
	start := time.Now()
	resp, err := w.httpClient.Do(req)
	duration := time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse response as JSON if possible
	var responseData interface{}
	if err := json.Unmarshal(respBody, &responseData); err != nil {
		// If not JSON, store as string
		responseData = string(respBody)
	}

	result := map[string]interface{}{
		"status":      "success",
		"status_code": resp.StatusCode,
		"headers":     resp.Header,
		"body":        responseData,
		"duration_ms": duration.Milliseconds(),
		"url":         url,
		"method":      method,
	}

	w.logger.Info("HTTP request completed",
		"url", url,
		"method", method,
		"status_code", resp.StatusCode,
		"duration_ms", duration.Milliseconds())

	return result, nil
}
