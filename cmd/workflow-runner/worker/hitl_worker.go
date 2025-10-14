package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/sdk"
	"github.com/redis/go-redis/v9"
)

// HITLWorker processes Human-in-the-Loop tasks from Redis stream
type HITLWorker struct {
	redis         *redis.Client
	sdk           *sdk.SDK
	logger        sdk.Logger
	stream        string
	consumerGroup string
	consumerName  string
}

// NewHITLWorker creates a new HITL worker
func NewHITLWorker(redisClient *redis.Client, workflowSDK *sdk.SDK, logger sdk.Logger) *HITLWorker {
	return &HITLWorker{
		redis:         redisClient,
		sdk:           workflowSDK,
		logger:        logger,
		stream:        "wf.tasks.hitl",
		consumerGroup: "hitl_workers",
		consumerName:  fmt.Sprintf("hitl_worker_%s", uuid.New().String()[:8]),
	}
}

// Start begins processing HITL tasks
func (w *HITLWorker) Start(ctx context.Context) error {
	w.logger.Info("starting HITL worker",
		"stream", w.stream,
		"consumer_group", w.consumerGroup,
		"consumer_name", w.consumerName)

	// Create consumer group if it doesn't exist
	err := w.redis.XGroupCreateMkStream(ctx, w.stream, w.consumerGroup, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("failed to create consumer group: %w", err)
	}

	// Process messages in a loop
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("HITL worker stopping")
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
func (w *HITLWorker) processNextMessage(ctx context.Context) error {
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

// handleMessage processes a single HITL message
func (w *HITLWorker) handleMessage(ctx context.Context, message redis.XMessage) error {
	// Parse token from message
	tokenJSON, ok := message.Values["token"].(string)
	if !ok {
		return fmt.Errorf("message missing token field")
	}

	var token sdk.Token
	if err := json.Unmarshal([]byte(tokenJSON), &token); err != nil {
		return fmt.Errorf("failed to unmarshal token: %w", err)
	}

	w.logger.Info("processing HITL task",
		"run_id", token.RunID,
		"node_id", token.ToNode,
		"token_id", token.ID)

	// Use pre-resolved config from token
	var config map[string]interface{}
	if token.Config != nil {
		config = token.Config
	} else {
		// Fallback: Load config from IR
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

	// Create approval request
	approvalKey := fmt.Sprintf("hitl:approval:%s:%s", token.RunID, token.ToNode)

	// Store approval request in Redis with metadata
	approvalRequest := map[string]interface{}{
		"run_id":     token.RunID,
		"node_id":    token.ToNode,
		"token_id":   token.ID,
		"message":    config["message"],
		"created_at": time.Now().Unix(),
		"status":     "pending",
	}

	requestJSON, err := json.Marshal(approvalRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal approval request: %w", err)
	}

	// Store approval request
	if err := w.redis.Set(ctx, approvalKey, requestJSON, 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("failed to store approval request: %w", err)
	}

	// Publish event to notify user (via fanout service)
	if err := w.publishApprovalRequest(ctx, token.RunID, token.ToNode, config); err != nil {
		w.logger.Error("failed to publish approval request event", "error", err)
	}

	w.logger.Info("approval request created",
		"run_id", token.RunID,
		"node_id", token.ToNode,
		"approval_key", approvalKey)

	// Wait for approval (poll Redis for approval response)
	// Default timeout: 24 hours (configurable)
	timeout := 24 * time.Hour
	if timeoutSec, ok := config["timeout"].(float64); ok {
		timeout = time.Duration(timeoutSec) * time.Second
	}

	approved, approvalData, err := w.waitForApproval(ctx, approvalKey, timeout)
	if err != nil {
		w.logger.Error("failed to wait for approval", "error", err)
		return SignalCompletion(ctx, w.redis, w.logger, &CompletionOpts{
			Token:  &token,
			Status: "failed",
			Metadata: map[string]interface{}{
				"error_type":    "ApprovalError",
				"error_message": err.Error(),
			},
		})
	}

	// Create result
	result := map[string]interface{}{
		"status":        "completed",
		"approved":      approved,
		"approval_data": approvalData,
		"node_id":       token.ToNode,
		"timestamp":     time.Now().Unix(),
	}

	// Signal completion with result data (Option B: coordinator stores in CAS)
	return SignalCompletion(ctx, w.redis, w.logger, &CompletionOpts{
		Token:      &token,
		Status:     "completed",
		ResultData: result,
		Metadata: map[string]interface{}{
			"approved": approved,
		},
	})
}

// waitForApproval waits for user approval by polling Redis
func (w *HITLWorker) waitForApproval(ctx context.Context, approvalKey string, timeout time.Duration) (bool, map[string]interface{}, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := 2 * time.Second

	for {
		// Check if deadline exceeded
		if time.Now().After(deadline) {
			w.logger.Warn("approval timeout", "approval_key", approvalKey)
			return false, nil, fmt.Errorf("approval timeout after %v", timeout)
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return false, nil, ctx.Err()
		default:
		}

		// Poll Redis for approval response
		data, err := w.redis.Get(ctx, approvalKey).Result()
		if err == redis.Nil {
			// Key expired or deleted, treat as timeout
			return false, nil, fmt.Errorf("approval request expired")
		}
		if err != nil {
			w.logger.Error("failed to get approval status", "error", err)
			time.Sleep(pollInterval)
			continue
		}

		// Parse approval response
		var approvalRequest map[string]interface{}
		if err := json.Unmarshal([]byte(data), &approvalRequest); err != nil {
			w.logger.Error("failed to parse approval response", "error", err)
			time.Sleep(pollInterval)
			continue
		}

		status, _ := approvalRequest["status"].(string)

		if status == "approved" {
			w.logger.Info("approval granted", "approval_key", approvalKey)
			return true, approvalRequest, nil
		} else if status == "rejected" {
			w.logger.Info("approval rejected", "approval_key", approvalKey)
			return false, approvalRequest, nil
		}

		// Still pending, wait and retry
		time.Sleep(pollInterval)
	}
}

// loadConfig loads node config from CAS
func (w *HITLWorker) loadConfig(ctx context.Context, configRef string) (map[string]interface{}, error) {
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

// publishApprovalRequest publishes an approval request event
func (w *HITLWorker) publishApprovalRequest(ctx context.Context, runID, nodeID string, config map[string]interface{}) error {
	// Load IR to get username
	irKey := fmt.Sprintf("ir:%s", runID)
	irJSON, err := w.redis.Get(ctx, irKey).Result()
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
		"type":      "approval_required",
		"run_id":    runID,
		"node_id":   nodeID,
		"message":   config["message"],
		"timestamp": time.Now().Unix(),
	}

	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	channel := fmt.Sprintf("workflow:events:%s", username)
	if err := w.redis.Publish(ctx, channel, eventJSON).Err(); err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}

	w.logger.Info("published approval request event",
		"channel", channel,
		"run_id", runID,
		"node_id", nodeID)

	return nil
}
