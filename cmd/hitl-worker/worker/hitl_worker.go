package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lyzr/orchestrator/common/metrics"
	redisWrapper "github.com/lyzr/orchestrator/common/redis"
	"github.com/lyzr/orchestrator/common/sdk"
	"github.com/lyzr/orchestrator/common/worker"
	"github.com/redis/go-redis/v9"
)

// HITLWorker processes Human-in-the-Loop tasks from Redis streams
// It handles two streams:
// 1. wf.tasks.hitl - New approval requests (creates approval, INCR counter, exits)
// 2. wf.tasks.hitl.responses - Approval decisions (DECR counter, sends completion, exits)
type HITLWorker struct {
	redis                 *redisWrapper.Client
	sdk                   *sdk.SDK
	logger                sdk.Logger
	requestStream         string
	responseStream        string
	requestConsumerGroup  string
	responseConsumerGroup string
	consumerName          string
}

// NewHITLWorker creates a new HITL worker
func NewHITLWorker(redisClient *redis.Client, workflowSDK *sdk.SDK, logger sdk.Logger) *HITLWorker {
	return &HITLWorker{
		redis:                 redisWrapper.NewClient(redisClient, logger),
		sdk:                   workflowSDK,
		logger:                logger,
		requestStream:         "wf.tasks.hitl",
		responseStream:        "wf.tasks.hitl.responses",
		requestConsumerGroup:  "hitl_request_workers",
		responseConsumerGroup: "hitl_response_workers",
		consumerName:          fmt.Sprintf("hitl_worker_%s", uuid.New().String()[:8]),
	}
}

// Start begins processing HITL tasks from both streams
func (w *HITLWorker) Start(ctx context.Context) error {
	w.logger.Info("starting HITL worker",
		"request_stream", w.requestStream,
		"response_stream", w.responseStream,
		"consumer_name", w.consumerName)

	// Create consumer groups if they don't exist
	if err := w.redis.CreateStreamGroup(ctx, w.requestStream, w.requestConsumerGroup); err != nil {
		return fmt.Errorf("failed to create request consumer group: %w", err)
	}

	if err := w.redis.CreateStreamGroup(ctx, w.responseStream, w.responseConsumerGroup); err != nil {
		return fmt.Errorf("failed to create response consumer group: %w", err)
	}

	// Start two goroutines - one for each stream
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errChan := make(chan error, 2)

	// Goroutine 1: Process approval requests
	go func() {
		w.logger.Info("starting request handler goroutine")
		errChan <- w.processRequestStream(ctx)
	}()

	// Goroutine 2: Process approval responses
	go func() {
		w.logger.Info("starting response handler goroutine")
		errChan <- w.processResponseStream(ctx)
	}()

	// Wait for either goroutine to error or context cancellation
	select {
	case <-ctx.Done():
		w.logger.Info("HITL worker stopping")
		return nil
	case err := <-errChan:
		w.logger.Error("HITL worker goroutine failed", "error", err)
		cancel() // Cancel the other goroutine
		return err
	}
}

// processRequestStream handles approval requests from wf.tasks.hitl
func (w *HITLWorker) processRequestStream(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("request stream handler stopping")
			return nil
		default:
			if err := w.processNextRequest(ctx); err != nil {
				w.logger.Error("failed to process request", "error", err)
				time.Sleep(1 * time.Second) // Back off on error
			}
		}
	}
}

// processResponseStream handles approval decisions from wf.tasks.hitl.responses
func (w *HITLWorker) processResponseStream(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("response stream handler stopping")
			return nil
		default:
			if err := w.processNextResponse(ctx); err != nil {
				w.logger.Error("failed to process response", "error", err)
				time.Sleep(1 * time.Second) // Back off on error
			}
		}
	}
}

// processNextRequest reads and processes one approval request
func (w *HITLWorker) processNextRequest(ctx context.Context) error {
	streams, err := w.redis.ReadFromStreamGroup(ctx, w.requestConsumerGroup, w.consumerName, w.requestStream, 1, 5*time.Second)
	if err != nil {
		return fmt.Errorf("XREADGROUP error: %w", err)
	}

	if streams == nil {
		// Timeout, no messages
		return nil
	}

	for _, stream := range streams {
		for _, message := range stream.Messages {
			if err := w.handleApprovalRequest(ctx, message); err != nil {
				w.logger.Error("failed to handle approval request", "message_id", message.ID, "error", err)
			}

			// ACK message
			if err := w.redis.AckStreamMessage(ctx, w.requestStream, w.requestConsumerGroup, message.ID); err != nil {
				w.logger.Error("failed to ACK request message", "message_id", message.ID, "error", err)
			}
		}
	}

	return nil
}

// processNextResponse reads and processes one approval decision
func (w *HITLWorker) processNextResponse(ctx context.Context) error {
	streams, err := w.redis.ReadFromStreamGroup(ctx, w.responseConsumerGroup, w.consumerName, w.responseStream, 1, 5*time.Second)
	if err != nil {
		return fmt.Errorf("XREADGROUP error: %w", err)
	}

	if streams == nil {
		// Timeout, no messages
		return nil
	}

	for _, stream := range streams {
		for _, message := range stream.Messages {
			if err := w.handleApprovalResponse(ctx, message); err != nil {
				w.logger.Error("failed to handle approval response", "message_id", message.ID, "error", err)
			}

			// ACK message
			if err := w.redis.AckStreamMessage(ctx, w.responseStream, w.responseConsumerGroup, message.ID); err != nil {
				w.logger.Error("failed to ACK response message", "message_id", message.ID, "error", err)
			}
		}
	}

	return nil
}

// handleApprovalRequest processes a new approval request
// Creates approval in Redis, increments pending counter, publishes notification, exits
func (w *HITLWorker) handleApprovalRequest(ctx context.Context, message redis.XMessage) error {
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

	w.logger.Info("processing approval request",
		"run_id", token.RunID,
		"node_id", token.ToNode,
		"token_id", token.ID)

	// Capture metrics at start
	runtimeMetrics := metrics.CaptureStart(ctx)
	startTime := time.Now()

	// Calculate queue time
	var queueTimeMs int64 = 0
	if sentAt, ok := tokenMap["sent_at"].(string); ok && sentAt != "" {
		if sentTime, err := time.Parse(time.RFC3339Nano, sentAt); err == nil {
			queueTimeMs = startTime.Sub(sentTime).Milliseconds()
		}
	}

	// Get config from token
	var config map[string]interface{}
	if token.Config != nil {
		config = token.Config
	} else {
		config = make(map[string]interface{})
	}

	// Load IR to get workflow tag and username for counter
	irKey := fmt.Sprintf("ir:%s", token.RunID)
	irJSON, err := w.redis.Get(ctx, irKey)
	if err != nil {
		return fmt.Errorf("failed to load IR: %w", err)
	}

	var ir sdk.IR
	if err := json.Unmarshal([]byte(irJSON), &ir); err != nil {
		return fmt.Errorf("failed to unmarshal IR: %w", err)
	}

	workflowTag, _ := ir.Metadata["tag"].(string)
	if workflowTag == "" {
		workflowTag = "unknown"
	}

	username, _ := ir.Metadata["username"].(string)
	if username == "" {
		username = "unknown"
	}

	approvalKey := fmt.Sprintf("hitl:approval:%s:%s", token.RunID, token.ToNode)
	// Counter keys: track both workflow-level and run-level pending approvals
	// workflow-level: Shows how many approvals pending for this workflow tag (across all runs/versions)
	//   - Tag name stays constant, only target_id changes during patches
	//   - Works across patches since tag_name doesn't change
	// run-level: Shows how many approvals pending for this specific run
	workflowCounterKey := fmt.Sprintf("workflow:%s:%s:pending_approvals", username, workflowTag)
	runCounterKey := fmt.Sprintf("run:%s:pending_approvals", token.RunID)

	// Atomic operation: SETNX approval + INCR both counters using Redis transaction
	tx := w.redis.NewTransaction()

	// SETNX: only set if key doesn't exist (idempotency)
	approvalRequest := map[string]interface{}{
		"run_id":       token.RunID,
		"node_id":      token.ToNode,
		"token_id":     token.ID,
		"username":     username,
		"workflow_tag": workflowTag,
		"message":      config["message"],
		"created_at":   time.Now().Unix(),
		"status":       "pending",
	}

	requestJSON, err := json.Marshal(approvalRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal approval request: %w", err)
	}

	setNXLabel := tx.SetNX(ctx, approvalKey, string(requestJSON), 24*time.Hour)
	workflowIncrLabel := tx.Incr(ctx, workflowCounterKey)
	runIncrLabel := tx.Incr(ctx, runCounterKey)

	if err = tx.Exec(ctx); err != nil {
		return fmt.Errorf("failed to execute Redis transaction: %w", err)
	}

	// Check if approval was newly created (SETNX returned true)
	wasCreated, err := tx.GetBoolResult(setNXLabel)
	if err != nil {
		return fmt.Errorf("failed to check SETNX result: %w", err)
	}

	if !wasCreated {
		w.logger.Warn("approval already exists, skipping",
			"run_id", token.RunID,
			"node_id", token.ToNode)
		// INCR still happened for both counters, need to DECR both to maintain accuracy
		if _, err := w.redis.Decrement(ctx, workflowCounterKey); err != nil {
			w.logger.Error("failed to decrement workflow counter after duplicate", "error", err)
		}
		if _, err := w.redis.Decrement(ctx, runCounterKey); err != nil {
			w.logger.Error("failed to decrement run counter after duplicate", "error", err)
		}
		return nil
	}

	workflowCount, _ := tx.GetIntResult(workflowIncrLabel)
	runCount, _ := tx.GetIntResult(runIncrLabel)
	w.logger.Info("approval request created",
		"run_id", token.RunID,
		"node_id", token.ToNode,
		"username", username,
		"workflow_tag", workflowTag,
		"workflow_pending_count", workflowCount,
		"run_pending_count", runCount)

	// Set node status to "waiting_for_approval" in Redis
	nodeStatusKey := fmt.Sprintf("run:%s:node:%s:status", token.RunID, token.ToNode)
	if err := w.redis.Set(ctx, nodeStatusKey, "waiting_for_approval", 24*time.Hour); err != nil {
		w.logger.Error("failed to set node status", "error", err)
	}

	// Set run status to "WAITING_FOR_APPROVAL"
	runStatusKey := fmt.Sprintf("run:%s:status", token.RunID)
	if err := w.redis.Set(ctx, runStatusKey, "WAITING_FOR_APPROVAL", 24*time.Hour); err != nil {
		w.logger.Error("failed to set run status", "error", err)
	}

	// Publish event to notify user via fanout
	if err := w.publishApprovalRequest(ctx, token.RunID, token.ToNode, workflowTag, config); err != nil {
		w.logger.Error("failed to publish approval request event", "error", err)
	}

	// Finalize metrics
	endTime := time.Now()
	runtimeMetrics.Finalize(ctx)
	executionTimeMs := endTime.Sub(startTime).Milliseconds()

	w.logger.Info("approval request processed",
		"run_id", token.RunID,
		"node_id", token.ToNode,
		"queue_time_ms", queueTimeMs,
		"execution_time_ms", executionTimeMs)

	return nil
}

// handleApprovalResponse processes an approval decision
// Decrements counter (if status was pending), sends completion signal to coordinator, exits
func (w *HITLWorker) handleApprovalResponse(ctx context.Context, message redis.XMessage) error {
	// Parse approval decision from message
	approvalJSON, ok := message.Values["approval"].(string)
	if !ok {
		return fmt.Errorf("message missing approval field")
	}

	var approval map[string]interface{}
	if err := json.Unmarshal([]byte(approvalJSON), &approval); err != nil {
		return fmt.Errorf("failed to unmarshal approval: %w", err)
	}

	runID, _ := approval["run_id"].(string)
	nodeID, _ := approval["node_id"].(string)
	approved, _ := approval["approved"].(bool)
	workflowTag, _ := approval["workflow_tag"].(string)

	if runID == "" || nodeID == "" {
		return fmt.Errorf("approval missing run_id or node_id")
	}

	w.logger.Info("processing approval response",
		"run_id", runID,
		"node_id", nodeID,
		"approved", approved)

	// Capture metrics
	runtimeMetrics := metrics.CaptureStart(ctx)
	startTime := time.Now()

	// Load approval from Redis to check status
	approvalKey := fmt.Sprintf("hitl:approval:%s:%s", runID, nodeID)
	data, err := w.redis.Get(ctx, approvalKey)

	// Retry logic for race condition (approval might not exist yet)
	if err != nil {
		w.logger.Warn("approval not found, retrying", "run_id", runID, "node_id", nodeID)
		for i := 0; i < 3; i++ {
			time.Sleep(time.Duration(i+1) * time.Second)
			data, err = w.redis.Get(ctx, approvalKey)
			if err == nil {
				break
			}
		}
	}

	if err != nil {
		return fmt.Errorf("failed to load approval after retries: %w", err)
	}

	var approvalData map[string]interface{}
	if err := json.Unmarshal([]byte(data), &approvalData); err != nil {
		return fmt.Errorf("failed to unmarshal approval data: %w", err)
	}

	previousStatus, _ := approvalData["status"].(string)

	// Idempotency check: only proceed if status was "pending"
	if previousStatus != "pending" {
		w.logger.Warn("approval already processed",
			"run_id", runID,
			"node_id", nodeID,
			"previous_status", previousStatus)
		return nil
	}

	// Get workflow tag from approval data if not in message
	if workflowTag == "" {
		workflowTag, _ = approvalData["workflow_tag"].(string)
	}
	if workflowTag == "" {
		workflowTag = "unknown"
	}

	// Get username from approval data
	username, _ := approvalData["username"].(string)
	if username == "" {
		username = "unknown"
	}

	// Load token from approval data
	tokenID, _ := approvalData["token_id"].(string)
	if tokenID == "" {
		return fmt.Errorf("approval missing token_id")
	}

	// Reconstruct token (we need full token for SignalCompletion)
	// For now, we'll create a minimal token - in production might need to store full token
	token := sdk.Token{
		ID:       tokenID,
		RunID:    runID,
		ToNode:   nodeID,
		FromNode: "", // Not available, but OK for completion signal
	}

	// DECR both counters atomically (use same key format as INCR)
	workflowCounterKey := fmt.Sprintf("workflow:%s:%s:pending_approvals", username, workflowTag)
	runCounterKey := fmt.Sprintf("run:%s:pending_approvals", runID)

	// Use transaction to decrement both counters atomically
	tx := w.redis.NewTransaction()
	workflowDecrLabel := tx.Decr(ctx, workflowCounterKey)
	runDecrLabel := tx.Decr(ctx, runCounterKey)

	if err = tx.Exec(ctx); err != nil {
		w.logger.Error("failed to decrement counters", "error", err)
	} else {
		workflowCount, _ := tx.GetIntResult(workflowDecrLabel)
		runCount, _ := tx.GetIntResult(runDecrLabel)
		w.logger.Info("decremented approval counters",
			"username", username,
			"workflow_tag", workflowTag,
			"run_id", runID,
			"workflow_count", workflowCount,
			"run_count", runCount)
	}

	// Finalize metrics
	endTime := time.Now()
	runtimeMetrics.Finalize(ctx)
	executionTimeMs := endTime.Sub(startTime).Milliseconds()

	// Build metrics map
	metricsMap := map[string]interface{}{
		"start_time":        startTime.Format(time.RFC3339Nano),
		"end_time":          endTime.Format(time.RFC3339Nano),
		"execution_time_ms": executionTimeMs,
		"total_duration_ms": executionTimeMs,
	}

	// Merge runtime metrics
	for k, v := range runtimeMetrics.ToMap() {
		metricsMap[k] = v
	}

	// Add system info
	systemInfo := metrics.GetSystemInfo()
	metricsMap["system"] = systemInfo.ToMap()

	// Create result data
	result := map[string]interface{}{
		"status":        "completed",
		"approved":      approved,
		"approval_data": approvalData,
		"node_id":       nodeID,
		"timestamp":     time.Now().Unix(),
		"metrics":       metricsMap,
	}

	// Signal completion to coordinator
	w.logger.Info("sending completion signal",
		"run_id", runID,
		"node_id", nodeID,
		"approved", approved)

	err = worker.SignalCompletion(ctx, w.redis.GetUnderlying(), w.logger, &worker.CompletionOpts{
		Token:      &token,
		Status:     "completed",
		ResultData: result,
		Metadata: map[string]interface{}{
			"approved": approved,
		},
	})

	if err != nil {
		return fmt.Errorf("failed to signal completion: %w", err)
	}

	// Update approval status in Redis to prevent duplicate processing
	// This must happen AFTER successful completion signal
	var newStatus string
	if approved {
		newStatus = "approved"
	} else {
		newStatus = "rejected"
	}

	approvalData["status"] = newStatus
	approvalData["processed_at"] = time.Now().Unix()

	updatedJSON, err := json.Marshal(approvalData)
	if err != nil {
		w.logger.Error("failed to marshal updated approval data", "error", err)
		// Don't return error - completion signal already sent successfully
	} else {
		if err := w.redis.Set(ctx, approvalKey, string(updatedJSON), 24*time.Hour); err != nil {
			w.logger.Error("failed to update approval status", "error", err)
			// Don't return error - completion signal already sent successfully
		} else {
			w.logger.Info("updated approval status",
				"run_id", runID,
				"node_id", nodeID,
				"status", newStatus)
		}
	}

	// Clear node waiting status (node is now completed)
	nodeStatusKey := fmt.Sprintf("run:%s:node:%s:status", runID, nodeID)
	if err := w.redis.Set(ctx, nodeStatusKey, "completed", 24*time.Hour); err != nil {
		w.logger.Error("failed to update node status", "error", err)
	}

	// Note: Run status will be updated by coordinator/status manager based on overall workflow state
	// If there are more pending approvals, it stays "WAITING_FOR_APPROVAL"
	// If all approvals complete, it becomes "RUNNING" or "COMPLETED"

	return nil
}

// publishApprovalRequest publishes an approval request event to fanout service
func (w *HITLWorker) publishApprovalRequest(ctx context.Context, runID, nodeID, workflowTag string, config map[string]interface{}) error {
	// Load IR to get username
	irKey := fmt.Sprintf("ir:%s", runID)
	irJSON, err := w.redis.Get(ctx, irKey)
	if err != nil {
		return fmt.Errorf("failed to load IR: %w", err)
	}

	var ir sdk.IR
	if err := json.Unmarshal([]byte(irJSON), &ir); err != nil {
		return fmt.Errorf("failed to unmarshal IR: %w", err)
	}

	username, ok := ir.Metadata["username"].(string)
	if !ok {
		return fmt.Errorf("username not found in IR metadata")
	}

	// Publish approval request event
	event := map[string]interface{}{
		"type":         "approval_required",
		"run_id":       runID,
		"node_id":      nodeID,
		"workflow_tag": workflowTag,
		"message":      config["message"],
		"timestamp":    time.Now().Unix(),
	}

	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	channel := fmt.Sprintf("workflow:events:%s", username)
	if err := w.redis.PublishEvent(ctx, channel, string(eventJSON)); err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}

	w.logger.Info("published approval request event",
		"channel", channel,
		"run_id", runID,
		"node_id", nodeID,
		"workflow_tag", workflowTag)

	return nil
}
