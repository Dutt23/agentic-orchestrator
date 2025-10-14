package workflow_lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	redisWrapper "github.com/lyzr/orchestrator/common/redis"
)

// StatusManager handles run status updates (both Redis hot path and DB cold path)
type StatusManager struct {
	redis  *redisWrapper.Client
	logger Logger
}

// NewStatusManager creates a new status manager
func NewStatusManager(redis *redisWrapper.Client, logger Logger) *StatusManager {
	return &StatusManager{
		redis:  redis,
		logger: logger,
	}
}

// UpdateRunStatus updates run status in both Redis (hot path) and queues for DB update (cold path)
// Uses pipelining to batch both operations into a single network round-trip
func (m *StatusManager) UpdateRunStatus(ctx context.Context, runID, status string) {
	// Prepare status update data
	statusUpdate := map[string]interface{}{
		"run_id":    runID,
		"status":    status,
		"timestamp": time.Now().Unix(),
	}

	updateJSON, err := json.Marshal(statusUpdate)
	if err != nil {
		m.logger.Error("failed to marshal status update",
			"run_id", runID,
			"error", err)
		return
	}

	// Use pipeline to batch SET + XADD operations (reduces network round-trips)
	key := fmt.Sprintf("run:status:%s", runID)
	pipeline := m.redis.NewPipeline()

	// Queue SET operation (hot path - in-memory status)
	pipeline.SetWithExpiry(ctx, key, status, 24*time.Hour)

	// Queue XADD operation (cold path - async DB update)
	pipeline.AddToStream(ctx, "run.status.updates", map[string]interface{}{
		"update": string(updateJSON),
	})

	// Execute both operations in one network round-trip
	if err := pipeline.Exec(ctx); err != nil {
		m.logger.Error("failed to update run status with pipeline",
			"run_id", runID,
			"status", status,
			"error", err)
		return
	}

	m.logger.Info("updated run status (Redis + queued for DB)",
		"run_id", runID,
		"status", status)
}
