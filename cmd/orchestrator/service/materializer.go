package service

import (
	"context"
	"encoding/json"
	"fmt"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/lyzr/orchestrator/cmd/orchestrator/models"
	"github.com/lyzr/orchestrator/common/logger"
)

// MaterializerService handles workflow materialization (base + patches)
type MaterializerService struct {
	log *logger.Logger
}

// NewMaterializerService creates a new materializer service
func NewMaterializerService(log *logger.Logger) *MaterializerService {
	return &MaterializerService{
		log: log,
	}
}

// Materialize applies all patches to the base workflow and returns the final result
func (s *MaterializerService) Materialize(ctx context.Context, components *models.WorkflowComponents) (map[string]interface{}, error) {
	s.log.Info("materializing workflow",
		"kind", components.Kind,
		"depth", components.Depth,
		"patch_count", components.PatchCount,
	)

	// If it's a dag_version, just return base content
	if components.IsDAGVersion() {
		return s.unmarshalWorkflow(components.BaseContent)
	}

	// For patch_set, apply patches sequentially
	if components.IsPatchSet() {
		return s.materializePatchSet(ctx, components)
	}

	return nil, fmt.Errorf("unsupported artifact kind: %s", components.Kind)
}

// materializePatchSet applies all patches in order to the base workflow
func (s *MaterializerService) materializePatchSet(ctx context.Context, components *models.WorkflowComponents) (map[string]interface{}, error) {
	if len(components.PatchChain) == 0 {
		s.log.Warn("patch_set has no patches, returning base")
		return s.unmarshalWorkflow(components.BaseContent)
	}

	// Start with base workflow
	currentJSON := components.BaseContent

	s.log.Info("applying patches", "count", len(components.PatchChain))

	// Apply each patch in sequence
	for i, patchInfo := range components.PatchChain {
		s.log.Debug("applying patch",
			"seq", patchInfo.Seq,
			"artifact_id", patchInfo.ArtifactID,
			"depth", patchInfo.Depth,
		)

		// Apply this patch
		resultJSON, err := s.applyPatch(currentJSON, patchInfo.Content)
		if err != nil {
			return nil, fmt.Errorf("failed to apply patch %d (seq=%d, artifact=%s): %w",
				i+1, patchInfo.Seq, patchInfo.ArtifactID, err)
		}

		currentJSON = resultJSON
	}

	s.log.Info("materialization complete", "patches_applied", len(components.PatchChain))

	// Parse final result
	return s.unmarshalWorkflow(currentJSON)
}

// applyPatch applies a JSON Patch to the workflow
func (s *MaterializerService) applyPatch(workflowJSON []byte, patchJSON []byte) ([]byte, error) {
	// Parse the patch operations
	patch, err := jsonpatch.DecodePatch(patchJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to decode patch: %w", err)
	}

	// Apply the patch
	modifiedJSON, err := patch.Apply(workflowJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to apply patch operations: %w", err)
	}

	return modifiedJSON, nil
}

// unmarshalWorkflow converts JSON bytes to map
func (s *MaterializerService) unmarshalWorkflow(workflowJSON []byte) (map[string]interface{}, error) {
	var workflow map[string]interface{}
	if err := json.Unmarshal(workflowJSON, &workflow); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow: %w", err)
	}

	return workflow, nil
}

// ValidatePatch validates a patch before storing (optional safety check)
func (s *MaterializerService) ValidatePatch(baseWorkflow []byte, patchOperations []byte) error {
	// Try applying the patch to ensure it's valid
	_, err := s.applyPatch(baseWorkflow, patchOperations)
	if err != nil {
		return fmt.Errorf("patch validation failed: %w", err)
	}

	return nil
}

// ComputeResultHash computes hash of materialized workflow (for caching)
func (s *MaterializerService) ComputeResultHash(materializedWorkflow map[string]interface{}) (string, error) {
	// Serialize to canonical JSON (sorted keys)
	canonicalJSON, err := json.Marshal(materializedWorkflow)
	if err != nil {
		return "", fmt.Errorf("failed to marshal workflow: %w", err)
	}

	// Compute hash (could use CASService for consistency)
	// For now, just return as string - integrate with CASService later
	return fmt.Sprintf("materialized:%x", len(canonicalJSON)), nil
}
