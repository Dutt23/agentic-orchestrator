package handlers

import (
	"context"
	"fmt"

	"github.com/lyzr/orchestrator/cmd/orchestrator/models"
	"github.com/lyzr/orchestrator/cmd/orchestrator/service"
)

// WorkflowResponseBuilder builds HTTP responses for workflow endpoints
type WorkflowResponseBuilder struct {
	materializerService *service.MaterializerService
	logger              Logger
}

// Logger interface for logging (subset of what's needed)
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
}

// BuildWorkflowResponse constructs the response with metadata and components
func (b *WorkflowResponseBuilder) BuildWorkflowResponse(tagName, owner string, components *models.WorkflowComponents) map[string]interface{} {
	response := map[string]interface{}{
		"tag":         tagName,            // Tag name (e.g., "main")
		"owner":       owner,              // Owner username (e.g., "sdutt")
		"artifact_id": components.ArtifactID,
		"kind":        components.Kind,
		"depth":       components.Depth,
		"patch_count": components.PatchCount,
		"created_at":  components.CreatedAt,
	}

	if components.CreatedBy != nil {
		response["created_by"] = *components.CreatedBy
	}

	response["components"] = b.buildComponentDetails(components)

	return response
}

// buildComponentDetails constructs the component details section of the response
func (b *WorkflowResponseBuilder) buildComponentDetails(components *models.WorkflowComponents) map[string]interface{} {
	details := map[string]interface{}{
		"base_cas_id":       components.BaseCASID,
		"base_version_hash": components.BaseVersionHash,
	}

	if components.BaseVersion != nil {
		details["base_version"] = *components.BaseVersion
	}

	if len(components.PatchChain) > 0 {
		details["patches"] = b.buildPatchChainMetadata(components.PatchChain)
	}

	return details
}

// buildPatchChainMetadata converts patch chain to metadata format (without content)
func (b *WorkflowResponseBuilder) buildPatchChainMetadata(patchChain []models.PatchInfo) []map[string]interface{} {
	patches := make([]map[string]interface{}, 0, len(patchChain))

	for _, patch := range patchChain {
		patchInfo := map[string]interface{}{
			"seq":         patch.Seq,
			"artifact_id": patch.ArtifactID,
			"cas_id":      patch.CASID,
			"depth":       patch.Depth,
			"created_at":  patch.CreatedAt,
		}

		if patch.OpCount != nil {
			patchInfo["op_count"] = *patch.OpCount
		}
		if patch.CreatedBy != nil {
			patchInfo["created_by"] = *patch.CreatedBy
		}

		patches = append(patches, patchInfo)
	}

	return patches
}

// AddMaterializedWorkflow materializes the workflow and adds it to the response
func (b *WorkflowResponseBuilder) AddMaterializedWorkflow(response map[string]interface{}, components *models.WorkflowComponents) error {
	b.logger.Info("materialization requested",
		"tag", components.TagName,
		"kind", components.Kind,
		"depth", components.Depth,
		"patch_count", components.PatchCount,
	)

	// Use MaterializerService to apply patches
	workflow, err := b.materializerService.Materialize(context.Background(), components)
	if err != nil {
		b.logger.Error("materialization failed",
			"tag", components.TagName,
			"error", err,
		)
		return fmt.Errorf("failed to materialize workflow: %w", err)
	}

	response["workflow"] = workflow
	return nil
}
