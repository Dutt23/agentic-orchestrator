package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/lyzr/orchestrator/common/db"
	"github.com/lyzr/orchestrator/common/models"
)

// RunRepository handles database operations for workflow runs
type RunRepository struct {
	db *db.DB
}

// NewRunRepository creates a new run repository
func NewRunRepository(database *db.DB) *RunRepository {
	return &RunRepository{db: database}
}

// Create inserts a new workflow run
func (r *RunRepository) Create(ctx context.Context, run *models.Run) error {
	query := `
		INSERT INTO run (run_id, base_kind, base_ref, tags_snapshot, status, submitted_by, submitted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := r.db.Exec(
		ctx,
		query,
		run.RunID,
		run.BaseKind,
		run.BaseRef,
		run.TagsSnapshot,
		run.Status,
		run.SubmittedBy,
		run.SubmittedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create run: %w", err)
	}

	return nil
}

// GetByID retrieves a run by its ID
func (r *RunRepository) GetByID(ctx context.Context, runID uuid.UUID) (*models.Run, error) {
	query := `
		SELECT run_id, base_kind, base_ref, tags_snapshot, status, submitted_by, submitted_at
		FROM run
		WHERE run_id = $1
	`

	run := &models.Run{}
	err := r.db.QueryRow(ctx, query, runID).Scan(
		&run.RunID,
		&run.BaseKind,
		&run.BaseRef,
		&run.TagsSnapshot,
		&run.Status,
		&run.SubmittedBy,
		&run.SubmittedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get run: %w", err)
	}

	return run, nil
}

// UpdateStatus updates the status of a run
func (r *RunRepository) UpdateStatus(ctx context.Context, runID uuid.UUID, status models.RunStatus) error {
	query := `
		UPDATE run
		SET status = $2
		WHERE run_id = $1
	`

	_, err := r.db.Exec(ctx, query, runID, status)
	if err != nil {
		return fmt.Errorf("failed to update run status: %w", err)
	}

	return nil
}

// ListByUser retrieves runs submitted by a specific user
func (r *RunRepository) ListByUser(ctx context.Context, username string, limit int) ([]*models.Run, error) {
	query := `
		SELECT run_id, base_kind, base_ref, tags_snapshot, status, submitted_by, submitted_at
		FROM run
		WHERE submitted_by = $1
		ORDER BY submitted_at DESC
		LIMIT $2
	`

	rows, err := r.db.Query(ctx, query, username, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list runs: %w", err)
	}
	defer rows.Close()

	var runs []*models.Run
	for rows.Next() {
		run := &models.Run{}
		err := rows.Scan(
			&run.RunID,
			&run.BaseKind,
			&run.BaseRef,
			&run.TagsSnapshot,
			&run.Status,
			&run.SubmittedBy,
			&run.SubmittedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan run: %w", err)
		}
		runs = append(runs, run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating runs: %w", err)
	}

	return runs, nil
}

// ListByWorkflowTag retrieves runs for a specific workflow tag
// Ordered by submitted_at DESC
func (r *RunRepository) ListByWorkflowTag(ctx context.Context, tag string, limit int) ([]*models.Run, error) {
	query := `
		SELECT run_id, base_kind, base_ref, tags_snapshot, status, submitted_by, submitted_at
		FROM run
		WHERE tags_snapshot ? $1
		ORDER BY submitted_at DESC
		LIMIT $2
	`

	rows, err := r.db.Query(ctx, query, tag, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list runs by workflow tag: %w", err)
	}
	defer rows.Close()

	var runs []*models.Run
	for rows.Next() {
		run := &models.Run{}
		err := rows.Scan(
			&run.RunID,
			&run.BaseKind,
			&run.BaseRef,
			&run.TagsSnapshot,
			&run.Status,
			&run.SubmittedBy,
			&run.SubmittedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan run: %w", err)
		}
		runs = append(runs, run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating runs: %w", err)
	}

	return runs, nil
}
