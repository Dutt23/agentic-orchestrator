package workflow_lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lyzr/orchestrator/cmd/workflow-runner/sdk"
	redisWrapper "github.com/lyzr/orchestrator/common/redis"
)

// Logger interface for logging
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Debug(msg string, keysAndValues ...interface{})
}

// CompletionChecker checks if a workflow run is complete
type CompletionChecker struct {
	redis     *redisWrapper.Client
	sdk       *sdk.SDK
	logger    Logger
	publisher *EventPublisher
	statusMgr *StatusManager
}

// NewCompletionChecker creates a new completion checker
func NewCompletionChecker(redis *redisWrapper.Client, workflowSDK *sdk.SDK, logger Logger, publisher *EventPublisher, statusMgr *StatusManager) *CompletionChecker {
	return &CompletionChecker{
		redis:     redis,
		sdk:       workflowSDK,
		logger:    logger,
		publisher: publisher,
		statusMgr: statusMgr,
	}
}

// CheckCompletion checks if the workflow run is complete
func (c *CompletionChecker) CheckCompletion(ctx context.Context, runID string) {
	// Get counter value
	counter, err := c.sdk.GetCounter(ctx, runID)
	if err != nil {
		c.logger.Error("failed to get counter",
			"run_id", runID,
			"error", err)
		return
	}

	c.logger.Debug("checking completion",
		"run_id", runID,
		"counter", counter)

	if counter == 0 {
		c.logger.Info("workflow completed",
			"run_id", runID)

		// Load IR to get username for event publishing
		ir, err := c.loadIR(ctx, runID)
		if err == nil && ir.Metadata != nil {
			if username, ok := ir.Metadata["username"].(string); ok {
				// Publish workflow_completed event
				c.publisher.PublishWorkflowEvent(ctx, username, map[string]interface{}{
					"type":      "workflow_completed",
					"run_id":    runID,
					"counter":   0,
					"timestamp": time.Now().Unix(),
				})
			}
		}

		// Update run status (both Redis hot path and DB cold path)
		c.statusMgr.UpdateRunStatus(ctx, runID, "COMPLETED")

		// TODO: Cleanup Redis keys
	}
}

// loadIR loads the latest IR from Redis
func (c *CompletionChecker) loadIR(ctx context.Context, runID string) (*sdk.IR, error) {
	key := fmt.Sprintf("ir:%s", runID)
	data, err := c.redis.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get IR from Redis: %w", err)
	}

	var ir sdk.IR
	if err := json.Unmarshal([]byte(data), &ir); err != nil {
		return nil, fmt.Errorf("failed to unmarshal IR: %w", err)
	}

	return &ir, nil
}

// EventPublisher publishes workflow events to Redis PubSub
type EventPublisher struct {
	redis  *redisWrapper.Client
	logger Logger
}

// NewEventPublisher creates a new event publisher
func NewEventPublisher(redis *redisWrapper.Client, logger Logger) *EventPublisher {
	return &EventPublisher{
		redis:  redis,
		logger: logger,
	}
}

// PublishWorkflowEvent publishes an event to Redis PubSub for fanout service
func (p *EventPublisher) PublishWorkflowEvent(ctx context.Context, username string, event map[string]interface{}) {
	channel := fmt.Sprintf("workflow:events:%s", username)

	eventJSON, err := json.Marshal(event)
	if err != nil {
		p.logger.Error("failed to marshal workflow event", "error", err)
		return
	}

	if err := p.redis.PublishEvent(ctx, channel, string(eventJSON)); err != nil {
		p.logger.Error("failed to publish workflow event",
			"channel", channel,
			"error", err)
		return
	}

	p.logger.Debug("published workflow event",
		"channel", channel,
		"type", event["type"])
}
