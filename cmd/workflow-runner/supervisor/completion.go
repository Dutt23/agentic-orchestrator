package supervisor

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// CompletionSupervisor handles final completion verification and database updates
type CompletionSupervisor struct {
	redis  *redis.Client
	db     *sql.DB
	logger Logger
}

// Logger interface for logging
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Debug(msg string, keysAndValues ...interface{})
}

// NewCompletionSupervisor creates a new completion supervisor
func NewCompletionSupervisor(redis *redis.Client, db *sql.DB, logger Logger) *CompletionSupervisor {
	return &CompletionSupervisor{
		redis:  redis,
		db:     db,
		logger: logger,
	}
}

// Start begins the completion supervisor
// It listens for completion events published by the Lua script when counter hits 0
func (s *CompletionSupervisor) Start(ctx context.Context) error {
	s.logger.Info("completion supervisor starting", "channel", "completion_events")

	// Subscribe to completion_events channel
	pubsub := s.redis.Subscribe(ctx, "completion_events")
	defer pubsub.Close()

	// Wait for subscription confirmation
	_, err := pubsub.Receive(ctx)
	if err != nil {
		return fmt.Errorf("failed to subscribe to completion_events: %w", err)
	}

	s.logger.Info("subscribed to completion events")

	// Process events
	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("completion supervisor shutting down")
			return ctx.Err()
		case msg := <-ch:
			if msg == nil {
				continue
			}
			// Message payload is the run_id
			runID := msg.Payload
			s.logger.Debug("received completion event", "run_id", runID)
			go s.handleCompletionEvent(ctx, runID)
		}
	}
}

// handleCompletionEvent verifies completion and marks run as completed
func (s *CompletionSupervisor) handleCompletionEvent(ctx context.Context, runID string) {
	s.logger.Info("verifying completion", "run_id", runID)

	// 1. Double-check counter is still 0
	counterKey := fmt.Sprintf("counter:%s", runID)
	counter, err := s.redis.Get(ctx, counterKey).Int()
	if err != nil && err != redis.Nil {
		s.logger.Error("failed to get counter", "run_id", runID, "error", err)
		return
	}

	if counter != 0 {
		s.logger.Warn("counter not zero, skipping completion",
			"run_id", runID,
			"counter", counter)
		return
	}

	// 2. Check for pending approvals (HITL)
	pendingApprovalsKey := fmt.Sprintf("pending_approvals:%s", runID)
	pendingApprovals, err := s.redis.SCard(ctx, pendingApprovalsKey).Result()
	if err != nil && err != redis.Nil {
		s.logger.Error("failed to check pending approvals",
			"run_id", runID,
			"error", err)
		return
	}

	if pendingApprovals > 0 {
		s.logger.Info("pending approvals exist, not complete",
			"run_id", runID,
			"pending", pendingApprovals)
		return
	}

	// 3. Check for pending tokens (join pattern)
	pendingTokensPattern := fmt.Sprintf("pending_tokens:%s:*", runID)
	pendingTokenKeys, err := s.redis.Keys(ctx, pendingTokensPattern).Result()
	if err != nil && err != redis.Nil {
		s.logger.Error("failed to check pending tokens",
			"run_id", runID,
			"error", err)
		return
	}

	if len(pendingTokenKeys) > 0 {
		s.logger.Info("pending tokens exist, not complete",
			"run_id", runID,
			"pending_keys", len(pendingTokenKeys))
		return
	}

	// 4. All checks passed, mark as completed
	s.logger.Info("all checks passed, marking as completed", "run_id", runID)

	if err := s.markCompleted(ctx, runID); err != nil {
		s.logger.Error("failed to mark as completed",
			"run_id", runID,
			"error", err)
		return
	}

	// 5. Cleanup Redis keys
	if err := s.cleanup(ctx, runID); err != nil {
		s.logger.Error("failed to cleanup Redis",
			"run_id", runID,
			"error", err)
		// Don't return, completion is already recorded
	}

	s.logger.Info("workflow completed successfully", "run_id", runID)
}

// markCompleted updates the database to mark the run as completed
func (s *CompletionSupervisor) markCompleted(ctx context.Context, runID string) error {
	query := `
		UPDATE run
		SET
			status = 'COMPLETED',
			ended_at = $1,
			last_event_at = $1
		WHERE run_id = $2
	`

	_, err := s.db.ExecContext(ctx, query, time.Now().UTC(), runID)
	if err != nil {
		return fmt.Errorf("failed to update run status: %w", err)
	}

	return nil
}

// cleanup removes Redis keys for completed run
func (s *CompletionSupervisor) cleanup(ctx context.Context, runID string) error {
	// Keys to clean up
	keys := []string{
		fmt.Sprintf("counter:%s", runID),
		fmt.Sprintf("applied:%s", runID),
		fmt.Sprintf("context:%s", runID),
		fmt.Sprintf("ir:%s", runID),
	}

	// Use pipeline for efficiency
	pipe := s.redis.Pipeline()
	for _, key := range keys {
		pipe.Del(ctx, key)
	}

	// Also delete any loop state keys
	loopPattern := fmt.Sprintf("loop:%s:*", runID)
	loopKeys, err := s.redis.Keys(ctx, loopPattern).Result()
	if err == nil {
		for _, key := range loopKeys {
			pipe.Del(ctx, key)
		}
	}

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete Redis keys: %w", err)
	}

	s.logger.Debug("cleaned up Redis keys",
		"run_id", runID,
		"keys_deleted", len(keys)+len(loopKeys))

	return nil
}
