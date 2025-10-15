package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lyzr/orchestrator/cmd/workflow-runner/operators"
	"github.com/lyzr/orchestrator/common/sdk"
)

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
