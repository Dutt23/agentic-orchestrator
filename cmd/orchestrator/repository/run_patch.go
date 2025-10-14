package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/lyzr/orchestrator/cmd/orchestrator/models"
	"github.com/lyzr/orchestrator/common/db"
)

// RunPatchRepository handles database operations for run patches
type RunPatchRepository struct {
	db *db.DB
}

// NewRunPatchRepository creates a new run patch repository
func NewRunPatchRepository(database *db.DB) *RunPatchRepository {
	return &RunPatchRepository{db: database}
}

// Create inserts a new run patch
func (r *RunPatchRepository) Create(ctx context.Context, runPatch *models.RunPatch) error {
	query := `
		INSERT INTO run_patches (id, run_id, artifact_id, seq, node_id, description, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at
	`

	_, err := r.db.Exec(
		ctx,
		query,
		runPatch.ID,
		runPatch.RunID,
		runPatch.ArtifactID,
		runPatch.Seq,
		runPatch.NodeID,
		runPatch.Description,
		runPatch.CreatedBy,
	)

	if err != nil {
		return fmt.Errorf("failed to create run patch: %w", err)
	}

	return nil
}

// GetByRunID retrieves all patches for a specific run, ordered by sequence
func (r *RunPatchRepository) GetByRunID(ctx context.Context, runID string) ([]*models.RunPatch, error) {
	query := `
		SELECT id, run_id, artifact_id, seq, node_id, description, created_at, created_by
		FROM run_patches
		WHERE run_id = $1
		ORDER BY seq ASC
	`

	rows, err := r.db.Query(ctx, query, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get run patches: %w", err)
	}
	defer rows.Close()

	var patches []*models.RunPatch
	for rows.Next() {
		patch := &models.RunPatch{}
		err := rows.Scan(&patch.ID, &patch.RunID, &patch.ArtifactID, &patch.Seq, &patch.NodeID, &patch.Description, &patch.CreatedAt, &patch.CreatedBy)
		if err != nil {
			return nil, fmt.Errorf("failed to scan run patch: %w", err)
		}
		patches = append(patches, patch)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating run patches: %w", err)
	}

	return patches, nil
}

// GetByRunIDWithDetails retrieves patches with artifact and CAS details
func (r *RunPatchRepository) GetByRunIDWithDetails(ctx context.Context, runID string) ([]*models.RunPatchWithDetails, error) {
	query := `
		SELECT
			rp.id,
			rp.run_id,
			rp.artifact_id,
			rp.seq,
			rp.node_id,
			rp.description,
			rp.created_at,
			rp.created_by,
			a.cas_id,
			a.depth,
			a.op_count
		FROM run_patches rp
		JOIN artifact a ON rp.artifact_id = a.artifact_id
		WHERE rp.run_id = $1
		ORDER BY rp.seq ASC
	`

	rows, err := r.db.Query(ctx, query, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get run patches with details: %w", err)
	}
	defer rows.Close()

	var patches []*models.RunPatchWithDetails
	for rows.Next() {
		patch := &models.RunPatchWithDetails{}
		err := rows.Scan(
			&patch.ID,
			&patch.RunID,
			&patch.ArtifactID,
			&patch.Seq,
			&patch.NodeID,
			&patch.Description,
			&patch.CreatedAt,
			&patch.CreatedBy,
			&patch.CASID,
			&patch.Depth,
			&patch.OpCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan run patch with details: %w", err)
		}
		patches = append(patches, patch)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating run patches: %w", err)
	}

	return patches, nil
}

// GetNextSeq gets the next sequence number for a run
func (r *RunPatchRepository) GetNextSeq(ctx context.Context, runID string) (int, error) {
	query := `
		SELECT COALESCE(MAX(seq), 0) + 1
		FROM run_patches
		WHERE run_id = $1
	`

	var nextSeq int
	err := r.db.QueryRow(ctx, query, runID).Scan(&nextSeq)
	if err != nil {
		return 0, fmt.Errorf("failed to get next sequence: %w", err)
	}

	return nextSeq, nil
}

// GetByID retrieves a specific run patch by ID
func (r *RunPatchRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.RunPatch, error) {
	query := `
		SELECT id, run_id, artifact_id, seq, node_id, description, created_at, created_by
		FROM run_patches
		WHERE id = $1
	`

	patch := &models.RunPatch{}
	err := r.db.QueryRow(ctx, query, id).Scan(&patch.ID, &patch.RunID, &patch.ArtifactID, &patch.Seq, &patch.NodeID, &patch.Description, &patch.CreatedAt, &patch.CreatedBy)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get run patch: %w", err)
	}

	return patch, nil
}

// DeleteByRunID deletes all patches for a specific run
func (r *RunPatchRepository) DeleteByRunID(ctx context.Context, runID string) error {
	query := `DELETE FROM run_patches WHERE run_id = $1`
	_, err := r.db.Exec(ctx, query, runID)
	if err != nil {
		return fmt.Errorf("failed to delete run patches: %w", err)
	}
	return nil
}

// CountByRunID returns the number of patches for a run
func (r *RunPatchRepository) CountByRunID(ctx context.Context, runID string) (int, error) {
	query := `SELECT COUNT(*) FROM run_patches WHERE run_id = $1`

	var count int
	err := r.db.QueryRow(ctx, query, runID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count run patches: %w", err)
	}

	return count, nil
}
