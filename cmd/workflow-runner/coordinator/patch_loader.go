package coordinator

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lyzr/orchestrator/cmd/workflow-runner/compiler"
	"github.com/lyzr/orchestrator/common/ratelimit"
	"github.com/lyzr/orchestrator/common/sdk"
	"github.com/lyzr/orchestrator/common/clients"
)

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

// reloadIRIfPatched checks if patches exist and reloads the IR with patches applied
func (c *Coordinator) reloadIRIfPatched(ctx context.Context, runID string, currentIR *sdk.IR) error {
	c.logger.Info("=== PATCH RELOAD START ===",
		"run_id", runID,
		"current_nodes", len(currentIR.Nodes))

	// Extract username from IR metadata and add to context
	// This will automatically be used for authentication headers in HTTP requests
	if currentIR.Metadata != nil {
		if username, ok := currentIR.Metadata["username"].(string); ok {
			ctx = clients.WithUserID(ctx, username)
			c.logger.Info("added username to context for orchestrator client", "username", username)
		}
	}

	// Fetch patches from orchestrator (context automatically includes auth)
	c.logger.Info("fetching patches from orchestrator API", "run_id", runID)
	patches, err := c.orchestratorClient.GetRunPatchesWithOperations(ctx, runID)
	if err != nil {
		c.logger.Error("ERROR: failed to fetch patches from orchestrator",
			"run_id", runID,
			"error", err)
		return fmt.Errorf("failed to get run patches: %w", err)
	}

	c.logger.Info("patches fetched successfully",
		"run_id", runID,
		"patch_count", len(patches))

	if len(patches) == 0 {
		c.logger.Debug("no run patches found, IR unchanged")
		return nil
	}

	c.logger.Info("run patches found, fetching base workflow from DB",
		"patch_count", len(patches),
		"first_patch_ops", len(patches[0].Operations))

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
	c.logger.Info("compiling patched workflow to IR...",
		"run_id", runID,
		"nodes_in_schema", len(workflowSchema.Nodes),
		"edges_in_schema", len(workflowSchema.Edges))

	patchedIR, err := compiler.CompileWorkflowSchema(workflowSchema, c.casClient)
	if err != nil {
		c.logger.Error("ERROR: failed to compile patched workflow",
			"run_id", runID,
			"error", err,
			"nodes_attempted", len(workflowSchema.Nodes))
		return fmt.Errorf("failed to compile patched workflow: %w", err)
	}

	c.logger.Info("compilation successful",
		"run_id", runID,
		"compiled_nodes", len(patchedIR.Nodes))

	// SECURITY: Check if too many agents were added (prevent runaway workflows)
	c.logger.Info("=== SECURITY CHECK START ===",
		"run_id", runID,
		"rate_limiter_exists", c.rateLimiter != nil)

	if c.rateLimiter == nil {
		c.logger.Error("CRITICAL: rate limiter is nil, cannot enforce agent limits!",
			"run_id", runID)
	} else {
		workflowMap := map[string]interface{}{
			"nodes": patchedIR.Nodes,
		}
		profile := ratelimit.InspectWorkflow(workflowMap)

		c.logger.Info("workflow re-inspected after patch",
			"run_id", runID,
			"agent_count", profile.AgentCount,
			"tier", profile.Tier,
			"total_nodes", profile.TotalNodes,
			"max_allowed", 5)

		// Block if too many agents (5 or more)
		// Allow up to 4 spawned (1 original + 4 spawned = 5 total)
		if profile.AgentCount > 5 {
			c.logger.Error("!!! BLOCKING WORKFLOW !!!",
				"run_id", runID,
				"agent_count", profile.AgentCount,
				"max_allowed", 5,
				"reason", "excessive_agents")
			return fmt.Errorf("SECURITY: workflow has %d agent nodes (max 5 allowed). Blocking to protect OpenAI quota.", profile.AgentCount)
		} else {
			c.logger.Info("security check passed",
				"run_id", runID,
				"agent_count", profile.AgentCount,
				"status", "allowed")
		}
	}

	c.logger.Info("=== SECURITY CHECK END ===")

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
	c.logger.Info("marshaling IR for storage...", "run_id", runID)
	patchedIRJSON, err := json.Marshal(patchedIR)
	if err != nil {
		c.logger.Error("ERROR: failed to marshal patched IR",
			"run_id", runID,
			"error", err)
		return fmt.Errorf("failed to marshal patched IR: %w", err)
	}

	c.logger.Info("storing IR in Redis...",
		"run_id", runID,
		"ir_size_bytes", len(patchedIRJSON))

	irKey := fmt.Sprintf("ir:%s", runID)
	if err := c.redisWrapper.Set(ctx, irKey, string(patchedIRJSON), 0); err != nil {
		c.logger.Error("ERROR: failed to store patched IR in Redis",
			"run_id", runID,
			"error", err)
		return fmt.Errorf("failed to store patched IR: %w", err)
	}

	c.logger.Info("=== PATCH RELOAD SUCCESS ===",
		"run_id", runID,
		"ir_key", irKey,
		"patches_applied", len(patches),
		"final_node_count", len(patchedIR.Nodes))

	return nil
}
