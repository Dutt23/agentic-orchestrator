package operators

import (
	"context"
	"fmt"

	"github.com/lyzr/orchestrator/cmd/workflow-runner/condition"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/sdk"
	redisWrapper "github.com/lyzr/orchestrator/common/redis"
	"github.com/redis/go-redis/v9"
)

// Logger interface for logging
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Debug(msg string, keysAndValues ...interface{})
}

// CompletionSignal represents a worker's completion notification
type CompletionSignal struct {
	Version   string                 `json:"version"`
	JobID     string                 `json:"job_id"`
	RunID     string                 `json:"run_id"`
	NodeID    string                 `json:"node_id"`
	Status    string                 `json:"status"`
	ResultRef string                 `json:"result_ref"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ControlFlowRouter determines which nodes to route to based on node config
type ControlFlowRouter struct {
	loopOperator   *LoopOperator
	branchOperator *BranchOperator
}

// NewControlFlowRouter creates a new control flow router
func NewControlFlowRouter(redis *redis.Client, workflowSDK *sdk.SDK, evaluator *condition.Evaluator, logger Logger) *ControlFlowRouter {
	// Wrap Redis client for better abstractions
	redisWrapper := redisWrapper.NewClient(redis, logger)

	return &ControlFlowRouter{
		loopOperator:   NewLoopOperator(redisWrapper, workflowSDK, evaluator, logger),
		branchOperator: NewBranchOperator(workflowSDK, evaluator, logger),
	}
}

// DetermineNextNodes determines which nodes to route to based on node config
func (r *ControlFlowRouter) DetermineNextNodes(ctx context.Context, signal *CompletionSignal, node *sdk.Node, ir *sdk.IR) ([]string, error) {
	// 1. Check for loop configuration
	if node.Loop != nil && node.Loop.Enabled {
		return r.loopOperator.HandleLoop(ctx, signal, node)
	}

	// 2. Check for branch configuration
	if node.Branch != nil && node.Branch.Enabled {
		return r.branchOperator.HandleBranch(ctx, signal, node)
	}

	// 3. Default: static dependents
	return node.Dependents, nil
}

// LoopOperator handles loop iteration logic
type LoopOperator struct {
	redis     *redisWrapper.Client
	sdk       *sdk.SDK
	evaluator *condition.Evaluator
	logger    Logger
}

// NewLoopOperator creates a new loop operator
func NewLoopOperator(redis *redisWrapper.Client, workflowSDK *sdk.SDK, evaluator *condition.Evaluator, logger Logger) *LoopOperator {
	return &LoopOperator{
		redis:     redis,
		sdk:       workflowSDK,
		evaluator: evaluator,
		logger:    logger,
	}
}

// HandleLoop determines next nodes for loop configuration
func (o *LoopOperator) HandleLoop(ctx context.Context, signal *CompletionSignal, node *sdk.Node) ([]string, error) {
	loopKey := fmt.Sprintf("loop:%s:%s", signal.RunID, signal.NodeID)

	// Increment iteration counter
	iteration, err := o.redis.IncrementHash(ctx, loopKey, "current_iteration", 1)
	if err != nil {
		return nil, fmt.Errorf("failed to increment loop iteration: %w", err)
	}

	o.logger.Debug("loop iteration",
		"run_id", signal.RunID,
		"node_id", signal.NodeID,
		"iteration", iteration,
		"max", node.Loop.MaxIterations)

	// Check max iterations
	if int(iteration) >= node.Loop.MaxIterations {
		o.logger.Info("loop max iterations reached",
			"run_id", signal.RunID,
			"node_id", signal.NodeID,
			"iterations", iteration)
		// Cleanup loop state
		o.redis.Delete(ctx, loopKey)
		// Exit to timeout_path
		return node.Loop.TimeoutPath, nil
	}

	// Evaluate condition if present
	if node.Loop.Condition != nil {
		// Load output from CAS for condition evaluation
		output, err := o.sdk.LoadPayload(ctx, signal.ResultRef)
		if err != nil {
			o.logger.Error("failed to load output for loop condition",
				"run_id", signal.RunID,
				"node_id", signal.NodeID,
				"error", err)
			// On error, break loop
			o.redis.Delete(ctx, loopKey)
			return node.Loop.BreakPath, nil
		}

		// Load context
		context, err := o.sdk.LoadContext(ctx, signal.RunID)
		if err != nil {
			o.logger.Warn("failed to load context for loop condition",
				"run_id", signal.RunID,
				"error", err)
			context = make(map[string]interface{})
		}

		// Evaluate condition
		conditionMet, err := o.evaluator.Evaluate(node.Loop.Condition, output, context)
		if err != nil {
			o.logger.Error("loop condition evaluation failed",
				"run_id", signal.RunID,
				"node_id", signal.NodeID,
				"expression", node.Loop.Condition.Expression,
				"error", err)
			// On error, break loop
			o.redis.Delete(ctx, loopKey)
			return node.Loop.BreakPath, nil
		}

		o.logger.Debug("loop condition evaluated",
			"run_id", signal.RunID,
			"node_id", signal.NodeID,
			"condition_met", conditionMet)

		if conditionMet {
			// Continue looping
			return []string{node.Loop.LoopBackTo}, nil
		}

		// Condition not met, break loop
		o.redis.Delete(ctx, loopKey)
		return node.Loop.BreakPath, nil
	}

	// No condition, continue looping (will eventually hit max iterations)
	return []string{node.Loop.LoopBackTo}, nil
}

// BranchOperator handles conditional branch evaluation
type BranchOperator struct {
	sdk       *sdk.SDK
	evaluator *condition.Evaluator
	logger    Logger
}

// NewBranchOperator creates a new branch operator
func NewBranchOperator(workflowSDK *sdk.SDK, evaluator *condition.Evaluator, logger Logger) *BranchOperator {
	return &BranchOperator{
		sdk:       workflowSDK,
		evaluator: evaluator,
		logger:    logger,
	}
}

// HandleBranch determines next nodes for branch configuration
func (o *BranchOperator) HandleBranch(ctx context.Context, signal *CompletionSignal, node *sdk.Node) ([]string, error) {
	// Load output from CAS for condition evaluation
	output, err := o.sdk.LoadPayload(ctx, signal.ResultRef)
	if err != nil {
		o.logger.Error("failed to load output for branch condition",
			"run_id", signal.RunID,
			"node_id", signal.NodeID,
			"error", err)
		// On error, use default path
		return node.Branch.Default, nil
	}

	// Load context
	context, err := o.sdk.LoadContext(ctx, signal.RunID)
	if err != nil {
		o.logger.Warn("failed to load context for branch condition",
			"run_id", signal.RunID,
			"error", err)
		context = make(map[string]interface{})
	}

	// Evaluate rules in order
	for i, rule := range node.Branch.Rules {
		if rule.Condition == nil {
			o.logger.Warn("branch rule has nil condition, skipping",
				"run_id", signal.RunID,
				"node_id", signal.NodeID,
				"rule_index", i)
			continue
		}

		conditionMet, err := o.evaluator.Evaluate(rule.Condition, output, context)
		if err != nil {
			o.logger.Warn("branch rule evaluation failed",
				"run_id", signal.RunID,
				"node_id", signal.NodeID,
				"rule_index", i,
				"expression", rule.Condition.Expression,
				"error", err)
			continue
		}

		o.logger.Debug("branch rule evaluated",
			"run_id", signal.RunID,
			"node_id", signal.NodeID,
			"rule_index", i,
			"condition_met", conditionMet)

		if conditionMet {
			o.logger.Info("branch rule matched",
				"run_id", signal.RunID,
				"node_id", signal.NodeID,
				"rule_index", i,
				"next_nodes", rule.NextNodes)
			return rule.NextNodes, nil
		}
	}

	// No rule matched, use default
	o.logger.Debug("no branch rule matched, using default",
		"run_id", signal.RunID,
		"node_id", signal.NodeID,
		"default", node.Branch.Default)
	return node.Branch.Default, nil
}
