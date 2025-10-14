package coordinator

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lyzr/orchestrator/cmd/workflow-runner/compiler"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/sdk"
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
