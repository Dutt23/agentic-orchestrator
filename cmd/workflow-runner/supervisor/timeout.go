package supervisor

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// TimeoutDetector monitors for hanging workflows and marks them as failed
type TimeoutDetector struct {
	redis         *redis.Client
	db            *sql.DB
	logger        Logger
	checkInterval time.Duration
	timeout       time.Duration
}

// NewTimeoutDetector creates a new timeout detector
func NewTimeoutDetector(redis *redis.Client, db *sql.DB, logger Logger) *TimeoutDetector {
	return &TimeoutDetector{
		redis:         redis,
		db:            db,
		logger:        logger,
		checkInterval: 30 * time.Second, // Check every 30 seconds
		timeout:       5 * time.Minute,  // Consider hung after 5 minutes of inactivity
	}
}

// WithCheckInterval sets the check interval
func (t *TimeoutDetector) WithCheckInterval(interval time.Duration) *TimeoutDetector {
	t.checkInterval = interval
	return t
}

// WithTimeout sets the inactivity timeout
func (t *TimeoutDetector) WithTimeout(timeout time.Duration) *TimeoutDetector {
	t.timeout = timeout
	return t
}

// Start begins the timeout detector
func (t *TimeoutDetector) Start(ctx context.Context) error {
	t.logger.Info("timeout detector starting",
		"check_interval", t.checkInterval,
		"timeout", t.timeout)

	ticker := time.NewTicker(t.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.logger.Info("timeout detector shutting down")
			return ctx.Err()
		case <-ticker.C:
			if err := t.checkHangingWorkflows(ctx); err != nil {
				t.logger.Error("failed to check hanging workflows", "error", err)
			}
		}
	}
}

// checkHangingWorkflows finds and marks hanging workflows as failed
func (t *TimeoutDetector) checkHangingWorkflows(ctx context.Context) error {
	// Query for runs that are RUNNING but have no activity for > timeout
	query := `
		SELECT run_id, last_event_at
		FROM run
		WHERE status = 'RUNNING'
		  AND last_event_at < $1
		LIMIT 100
	`

	cutoff := time.Now().UTC().Add(-t.timeout)
	rows, err := t.db.QueryContext(ctx, query, cutoff)
	if err != nil {
		return fmt.Errorf("failed to query hanging workflows: %w", err)
	}
	defer rows.Close()

	var hangingCount int
	for rows.Next() {
		var runID string
		var lastEventAt time.Time

		if err := rows.Scan(&runID, &lastEventAt); err != nil {
			t.logger.Error("failed to scan row", "error", err)
			continue
		}

		t.logger.Warn("detected hanging workflow",
			"run_id", runID,
			"last_event_at", lastEventAt,
			"inactive_duration", time.Since(lastEventAt))

		// Check if counter is stuck (non-zero)
		counter, err := t.getCounter(ctx, runID)
		if err != nil {
			t.logger.Error("failed to get counter",
				"run_id", runID,
				"error", err)
			continue
		}

		t.logger.Debug("hanging workflow counter check",
			"run_id", runID,
			"counter", counter)

		// If counter is 0 but workflow is still RUNNING, something is wrong
		// If counter is > 0, workflow is stuck
		if err := t.markFailed(ctx, runID, fmt.Sprintf("timeout: no activity for %s, counter=%d", t.timeout, counter)); err != nil {
			t.logger.Error("failed to mark as failed",
				"run_id", runID,
				"error", err)
			continue
		}

		hangingCount++
	}

	if hangingCount > 0 {
		t.logger.Info("marked hanging workflows as failed", "count", hangingCount)
	}

	return nil
}

// getCounter gets the counter value from Redis
func (t *TimeoutDetector) getCounter(ctx context.Context, runID string) (int, error) {
	key := fmt.Sprintf("counter:%s", runID)
	val, err := t.redis.Get(ctx, key).Int()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return val, nil
}

// markFailed marks a workflow as failed in the database
func (t *TimeoutDetector) markFailed(ctx context.Context, runID, reason string) error {
	query := `
		UPDATE run
		SET
			status = 'FAILED',
			ended_at = $1,
			last_event_at = $1
		WHERE run_id = $2
		  AND status = 'RUNNING'
	`

	result, err := t.db.ExecContext(ctx, query, time.Now().UTC(), runID)
	if err != nil {
		return fmt.Errorf("failed to update run status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		t.logger.Warn("run already completed or failed",
			"run_id", runID)
		return nil
	}

	// Log event
	t.logger.Info("marked workflow as failed",
		"run_id", runID,
		"reason", reason)

	// TODO: Write to event_log table for audit trail

	// Cleanup Redis keys
	t.cleanupFailedRun(ctx, runID)

	return nil
}

// cleanupFailedRun removes Redis keys for failed run
func (t *TimeoutDetector) cleanupFailedRun(ctx context.Context, runID string) {
	keys := []string{
		fmt.Sprintf("counter:%s", runID),
		fmt.Sprintf("applied:%s", runID),
		fmt.Sprintf("context:%s", runID),
		fmt.Sprintf("ir:%s", runID),
	}

	pipe := t.redis.Pipeline()
	for _, key := range keys {
		pipe.Del(ctx, key)
	}

	// Also delete any loop state keys
	loopPattern := fmt.Sprintf("loop:%s:*", runID)
	loopKeys, err := t.redis.Keys(ctx, loopPattern).Result()
	if err == nil {
		for _, key := range loopKeys {
			pipe.Del(ctx, key)
		}
	}

	// Also delete any pending token keys
	pendingPattern := fmt.Sprintf("pending_tokens:%s:*", runID)
	pendingKeys, err := t.redis.Keys(ctx, pendingPattern).Result()
	if err == nil {
		for _, key := range pendingKeys {
			pipe.Del(ctx, key)
		}
	}

	_, err = pipe.Exec(ctx)
	if err != nil {
		t.logger.Error("failed to cleanup Redis keys",
			"run_id", runID,
			"error", err)
	} else {
		t.logger.Debug("cleaned up Redis keys for failed run",
			"run_id", runID)
	}
}
