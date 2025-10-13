package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lyzr/orchestrator/cmd/workflow-runner/condition"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/resolver"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/sdk"
	"github.com/redis/go-redis/v9"
)

// CompletionSignal represents a worker's completion notification
type CompletionSignal struct {
	Version   string                 `json:"version"`    // Protocol version (1.0)
	JobID     string                 `json:"job_id"`     // Unique job ID
	RunID     string                 `json:"run_id"`     // Workflow run ID
	NodeID    string                 `json:"node_id"`    // Node that completed
	Status    string                 `json:"status"`     // completed|failed
	ResultRef string                 `json:"result_ref"` // CAS reference to result
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Coordinator handles choreography for workflow execution
type Coordinator struct {
	redis     *redis.Client
	sdk       *sdk.SDK
	logger    Logger
	router    *StreamRouter
	evaluator *condition.Evaluator
	resolver  *resolver.Resolver
}

// Logger interface for logging
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Debug(msg string, keysAndValues ...interface{})
}

// NewCoordinator creates a new coordinator instance
func NewCoordinator(redis *redis.Client, sdk *sdk.SDK, logger Logger) *Coordinator {
	return &Coordinator{
		redis:     redis,
		sdk:       sdk,
		logger:    logger,
		router:    NewStreamRouter(),
		evaluator: condition.NewEvaluator(),
		resolver:  resolver.NewResolver(sdk, logger),
	}
}

// Start begins the coordinator main loop
func (c *Coordinator) Start(ctx context.Context) error {
	c.logger.Info("coordinator starting", "queue", "completion_signals")

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("coordinator shutting down")
			return ctx.Err()
		default:
			// Block waiting for completion signals (5 second timeout)
			result := c.redis.BLPop(ctx, 5*time.Second, "completion_signals")
			if result.Err() == redis.Nil {
				// Timeout, continue loop
				continue
			}
			if result.Err() != nil {
				c.logger.Error("failed to read completion signal", "error", result.Err())
				continue
			}

			// Parse signal (result.Val()[1] contains the JSON payload)
			if len(result.Val()) < 2 {
				c.logger.Error("invalid completion signal format")
				continue
			}

			var signal CompletionSignal
			if err := json.Unmarshal([]byte(result.Val()[1]), &signal); err != nil {
				c.logger.Error("failed to parse completion signal", "error", err)
				continue
			}

			// Handle completion in goroutine for parallel processing
			go c.handleCompletion(ctx, &signal)
		}
	}
}

// handleCompletion processes a completion signal and routes to next nodes
func (c *Coordinator) handleCompletion(ctx context.Context, signal *CompletionSignal) {
	c.logger.Info("handling completion",
		"run_id", signal.RunID,
		"node_id", signal.NodeID,
		"status", signal.Status)

	// 1. Load latest IR from Redis (might be patched!)
	ir, err := c.loadIR(ctx, signal.RunID)
	if err != nil {
		c.logger.Error("failed to load IR",
			"run_id", signal.RunID,
			"error", err)
		return
	}

	node, exists := ir.Nodes[signal.NodeID]
	if !exists {
		c.logger.Error("node not found in IR",
			"run_id", signal.RunID,
			"node_id", signal.NodeID)
		return
	}

	// 2. Handle failed execution
	if signal.Status == "failed" {
		c.logger.Error("node execution failed",
			"run_id", signal.RunID,
			"node_id", signal.NodeID,
			"result_ref", signal.ResultRef,
			"error", signal.Metadata)

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
		if ir.Metadata != nil {
			if username, ok := ir.Metadata["username"].(string); ok {
				c.publishWorkflowEvent(ctx, username, map[string]interface{}{
					"type":      "node_failed",
					"run_id":    signal.RunID,
					"node_id":   signal.NodeID,
					"error":     signal.Metadata,
					"timestamp": time.Now().Unix(),
				})

				// Also publish workflow_failed event to indicate the entire workflow failed
				c.publishWorkflowEvent(ctx, username, map[string]interface{}{
					"type":      "workflow_failed",
					"run_id":    signal.RunID,
					"node_id":   signal.NodeID,
					"error":     signal.Metadata,
					"timestamp": time.Now().Unix(),
				})
			}
		}

		// TODO: Handle failure (DLQ, retry, etc.)
		return
	}

	// 3. Consume token (apply -1 to counter)
	if err := c.sdk.Consume(ctx, signal.RunID, signal.NodeID); err != nil {
		c.logger.Error("failed to consume token",
			"run_id", signal.RunID,
			"node_id", signal.NodeID,
			"error", err)
		return
	}

	// Get counter after consumption for event
	counter, _ := c.sdk.GetCounter(ctx, signal.RunID)

	// Publish node_completed event
	if ir.Metadata != nil {
		if username, ok := ir.Metadata["username"].(string); ok {
			c.publishWorkflowEvent(ctx, username, map[string]interface{}{
				"type":       "node_completed",
				"run_id":     signal.RunID,
				"node_id":    signal.NodeID,
				"status":     signal.Status,
				"counter":    counter,
				"result_ref": signal.ResultRef,
				"timestamp":  time.Now().Unix(),
			})
		}
	}

	// 4. Load result from CAS for context
	if signal.ResultRef != "" {
		// Store in context for downstream nodes
		if err := c.sdk.StoreContext(ctx, signal.RunID, signal.NodeID+":output", signal.ResultRef); err != nil {
			c.logger.Error("failed to store context",
				"run_id", signal.RunID,
				"node_id", signal.NodeID,
				"error", err)
		}
	}

	// 5. Determine next nodes (handles branches, loops, etc.)
	nextNodes, err := c.determineNextNodes(ctx, signal, node, ir)
	if err != nil {
		c.logger.Error("failed to determine next nodes",
			"run_id", signal.RunID,
			"node_id", signal.NodeID,
			"error", err)
		return
	}

	c.logger.Debug("routing to next nodes",
		"run_id", signal.RunID,
		"from_node", signal.NodeID,
		"next_nodes", nextNodes,
		"count", len(nextNodes))

	// 6. Emit to appropriate streams and update counter
	if len(nextNodes) > 0 {
		for _, nextNodeID := range nextNodes {
			nextNode, exists := ir.Nodes[nextNodeID]
			if !exists {
				c.logger.Error("next node not found in IR",
					"run_id", signal.RunID,
					"next_node_id", nextNodeID)
				continue
			}

			// Load node config (inline or CAS)
			c.logger.Info("loading node config",
				"run_id", signal.RunID,
				"node_id", nextNodeID,
				"node_type", nextNode.Type,
				"has_inline_config", len(nextNode.Config) > 0,
				"has_config_ref", nextNode.ConfigRef != "",
				"inline_config", nextNode.Config,
				"config_ref", nextNode.ConfigRef)

			var config map[string]interface{}
			c.logger.Info("config is ehre",
				"config", nextNode.Config)
			if len(nextNode.Config) > 0 {
				config = nextNode.Config
				c.logger.Info("using inline config", "config", config)
			} else if nextNode.ConfigRef != "" {
				c.logger.Info("loading config from CAS", "config_ref", nextNode.ConfigRef)
				configData, err := c.sdk.LoadConfig(ctx, nextNode.ConfigRef)
				if err != nil {
					c.logger.Error("failed to load config from CAS",
						"run_id", signal.RunID,
						"node_id", nextNodeID,
						"config_ref", nextNode.ConfigRef,
						"error", err)
					continue
				}
				// Convert to map
				if configMap, ok := configData.(map[string]interface{}); ok {
					config = configMap
					c.logger.Info("loaded config from CAS", "config", config)
				} else {
					c.logger.Error("config is not a map",
						"run_id", signal.RunID,
						"node_id", nextNodeID)
					continue
				}
			} else {
				c.logger.Warn("node has no config (neither inline nor CAS ref)",
					"run_id", signal.RunID,
					"node_id", nextNodeID)
			}

			// Resolve variables in config (e.g., $nodes.node_id)
			c.logger.Info("about to resolve config",
				"run_id", signal.RunID,
				"node_id", nextNodeID,
				"config_is_nil", config == nil,
				"config", config)

			var resolvedConfig map[string]interface{}
			if config != nil {
				var err error
				resolvedConfig, err = c.resolver.ResolveConfig(ctx, signal.RunID, config)
				if err != nil {
					c.logger.Error("failed to resolve config variables",
						"run_id", signal.RunID,
						"node_id", nextNodeID,
						"error", err)
					// Continue with unresolved config as fallback
					resolvedConfig = config
				} else {
					c.logger.Info("resolved config variables successfully",
						"run_id", signal.RunID,
						"node_id", nextNodeID,
						"resolvedConfig", resolvedConfig)
				}
			} else {
				c.logger.Warn("config is nil, cannot resolve - resolvedConfig will be nil",
					"run_id", signal.RunID,
					"node_id", nextNodeID)
			}

			// Get appropriate stream for node type
			stream := c.router.GetStreamForNodeType(nextNode.Type)

			// Publish token to stream with resolved config
			if err := c.publishToken(ctx, stream, signal.RunID, signal.NodeID, nextNodeID, signal.ResultRef, resolvedConfig); err != nil {
				c.logger.Error("failed to publish token",
					"run_id", signal.RunID,
					"to_node", nextNodeID,
					"stream", stream,
					"error", err)
				continue
			}

			c.logger.Debug("published token",
				"run_id", signal.RunID,
				"from_node", signal.NodeID,
				"to_node", nextNodeID,
				"stream", stream)
		}

		// Apply counter update (+N)
		if err := c.sdk.Emit(ctx, signal.RunID, signal.NodeID, nextNodes, signal.ResultRef); err != nil {
			c.logger.Error("failed to emit counter update",
				"run_id", signal.RunID,
				"node_id", signal.NodeID,
				"next_nodes_count", len(nextNodes),
				"error", err)
		}
	}

	// 7. Terminal node check
	if node.IsTerminal {
		c.logger.Debug("terminal node completed, checking for run completion",
			"run_id", signal.RunID,
			"node_id", signal.NodeID)
		c.checkCompletion(ctx, signal.RunID)
	}
}

// loadIR loads the latest IR from Redis (no caching for patch support)
func (c *Coordinator) loadIR(ctx context.Context, runID string) (*sdk.IR, error) {
	key := fmt.Sprintf("ir:%s", runID)
	data, err := c.redis.Get(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get IR from Redis: %w", err)
	}

	var ir sdk.IR
	if err := json.Unmarshal([]byte(data), &ir); err != nil {
		return nil, fmt.Errorf("failed to unmarshal IR: %w", err)
	}

	return &ir, nil
}

// determineNextNodes determines which nodes to route to based on node config
func (c *Coordinator) determineNextNodes(ctx context.Context, signal *CompletionSignal, node *sdk.Node, ir *sdk.IR) ([]string, error) {
	// 1. Check for loop configuration
	if node.Loop != nil && node.Loop.Enabled {
		return c.handleLoop(ctx, signal, node)
	}

	// 2. Check for branch configuration
	if node.Branch != nil && node.Branch.Enabled {
		return c.handleBranch(ctx, signal, node)
	}

	// 3. Default: static dependents
	return node.Dependents, nil
}

// handleLoop determines next nodes for loop configuration
func (c *Coordinator) handleLoop(ctx context.Context, signal *CompletionSignal, node *sdk.Node) ([]string, error) {
	loopKey := fmt.Sprintf("loop:%s:%s", signal.RunID, signal.NodeID)

	// Increment iteration counter
	iteration, err := c.redis.HIncrBy(ctx, loopKey, "current_iteration", 1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to increment loop iteration: %w", err)
	}

	c.logger.Debug("loop iteration",
		"run_id", signal.RunID,
		"node_id", signal.NodeID,
		"iteration", iteration,
		"max", node.Loop.MaxIterations)

	// Check max iterations
	if int(iteration) >= node.Loop.MaxIterations {
		c.logger.Info("loop max iterations reached",
			"run_id", signal.RunID,
			"node_id", signal.NodeID,
			"iterations", iteration)
		// Cleanup loop state
		c.redis.Del(ctx, loopKey)
		// Exit to timeout_path
		return node.Loop.TimeoutPath, nil
	}

	// Evaluate condition if present
	if node.Loop.Condition != nil {
		// Load output from CAS for condition evaluation
		output, err := c.sdk.LoadPayload(ctx, signal.ResultRef)
		if err != nil {
			c.logger.Error("failed to load output for loop condition",
				"run_id", signal.RunID,
				"node_id", signal.NodeID,
				"error", err)
			// On error, break loop
			c.redis.Del(ctx, loopKey)
			return node.Loop.BreakPath, nil
		}

		// Load context
		context, err := c.sdk.LoadContext(ctx, signal.RunID)
		if err != nil {
			c.logger.Warn("failed to load context for loop condition",
				"run_id", signal.RunID,
				"error", err)
			context = make(map[string]interface{})
		}

		// Evaluate condition
		conditionMet, err := c.evaluator.Evaluate(node.Loop.Condition, output, context)
		if err != nil {
			c.logger.Error("loop condition evaluation failed",
				"run_id", signal.RunID,
				"node_id", signal.NodeID,
				"expression", node.Loop.Condition.Expression,
				"error", err)
			// On error, break loop
			c.redis.Del(ctx, loopKey)
			return node.Loop.BreakPath, nil
		}

		c.logger.Debug("loop condition evaluated",
			"run_id", signal.RunID,
			"node_id", signal.NodeID,
			"condition_met", conditionMet)

		if conditionMet {
			// Continue looping
			return []string{node.Loop.LoopBackTo}, nil
		}

		// Condition not met, break loop
		c.redis.Del(ctx, loopKey)
		return node.Loop.BreakPath, nil
	}

	// No condition, continue looping (will eventually hit max iterations)
	return []string{node.Loop.LoopBackTo}, nil
}

// handleBranch determines next nodes for branch configuration
func (c *Coordinator) handleBranch(ctx context.Context, signal *CompletionSignal, node *sdk.Node) ([]string, error) {
	// Load output from CAS for condition evaluation
	output, err := c.sdk.LoadPayload(ctx, signal.ResultRef)
	if err != nil {
		c.logger.Error("failed to load output for branch condition",
			"run_id", signal.RunID,
			"node_id", signal.NodeID,
			"error", err)
		// On error, use default path
		return node.Branch.Default, nil
	}

	// Load context
	context, err := c.sdk.LoadContext(ctx, signal.RunID)
	if err != nil {
		c.logger.Warn("failed to load context for branch condition",
			"run_id", signal.RunID,
			"error", err)
		context = make(map[string]interface{})
	}

	// Evaluate rules in order
	for i, rule := range node.Branch.Rules {
		if rule.Condition == nil {
			c.logger.Warn("branch rule has nil condition, skipping",
				"run_id", signal.RunID,
				"node_id", signal.NodeID,
				"rule_index", i)
			continue
		}

		conditionMet, err := c.evaluator.Evaluate(rule.Condition, output, context)
		if err != nil {
			c.logger.Warn("branch rule evaluation failed",
				"run_id", signal.RunID,
				"node_id", signal.NodeID,
				"rule_index", i,
				"expression", rule.Condition.Expression,
				"error", err)
			continue
		}

		c.logger.Debug("branch rule evaluated",
			"run_id", signal.RunID,
			"node_id", signal.NodeID,
			"rule_index", i,
			"condition_met", conditionMet)

		if conditionMet {
			c.logger.Info("branch rule matched",
				"run_id", signal.RunID,
				"node_id", signal.NodeID,
				"rule_index", i,
				"next_nodes", rule.NextNodes)
			return rule.NextNodes, nil
		}
	}

	// No rule matched, use default
	c.logger.Debug("no branch rule matched, using default",
		"run_id", signal.RunID,
		"node_id", signal.NodeID,
		"default", node.Branch.Default)
	return node.Branch.Default, nil
}

// publishToken publishes a token to a Redis stream with resolved config
func (c *Coordinator) publishToken(ctx context.Context, stream, runID, fromNode, toNode, payloadRef string, resolvedConfig map[string]interface{}) error {
	// Generate unique job ID for this token
	jobID := fmt.Sprintf("%s-%s-%s", runID, toNode, time.Now().UnixNano())

	// Debug log the resolvedConfig
	c.logger.Info("publishToken called",
		"run_id", runID,
		"to_node", toNode,
		"resolvedConfig_nil", resolvedConfig == nil,
		"resolvedConfig", resolvedConfig)

	token := map[string]interface{}{
		"id":          jobID, // Add job ID for agent-runner-py
		"run_id":      runID,
		"from_node":   fromNode,
		"to_node":     toNode,
		"payload_ref": payloadRef,
		"created_at":  time.Now().UTC().Format(time.RFC3339),
	}

	// Include resolved config if available
	if resolvedConfig != nil {
		token["config"] = resolvedConfig
		c.logger.Info("added config to token", "config", resolvedConfig)
	} else {
		c.logger.Warn("resolvedConfig is nil, skipping config and metadata")
	}

	// Extract task from config and add to metadata for agent-runner-py
	// Agent runner expects metadata.task
	// Support both "task" (new) and "prompt" (old) for backward compatibility
	metadata := make(map[string]interface{})
	if resolvedConfig != nil {
		// Try "task" first (new field name)
		if task, ok := resolvedConfig["task"]; ok {
			metadata["task"] = task
		} else if prompt, ok := resolvedConfig["prompt"]; ok {
			// Fall back to "prompt" for backward compatibility
			metadata["task"] = prompt
		}
		// Also pass the entire workflow context if available
		if workflow, ok := resolvedConfig["workflow"]; ok {
			metadata["workflow"] = workflow
		}
	}
	if len(metadata) > 0 {
		token["metadata"] = metadata
		c.logger.Info("added metadata to token",
			"metadata", metadata,
			"task_value", metadata["task"])
	} else {
		c.logger.Warn("metadata is empty, not adding to token",
			"resolvedConfig_nil", resolvedConfig == nil)
	}

	tokenJSON, err := json.Marshal(token)
	c.logger.Info("marshaled token", "token_json", string(tokenJSON))
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}

	_, err = c.redis.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]interface{}{
			"token":   string(tokenJSON),
			"run_id":  runID,
			"to_node": toNode,
		},
	}).Result()

	if err != nil {
		return fmt.Errorf("failed to add to stream: %w", err)
	}

	c.logger.Debug("published token with job_id",
		"run_id", runID,
		"job_id", jobID,
		"to_node", toNode,
		"has_task", metadata["task"] != nil)

	return nil
}

// checkCompletion checks if the workflow run is complete
func (c *Coordinator) checkCompletion(ctx context.Context, runID string) {
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
				c.publishWorkflowEvent(ctx, username, map[string]interface{}{
					"type":      "workflow_completed",
					"run_id":    runID,
					"counter":   0,
					"timestamp": time.Now().Unix(),
				})
			}
		}

		// TODO: Mark run as COMPLETED in database
		// TODO: Cleanup Redis keys
	}
}

// publishWorkflowEvent publishes an event to Redis PubSub for fanout service
func (c *Coordinator) publishWorkflowEvent(ctx context.Context, username string, event map[string]interface{}) {
	channel := fmt.Sprintf("workflow:events:%s", username)

	eventJSON, err := json.Marshal(event)
	if err != nil {
		c.logger.Error("failed to marshal workflow event", "error", err)
		return
	}

	err = c.redis.Publish(ctx, channel, eventJSON).Err()
	if err != nil {
		c.logger.Error("failed to publish workflow event",
			"channel", channel,
			"error", err)
		return
	}

	c.logger.Debug("published workflow event",
		"channel", channel,
		"type", event["type"])
}
