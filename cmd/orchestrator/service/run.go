package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lyzr/orchestrator/cmd/orchestrator/models"
	"github.com/lyzr/orchestrator/cmd/orchestrator/repository"
	"github.com/lyzr/orchestrator/common/bootstrap"
	"github.com/redis/go-redis/v9"
)

// RunService handles business logic for workflow runs
type RunService struct {
	runRepo         *repository.RunRepository
	artifactRepo    *repository.ArtifactRepository
	casService      *CASService
	workflowSvc     *WorkflowServiceV2
	materializerSvc *MaterializerService
	components      *bootstrap.Components
	redis           *redis.Client
}

// NewRunService creates a new run service
func NewRunService(
	runRepo *repository.RunRepository,
	artifactRepo *repository.ArtifactRepository,
	casService *CASService,
	workflowSvc *WorkflowServiceV2,
	materializerSvc *MaterializerService,
	components *bootstrap.Components,
	redis *redis.Client,
) *RunService {
	return &RunService{
		runRepo:         runRepo,
		artifactRepo:    artifactRepo,
		casService:      casService,
		workflowSvc:     workflowSvc,
		materializerSvc: materializerSvc,
		components:      components,
		redis:           redis,
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

	err = s.redis.XAdd(ctx, &redis.XAddArgs{
		Stream: "wf.run.requests",
		Values: map[string]interface{}{
			"request": string(requestJSON),
		},
	}).Err()

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
	Run            *models.Run                   `json:"run"`
	WorkflowIR     map[string]interface{}        `json:"workflow_ir"`
	NodeExecutions map[string]*NodeExecution     `json:"node_executions"`
	Patches        []PatchInfo                   `json:"patches,omitempty"`
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
}

// PatchInfo represents a patch applied during execution
type PatchInfo struct {
	Seq         int                      `json:"seq"`
	Operations  []map[string]interface{} `json:"operations"`
	Description string                   `json:"description"`
}

// GetRunDetails retrieves comprehensive run details including execution data
func (s *RunService) GetRunDetails(ctx context.Context, runID uuid.UUID) (*RunDetails, error) {
	// 1. Get run from database
	run, err := s.runRepo.GetByID(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get run: %w", err)
	}

	// 2. Load workflow IR from Redis
	irKey := fmt.Sprintf("ir:%s", runID.String())
	irJSON, err := s.redis.Get(ctx, irKey).Result()
	if err != nil {
		s.components.Logger.Warn("failed to load IR from Redis (may have expired)", "run_id", runID, "error", err)
		// Return partial data without execution details
		return &RunDetails{
			Run:            run,
			WorkflowIR:     make(map[string]interface{}),
			NodeExecutions: make(map[string]*NodeExecution),
		}, nil
	}

	var workflowIR map[string]interface{}
	if err := json.Unmarshal([]byte(irJSON), &workflowIR); err != nil {
		return nil, fmt.Errorf("failed to unmarshal IR: %w", err)
	}

	// 3. Load node execution context from Redis
	contextKey := fmt.Sprintf("context:%s", runID.String())
	nodeExecutions := make(map[string]*NodeExecution)

	// Get all fields from the context hash
	contextData, err := s.redis.HGetAll(ctx, contextKey).Result()
	if err != nil {
		s.components.Logger.Warn("failed to load context", "run_id", runID, "error", err)
	}

	// Parse node outputs and build execution map
	nodes, ok := workflowIR["nodes"].(map[string]interface{})
	if ok {
		// Determine default status based on overall run status
		defaultStatus := "pending"
		if run.Status == models.StatusCompleted {
			defaultStatus = "completed" // If run completed, assume nodes completed
		} else if run.Status == models.StatusFailed {
			defaultStatus = "failed" // If run failed, nodes may have failed
		} else if run.Status == models.StatusRunning {
			defaultStatus = "running"
		}

		for nodeID := range nodes {
			execution := &NodeExecution{
				NodeID: nodeID,
				Status: defaultStatus, // Use inferred status
			}

			// Check if node has output in context (more specific status)
			outputKey := nodeID + ":output"
			if outputRef, exists := contextData[outputKey]; exists {
				// Load output from CAS
				if output, err := s.loadFromCAS(ctx, outputRef); err == nil {
					execution.Output = output
					execution.Status = "completed"
				} else {
					s.components.Logger.Warn("failed to load output from CAS",
						"node_id", nodeID,
						"cas_ref", outputRef,
						"error", err)
				}
			}

			// Check for failure (overrides other status)
			failureKey := nodeID + ":failure"
			if failureData, exists := contextData[failureKey]; exists {
				var failure map[string]interface{}
				if err := json.Unmarshal([]byte(failureData), &failure); err == nil {
					execution.Status = "failed"
					if errMsg, ok := failure["error"].(string); ok {
						execution.Error = &errMsg
					}
				}
			}

			nodeExecutions[nodeID] = execution
		}
	}

	// 4. TODO: Load patches if this run had any
	// For now, return empty patches array
	patches := []PatchInfo{}

	return &RunDetails{
		Run:            run,
		WorkflowIR:     workflowIR,
		NodeExecutions: nodeExecutions,
		Patches:        patches,
	}, nil
}

// loadFromCAS helper to load and parse CAS data
func (s *RunService) loadFromCAS(ctx context.Context, casRef string) (map[string]interface{}, error) {
	casKey := fmt.Sprintf("cas:%s", casRef)
	data, err := s.redis.Get(ctx, casKey).Result()
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return nil, err
	}

	return result, nil
}
