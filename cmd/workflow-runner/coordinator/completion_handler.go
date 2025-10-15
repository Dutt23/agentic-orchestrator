package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lyzr/orchestrator/cmd/workflow-runner/operators"
)

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

		// Store initial node count before patch
		initialNodeCount := len(ir.Nodes)

		// Check if patches were created during this run
		if err := c.reloadIRIfPatched(ctx, signal.RunID, ir); err != nil {
			c.logger.Error("CRITICAL: failed to reload IR after patch",
				"run_id", signal.RunID,
				"node_id", signal.NodeID,
				"error", err,
				"error_type", fmt.Sprintf("%T", err))
			// Continue execution even if patch reload fails
		} else {
			// Reload successful, check if IR actually changed
			newIR, _ := c.loadIR(ctx, signal.RunID)
			if newIR != nil {
				newNodeCount := len(newIR.Nodes)
				if newNodeCount != initialNodeCount {
					c.logger.Info("IR updated successfully after patch",
						"run_id", signal.RunID,
						"node_id", signal.NodeID,
						"initial_nodes", initialNodeCount,
						"new_nodes", newNodeCount,
						"nodes_added", newNodeCount-initialNodeCount)
				} else {
					c.logger.Warn("IR reload completed but node count unchanged",
						"run_id", signal.RunID,
						"node_count", newNodeCount)
				}
			}
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
