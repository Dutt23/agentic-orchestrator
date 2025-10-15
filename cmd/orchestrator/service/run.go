package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lyzr/orchestrator/common/models"
	"github.com/lyzr/orchestrator/common/repository"
	"github.com/lyzr/orchestrator/common/bootstrap"
	"github.com/lyzr/orchestrator/common/ratelimit"
	rediscommon "github.com/lyzr/orchestrator/common/redis"
)

// RunService handles business logic for workflow runs
type RunService struct {
	runRepo         *repository.RunRepository
	artifactRepo    *repository.ArtifactRepository
	casService      *CASService
	workflowSvc     *WorkflowServiceV2
	materializerSvc *MaterializerService
	runPatchService *RunPatchService
	components      *bootstrap.Components
	redis           *rediscommon.Client
	rateLimiter     *ratelimit.RateLimiter
}

// RunServiceOpts contains options for creating a RunService
type RunServiceOpts struct {
	RunRepo         *repository.RunRepository
	ArtifactRepo    *repository.ArtifactRepository
	CASService      *CASService
	WorkflowSvc     *WorkflowServiceV2
	MaterializerSvc *MaterializerService
	RunPatchService *RunPatchService
	Components      *bootstrap.Components
	Redis           *rediscommon.Client
	RateLimiter     *ratelimit.RateLimiter
}

// NewRunService creates a new run service with options pattern
func NewRunService(opts *RunServiceOpts) *RunService {
	return &RunService{
		runRepo:         opts.RunRepo,
		artifactRepo:    opts.ArtifactRepo,
		casService:      opts.CASService,
		workflowSvc:     opts.WorkflowSvc,
		materializerSvc: opts.MaterializerSvc,
		runPatchService: opts.RunPatchService,
		components:      opts.Components,
		redis:           opts.Redis,
		rateLimiter:     opts.RateLimiter,
	}
}

// CreateRunRequest represents a request to create a workflow run
type CreateRunRequest struct {
	Tag      string                 `json:"tag"`
	Username string                 `json:"username"`
	Inputs   map[string]interface{} `json:"inputs"`
}

// CreateRunResponse represents the response after creating a run
type CreateRunResponse struct {
	RunID      uuid.UUID `json:"run_id"`
	ArtifactID uuid.UUID `json:"artifact_id"`
	Status     string    `json:"status"`
	Tag        string    `json:"tag"`
}

// RateLimitError represents a rate limit exceeded error
type RateLimitError struct {
	Tier              ratelimit.WorkflowTier
	Limit             int64
	CurrentCount      int64
	RetryAfterSeconds int64
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limit exceeded: %s tier allows %d runs/minute, retry after %d seconds",
		e.Tier, e.Limit, e.RetryAfterSeconds)
}

// CreateRun creates a new workflow run with materialized workflow
func (s *RunService) CreateRun(ctx context.Context, req *CreateRunRequest) (*CreateRunResponse, error) {
	s.components.Logger.Info("creating workflow run",
		"tag", req.Tag,
		"username", req.Username)

	// 1. Get workflow components (handles both dag_version and patch_set)
	components, err := s.workflowSvc.GetWorkflowComponents(ctx, req.Username, req.Tag)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow components: %w", err)
	}

	s.components.Logger.Info("retrieved workflow components",
		"kind", components.Kind,
		"depth", components.Depth,
		"patch_count", components.PatchCount)

	// 2. Materialize workflow (apply patches if needed)
	materializedWorkflow, err := s.materializerSvc.Materialize(ctx, components)
	if err != nil {
		return nil, fmt.Errorf("failed to materialize workflow: %w", err)
	}

	// 2.5. Check rate limit based on workflow complexity (agent-aware)
	profile := ratelimit.InspectWorkflow(materializedWorkflow)
	s.components.Logger.Info("workflow inspected for rate limiting",
		"tier", profile.Tier,
		"agent_count", profile.AgentCount,
		"total_nodes", profile.TotalNodes)

	// Check tiered rate limit (separate counters per tier)
	result, err := s.rateLimiter.CheckTieredLimit(ctx, req.Username, profile.Tier)
	if err != nil {
		s.components.Logger.Error("rate limit check failed", "error", err)
		// On error, allow request (fail open for availability)
	} else if !result.Allowed {
		s.components.Logger.Warn("rate limit exceeded",
			"username", req.Username,
			"tier", profile.Tier,
			"limit", result.Limit,
			"current", result.CurrentCount,
			"retry_after", result.RetryAfterSeconds)

		return nil, &RateLimitError{
			Tier:              profile.Tier,
			Limit:             result.Limit,
			CurrentCount:      result.CurrentCount,
			RetryAfterSeconds: result.RetryAfterSeconds,
		}
	}

	// 3. Store materialized workflow as artifact
	workflowJSON, err := json.Marshal(materializedWorkflow)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal workflow: %w", err)
	}

	// 4. Store workflow in CAS
	casID, err := s.casService.StoreContent(ctx, workflowJSON, "application/json;type=workflow")
	if err != nil {
		return nil, fmt.Errorf("failed to store workflow in CAS: %w", err)
	}

	// 5. Create artifact pointing to CAS blob (frozen workflow for this run)
	versionHash := casID // For dag_version, version_hash = cas_id (content-addressed)
	artifact := &models.Artifact{
		ArtifactID:  uuid.New(),
		Kind:        "dag_version",
		CasID:       casID,
		VersionHash: &versionHash, // Required for dag_version
		CreatedBy:   req.Username,
		Meta:        make(map[string]interface{}), // Required field
	}

	if err := s.artifactRepo.Create(ctx, artifact); err != nil {
		return nil, fmt.Errorf("failed to create artifact: %w", err)
	}

	s.components.Logger.Info("created artifact for run",
		"artifact_id", artifact.ArtifactID,
		"cas_id", casID)

	// 6. Get current tag positions for snapshot
	// TODO: Implement GetAllTagPositions in WorkflowService
	// For now, just record the requested tag
	tagsSnapshot := map[string]string{
		req.Tag: artifact.ArtifactID.String(),
	}

	// 7. Create run entry
	runID := uuid.New()
	run := &models.Run{
		RunID:        runID,
		BaseKind:     models.BaseKindDAGVersion,
		BaseRef:      artifact.ArtifactID.String(),
		TagsSnapshot: tagsSnapshot,
		Status:       models.StatusQueued,
		SubmittedBy:  &req.Username,
		SubmittedAt:  time.Now(),
	}

	if err := s.runRepo.Create(ctx, run); err != nil {
		return nil, fmt.Errorf("failed to create run: %w", err)
	}

	s.components.Logger.Info("run created",
		"run_id", runID,
		"artifact_id", artifact.ArtifactID,
		"tag", req.Tag)

	// 8. Publish to wf.run.requests stream
	runRequest := map[string]interface{}{
		"run_id":      runID.String(),
		"artifact_id": artifact.ArtifactID.String(),
		"tag":         req.Tag,
		"username":    req.Username,
		"inputs":      req.Inputs,
		"created_at":  time.Now().Unix(),
	}

	requestJSON, err := json.Marshal(runRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal run request: %w", err)
	}

	_, err = s.redis.AddToStream(ctx, "wf.run.requests", map[string]interface{}{
		"request": string(requestJSON),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to publish run request: %w", err)
	}

	s.components.Logger.Info("published run request to stream",
		"run_id", runID,
		"stream", "wf.run.requests")

	return &CreateRunResponse{
		RunID:      runID,
		ArtifactID: artifact.ArtifactID,
		Status:     string(models.StatusQueued),
		Tag:        req.Tag,
	}, nil
}

// GetRun retrieves a run by ID
func (s *RunService) GetRun(ctx context.Context, runID uuid.UUID) (*models.Run, error) {
	return s.runRepo.GetByID(ctx, runID)
}

// UpdateRunStatus updates the status of a run
func (s *RunService) UpdateRunStatus(ctx context.Context, runID uuid.UUID, status models.RunStatus) error {
	return s.runRepo.UpdateStatus(ctx, runID, status)
}

// ListUserRuns lists runs for a specific user
func (s *RunService) ListUserRuns(ctx context.Context, username string, limit int) ([]*models.Run, error) {
	return s.runRepo.ListByUser(ctx, username, limit)
}

// ListRunsForWorkflow lists runs for a specific workflow tag
func (s *RunService) ListRunsForWorkflow(ctx context.Context, tag string, limit int) ([]*models.Run, error) {
	return s.runRepo.ListByWorkflowTag(ctx, tag, limit)
}

// RunDetails represents comprehensive run information
type RunDetails struct {
	Run             *models.Run                   `json:"run"`
	BaseWorkflowIR  map[string]interface{}        `json:"base_workflow_ir"` // Workflow before any patches
	WorkflowIR      map[string]interface{}        `json:"workflow_ir"`      // Workflow after all patches
	NodeExecutions  map[string]*NodeExecution     `json:"node_executions"`
	NodeOutputsRaw  map[string]interface{}        `json:"node_outputs_raw,omitempty"` // Raw node outputs from Redis context
	Patches         []PatchInfo                   `json:"patches,omitempty"`
}

// NodeExecution represents execution details for a single node
type NodeExecution struct {
	NodeID      string                 `json:"node_id"`
	Status      string                 `json:"status"` // completed, failed, running, pending
	Input       map[string]interface{} `json:"input,omitempty"`
	Output      map[string]interface{} `json:"output,omitempty"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	Error       *string                `json:"error,omitempty"`
	Metrics     *ExecutionMetrics      `json:"metrics,omitempty"`
}

// ExecutionMetrics represents performance metrics for node execution
type ExecutionMetrics struct {
	SentAt          string                 `json:"sent_at"`
	StartTime       string                 `json:"start_time"`
	EndTime         string                 `json:"end_time"`
	QueueTimeMs     int                    `json:"queue_time_ms"`
	ExecutionTimeMs int                    `json:"execution_time_ms"`
	TotalDurationMs int                    `json:"total_duration_ms"`
	MemoryStartMb   float64                `json:"memory_start_mb"`
	MemoryPeakMb    float64                `json:"memory_peak_mb"`
	MemoryEndMb     float64                `json:"memory_end_mb"`
	CpuPercent      float64                `json:"cpu_percent"`
	ThreadCount     int                    `json:"thread_count"`
	System          map[string]interface{} `json:"system,omitempty"` // System information
}

// PatchInfo represents a patch applied during execution
type PatchInfo struct {
	Seq         int                      `json:"seq"`
	NodeID      *string                  `json:"node_id,omitempty"` // Which node generated this patch
	Operations  []map[string]interface{} `json:"operations"`
	Description string                   `json:"description"`
}

// parseMetrics extracts metrics from output data
func parseMetrics(metricsData map[string]interface{}) *ExecutionMetrics {
	metrics := &ExecutionMetrics{}

	// Extract string fields
	if v, ok := metricsData["sent_at"].(string); ok {
		metrics.SentAt = v
	}
	if v, ok := metricsData["start_time"].(string); ok {
		metrics.StartTime = v
	}
	if v, ok := metricsData["end_time"].(string); ok {
		metrics.EndTime = v
	}

	// Extract int fields (may come as float64 from JSON)
	if v, ok := metricsData["queue_time_ms"].(float64); ok {
		metrics.QueueTimeMs = int(v)
	}
	if v, ok := metricsData["execution_time_ms"].(float64); ok {
		metrics.ExecutionTimeMs = int(v)
	}
	if v, ok := metricsData["total_duration_ms"].(float64); ok {
		metrics.TotalDurationMs = int(v)
	}
	if v, ok := metricsData["thread_count"].(float64); ok {
		metrics.ThreadCount = int(v)
	}

	// Extract float64 fields
	if v, ok := metricsData["memory_start_mb"].(float64); ok {
		metrics.MemoryStartMb = v
	}
	if v, ok := metricsData["memory_peak_mb"].(float64); ok {
		metrics.MemoryPeakMb = v
	}
	if v, ok := metricsData["memory_end_mb"].(float64); ok {
		metrics.MemoryEndMb = v
	}
	if v, ok := metricsData["cpu_percent"].(float64); ok {
		metrics.CpuPercent = v
	}

	// Extract system information (nested map)
	if systemData, ok := metricsData["system"].(map[string]interface{}); ok {
		metrics.System = systemData
	}

	return metrics
}

// loadWorkflowIR loads the workflow IR from Redis for a given run
func (s *RunService) loadWorkflowIR(ctx context.Context, runID uuid.UUID) (map[string]interface{}, error) {
	irKey := fmt.Sprintf("ir:%s", runID.String())
	irJSON, err := s.redis.Get(ctx, irKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load IR from Redis: %w", err)
	}

	var workflowIR map[string]interface{}
	if err := json.Unmarshal([]byte(irJSON), &workflowIR); err != nil {
		return nil, fmt.Errorf("failed to unmarshal IR: %w", err)
	}

	return workflowIR, nil
}

// loadBaseWorkflow loads the base workflow (before patches) from the artifact
func (s *RunService) loadBaseWorkflow(ctx context.Context, run *models.Run) (map[string]interface{}, error) {
	// Parse base_ref to get artifact ID
	artifactID, err := uuid.Parse(run.BaseRef)
	if err != nil {
		return nil, fmt.Errorf("invalid base_ref: %w", err)
	}

	// Get artifact
	artifact, err := s.artifactRepo.GetByID(ctx, artifactID)
	if err != nil {
		return nil, fmt.Errorf("failed to get base artifact: %w", err)
	}

	// Load workflow from CAS
	workflowJSON, err := s.casService.GetContent(ctx, artifact.CasID)
	if err != nil {
		return nil, fmt.Errorf("failed to load workflow from CAS: %w", err)
	}

	var baseWorkflow map[string]interface{}
	if err := json.Unmarshal(workflowJSON, &baseWorkflow); err != nil {
		return nil, fmt.Errorf("failed to unmarshal base workflow: %w", err)
	}

	return baseWorkflow, nil
}

// loadContextData loads the context hash data from Redis for a given run
func (s *RunService) loadContextData(ctx context.Context, runID uuid.UUID) (map[string]string, error) {
	contextKey := fmt.Sprintf("context:%s", runID.String())
	contextData, err := s.redis.GetAllHash(ctx, contextKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load context: %w", err)
	}
	return contextData, nil
}

// bulkFetchCASData fetches CAS data in bulk for the given node outputs
func (s *RunService) bulkFetchCASData(ctx context.Context, contextData map[string]string, nodes map[string]interface{}) (map[string]map[string]interface{}, error) {
	// Collect all CAS IDs for bulk fetch
	casRefs := make([]string, 0)
	casRefToNodeID := make(map[string]string)

	for nodeID := range nodes {
		outputKey := nodeID + ":output"
		if outputRef, exists := contextData[outputKey]; exists {
			casRefs = append(casRefs, outputRef)
			casRefToNodeID[outputRef] = nodeID
		}
	}

	casDataMap := make(map[string]map[string]interface{})
	if len(casRefs) == 0 {
		return casDataMap, nil
	}

	// Build cas keys
	casKeys := make([]string, len(casRefs))
	for i, casRef := range casRefs {
		casKeys[i] = fmt.Sprintf("cas:%s", casRef)
	}

	// Bulk GET with pipeline
	casResults, err := s.redis.GetMultiple(ctx, casKeys)
	if err != nil {
		return nil, fmt.Errorf("failed to bulk fetch CAS data: %w", err)
	}

	// Parse all CAS results
	for casKey, data := range casResults {
		// Extract casRef from "cas:{casRef}"
		casRef := casKey[4:] // Remove "cas:" prefix

		var result map[string]interface{}
		if err := json.Unmarshal([]byte(data), &result); err != nil {
			s.components.Logger.Warn("failed to unmarshal CAS data",
				"cas_ref", casRef,
				"error", err)
			continue
		}
		casDataMap[casRef] = result
	}

	return casDataMap, nil
}

// bulkFetchAllCASFromContext fetches ALL CAS references from context data (not limited to IR nodes)
func (s *RunService) bulkFetchAllCASFromContext(ctx context.Context, contextData map[string]string) (map[string]map[string]interface{}, error) {
	// Collect all CAS references from ALL context keys ending with :output
	casRefs := make([]string, 0)

	for key, value := range contextData {
		// Check if this is an output key and the value looks like a CAS reference
		if strings.HasSuffix(key, ":output") && strings.HasPrefix(value, "artifact://") {
			casRefs = append(casRefs, value)
		}
	}

	casDataMap := make(map[string]map[string]interface{})
	if len(casRefs) == 0 {
		return casDataMap, nil
	}

	// Build cas keys
	casKeys := make([]string, len(casRefs))
	for i, casRef := range casRefs {
		casKeys[i] = fmt.Sprintf("cas:%s", casRef)
	}

	// Bulk GET with pipeline
	casResults, err := s.redis.GetMultiple(ctx, casKeys)
	if err != nil {
		return nil, fmt.Errorf("failed to bulk fetch CAS data: %w", err)
	}

	// Parse all CAS results
	for casKey, data := range casResults {
		// Extract casRef from "cas:{casRef}"
		casRef := strings.TrimPrefix(casKey, "cas:")

		var result map[string]interface{}
		if err := json.Unmarshal([]byte(data), &result); err != nil {
			s.components.Logger.Warn("failed to unmarshal CAS data",
				"cas_ref", casRef,
				"error", err)
			continue
		}
		casDataMap[casRef] = result
	}

	return casDataMap, nil
}

// buildNodeExecutions builds the node execution map from workflow IR and node_outputs_raw
// Uses node_outputs_raw as the source of truth for status - if a node isn't there, it wasn't executed
func (s *RunService) buildNodeExecutions(
	ctx context.Context,
	run *models.Run,
	workflowIR map[string]interface{},
	nodeOutputsRaw map[string]interface{},
) map[string]*NodeExecution {
	nodeExecutions := make(map[string]*NodeExecution)

	nodes, ok := workflowIR["nodes"].(map[string]interface{})
	if !ok {
		return nodeExecutions
	}

	for nodeID := range nodes {
		execution := &NodeExecution{
			NodeID: nodeID,
			Status: "not_executed", // Default to not_executed
		}

		// Check for node-specific status in Redis (e.g., waiting_for_approval)
		nodeStatusKey := fmt.Sprintf("run:%s:node:%s:status", run.RunID.String(), nodeID)
		if nodeStatus, err := s.redis.Get(ctx, nodeStatusKey); err == nil {
			execution.Status = nodeStatus
		}

		// Check if node has output in node_outputs_raw
		if outputData, exists := nodeOutputsRaw[nodeID]; exists {
			if output, ok := outputData.(map[string]interface{}); ok {
				execution.Output = output

				// Use status directly from output data (source of truth)
				if status, ok := output["status"].(string); ok {
					// Normalize "success" to "completed" for consistency
					if status == "success" {
						execution.Status = "completed"
					} else {
						execution.Status = status
					}
				} else {
					// No explicit status means completed successfully
					execution.Status = "completed"
				}

				// Extract error message if present
				if errMsg, ok := output["error"].(string); ok {
					execution.Error = &errMsg
				} else if errMap, ok := output["error"].(map[string]interface{}); ok {
					if msg, ok := errMap["error_message"].(string); ok {
						execution.Error = &msg
					}
				}

				// Extract metrics if present in output
				if metricsData, ok := output["metrics"].(map[string]interface{}); ok {
					execution.Metrics = parseMetrics(metricsData)
				}
			}
		}

		// Also check for failure entry (nodeID_failure key)
		failureKey := nodeID + "_failure"
		if failureData, exists := nodeOutputsRaw[failureKey]; exists {
			if failure, ok := failureData.(map[string]interface{}); ok {
				execution.Status = "failed"

				// Extract error message
				if errMsg, ok := failure["error"].(string); ok {
					execution.Error = &errMsg
				} else if errMap, ok := failure["error"].(map[string]interface{}); ok {
					if msg, ok := errMap["error_message"].(string); ok {
						execution.Error = &msg
					}
				}

				// Extract metrics from failure if present
				if metricsData, ok := failure["metrics"].(map[string]interface{}); ok {
					execution.Metrics = parseMetrics(metricsData)
				}
			}
		}

		nodeExecutions[nodeID] = execution
	}

	return nodeExecutions
}

// buildNodeOutputsRaw builds a raw map of node outputs directly from Redis context
func (s *RunService) buildNodeOutputsRaw(
	ctx context.Context,
	contextData map[string]string,
	casDataMap map[string]map[string]interface{},
) map[string]interface{} {
	nodeOutputsRaw := make(map[string]interface{})

	// Iterate through all context data
	for key, value := range contextData {
		// Check if this is an output key (format: "nodeID:output")
		if strings.HasSuffix(key, ":output") {
			nodeID := strings.TrimSuffix(key, ":output")

			// Try to get the actual output data from CAS
			if output, found := casDataMap[value]; found {
				nodeOutputsRaw[nodeID] = output
			} else {
				// If not in CAS, store the reference itself
				nodeOutputsRaw[nodeID] = map[string]interface{}{
					"ref": value,
				}
			}
		} else if strings.HasSuffix(key, ":failure") {
			// Also capture failure data
			nodeID := strings.TrimSuffix(key, ":failure")

			var failureData map[string]interface{}
			if err := json.Unmarshal([]byte(value), &failureData); err == nil {
				// Store under a special key to indicate failure
				nodeOutputsRaw[nodeID+"_failure"] = failureData
			}
		}
	}

	return nodeOutputsRaw
}

// loadRunPatches loads patches for the given run with operations
func (s *RunService) loadRunPatches(ctx context.Context, runID uuid.UUID) ([]PatchInfo, error) {
	patches := []PatchInfo{}

	patchesWithOps, err := s.runPatchService.GetRunPatchesWithOperations(ctx, runID.String())
	if err != nil {
		return patches, fmt.Errorf("failed to load patches with operations: %w", err)
	}

	// Convert to PatchInfo format
	for _, p := range patchesWithOps {
		patches = append(patches, PatchInfo{
			Seq:         p.Seq,
			Operations:  p.Operations,
			Description: p.Description,
		})
	}

	return patches, nil
}

// GetRunDetails retrieves comprehensive run details including execution data
// This method acts as a facade, delegating work to specialized helper methods
func (s *RunService) GetRunDetails(ctx context.Context, runID uuid.UUID) (*RunDetails, error) {
	// 1. Get run from database
	run, err := s.runRepo.GetByID(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get run: %w", err)
	}

	// 2. Load base workflow from artifact (before any patches)
	baseWorkflowIR, err := s.loadBaseWorkflow(ctx, run)
	if err != nil {
		s.components.Logger.Warn("failed to load base workflow", "run_id", runID, "error", err)
		baseWorkflowIR = make(map[string]interface{}) // Continue with empty base
	}

	// 3. Load workflow IR from Redis (after patches applied)
	workflowIR, err := s.loadWorkflowIR(ctx, runID)
	if err != nil {
		s.components.Logger.Warn("failed to load IR from Redis (may have expired)", "run_id", runID, "error", err)
		// Return partial data without execution details
		return &RunDetails{
			Run:             run,
			BaseWorkflowIR:  baseWorkflowIR,
			WorkflowIR:      make(map[string]interface{}),
			NodeExecutions:  make(map[string]*NodeExecution),
		}, nil
	}

	// 4. Load context data from Redis
	contextData, err := s.loadContextData(ctx, runID)
	if err != nil {
		s.components.Logger.Warn("failed to load context", "run_id", runID, "error", err)
		contextData = make(map[string]string) // Continue with empty context
	}

	// 5. Bulk fetch ALL CAS data from context (including dynamically added nodes)
	casDataMap := make(map[string]map[string]interface{})
	if len(contextData) > 0 {
		var err error
		casDataMap, err = s.bulkFetchAllCASFromContext(ctx, contextData)
		if err != nil {
			s.components.Logger.Warn("failed to bulk fetch CAS data", "error", err)
			casDataMap = make(map[string]map[string]interface{}) // Continue with empty CAS data
		}
	}

	// 6. Build raw node outputs map FIRST (all nodes from Redis context, including dynamically added ones)
	var nodeOutputsRaw map[string]interface{}
	if len(contextData) > 0 {
		nodeOutputsRaw = s.buildNodeOutputsRaw(ctx, contextData, casDataMap)
	}

	// 7. Build node executions using nodeOutputsRaw as source of truth for status
	var nodeExecutions map[string]*NodeExecution
	if _, ok := workflowIR["nodes"].(map[string]interface{}); ok {
		nodeExecutions = s.buildNodeExecutions(ctx, run, workflowIR, nodeOutputsRaw)
	} else {
		nodeExecutions = make(map[string]*NodeExecution)
	}

	// 8. Load patches for this run
	patches, err := s.loadRunPatches(ctx, runID)
	if err != nil {
		s.components.Logger.Warn("failed to load patches with operations", "run_id", runID, "error", err)
		patches = []PatchInfo{} // Continue with empty patches
	}

	// 9. Enrich run status based on actual node execution state
	// This provides real-time status without constantly updating the DB
	hasWaitingNode := false
	hasFailedNode := false
	hasCompletedNode := false
	totalNodes := len(nodeExecutions)
	completedCount := 0

	for _, execution := range nodeExecutions {
		switch execution.Status {
		case "waiting_for_approval":
			hasWaitingNode = true
		case "failed", "error": // Treat error same as failed
			hasFailedNode = true
		case "completed":
			hasCompletedNode = true
			completedCount++
		}
	}

	// Determine display status based on node execution state
	displayStatus := run.Status

	// Priority order (most important first):
	// 1. Any node failed → FAILED
	// 2. Any node waiting for approval → WAITING_FOR_APPROVAL
	// 3. Any node executed (completed/failed) → RUNNING
	// 4. All nodes completed → COMPLETED
	// 5. Otherwise → Keep DB status (QUEUED, etc.)

	if hasFailedNode {
		displayStatus = models.StatusFailed
	} else if hasWaitingNode {
		displayStatus = models.StatusWaitingForApproval
	} else if completedCount == totalNodes && totalNodes > 0 {
		displayStatus = models.StatusCompleted
	} else if hasCompletedNode {
		displayStatus = models.StatusRunning
	}

	// Create a copy of run with updated display status
	runCopy := *run
	runCopy.Status = displayStatus

	return &RunDetails{
		Run:             &runCopy,
		BaseWorkflowIR:  baseWorkflowIR,
		WorkflowIR:      workflowIR,
		NodeExecutions:  nodeExecutions,
		NodeOutputsRaw:  nodeOutputsRaw,
		Patches:         patches,
	}, nil
}
