package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lyzr/orchestrator/cmd/orchestrator/models"
	"github.com/lyzr/orchestrator/cmd/orchestrator/repository"
	"github.com/lyzr/orchestrator/common/logger"
)

// ArtifactService handles artifact catalog operations
type ArtifactService struct {
	repo *repository.ArtifactRepository
	log  *logger.Logger
}

// NewArtifactService creates a new artifact service
func NewArtifactService(repo *repository.ArtifactRepository, log *logger.Logger) *ArtifactService {
	return &ArtifactService{
		repo: repo,
		log:  log,
	}
}

// CreateDAGVersion creates a DAG version artifact
func (s *ArtifactService) CreateDAGVersion(ctx context.Context, casID, versionHash, name, createdBy string, nodesCount, edgesCount int) (uuid.UUID, error) {
	artifact := &models.Artifact{
		ArtifactID:  uuid.New(),
		Kind:        models.KindDAGVersion,
		CasID:       casID,
		Name:        &name,
		VersionHash: &versionHash,
		NodesCount:  &nodesCount,
		EdgesCount:  &edgesCount,
		Meta:        make(map[string]interface{}),
		CreatedBy:   createdBy,
		CreatedAt:   time.Now(),
	}

	if err := s.repo.Create(ctx, artifact); err != nil {
		return uuid.Nil, fmt.Errorf("failed to create DAG version artifact: %w", err)
	}

	s.log.Info("created DAG version artifact",
		"artifact_id", artifact.ArtifactID,
		"cas_id", casID,
		"nodes", nodesCount,
		"edges", edgesCount,
	)

	return artifact.ArtifactID, nil
}

// CreatePatchSet creates a patch set artifact
func (s *ArtifactService) CreatePatchSet(ctx context.Context, casID string, baseVersion uuid.UUID, depth, opCount int, createdBy string) (uuid.UUID, error) {
	artifact := &models.Artifact{
		ArtifactID:  uuid.New(),
		Kind:        models.KindPatchSet,
		CasID:       casID,
		BaseVersion: &baseVersion,
		Depth:       &depth,
		OpCount:     &opCount,
		Meta:        make(map[string]interface{}),
		CreatedBy:   createdBy,
		CreatedAt:   time.Now(),
	}

	if err := s.repo.Create(ctx, artifact); err != nil {
		return uuid.Nil, fmt.Errorf("failed to create patch set artifact: %w", err)
	}

	s.log.Info("created patch set artifact",
		"artifact_id", artifact.ArtifactID,
		"base_version", baseVersion,
		"depth", depth,
	)

	return artifact.ArtifactID, nil
}

// CreatePatch creates a patch artifact with proper chain linking
// Note: previousPatchSet is used for metadata/logging but not stored in the artifact
// The patch chain is reconstructed via the patch_chain_member table
func (s *ArtifactService) CreatePatch(ctx context.Context, casID string, baseVersion uuid.UUID, previousPatchSet *uuid.UUID, depth, opCount int, createdBy string) (uuid.UUID, error) {
	artifact := &models.Artifact{
		ArtifactID:  uuid.New(),
		Kind:        models.KindPatchSet,
		CasID:       casID,
		BaseVersion: &baseVersion,
		Depth:       &depth,
		OpCount:     &opCount,
		Meta:        make(map[string]interface{}),
		CreatedBy:   createdBy,
		CreatedAt:   time.Now(),
	}

	if err := s.repo.Create(ctx, artifact); err != nil {
		return uuid.Nil, fmt.Errorf("failed to create patch artifact: %w", err)
	}

	// Build patch chain: get all previous patches + add this new one
	var patchChain []uuid.UUID
	if previousPatchSet != nil {
		// Get existing patch chain from previous head
		previousPatches, err := s.repo.GetPatchChain(ctx, *previousPatchSet)
		if err != nil {
			return uuid.Nil, fmt.Errorf("failed to get previous patch chain: %w", err)
		}
		// Add all previous patch IDs
		for _, p := range previousPatches {
			patchChain = append(patchChain, p.ArtifactID)
		}
	}
	// Add new patch as the last member
	patchChain = append(patchChain, artifact.ArtifactID)

	// Insert patch chain members
	if err := s.repo.InsertPatchChain(ctx, artifact.ArtifactID, patchChain); err != nil {
		return uuid.Nil, fmt.Errorf("failed to insert patch chain: %w", err)
	}

	s.log.Info("created patch artifact",
		"artifact_id", artifact.ArtifactID,
		"base_version", baseVersion,
		"depth", depth,
		"op_count", opCount,
		"chain_length", len(patchChain),
	)

	return artifact.ArtifactID, nil
}

// CreateRunSnapshot creates a run snapshot artifact
func (s *ArtifactService) CreateRunSnapshot(ctx context.Context, casID, planHash, versionHash string, nodesCount, edgesCount int, createdBy string) (uuid.UUID, error) {
	artifact := &models.Artifact{
		ArtifactID:  uuid.New(),
		Kind:        models.KindRunSnapshot,
		CasID:       casID,
		PlanHash:    &planHash,
		VersionHash: &versionHash,
		NodesCount:  &nodesCount,
		EdgesCount:  &edgesCount,
		Meta:        make(map[string]interface{}),
		CreatedBy:   createdBy,
		CreatedAt:   time.Now(),
	}

	if err := s.repo.Create(ctx, artifact); err != nil {
		return uuid.Nil, fmt.Errorf("failed to create run snapshot artifact: %w", err)
	}

	s.log.Info("created run snapshot artifact",
		"artifact_id", artifact.ArtifactID,
		"plan_hash", planHash,
	)

	return artifact.ArtifactID, nil
}

// GetByID retrieves an artifact by ID
func (s *ArtifactService) GetByID(ctx context.Context, artifactID uuid.UUID) (*models.Artifact, error) {
	artifact, err := s.repo.GetByID(ctx, artifactID)
	if err != nil {
		return nil, fmt.Errorf("artifact not found: %w", err)
	}

	return artifact, nil
}

// GetByVersionHash retrieves an artifact by version hash
func (s *ArtifactService) GetByVersionHash(ctx context.Context, versionHash string) (*models.Artifact, error) {
	artifact, err := s.repo.GetByVersionHash(ctx, versionHash)
	if err != nil {
		return nil, fmt.Errorf("artifact not found: %w", err)
	}

	return artifact, nil
}

// GetByPlanHash retrieves a snapshot by plan hash (cache lookup)
func (s *ArtifactService) GetByPlanHash(ctx context.Context, planHash string) (*models.Artifact, error) {
	artifact, err := s.repo.GetByPlanHash(ctx, planHash)
	if err != nil {
		return nil, fmt.Errorf("snapshot not found: %w", err)
	}

	return artifact, nil
}

// ListByKind lists artifacts by kind
func (s *ArtifactService) ListByKind(ctx context.Context, kind string, limit int) ([]*models.Artifact, error) {
	artifacts, err := s.repo.ListByKind(ctx, kind, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list artifacts: %w", err)
	}

	return artifacts, nil
}

// GetPatchChain retrieves the full patch chain for a head artifact
func (s *ArtifactService) GetPatchChain(ctx context.Context, headID uuid.UUID) ([]*models.Artifact, error) {
	patches, err := s.repo.GetPatchChain(ctx, headID)
	if err != nil {
		return nil, fmt.Errorf("failed to get patch chain: %w", err)
	}

	s.log.Info("retrieved patch chain", "head_id", headID, "patches", len(patches))
	return patches, nil
}
