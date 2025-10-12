package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lyzr/orchestrator/cmd/orchestrator/models"
	"github.com/lyzr/orchestrator/cmd/orchestrator/repository"
	"github.com/lyzr/orchestrator/common/logger"
)

// CompactionService handles workflow compaction operations
// Compaction "squashes" a long patch chain into a new base version
type CompactionService struct {
	artifactRepo *repository.ArtifactRepository
	casRepo      *repository.CASBlobRepository
	tagRepo      *repository.TagRepository
	casService   *CASService
	materializer *MaterializerService
	log          *logger.Logger
}

// NewCompactionService creates a new compaction service
func NewCompactionService(
	artifactRepo *repository.ArtifactRepository,
	casRepo *repository.CASBlobRepository,
	tagRepo *repository.TagRepository,
	casService *CASService,
	materializer *MaterializerService,
	log *logger.Logger,
) *CompactionService {
	return &CompactionService{
		artifactRepo: artifactRepo,
		casRepo:      casRepo,
		tagRepo:      tagRepo,
		casService:   casService,
		materializer: materializer,
		log:          log,
	}
}

// CompactionResult contains the results of a compaction operation
type CompactionResult struct {
	NewBaseID       uuid.UUID // V2 artifact ID
	OldChainDepth   int       // Original depth (e.g., 20)
	CompactedFromID uuid.UUID // P20 artifact ID
	NewCasID        string    // CAS ID of compacted workflow
	MaterializedAt  time.Time
}

// CompactWorkflow compacts a patch chain into a new base version
// This implements the 9-step algorithm:
// 1. Get patch metadata
// 2. Get patch chain from patch_chain_member
// 3. Fetch base version content
// 4. Fetch all patch contents
// 5. Materialize full workflow (apply patches)
// 6. Hash the materialized content
// 7. Insert CAS blob
// 8. Create new base artifact (V2)
// 9. Return new base ID
//
// IMPORTANT: This does NOT delete old chains or move tags!
// Old chain (V1+P1-P20) is preserved for undo/redo.
func (s *CompactionService) CompactWorkflow(ctx context.Context, patchID uuid.UUID, compactedBy string) (*CompactionResult, error) {
	s.log.Info("starting compaction workflow", "patch_id", patchID)

	// Step 1: Get patch metadata
	patch, err := s.artifactRepo.GetByID(ctx, patchID)
	if err != nil {
		return nil, fmt.Errorf("failed to get patch artifact: %w", err)
	}

	if patch.Kind != models.KindPatchSet {
		return nil, fmt.Errorf("artifact is not a patch_set (kind=%s)", patch.Kind)
	}

	if patch.Depth == nil || *patch.Depth == 0 {
		return nil, fmt.Errorf("cannot compact base version (depth=0)")
	}

	depth := *patch.Depth
	s.log.Info("patch metadata retrieved", "depth", depth, "base_version", patch.BaseVersion)

	// Step 2: Get patch chain (O(1) lookup via patch_chain_member)
	patchChain, err := s.artifactRepo.GetPatchChain(ctx, patchID)
	if err != nil {
		return nil, fmt.Errorf("failed to get patch chain: %w", err)
	}

	if len(patchChain) == 0 {
		return nil, fmt.Errorf("patch chain is empty")
	}

	s.log.Info("patch chain retrieved", "length", len(patchChain))

	// Step 3: Fetch base version content
	if patch.BaseVersion == nil {
		return nil, fmt.Errorf("patch has no base version")
	}

	baseArtifact, err := s.artifactRepo.GetByID(ctx, *patch.BaseVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get base version artifact: %w", err)
	}

	baseContent, err := s.casService.GetContent(ctx, baseArtifact.CasID)
	if err != nil {
		return nil, fmt.Errorf("failed to get base version content: %w", err)
	}

	s.log.Info("base version fetched", "cas_id", baseArtifact.CasID, "size_bytes", len(baseContent))

	// Step 4: Fetch all patch contents
	patchCasIDs := make([]string, 0, len(patchChain))
	for _, p := range patchChain {
		patchCasIDs = append(patchCasIDs, p.CasID)
	}

	patchContents, err := s.casService.GetContentBulk(ctx, patchCasIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch patch contents: %w", err)
	}

	if len(patchContents) != len(patchChain) {
		return nil, fmt.Errorf("expected %d patch contents, got %d", len(patchChain), len(patchContents))
	}

	s.log.Info("patch contents fetched", "count", len(patchContents))

	// Step 5: Materialize full workflow (apply patches sequentially)
	currentJSON := baseContent
	for i, p := range patchChain {
		patchJSON, ok := patchContents[p.CasID]
		if !ok {
			return nil, fmt.Errorf("missing patch content for cas_id=%s", p.CasID)
		}

		s.log.Debug("applying patch", "seq", i+1, "artifact_id", p.ArtifactID, "cas_id", p.CasID)

		resultJSON, err := s.materializer.applyPatch(currentJSON, patchJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to apply patch %d (artifact=%s): %w", i+1, p.ArtifactID, err)
		}

		currentJSON = resultJSON
	}

	materializedWorkflow := currentJSON
	s.log.Info("materialization complete", "size_bytes", len(materializedWorkflow))

	// Step 6: Compute hash and validate JSON
	var workflowMap map[string]interface{}
	if err := json.Unmarshal(materializedWorkflow, &workflowMap); err != nil {
		return nil, fmt.Errorf("materialized workflow is not valid JSON: %w", err)
	}

	versionHash := s.casService.ComputeHash(materializedWorkflow)
	s.log.Info("computed version hash", "version_hash", versionHash)

	// Step 7: Store materialized workflow in CAS
	casID, err := s.casService.StoreContent(ctx, materializedWorkflow, "application/json;type=dag")
	if err != nil {
		return nil, fmt.Errorf("failed to store compacted workflow in CAS: %w", err)
	}

	s.log.Info("stored compacted workflow in CAS", "cas_id", casID)

	// Step 8: Create new base version artifact (V2)
	nodesCount := 0
	edgesCount := 0
	if nodes, ok := workflowMap["nodes"].([]interface{}); ok {
		nodesCount = len(nodes)
	}
	if edges, ok := workflowMap["edges"].([]interface{}); ok {
		edgesCount = len(edges)
	}

	newBase := &models.Artifact{
		ArtifactID:      uuid.New(),
		Kind:            models.KindDAGVersion,
		CasID:           casID,
		BaseVersion:     nil, // This is a base version, not a patch
		Depth:           intPtr(0),
		VersionHash:     &versionHash,
		NodesCount:      &nodesCount,
		EdgesCount:      &edgesCount,
		CompactedFromID: &patchID, // NEW: Indexed column for fast lookups
		Meta: map[string]interface{}{
			"compacted_at":     time.Now().Format(time.RFC3339),
			"original_depth":   depth,
			"original_patches": len(patchChain),
		},
		CreatedBy: &compactedBy,
		CreatedAt: time.Now(),
	}

	if err := s.artifactRepo.Create(ctx, newBase); err != nil {
		return nil, fmt.Errorf("failed to create compacted base artifact: %w", err)
	}

	s.log.Info("created compacted base version",
		"artifact_id", newBase.ArtifactID,
		"old_depth", depth,
		"compacted_from", patchID,
		"nodes", nodesCount,
		"edges", edgesCount,
	)

	// Step 9: Return result
	result := &CompactionResult{
		NewBaseID:       newBase.ArtifactID,
		OldChainDepth:   depth,
		CompactedFromID: patchID,
		NewCasID:        casID,
		MaterializedAt:  time.Now(),
	}

	s.log.Info("compaction complete",
		"new_base_id", result.NewBaseID,
		"old_depth", result.OldChainDepth,
		"saved_depth", result.OldChainDepth, // Future patches start at depth 1 instead of 21+
	)

	return result, nil
}

// MigrateTagToCompactedBase migrates a tag from old patch chain to new base version
// This updates the tag and records the move in tag_move for undo/redo support
func (s *CompactionService) MigrateTagToCompactedBase(
	ctx context.Context,
	tagName string,
	newBaseID uuid.UUID,
	movedBy string,
) error {
	s.log.Info("migrating tag to compacted base", "tag_name", tagName, "new_base_id", newBaseID)

	// Get current tag position
	tag, err := s.tagRepo.GetByName(ctx, tagName)
	if err != nil {
		return fmt.Errorf("failed to get tag: %w", err)
	}

	oldTargetID := tag.TargetID
	oldTargetKind := tag.TargetKind

	s.log.Info("current tag position",
		"tag_name", tagName,
		"old_target_id", oldTargetID,
		"old_target_kind", oldTargetKind,
	)

	// Verify new base exists
	newBase, err := s.artifactRepo.GetByID(ctx, newBaseID)
	if err != nil {
		return fmt.Errorf("new base artifact not found: %w", err)
	}

	if newBase.Kind != models.KindDAGVersion {
		return fmt.Errorf("new base is not a dag_version (kind=%s)", newBase.Kind)
	}

	// Update tag to point to new base
	tag.TargetID = newBaseID
	tag.TargetKind = models.KindDAGVersion
	tag.MovedBy = &movedBy
	tag.MovedAt = time.Now()

	if err := s.tagRepo.Update(ctx, tag); err != nil {
		return fmt.Errorf("failed to update tag: %w", err)
	}

	s.log.Info("tag migrated successfully",
		"tag_name", tagName,
		"from", oldTargetID,
		"to", newBaseID,
		"moved_by", movedBy,
	)

	return nil
}

// ShouldCompact determines if a patch chain should be compacted
// Returns true if depth exceeds threshold
func (s *CompactionService) ShouldCompact(ctx context.Context, artifactID uuid.UUID, depthThreshold int) (bool, error) {
	artifact, err := s.artifactRepo.GetByID(ctx, artifactID)
	if err != nil {
		return false, fmt.Errorf("failed to get artifact: %w", err)
	}

	// Only patch_sets can be compacted
	if artifact.Kind != models.KindPatchSet {
		return false, nil
	}

	// Check depth
	if artifact.Depth == nil {
		return false, nil
	}

	depth := *artifact.Depth
	shouldCompact := depth >= depthThreshold

	s.log.Info("compaction check",
		"artifact_id", artifactID,
		"depth", depth,
		"threshold", depthThreshold,
		"should_compact", shouldCompact,
	)

	return shouldCompact, nil
}

// FindCompactedBase finds a compacted version for a given patch
// Returns the compacted base if it exists, nil otherwise
func (s *CompactionService) FindCompactedBase(ctx context.Context, patchID uuid.UUID) (*models.Artifact, error) {
	artifact, err := s.artifactRepo.FindCompactedBase(ctx, patchID)
	if err != nil {
		return nil, fmt.Errorf("failed to find compacted base: %w", err)
	}

	if artifact == nil {
		s.log.Debug("no compacted base found", "patch_id", patchID)
		return nil, nil
	}

	s.log.Info("found compacted base",
		"patch_id", patchID,
		"compacted_base_id", artifact.ArtifactID,
	)

	return artifact, nil
}

// GetCompactionStats returns statistics about potential compaction savings
type CompactionStats struct {
	CandidatePatches   int     // Number of patches exceeding threshold
	TotalDepth         int     // Sum of all depths
	EstimatedSavings   int     // Estimated rows saved in patch_chain_member
	LongestChainDepth  int     // Deepest chain
	LongestChainID     *uuid.UUID
}

func (s *CompactionService) GetCompactionStats(ctx context.Context, depthThreshold int) (*CompactionStats, error) {
	candidates, err := s.artifactRepo.GetCompactionCandidates(ctx, depthThreshold)
	if err != nil {
		return nil, fmt.Errorf("failed to get compaction candidates: %w", err)
	}

	stats := &CompactionStats{}

	for _, artifact := range candidates {
		if artifact.Depth == nil {
			continue
		}

		depth := *artifact.Depth

		stats.CandidatePatches++
		stats.TotalDepth += depth

		// Estimate savings: n(n+1)/2 rows currently, will be ~1 after compaction
		stats.EstimatedSavings += (depth * (depth + 1) / 2) - 1

		if depth > stats.LongestChainDepth {
			stats.LongestChainDepth = depth
			stats.LongestChainID = &artifact.ArtifactID
		}
	}

	s.log.Info("compaction statistics",
		"candidates", stats.CandidatePatches,
		"total_depth", stats.TotalDepth,
		"estimated_savings_rows", stats.EstimatedSavings,
		"longest_chain_depth", stats.LongestChainDepth,
	)

	return stats, nil
}

// Helper function to create int pointer
func intPtr(i int) *int {
	return &i
}
