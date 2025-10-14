package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OrchestratorClient handles communication with the orchestrator API
// It uses context to pass authentication and other metadata
type OrchestratorClient struct {
	baseURL string
	http    *HTTPClient
	logger  Logger
}

// NewOrchestratorClient creates a new orchestrator client
func NewOrchestratorClient(baseURL string, logger Logger) *OrchestratorClient {
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	return &OrchestratorClient{
		baseURL: baseURL,
		http:    NewHTTPClient(httpClient, logger),
		logger:  logger,
	}
}

// RunPatchInfo represents a patch applied during a run
type RunPatchInfo struct {
	Seq         int                      `json:"seq"`
	CASID       string                   `json:"cas_id"`
	Operations  []map[string]interface{} `json:"operations"`
	Description string                   `json:"description"`
}

// GetRunPatchesWithOperations fetches all patches for a run with their operations
// Requires: ctx with UserID set via WithUserID()
func (c *OrchestratorClient) GetRunPatchesWithOperations(ctx context.Context, runID string) ([]RunPatchInfo, error) {
	c.logger.Info("fetching run patches from orchestrator", "run_id", runID)

	// Get list of patches
	url := fmt.Sprintf("%s/api/v1/runs/%s/patches", c.baseURL, runID)
	resp, err := c.http.DoRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch patches: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("patches request failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var patchListResponse struct {
		Patches []struct {
			Seq         int    `json:"seq"`
			CASID       string `json:"cas_id"`
			Description string `json:"description"`
		} `json:"patches"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&patchListResponse); err != nil {
		return nil, fmt.Errorf("failed to decode patches response: %w", err)
	}

	c.logger.Info("fetched run patches list",
		"run_id", runID,
		"count", len(patchListResponse.Patches))

	// Fetch operations for each patch
	patches := make([]RunPatchInfo, 0, len(patchListResponse.Patches))
	for _, p := range patchListResponse.Patches {
		operations, err := c.GetPatchOperations(ctx, runID, p.CASID)
		if err != nil {
			c.logger.Error("failed to get patch operations",
				"run_id", runID,
				"seq", p.Seq,
				"cas_id", p.CASID,
				"error", err)
			return nil, fmt.Errorf("failed to get operations for patch seq=%d: %w", p.Seq, err)
		}

		patches = append(patches, RunPatchInfo{
			Seq:         p.Seq,
			CASID:       p.CASID,
			Operations:  operations,
			Description: p.Description,
		})
	}

	c.logger.Info("fetched all run patch operations",
		"run_id", runID,
		"total_patches", len(patches))

	return patches, nil
}

// GetPatchOperations fetches the operations for a specific patch
// Requires: ctx with UserID set via WithUserID()
func (c *OrchestratorClient) GetPatchOperations(ctx context.Context, runID, casID string) ([]map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/runs/%s/patches/%s/operations", c.baseURL, runID, casID)
	resp, err := c.http.DoRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch patch operations: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("operations request failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var opsResponse struct {
		Operations []map[string]interface{} `json:"operations"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&opsResponse); err != nil {
		return nil, fmt.Errorf("failed to decode operations response: %w", err)
	}

	return opsResponse.Operations, nil
}

// MaterializeWorkflowForRun applies all run patches to the base workflow
// Requires: ctx with UserID set via WithUserID()
func (c *OrchestratorClient) MaterializeWorkflowForRun(ctx context.Context, baseWorkflow map[string]interface{}, runID string) (map[string]interface{}, error) {
	c.logger.Info("materializing workflow for run",
		"run_id", runID)

	// Get all run patches
	patches, err := c.GetRunPatchesWithOperations(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get run patches: %w", err)
	}

	if len(patches) == 0 {
		c.logger.Info("no run patches to apply, returning base workflow")
		return baseWorkflow, nil
	}

	// Apply patches sequentially
	c.logger.Info("applying run patches to base workflow",
		"patch_count", len(patches))

	currentWorkflow := baseWorkflow
	for _, patch := range patches {
		c.logger.Debug("applying patch",
			"seq", patch.Seq,
			"op_count", len(patch.Operations))

		// Apply each operation in the patch
		for _, op := range patch.Operations {
			var err error
			currentWorkflow, err = c.applyOperation(currentWorkflow, op)
			if err != nil {
				return nil, fmt.Errorf("failed to apply patch seq=%d operation: %w", patch.Seq, err)
			}
		}
	}

	c.logger.Info("workflow materialization complete",
		"run_id", runID,
		"patches_applied", len(patches))

	return currentWorkflow, nil
}

// applyOperation applies a single JSON Patch operation to a workflow
func (c *OrchestratorClient) applyOperation(workflow map[string]interface{}, operation map[string]interface{}) (map[string]interface{}, error) {
	op, ok := operation["op"].(string)
	if !ok {
		return nil, fmt.Errorf("operation missing 'op' field")
	}

	path, ok := operation["path"].(string)
	if !ok {
		return nil, fmt.Errorf("operation missing 'path' field")
	}

	switch op {
	case "add":
		return c.applyAdd(workflow, path, operation["value"])
	case "remove":
		return c.applyRemove(workflow, path)
	case "replace":
		return c.applyReplace(workflow, path, operation["value"])
	default:
		return nil, fmt.Errorf("unsupported operation: %s", op)
	}
}

// applyAdd handles "add" operations
func (c *OrchestratorClient) applyAdd(workflow map[string]interface{}, path string, value interface{}) (map[string]interface{}, error) {
	// Simple implementation for /nodes/- and /edges/-
	if path == "/nodes/-" {
		// Add node
		nodes, ok := workflow["nodes"].([]interface{})
		if !ok {
			nodes = []interface{}{}
		}
		workflow["nodes"] = append(nodes, value)
		return workflow, nil
	}

	if path == "/edges/-" {
		// Add edge
		edges, ok := workflow["edges"].([]interface{})
		if !ok {
			edges = []interface{}{}
		}
		workflow["edges"] = append(edges, value)
		return workflow, nil
	}

	return nil, fmt.Errorf("unsupported add path: %s", path)
}

// applyRemove handles "remove" operations
func (c *OrchestratorClient) applyRemove(workflow map[string]interface{}, path string) (map[string]interface{}, error) {
	// TODO: Implement if needed
	return nil, fmt.Errorf("remove operation not yet implemented")
}

// applyReplace handles "replace" operations
func (c *OrchestratorClient) applyReplace(workflow map[string]interface{}, path string, value interface{}) (map[string]interface{}, error) {
	// TODO: Implement if needed
	return nil, fmt.Errorf("replace operation not yet implemented")
}

// ArtifactResponse represents the response from GET /api/v1/artifacts/:id
type ArtifactResponse struct {
	ArtifactID string                 `json:"artifact_id"`
	Kind       string                 `json:"kind"`
	CASID      string                 `json:"cas_id"`
	CreatedBy  string                 `json:"created_by"`
	Content    map[string]interface{} `json:"content"`
}

// GetArtifact fetches an artifact by ID from the orchestrator
// Requires: ctx with UserID set via WithUserID()
func (c *OrchestratorClient) GetArtifact(ctx context.Context, artifactID string) (*ArtifactResponse, error) {
	url := fmt.Sprintf("%s/api/v1/artifacts/%s", c.baseURL, artifactID)
	resp, err := c.http.DoRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch artifact: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("artifact request failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var artifact ArtifactResponse
	if err := json.NewDecoder(resp.Body).Decode(&artifact); err != nil {
		return nil, fmt.Errorf("failed to decode artifact response: %w", err)
	}

	c.logger.Info("fetched artifact from orchestrator",
		"artifact_id", artifact.ArtifactID,
		"kind", artifact.Kind)

	return &artifact, nil
}
