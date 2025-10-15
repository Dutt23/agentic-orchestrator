package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lyzr/orchestrator/common/models"
	"github.com/lyzr/orchestrator/common/repository"
	"github.com/redis/go-redis/v9"
)

// Logger interface for logging
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Debug(msg string, keysAndValues ...interface{})
}

// StatusUpdateConsumer consumes status updates from Redis stream and updates database
type StatusUpdateConsumer struct {
	redis         *redis.Client
	runRepo       *repository.RunRepository
	logger        Logger
	stream        string
	consumerGroup string
	consumerName  string
}

// StatusUpdate represents a status update message
type StatusUpdate struct {
	RunID     string `json:"run_id"`
	Status    string `json:"status"`
	Timestamp int64  `json:"timestamp"`
}

// NewStatusUpdateConsumer creates a new status update consumer
func NewStatusUpdateConsumer(redis *redis.Client, runRepo *repository.RunRepository, logger Logger) *StatusUpdateConsumer {
	return &StatusUpdateConsumer{
		redis:         redis,
		runRepo:       runRepo,
		logger:        logger,
		stream:        "run.status.updates",
		consumerGroup: "status_updaters",
		consumerName:  fmt.Sprintf("status_updater_%d", time.Now().Unix()),
	}
}

// Start begins consuming status updates
func (c *StatusUpdateConsumer) Start(ctx context.Context) error {
	c.logger.Info("starting status update consumer",
		"stream", c.stream,
		"consumer_group", c.consumerGroup,
		"consumer_name", c.consumerName)

	// Create consumer group if it doesn't exist
	err := c.redis.XGroupCreateMkStream(ctx, c.stream, c.consumerGroup, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("failed to create consumer group: %w", err)
	}

	// Process messages in a loop
	for {
		select {
		case <-ctx.Done():
			c.logger.Info("status update consumer stopping")
			return nil
		default:
			if err := c.processNextMessage(ctx); err != nil {
				c.logger.Error("failed to process message", "error", err)
				time.Sleep(1 * time.Second) // Back off on error
			}
		}
	}
}

// processNextMessage reads and processes one message from the stream
func (c *StatusUpdateConsumer) processNextMessage(ctx context.Context) error {
	// Read message from stream (XREADGROUP)
	streams, err := c.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    c.consumerGroup,
		Consumer: c.consumerName,
		Streams:  []string{c.stream, ">"},
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
			if err := c.handleMessage(ctx, message); err != nil {
				c.logger.Error("failed to handle message", "message_id", message.ID, "error", err)
				// Continue to next message even if this one fails
			}

			// Acknowledge message
			if err := c.redis.XAck(ctx, c.stream, c.consumerGroup, message.ID).Err(); err != nil {
				c.logger.Error("failed to ACK message", "message_id", message.ID, "error", err)
			}
		}
	}

	return nil
}

// handleMessage processes a single status update message
func (c *StatusUpdateConsumer) handleMessage(ctx context.Context, message redis.XMessage) error {
	// Parse update from message
	updateJSON, ok := message.Values["update"].(string)
	if !ok {
		return fmt.Errorf("message missing update field")
	}

	var statusUpdate StatusUpdate
	if err := json.Unmarshal([]byte(updateJSON), &statusUpdate); err != nil {
		return fmt.Errorf("failed to unmarshal status update: %w", err)
	}

	c.logger.Info("processing status update",
		"run_id", statusUpdate.RunID,
		"status", statusUpdate.Status)

	// Parse run ID
	runID, err := uuid.Parse(statusUpdate.RunID)
	if err != nil {
		return fmt.Errorf("invalid run_id: %w", err)
	}

	// Convert status string to RunStatus enum
	var runStatus models.RunStatus
	switch statusUpdate.Status {
	case "COMPLETED":
		runStatus = models.StatusCompleted
	case "FAILED":
		runStatus = models.StatusFailed
	case "RUNNING":
		runStatus = models.StatusRunning
	case "QUEUED":
		runStatus = models.StatusQueued
	default:
		return fmt.Errorf("unknown status: %s", statusUpdate.Status)
	}

	// Update database
	if err := c.runRepo.UpdateStatus(ctx, runID, runStatus); err != nil {
		return fmt.Errorf("failed to update run status in database: %w", err)
	}

	c.logger.Info("updated run status in database",
		"run_id", statusUpdate.RunID,
		"status", statusUpdate.Status)

	return nil
}
