package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lyzr/orchestrator/cmd/workflow-runner/condition"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/operators"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/resolver"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/sdk"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/workflow_lifecycle"
	"github.com/lyzr/orchestrator/common/clients"
	redisWrapper "github.com/lyzr/orchestrator/common/redis"
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
	redis               *redis.Client // Raw client for BLPOP and other blocking ops
	redisWrapper        *redisWrapper.Client // Wrapped client for common ops
	sdk                 *sdk.SDK
	logger              Logger
	router              *StreamRouter
	evaluator           *condition.Evaluator
	resolver            *resolver.Resolver
	orchestratorClient  *clients.OrchestratorClient
	orchestratorBaseURL string

	// Extracted modules for clean separation of concerns
	operators *OperatorOpts
	lifecycle *LifecycleHandlerOpts
}

// OperatorOpts contains all control flow operators
type OperatorOpts struct {
	ControlFlowRouter *operators.ControlFlowRouter
}

// LifecycleHandlerOpts contains all workflow lifecycle handlers
type LifecycleHandlerOpts struct {
	CompletionChecker *workflow_lifecycle.CompletionChecker
	EventPublisher    *workflow_lifecycle.EventPublisher
	StatusManager     *workflow_lifecycle.StatusManager
}

// Logger interface for logging
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Debug(msg string, keysAndValues ...interface{})
}

// CoordinatorOpts contains options for creating a coordinator
type CoordinatorOpts struct {
	Redis               *redis.Client
	SDK                 *sdk.SDK
	Logger              Logger
	OrchestratorBaseURL string
}

// NewCoordinator creates a new coordinator instance
func NewCoordinator(opts *CoordinatorOpts) *Coordinator {
	orchestratorClient := clients.NewOrchestratorClient(opts.OrchestratorBaseURL, opts.Logger)
	evaluator := condition.NewEvaluator()

	// Wrap Redis client for better abstractions and instrumentation
	redisClient := redisWrapper.NewClient(opts.Redis, opts.Logger)

	// Create workflow lifecycle modules with wrapped Redis client
	eventPublisher := workflow_lifecycle.NewEventPublisher(redisClient, opts.Logger)
	statusManager := workflow_lifecycle.NewStatusManager(redisClient, opts.Logger)
	completionChecker := workflow_lifecycle.NewCompletionChecker(redisClient, opts.SDK, opts.Logger, eventPublisher, statusManager)

	// Create control flow router (still uses raw Redis for complex operations like XREADGROUP)
	controlFlowRouter := operators.NewControlFlowRouter(opts.Redis, opts.SDK, evaluator, opts.Logger)

	return &Coordinator{
		redis:               opts.Redis, // Keep raw for BLPOP
		redisWrapper:        redisClient, // Use wrapper for common ops
		sdk:                 opts.SDK,
		logger:              opts.Logger,
		router:              NewStreamRouter(),
		evaluator:           evaluator,
		resolver:            resolver.NewResolver(opts.SDK, opts.Logger),
		orchestratorClient:  orchestratorClient,
		orchestratorBaseURL: opts.OrchestratorBaseURL,
		operators: &OperatorOpts{
			ControlFlowRouter: controlFlowRouter,
		},
		lifecycle: &LifecycleHandlerOpts{
			CompletionChecker: completionChecker,
			EventPublisher:    eventPublisher,
			StatusManager:     statusManager,
		},
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
				c.lifecycle.EventPublisher.PublishWorkflowEvent(ctx, username, map[string]interface{}{
					"type":      "node_failed",
					"run_id":    signal.RunID,
					"node_id":   signal.NodeID,
					"error":     signal.Metadata,
					"timestamp": time.Now().Unix(),
				})

				// Also publish workflow_failed event to indicate the entire workflow failed
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
		return
	}

	// 3. Check if this was an agent node that might have created patches
	if node.Type == "agent" {
		c.logger.Info("agent node completed, checking for run patches",
			"run_id", signal.RunID,
			"node_id", signal.NodeID)

		// Check if patches were created during this run
		if err := c.reloadIRIfPatched(ctx, signal.RunID, ir); err != nil {
			c.logger.Error("failed to reload IR after patch",
				"run_id", signal.RunID,
				"node_id", signal.NodeID,
				"error", err)
			// Continue execution even if patch reload fails
		}
	}

	// 4. Consume token (apply -1 to counter)
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
			c.lifecycle.EventPublisher.PublishWorkflowEvent(ctx, username, map[string]interface{}{
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

	// 5. Load result from CAS for context
	if signal.ResultRef != "" {
		// Store in context for downstream nodes
		if err := c.sdk.StoreContext(ctx, signal.RunID, signal.NodeID+":output", signal.ResultRef); err != nil {
			c.logger.Error("failed to store context",
				"run_id", signal.RunID,
				"node_id", signal.NodeID,
				"error", err)
		}
	}

	// 6. Reload IR to get latest version with patches (if any)
	ir, err = c.loadIR(ctx, signal.RunID)
	if err != nil {
		c.logger.Error("failed to reload IR",
			"run_id", signal.RunID,
			"error", err)
		return
	}

	// 7. Determine next nodes (handles branches, loops, etc.)
	nextNodes, err := c.operators.ControlFlowRouter.DetermineNextNodes(ctx, &operators.CompletionSignal{
		Version:   signal.Version,
		JobID:     signal.JobID,
		RunID:     signal.RunID,
		NodeID:    signal.NodeID,
		Status:    signal.Status,
		ResultRef: signal.ResultRef,
		Metadata:  signal.Metadata,
	}, node, ir)
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

	// 8. Emit to appropriate streams and update counter
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

			// Publish token to stream with resolved config and IR
			if err := c.publishToken(ctx, stream, signal.RunID, signal.NodeID, nextNodeID, signal.ResultRef, resolvedConfig, ir); err != nil {
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

	// 9. Terminal node check
	if node.IsTerminal {
		c.logger.Debug("terminal node completed, checking for run completion",
			"run_id", signal.RunID,
			"node_id", signal.NodeID)
		c.lifecycle.CompletionChecker.CheckCompletion(ctx, signal.RunID)
	}
}

// reloadIRIfPatched checks if patches exist and reloads the IR with patches applied
func (c *Coordinator) reloadIRIfPatched(ctx context.Context, runID string, currentIR *sdk.IR) error {
	c.logger.Info("checking for run patches", "run_id", runID)

	// Extract username from IR metadata and add to context
	// This will automatically be used for authentication headers in HTTP requests
	if currentIR.Metadata != nil {
		if username, ok := currentIR.Metadata["username"].(string); ok {
			ctx = clients.WithUserID(ctx, username)
			c.logger.Info("added username to context for orchestrator client", "username", username)
		}
	}

	// Fetch patches from orchestrator (context automatically includes auth)
	patches, err := c.orchestratorClient.GetRunPatchesWithOperations(ctx, runID)
	if err != nil {
		return fmt.Errorf("failed to get run patches: %w", err)
	}

	if len(patches) == 0 {
		c.logger.Debug("no run patches found, IR unchanged")
		return nil
	}

	c.logger.Info("run patches found, materializing workflow",
		"patch_count", len(patches))

	// Get base workflow from IR metadata
	// The IR was compiled from a workflow and should have the original workflow stored
	// We need to reconstruct the workflow format from the IR
	baseWorkflow := c.irToWorkflow(currentIR)

	// Materialize workflow with patches applied
	patchedWorkflow, err := c.orchestratorClient.MaterializeWorkflowForRun(ctx, baseWorkflow, runID)
	if err != nil {
		return fmt.Errorf("failed to materialize workflow: %w", err)
	}

	c.logger.Info("patched workflow materialized, updating IR in Redis")

	// TODO: Recompile the patched workflow to IR format
	// For now, we'll store both: keep the current IR structure but update it with new nodes/edges

	// Convert patched workflow back to IR format and update Redis
	// For now, we store the patched workflow in a way that loadIR can handle
	patchedIRJSON, err := json.Marshal(patchedWorkflow)
	if err != nil {
		return fmt.Errorf("failed to marshal patched workflow: %w", err)
	}

	// Store updated workflow in Redis at a temporary key
	// The IR will be recompiled on next load
	workflowKey := fmt.Sprintf("workflow:%s:patched", runID)
	if err := c.redisWrapper.SetWithExpiry(ctx, workflowKey, string(patchedIRJSON), 0); err != nil {
		return fmt.Errorf("failed to store patched workflow: %w", err)
	}

	c.logger.Info("patched workflow stored, will be recompiled on next IR load",
		"run_id", runID,
		"patches_applied", len(patches))

	return nil
}

// irToWorkflow converts IR back to workflow schema format
func (c *Coordinator) irToWorkflow(ir *sdk.IR) map[string]interface{} {
	// Build workflow from IR
	nodes := []map[string]interface{}{}
	edges := []map[string]interface{}{}

	// Convert nodes
	for _, node := range ir.Nodes {
		wfNode := map[string]interface{}{
			"id":   node.ID,
			"type": node.Type,
		}
		if node.Config != nil {
			wfNode["config"] = node.Config
		}
		nodes = append(nodes, wfNode)

		// Convert edges from dependents
		for _, dep := range node.Dependents {
			edges = append(edges, map[string]interface{}{
				"from": node.ID,
				"to":   dep,
			})
		}
	}

	workflow := map[string]interface{}{
		"nodes": nodes,
		"edges": edges,
	}

	// Add metadata if present
	if ir.Metadata != nil {
		workflow["metadata"] = ir.Metadata
	}

	return workflow
}

// loadIR loads the latest IR from Redis (no caching for patch support)
func (c *Coordinator) loadIR(ctx context.Context, runID string) (*sdk.IR, error) {
	key := fmt.Sprintf("ir:%s", runID)
	data, err := c.redisWrapper.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get IR from Redis: %w", err)
	}

	var ir sdk.IR
	if err := json.Unmarshal([]byte(data), &ir); err != nil {
		return nil, fmt.Errorf("failed to unmarshal IR: %w", err)
	}

	return &ir, nil
}

// publishToken publishes a token to a Redis stream with resolved config
func (c *Coordinator) publishToken(ctx context.Context, stream, runID, fromNode, toNode, payloadRef string, resolvedConfig map[string]interface{}, ir *sdk.IR) error {
	// Generate unique job ID for this token
	jobID := fmt.Sprintf("%s-%s-%d", runID, toNode, time.Now().UnixNano())

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

	// Add workflow_owner from IR metadata (required for patch_workflow tool)
	if ir.Metadata != nil {
		if username, ok := ir.Metadata["username"].(string); ok {
			token["workflow_owner"] = username
			c.logger.Info("added workflow_owner to metadata", "workflow_owner", username)
		}
		// Also add tag if available
		if tag, ok := ir.Metadata["tag"].(string); ok {
			metadata["workflow_tag"] = tag
			c.logger.Info("added workflow_tag to metadata", "workflow_tag", tag)
		}
	}

	if len(metadata) > 0 {
		token["metadata"] = metadata
		c.logger.Info("added metadata to token",
			"metadata", metadata,
			"task_value", metadata["task"],
			"workflow_owner", metadata["workflow_owner"])
	} else {
		c.logger.Warn("metadata is empty, not adding to token",
			"resolvedConfig_nil", resolvedConfig == nil)
	}

	tokenJSON, err := json.Marshal(token)
	c.logger.Info("marshaled token", "token_json", string(tokenJSON))
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}

	_, err = c.redisWrapper.AddToStream(ctx, stream, map[string]interface{}{
		"token":   string(tokenJSON),
		"run_id":  runID,
		"to_node": toNode,
	})

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

