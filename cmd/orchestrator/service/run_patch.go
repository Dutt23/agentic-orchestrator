package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/lyzr/orchestrator/common/models"
	"github.com/lyzr/orchestrator/common/repository"
	"github.com/lyzr/orchestrator/common/bootstrap"
)

// RunPatchService handles business logic for run-specific patches
type RunPatchService struct {
	runPatchRepo *repository.RunPatchRepository
	runRepo      *repository.RunRepository
	casService   *CASService
	artifactRepo *repository.ArtifactRepository
	components   *bootstrap.Components
}

// NewRunPatchService creates a new run patch service
func NewRunPatchService(
	runPatchRepo *repository.RunPatchRepository,
	runRepo *repository.RunRepository,
	casService *CASService,
	artifactRepo *repository.ArtifactRepository,
	components *bootstrap.Components,
) *RunPatchService {
	return &RunPatchService{
		runPatchRepo: runPatchRepo,
		runRepo:      runRepo,
		casService:   casService,
		artifactRepo: artifactRepo,
		components:   components,
	}
}

// CreateRunPatchRequest represents a request to create a run patch
type CreateRunPatchRequest struct {
	RunID       string                   `json:"run_id"`
	NodeID      string                   `json:"node_id"` // Which node generated this patch
	Operations  []map[string]interface{} `json:"operations"`
	Description string                   `json:"description"`
	CreatedBy   string                   `json:"created_by"`
}

// CreateRunPatchResponse represents the response after creating a run patch
type CreateRunPatchResponse struct {
	ID          uuid.UUID `json:"id"`
	RunID       string    `json:"run_id"`
	ArtifactID  uuid.UUID `json:"artifact_id"`
	CASID       string    `json:"cas_id"`
	Seq         int       `json:"seq"`
	OpCount     int       `json:"op_count"`
	Description string    `json:"description"`
	CreatedBy   string    `json:"created_by"`
}

// CreateRunPatch creates a new run-specific patch
func (s *RunPatchService) CreateRunPatch(ctx context.Context, req *CreateRunPatchRequest) (*CreateRunPatchResponse, error) {
	s.components.Logger.Info("creating run patch",
		"run_id", req.RunID,
		"operations", len(req.Operations),
		"created_by", req.CreatedBy)

	// Validate operations
	if len(req.Operations) == 0 {
		return nil, fmt.Errorf("operations cannot be empty")
	}

	// Get the run to find the base workflow artifact
	runID, err := uuid.Parse(req.RunID)
	if err != nil {
		return nil, fmt.Errorf("invalid run_id format: %w", err)
	}

	run, err := s.runRepo.GetByID(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get run: %w", err)
	}

	// Parse base_ref to get the base workflow artifact ID
	baseArtifactID, err := uuid.Parse(run.BaseRef)
	if err != nil {
		return nil, fmt.Errorf("invalid base_ref in run: %w", err)
	}

	// Get next sequence number for this run
	nextSeq, err := s.runPatchRepo.GetNextSeq(ctx, req.RunID)
	if err != nil {
		return nil, fmt.Errorf("failed to get next sequence: %w", err)
	}

	// Store patch operations in CAS
	patchData := map[string]interface{}{
		"operations": req.Operations,
	}

	patchJSON, err := json.Marshal(patchData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal patch data: %w", err)
	}

	casID, err := s.casService.StoreContent(ctx, patchJSON, "application/json;type=patch")
	if err != nil {
		return nil, fmt.Errorf("failed to store patch in CAS: %w", err)
	}

	s.components.Logger.Info("stored patch in CAS",
		"run_id", req.RunID,
		"cas_id", casID,
		"size", len(patchJSON))

	// Create artifact for this patch
	depth := nextSeq
	opCount := len(req.Operations)
	artifact := &models.Artifact{
		ArtifactID:  uuid.New(),
		Kind:        "patch_set",
		CasID:       casID,
		BaseVersion: &baseArtifactID, // Required: points to frozen workflow artifact
		Depth:       &depth,           // Use seq as depth for run patches
		OpCount:     &opCount,         // Store operation count
		CreatedBy:   req.CreatedBy,
		Meta:        make(map[string]interface{}), // Required field
	}

	if req.CreatedBy == "" {
		artifact.CreatedBy = "system"
	}

	// Add description to meta if provided
	if req.Description != "" {
		artifact.Meta["description"] = req.Description
	}
	artifact.Meta["run_id"] = req.RunID

	if err := s.artifactRepo.Create(ctx, artifact); err != nil {
		return nil, fmt.Errorf("failed to create artifact: %w", err)
	}

	s.components.Logger.Info("created artifact for run patch",
		"artifact_id", artifact.ArtifactID,
		"cas_id", casID)

	// Create run patch entry
	description := req.Description
	createdBy := req.CreatedBy
	nodeID := req.NodeID
	runPatch := &models.RunPatch{
		ID:          uuid.New(),
		RunID:       req.RunID,
		ArtifactID:  artifact.ArtifactID,
		Seq:         nextSeq,
		Description: &description,
		CreatedBy:   &createdBy,
	}

	// Add node_id if provided
	if nodeID != "" {
		runPatch.NodeID = &nodeID
	}

	if err := s.runPatchRepo.Create(ctx, runPatch); err != nil {
		return nil, fmt.Errorf("failed to create run patch: %w", err)
	}

	s.components.Logger.Info("run patch created successfully",
		"run_id", req.RunID,
		"seq", nextSeq,
		"artifact_id", artifact.ArtifactID)

	return &CreateRunPatchResponse{
		ID:          runPatch.ID,
		RunID:       req.RunID,
		ArtifactID:  artifact.ArtifactID,
		CASID:       casID,
		Seq:         nextSeq,
		OpCount:     len(req.Operations),
		Description: req.Description,
		CreatedBy:   req.CreatedBy,
	}, nil
}

// GetRunPatches retrieves all patches for a specific run
func (s *RunPatchService) GetRunPatches(ctx context.Context, runID string) ([]*models.RunPatchWithDetails, error) {
	patches, err := s.runPatchRepo.GetByRunIDWithDetails(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get run patches: %w", err)
	}

	s.components.Logger.Info("retrieved run patches",
		"run_id", runID,
		"count", len(patches))

	return patches, nil
}

// GetPatchOperations retrieves the operations from a specific patch
func (s *RunPatchService) GetPatchOperations(ctx context.Context, casID string) ([]map[string]interface{}, error) {
	data, err := s.casService.GetContent(ctx, casID)
	if err != nil {
		return nil, fmt.Errorf("failed to get patch from CAS: %w", err)
	}

	var patchData struct {
		Operations []map[string]interface{} `json:"operations"`
	}

	if err := json.Unmarshal(data, &patchData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal patch data: %w", err)
	}

	return patchData.Operations, nil
}

// PatchWithOperations represents a patch with its operations loaded
type PatchWithOperations struct {
	Seq         int                      `json:"seq"`
	NodeID      *string                  `json:"node_id,omitempty"`
	Operations  []map[string]interface{} `json:"operations"`
	Description string                   `json:"description"`
}

// GetRunPatchesWithOperations retrieves all patches for a run with operations in a single bulk call
// This avoids N+1 queries by fetching all CAS data at once
func (s *RunPatchService) GetRunPatchesWithOperations(ctx context.Context, runID string) ([]*PatchWithOperations, error) {
	// 1. Get all patches with details (single DB query)
	patches, err := s.runPatchRepo.GetByRunIDWithDetails(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get run patches: %w", err)
	}

	if len(patches) == 0 {
		return []*PatchWithOperations{}, nil
	}

	// 2. Collect all CAS IDs
	casIDs := make([]string, len(patches))
	for i, patch := range patches {
		casIDs[i] = patch.CASID
	}

	// 3. Bulk fetch all CAS content (single operation if CAS supports it)
	// For now, we'll fetch them all but cache in a map to avoid re-fetching
	casDataMap := make(map[string][]map[string]interface{})
	for _, casID := range casIDs {
		operations, err := s.GetPatchOperations(ctx, casID)
		if err != nil {
			s.components.Logger.Warn("failed to load patch operations",
				"run_id", runID,
				"cas_id", casID,
				"error", err)
			// Store empty array for failed operations
			casDataMap[casID] = []map[string]interface{}{}
			continue
		}
		casDataMap[casID] = operations
	}

	// 4. Build result with operations included
	result := make([]*PatchWithOperations, 0, len(patches))
	for _, patch := range patches {
		description := ""
		if patch.Description != nil {
			description = *patch.Description
		}

		result = append(result, &PatchWithOperations{
			Seq:         patch.Seq,
			NodeID:      patch.NodeID,
			Operations:  casDataMap[patch.CASID],
			Description: description,
		})
	}

	s.components.Logger.Info("retrieved run patches with operations",
		"run_id", runID,
		"count", len(result))

	return result, nil
}
