package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lyzr/orchestrator/cmd/workflow-runner/compiler"
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
	Version    string                 `json:"version"`              // Protocol version (1.0)
	JobID      string                 `json:"job_id"`               // Unique job ID
	RunID      string                 `json:"run_id"`               // Workflow run ID
	NodeID     string                 `json:"node_id"`              // Node that completed
	Status     string                 `json:"status"`               // completed|failed
	ResultData map[string]interface{} `json:"result_data,omitempty"` // Actual result data (coordinator stores in CAS)
	ResultRef  string                 `json:"result_ref,omitempty"` // CAS reference (deprecated, for backward compat)
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
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
	casClient           clients.CASClient // CAS client for compiler

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
	CASClient           clients.CASClient
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
		casClient:           opts.CASClient,
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
		c.handleFailedNode(ctx, signal, ir)
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

	// 5. Store result data in CAS and create reference
	resultRef := c.storeResultInCAS(ctx, signal)

	// Publish node_completed event
	if ir.Metadata != nil {
		if username, ok := ir.Metadata["username"].(string); ok {
			c.lifecycle.EventPublisher.PublishWorkflowEvent(ctx, username, map[string]interface{}{
				"type":       "node_completed",
				"run_id":     signal.RunID,
				"node_id":    signal.NodeID,
				"status":     signal.Status,
				"counter":    counter,
				"result_ref": resultRef,
				"timestamp":  time.Now().Unix(),
			})
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

	// Re-fetch node from patched IR (in case patches changed its edges or terminal status)
	node, exists = ir.Nodes[signal.NodeID]
	if !exists {
		c.logger.Error("node not found in patched IR",
			"run_id", signal.RunID,
			"node_id", signal.NodeID)
		return
	}

	c.logger.Info("re-fetched node from patched IR",
		"run_id", signal.RunID,
		"node_id", signal.NodeID,
		"is_terminal", node.IsTerminal,
		"dependents_count", len(node.Dependents))

	// 7. Determine next nodes (handles branches, loops, etc.)
	c.logger.Info("about to determine next nodes",
		"run_id", signal.RunID,
		"node_id", signal.NodeID,
		"node_type", node.Type,
		"has_branch", node.Branch != nil && node.Branch.Enabled,
		"has_loop", node.Loop != nil && node.Loop.Enabled,
		"dependents", node.Dependents)

	nextNodes, err := c.operators.ControlFlowRouter.DetermineNextNodes(ctx, &operators.CompletionSignal{
		Version:   signal.Version,
		JobID:     signal.JobID,
		RunID:     signal.RunID,
		NodeID:    signal.NodeID,
		Status:    signal.Status,
		ResultRef: resultRef, // Use the CAS ref we just created
		Metadata:  signal.Metadata,
	}, node, ir)
	if err != nil {
		c.logger.Error("failed to determine next nodes",
			"run_id", signal.RunID,
			"node_id", signal.NodeID,
			"error", err)
		return
	}

	c.logger.Info("determined next nodes from branch/loop logic",
		"run_id", signal.RunID,
		"from_node", signal.NodeID,
		"next_nodes", nextNodes,
		"count", len(nextNodes))

	// 8. Emit to appropriate streams and update counter
	c.routeToNextNodes(ctx, signal, nextNodes, resultRef, ir)

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

	c.logger.Info("run patches found, fetching base workflow from DB",
		"patch_count", len(patches))

	// Fetch base workflow from DB (via orchestrator API)
	// This ensures we always apply ALL patches to the original base workflow
	// Not the current cached IR (which may already have patches applied)
	run, err := c.orchestratorClient.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("failed to get run: %w", err)
	}

	c.logger.Info("fetched run info",
		"run_id", runID,
		"base_ref", run.BaseRef)

	// Fetch base artifact containing the original workflow
	artifact, err := c.orchestratorClient.GetArtifact(ctx, run.BaseRef)
	if err != nil {
		return fmt.Errorf("failed to get base artifact: %w", err)
	}

	c.logger.Info("fetched base workflow artifact",
		"artifact_id", artifact.ArtifactID,
		"cas_id", artifact.CASID)

	// Use the artifact content as base workflow
	baseWorkflow := artifact.Content

	// Materialize workflow with patches applied
	// This applies ALL patches (1, 2, 3, ...) cumulatively to the base
	patchedWorkflow, err := c.orchestratorClient.MaterializeWorkflowForRun(ctx, baseWorkflow, runID)
	if err != nil {
		return fmt.Errorf("failed to materialize workflow: %w", err)
	}

	c.logger.Info("patched workflow materialized, recompiling to IR format")

	// Parse patched workflow into WorkflowSchema format for compiler
	workflowSchema := &compiler.WorkflowSchema{}

	// Marshal and unmarshal to convert map to struct
	patchedWorkflowJSON, err := json.Marshal(patchedWorkflow)
	if err != nil {
		return fmt.Errorf("failed to marshal patched workflow: %w", err)
	}

	if err := json.Unmarshal(patchedWorkflowJSON, workflowSchema); err != nil {
		return fmt.Errorf("failed to unmarshal patched workflow to schema: %w", err)
	}

	// Recompile the patched workflow to IR format
	patchedIR, err := compiler.CompileWorkflowSchema(workflowSchema, c.casClient)
	if err != nil {
		return fmt.Errorf("failed to compile patched workflow: %w", err)
	}

	// Preserve runtime metadata (username, tag) from current IR
	// The compiler preserves workflow metadata, but we need to ensure runtime metadata is kept
	if patchedIR.Metadata == nil {
		patchedIR.Metadata = make(map[string]interface{})
	}
	if currentIR.Metadata != nil {
		if username, ok := currentIR.Metadata["username"].(string); ok {
			patchedIR.Metadata["username"] = username
		}
		if tag, ok := currentIR.Metadata["tag"].(string); ok {
			patchedIR.Metadata["tag"] = tag
		}
	}

	c.logger.Info("patched workflow recompiled to IR",
		"run_id", runID,
		"node_count", len(patchedIR.Nodes),
		"patches_applied", len(patches),
		"metadata_preserved", patchedIR.Metadata)

	// Store the recompiled IR in Redis (replacing the old IR)
	patchedIRJSON, err := json.Marshal(patchedIR)
	if err != nil {
		return fmt.Errorf("failed to marshal patched IR: %w", err)
	}

	irKey := fmt.Sprintf("ir:%s", runID)
	if err := c.redisWrapper.Set(ctx, irKey, string(patchedIRJSON), 0); err != nil {
		return fmt.Errorf("failed to store patched IR: %w", err)
	}

	c.logger.Info("patched IR stored successfully",
		"run_id", runID,
		"ir_key", irKey,
		"patches_applied", len(patches))

	return nil
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

// storeResultInCAS stores the result data in CAS and returns the result reference
// Handles both new ResultData field and legacy ResultRef field for backward compatibility
func (c *Coordinator) storeResultInCAS(ctx context.Context, signal *CompletionSignal) string {
	var resultRef string

	if signal.ResultData != nil {
		// Generate CAS key
		resultID := fmt.Sprintf("artifact://%s-%s-%d", signal.RunID, signal.NodeID, time.Now().UnixNano())
		casKey := fmt.Sprintf("cas:%s", resultID)

		// Store result data in CAS
		resultJSON, err := json.Marshal(signal.ResultData)
		if err != nil {
			c.logger.Error("failed to marshal result data",
				"run_id", signal.RunID,
				"node_id", signal.NodeID,
				"error", err)
		} else {
			if err := c.redisWrapper.Set(ctx, casKey, string(resultJSON), 0); err != nil {
				c.logger.Error("failed to store result in CAS",
					"run_id", signal.RunID,
					"node_id", signal.NodeID,
					"cas_key", casKey,
					"error", err)
			} else {
				resultRef = resultID
				c.logger.Info("stored result in CAS",
					"run_id", signal.RunID,
					"node_id", signal.NodeID,
					"result_ref", resultRef,
					"cas_key", casKey)
			}
		}
	} else if signal.ResultRef != "" {
		// Backward compatibility: use provided ResultRef
		resultRef = signal.ResultRef
		c.logger.Debug("using provided result_ref (backward compat)",
			"run_id", signal.RunID,
			"node_id", signal.NodeID,
			"result_ref", resultRef)
	}

	// Store reference in context for downstream nodes
	if resultRef != "" {
		if err := c.sdk.StoreContext(ctx, signal.RunID, signal.NodeID, resultRef); err != nil {
			c.logger.Error("failed to store context",
				"run_id", signal.RunID,
				"node_id", signal.NodeID,
				"error", err)
		}
	}

	return resultRef
}

// routeToNextNodes processes and routes execution to next nodes
// Handles both absorber nodes (branch/loop) and worker nodes (http, agent, etc.)
func (c *Coordinator) routeToNextNodes(ctx context.Context, signal *CompletionSignal, nextNodes []string, resultRef string, ir *sdk.IR) {
	if len(nextNodes) == 0 {
		return
	}

	// Track which nodes are absorbers (handled inline) vs. workers (published to streams)
	absorberNodes := []string{}
	workerNodes := []string{}

	for _, nextNodeID := range nextNodes {
		nextNode, exists := ir.Nodes[nextNodeID]
		if !exists {
			c.logger.Error("next node not found in IR",
				"run_id", signal.RunID,
				"next_node_id", nextNodeID)
			continue
		}

		// Check if this is an absorber node (branch or loop) - handle inline
		// Absorber logic is encapsulated in Node.IsAbsorber()
		if nextNode.IsAbsorber() {
			c.logger.Info("detected absorber node (branch/loop) - handling inline",
				"run_id", signal.RunID,
				"node_id", nextNodeID,
				"has_branch", nextNode.Branch != nil && nextNode.Branch.Enabled,
				"has_loop", nextNode.Loop != nil && nextNode.Loop.Enabled)

			absorberNodes = append(absorberNodes, nextNodeID)

			// Handle absorber node inline - immediately trigger downstream nodes
			go c.handleAbsorberNode(ctx, signal.RunID, signal.NodeID, nextNodeID, resultRef, nextNode, ir)
			continue
		}

		// Regular worker node - publish to stream
		workerNodes = append(workerNodes, nextNodeID)
		c.processWorkerNode(ctx, signal, nextNodeID, nextNode, resultRef, ir)
	}

	c.logger.Info("next nodes categorized",
		"run_id", signal.RunID,
		"absorber_nodes", absorberNodes,
		"worker_nodes", workerNodes)

	// Apply counter update (+N) only for worker nodes
	// Absorber nodes will handle their own counter updates when they complete
	if len(workerNodes) > 0 {
		if err := c.sdk.Emit(ctx, signal.RunID, signal.NodeID, workerNodes, resultRef); err != nil {
			c.logger.Error("failed to emit counter update",
				"run_id", signal.RunID,
				"node_id", signal.NodeID,
				"next_nodes_count", len(workerNodes),
				"error", err)
		}
	}
}

// loadAndResolveConfig loads node config (inline or from CAS) and resolves variables
// Returns the resolved config map, or nil if config loading/resolution fails
func (c *Coordinator) loadAndResolveConfig(ctx context.Context, runID, nodeID string, node *sdk.Node) map[string]interface{} {
	// Load node config (inline or CAS)
	c.logger.Info("loading node config",
		"run_id", runID,
		"node_id", nodeID,
		"node_type", node.Type,
		"has_inline_config", len(node.Config) > 0,
		"has_config_ref", node.ConfigRef != "",
		"inline_config", node.Config,
		"config_ref", node.ConfigRef)

	var config map[string]interface{}
	c.logger.Info("config is ehre",
		"config", node.Config)
	if len(node.Config) > 0 {
		config = node.Config
		c.logger.Info("using inline config", "config", config)
	} else if node.ConfigRef != "" {
		c.logger.Info("loading config from CAS", "config_ref", node.ConfigRef)
		configData, err := c.sdk.LoadConfig(ctx, node.ConfigRef)
		if err != nil {
			c.logger.Error("failed to load config from CAS",
				"run_id", runID,
				"node_id", nodeID,
				"config_ref", node.ConfigRef,
				"error", err)
			return nil
		}
		// Convert to map
		if configMap, ok := configData.(map[string]interface{}); ok {
			config = configMap
			c.logger.Info("loaded config from CAS", "config", config)
		} else {
			c.logger.Error("config is not a map",
				"run_id", runID,
				"node_id", nodeID)
			return nil
		}
	} else {
		c.logger.Warn("node has no config (neither inline nor CAS ref)",
			"run_id", runID,
			"node_id", nodeID)
	}

	// Resolve variables in config (e.g., $nodes.node_id)
	c.logger.Info("about to resolve config",
		"run_id", runID,
		"node_id", nodeID,
		"config_is_nil", config == nil,
		"config", config)

	var resolvedConfig map[string]interface{}
	if config != nil {
		var err error
		resolvedConfig, err = c.resolver.ResolveConfig(ctx, runID, config)
		if err != nil {
			c.logger.Error("failed to resolve config variables",
				"run_id", runID,
				"node_id", nodeID,
				"error", err)
			// Continue with unresolved config as fallback
			resolvedConfig = config
		} else {
			c.logger.Info("resolved config variables successfully",
				"run_id", runID,
				"node_id", nodeID,
				"resolvedConfig", resolvedConfig)
		}
	} else {
		c.logger.Warn("config is nil, cannot resolve - resolvedConfig will be nil",
			"run_id", runID,
			"node_id", nodeID)
	}

	return resolvedConfig
}

// processWorkerNode handles a regular worker node (http, agent, etc.)
// Loads config, resolves variables, and publishes token to worker stream
func (c *Coordinator) processWorkerNode(ctx context.Context, signal *CompletionSignal, nextNodeID string, nextNode *sdk.Node, resultRef string, ir *sdk.IR) {
	// Check if we have a worker for this node type
	supportedTypes := map[string]bool{
		"http":  true,
		"agent": true,
		"hitl":  true,
		// Add other types as workers are implemented
	}

	if !supportedTypes[nextNode.Type] {
		c.logger.Warn("no worker available for node type, skipping to next nodes",
			"run_id", signal.RunID,
			"node_id", nextNodeID,
			"node_type", nextNode.Type)

		// Create a passthrough completion - node is skipped with a warning
		go c.handleSkippedNode(ctx, signal.RunID, signal.NodeID, nextNodeID, nextNode, resultRef, ir)
		return
	}

	// Load and resolve config
	resolvedConfig := c.loadAndResolveConfig(ctx, signal.RunID, nextNodeID, nextNode)
	if resolvedConfig == nil && nextNode.ConfigRef != "" {
		// Config loading failed for a node that requires config
		return
	}

	// Get appropriate stream for node type
	stream := c.router.GetStreamForNodeType(nextNode.Type)

	// Publish token to stream with resolved config and IR
	if err := c.publishToken(ctx, stream, signal.RunID, signal.NodeID, nextNodeID, resultRef, resolvedConfig, ir); err != nil {
		c.logger.Error("failed to publish token",
			"run_id", signal.RunID,
			"to_node", nextNodeID,
			"stream", stream,
			"error", err)
		return
	}

	c.logger.Debug("published token",
		"run_id", signal.RunID,
		"from_node", signal.NodeID,
		"to_node", nextNodeID,
		"stream", stream)
}

// handleSkippedNode immediately completes a node that has no worker available
// This prevents the workflow from hanging when agents add unsupported node types
func (c *Coordinator) handleSkippedNode(ctx context.Context, runID, fromNode, skippedNodeID string, skippedNode *sdk.Node, payloadRef string, ir *sdk.IR) {
	c.logger.Warn("handling skipped node (no worker available)",
		"run_id", runID,
		"from_node", fromNode,
		"skipped_node", skippedNodeID,
		"node_type", skippedNode.Type)

	// Create a warning result
	skippedOutput := map[string]interface{}{
		"status":  "skipped",
		"warning": fmt.Sprintf("No worker available for node type: %s", skippedNode.Type),
		"node_id": skippedNodeID,
		"metrics": map[string]interface{}{
			"start_time":        time.Now().Format(time.RFC3339Nano),
			"end_time":          time.Now().Format(time.RFC3339Nano),
			"execution_time_ms": 0,
		},
	}

	// Store in CAS so it appears in node executions
	resultID := fmt.Sprintf("artifact://%s-%s-%d", runID, skippedNodeID, time.Now().UnixNano())
	casKey := fmt.Sprintf("cas:%s", resultID)
	skippedJSON, err := json.Marshal(skippedOutput)
	if err == nil {
		if err := c.redisWrapper.Set(ctx, casKey, string(skippedJSON), 0); err == nil {
			c.sdk.StoreContext(ctx, runID, skippedNodeID, resultID)
		}
	}

	// Create synthetic completion signal
	syntheticSignal := &CompletionSignal{
		Version:    "1.0",
		JobID:      fmt.Sprintf("%s-%s-skipped", runID, skippedNodeID),
		RunID:      runID,
		NodeID:     skippedNodeID,
		Status:     "completed",
		ResultData: skippedOutput,
		Metadata: map[string]interface{}{
			"skipped": true,
			"reason":  "no_worker_available",
		},
	}

	// Process completion immediately (this will route to next nodes)
	c.handleCompletion(ctx, syntheticSignal)
}

// handleAbsorberNode handles branch/loop nodes inline (no worker needed)
func (c *Coordinator) handleAbsorberNode(ctx context.Context, runID, fromNode, absorberNodeID, payloadRef string, absorberNode *sdk.Node, ir *sdk.IR) {
	startTime := time.Now()
	c.logger.Info("handling absorber node inline",
		"run_id", runID,
		"from_node", fromNode,
		"absorber_node", absorberNodeID)

	// Create output with metrics for the absorber node (so it shows up in UI)
	// Branch/loop nodes execute in ~1ms with zero resources
	absorberOutput := map[string]interface{}{
		"status": "completed",
		"metrics": map[string]interface{}{
			"sent_at":           startTime.Format(time.RFC3339Nano),
			"start_time":        startTime.Format(time.RFC3339Nano),
			"end_time":          startTime.Add(1 * time.Millisecond).Format(time.RFC3339Nano),
			"queue_time_ms":     0,
			"execution_time_ms": 1, // Absorbers execute in ~1ms
			"total_duration_ms": 1,
			"memory_start_mb":   0.0,
			"memory_peak_mb":    0.0,
			"memory_end_mb":     0.0,
			"cpu_percent":       0.0,
			"thread_count":      0,
		},
	}

	// Store absorber output in CAS (so it appears in node executions)
	absorberResultID := fmt.Sprintf("artifact://%s-%s-%d", runID, absorberNodeID, time.Now().UnixNano())
	absorberCASKey := fmt.Sprintf("cas:%s", absorberResultID)
	absorberJSON, err := json.Marshal(absorberOutput)
	if err == nil {
		if err := c.redisWrapper.Set(ctx, absorberCASKey, string(absorberJSON), 0); err == nil {
			// Store reference in context
			c.sdk.StoreContext(ctx, runID, absorberNodeID, absorberResultID)
			c.logger.Debug("stored absorber output in CAS",
				"run_id", runID,
				"absorber_node", absorberNodeID,
				"result_ref", absorberResultID)
		}
	}

	// Create a synthetic completion signal for the absorber node
	// This allows us to reuse the existing control flow logic
	absorberSignal := &operators.CompletionSignal{
		Version:   "1.0",
		JobID:     fmt.Sprintf("%s-%s-absorber", runID, absorberNodeID),
		RunID:     runID,
		NodeID:    absorberNodeID,
		Status:    "completed",
		ResultRef: payloadRef, // Use the output from the previous node for condition evaluation
		Metadata:  make(map[string]interface{}),
	}

	// Determine next nodes using control flow logic (handles branch/loop evaluation)
	nextNodes, err := c.operators.ControlFlowRouter.DetermineNextNodes(ctx, absorberSignal, absorberNode, ir)
	if err != nil {
		c.logger.Error("failed to determine next nodes for absorber",
			"run_id", runID,
			"absorber_node", absorberNodeID,
			"error", err)
		return
	}

	c.logger.Info("absorber node determined next nodes",
		"run_id", runID,
		"absorber_node", absorberNodeID,
		"next_nodes", nextNodes,
		"count", len(nextNodes))

	// Emit tokens to next nodes (recursively handles nested absorbers)
	if len(nextNodes) > 0 {
		for _, nextNodeID := range nextNodes {
			nextNode, exists := ir.Nodes[nextNodeID]
			if !exists {
				c.logger.Error("next node not found in IR",
					"run_id", runID,
					"next_node_id", nextNodeID)
				continue
			}

			// Check if next node is also an absorber - recurse
			if nextNode.IsAbsorber() {
				c.logger.Info("next node is also an absorber - recursing",
					"run_id", runID,
					"absorber_node", absorberNodeID,
					"next_absorber", nextNodeID)
				go c.handleAbsorberNode(ctx, runID, absorberNodeID, nextNodeID, payloadRef, nextNode, ir)
				continue
			}

			// Check if we have a worker for this node type
			supportedTypes := map[string]bool{
				"http":  true,
				"agent": true,
				"hitl":  true,
			}

			if !supportedTypes[nextNode.Type] {
				c.logger.Warn("no worker for node type from absorber, skipping",
					"run_id", runID,
					"absorber_node", absorberNodeID,
					"skipped_node", nextNodeID,
					"node_type", nextNode.Type)

				// Skip this node and move to its dependents
				go c.handleSkippedNode(ctx, runID, absorberNodeID, nextNodeID, nextNode, payloadRef, ir)
				continue
			}

			// Load and resolve config for worker node
			resolvedConfig := c.loadAndResolveConfig(ctx, runID, nextNodeID, nextNode)
			if resolvedConfig == nil && nextNode.ConfigRef != "" {
				// Config loading failed for a node that requires config
				continue
			}

			// Publish to worker stream
			stream := c.router.GetStreamForNodeType(nextNode.Type)
			if err := c.publishToken(ctx, stream, runID, absorberNodeID, nextNodeID, payloadRef, resolvedConfig, ir); err != nil {
				c.logger.Error("failed to publish token from absorber",
					"run_id", runID,
					"absorber_node", absorberNodeID,
					"to_node", nextNodeID,
					"error", err)
				continue
			}

			c.logger.Debug("absorber published token to worker",
				"run_id", runID,
				"absorber_node", absorberNodeID,
				"to_node", nextNodeID,
				"stream", stream)
		}

		// Update counter for worker nodes emitted by absorber
		if err := c.sdk.Emit(ctx, runID, absorberNodeID, nextNodes, payloadRef); err != nil {
			c.logger.Error("failed to emit counter update from absorber",
				"run_id", runID,
				"absorber_node", absorberNodeID,
				"next_nodes_count", len(nextNodes),
				"error", err)
		}
	}

	c.logger.Info("absorber node completed inline",
		"run_id", runID,
		"absorber_node", absorberNodeID,
		"emitted_count", len(nextNodes))
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

	sentAt := time.Now().UTC()
	token := map[string]interface{}{
		"id":          jobID, // Add job ID for agent-runner-py
		"run_id":      runID,
		"from_node":   fromNode,
		"to_node":     toNode,
		"payload_ref": payloadRef,
		"created_at":  sentAt.Format(time.RFC3339),
		"sent_at":     sentAt.Format(time.RFC3339Nano), // High precision timestamp for metrics
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

