package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lyzr/orchestrator/common/sdk"
	"github.com/redis/go-redis/v9"
)

// CompletionOpts contains options for sending a completion signal
type CompletionOpts struct {
	Token      *sdk.Token
	Status     string                 // "completed" or "failed"
	ResultData map[string]interface{} // Actual result data (coordinator stores in CAS)
	Metadata   map[string]interface{} // Additional metadata
}

// Validate checks if all required fields are present
func (opts *CompletionOpts) Validate() error {
	if opts.Token == nil {
		return fmt.Errorf("token is required")
	}
	if opts.Token.ID == "" {
		return fmt.Errorf("token ID is required")
	}
	if opts.Token.RunID == "" {
		return fmt.Errorf("run ID is required")
	}
	if opts.Token.ToNode == "" {
		return fmt.Errorf("node ID is required")
	}
	if opts.Status == "" {
		return fmt.Errorf("status is required")
	}
	if opts.Status != "completed" && opts.Status != "failed" {
		return fmt.Errorf("status must be 'completed' or 'failed', got: %s", opts.Status)
	}
	if opts.Status == "completed" && opts.ResultData == nil {
		return fmt.Errorf("result_data is required for completed status")
	}
	if opts.Status == "failed" && opts.Metadata == nil {
		return fmt.Errorf("metadata with error details is required for failed status")
	}
	return nil
}

// SignalCompletion sends a completion signal to the coordinator
// Uses Option B architecture: sends result_data, coordinator stores in CAS
func SignalCompletion(ctx context.Context, redis *redis.Client, logger sdk.Logger, opts *CompletionOpts) error {
	// Validate options
	if err := opts.Validate(); err != nil {
		return fmt.Errorf("invalid completion opts: %w", err)
	}

	// Build completion signal
	signal := map[string]interface{}{
		"version": "1.0",
		"job_id":  opts.Token.ID,
		"run_id":  opts.Token.RunID,
		"node_id": opts.Token.ToNode,
		"status":  opts.Status,
	}

	// Add result_data if present (Option B: coordinator will store in CAS)
	if opts.ResultData != nil {
		signal["result_data"] = opts.ResultData
	}

	// Add metadata if present
	if opts.Metadata != nil {
		signal["metadata"] = opts.Metadata
	}

	// Marshal to JSON
	signalJSON, err := json.Marshal(signal)
	if err != nil {
		return fmt.Errorf("failed to marshal signal: %w", err)
	}

	// Push to completion_signals queue
	if err := redis.RPush(ctx, "completion_signals", signalJSON).Err(); err != nil {
		return fmt.Errorf("failed to push completion signal: %w", err)
	}

	logger.Info("signaled completion",
		"run_id", opts.Token.RunID,
		"node_id", opts.Token.ToNode,
		"status", opts.Status,
		"has_result_data", opts.ResultData != nil,
		"has_metadata", opts.Metadata != nil)

	return nil
}
