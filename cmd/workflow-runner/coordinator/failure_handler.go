package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lyzr/orchestrator/common/sdk"
)

// getMapKeys returns the keys of a map as a slice (for logging)
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// handleFailedNode processes a failed node execution
// Stores failure data in CAS, publishes events, and updates run status
func (c *Coordinator) handleFailedNode(ctx context.Context, signal *CompletionSignal, ir *sdk.IR) {
	c.logger.Error("node execution failed",
		"run_id", signal.RunID,
		"node_id", signal.NodeID,
		"result_ref", signal.ResultRef,
		"error", signal.Metadata)

	// Store result_data in CAS even on failure (for metrics)
	var failureResultRef string
	if signal.ResultData != nil {
		resultID := fmt.Sprintf("artifact://%s-%s-%d", signal.RunID, signal.NodeID, time.Now().UnixNano())
		casKey := fmt.Sprintf("cas:%s", resultID)

		resultJSON, err := json.Marshal(signal.ResultData)
		if err == nil {
			if err := c.redisWrapper.Set(ctx, casKey, string(resultJSON), 0); err == nil {
				failureResultRef = resultID
				// Store at :output so RunService can find it
				c.sdk.StoreContext(ctx, signal.RunID, signal.NodeID, failureResultRef)
				c.logger.Info("stored failure result in CAS",
					"run_id", signal.RunID,
					"node_id", signal.NodeID,
					"result_ref", failureResultRef)
			}
		}
	}

	// Store failure information in context for debugging and retry logic
	failureData := map[string]interface{}{
		"status":     "failed",
		"node_id":    signal.NodeID,
		"error":      signal.Metadata,
		"timestamp":  time.Now().Unix(),
		"retryable":  signal.Metadata["retryable"],
		"error_type": signal.Metadata["error_type"],
	}

	// Marshal to JSON for storage
	failureJSON, err := json.Marshal(failureData)
	if err != nil {
		c.logger.Error("failed to marshal failure data",
			"run_id", signal.RunID,
			"node_id", signal.NodeID,
			"error", err)
	} else {
		// Store in Redis context so it can be retrieved later
		failureKey := fmt.Sprintf("%s:failure", signal.NodeID)
		if err := c.sdk.StoreContext(ctx, signal.RunID, failureKey, string(failureJSON)); err != nil {
			c.logger.Error("failed to store failure context",
				"run_id", signal.RunID,
				"node_id", signal.NodeID,
				"error", err)
		}
	}

	// Publish node_failed event
	c.logger.Info("attempting to publish node_failed event",
		"run_id", signal.RunID,
		"node_id", signal.NodeID,
		"ir_metadata_nil", ir.Metadata == nil,
		"ir_metadata", ir.Metadata)

	if ir.Metadata == nil {
		c.logger.Error("IR metadata is nil, cannot publish failure events",
			"run_id", signal.RunID,
			"node_id", signal.NodeID)
	} else {
		username, ok := ir.Metadata["username"].(string)
		c.logger.Info("checking username in IR metadata",
			"run_id", signal.RunID,
			"username_exists", ok,
			"username", username)

		if !ok {
			c.logger.Error("username not found in IR metadata, cannot publish failure events",
				"run_id", signal.RunID,
				"node_id", signal.NodeID,
				"metadata_keys", getMapKeys(ir.Metadata))
		} else {
			c.logger.Info("publishing node_failed event",
				"run_id", signal.RunID,
				"node_id", signal.NodeID,
				"username", username)

			c.lifecycle.EventPublisher.PublishWorkflowEvent(ctx, username, map[string]interface{}{
				"type":      "node_failed",
				"run_id":    signal.RunID,
				"node_id":   signal.NodeID,
				"error":     signal.Metadata,
				"timestamp": time.Now().Unix(),
			})

			// Also publish workflow_failed event to indicate the entire workflow failed
			c.logger.Info("publishing workflow_failed event",
				"run_id", signal.RunID,
				"node_id", signal.NodeID,
				"username", username)

			c.lifecycle.EventPublisher.PublishWorkflowEvent(ctx, username, map[string]interface{}{
				"type":      "workflow_failed",
				"run_id":    signal.RunID,
				"node_id":   signal.NodeID,
				"error":     signal.Metadata,
				"timestamp": time.Now().Unix(),
			})
		}
	}

	// Update run status (both Redis hot path and DB cold path)
	c.lifecycle.StatusManager.UpdateRunStatus(ctx, signal.RunID, "FAILED")

	// TODO: Handle failure (DLQ, retry, etc.)
}
