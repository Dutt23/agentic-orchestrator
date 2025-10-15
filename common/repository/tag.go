package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/lyzr/orchestrator/common/models"
	"github.com/lyzr/orchestrator/common/db"
)

// TagRepository handles database operations for tags
type TagRepository struct {
	db *db.DB
}

// NewTagRepository creates a new tag repository
func NewTagRepository(db *db.DB) *TagRepository {
	return &TagRepository{db: db}
}

// Create inserts a new tag
func (r *TagRepository) Create(ctx context.Context, tag *models.Tag) error {
	query := `
		INSERT INTO tag (username, tag_name, target_kind, target_id, target_hash, version, created_by, moved_by, moved_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := r.db.Exec(ctx, query,
		tag.Username,
		tag.TagName,
		tag.TargetKind,
		tag.TargetID,
		tag.TargetHash,
		tag.Version,
		tag.CreatedBy,
		tag.MovedBy,
		tag.MovedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create tag: %w", err)
	}

	return nil
}

// GetByName retrieves a tag by username and tag name (exact match)
func (r *TagRepository) GetByName(ctx context.Context, username, tagName string) (*models.Tag, error) {
	query := `
		SELECT username, tag_name, target_kind, target_id, target_hash, version, created_by, moved_by, moved_at
		FROM tag
		WHERE username = $1 AND tag_name = $2
	`

	tag := &models.Tag{}
	err := r.db.QueryRow(ctx, query, username, tagName).Scan(
		&tag.Username,
		&tag.TagName,
		&tag.TargetKind,
		&tag.TargetID,
		&tag.TargetHash,
		&tag.Version,
		&tag.CreatedBy,
		&tag.MovedBy,
		&tag.MovedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get tag: %w", err)
	}

	return tag, nil
}

// Update updates an existing tag (moves it to a new target)
func (r *TagRepository) Update(ctx context.Context, tag *models.Tag) error {
	query := `
		UPDATE tag
		SET target_kind = $3, target_id = $4, target_hash = $5,
		    version = version + 1, moved_by = $6, moved_at = $7
		WHERE username = $1 AND tag_name = $2
		RETURNING version
	`

	err := r.db.QueryRow(ctx, query,
		tag.Username,
		tag.TagName,
		tag.TargetKind,
		tag.TargetID,
		tag.TargetHash,
		tag.MovedBy,
		tag.MovedAt,
	).Scan(&tag.Version)

	if err != nil {
		return fmt.Errorf("failed to update tag: %w", err)
	}

	return nil
}

// CompareAndSwap performs an optimistic lock update (CAS operation)
func (r *TagRepository) CompareAndSwap(ctx context.Context, username, tagName string, expectedVersion int64, newTarget uuid.UUID, newTargetKind, newTargetHash, movedBy string) (bool, error) {
	query := `
		UPDATE tag
		SET target_kind = $4, target_id = $5, target_hash = $6,
		    version = version + 1, moved_by = $7, moved_at = NOW()
		WHERE username = $1 AND tag_name = $2 AND version = $3
		RETURNING version
	`

	var newVersion int64
	err := r.db.QueryRow(ctx, query,
		username,
		tagName,
		expectedVersion,
		newTargetKind,
		newTarget,
		newTargetHash,
		movedBy,
	).Scan(&newVersion)

	if err != nil {
		// If no rows affected, CAS failed
		return false, nil
	}

	return true, nil
}

// Delete removes a tag
func (r *TagRepository) Delete(ctx context.Context, username, tagName string) error {
	query := `DELETE FROM tag WHERE username = $1 AND tag_name = $2`

	result, err := r.db.Exec(ctx, query, username, tagName)
	if err != nil {
		return fmt.Errorf("failed to delete tag: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("tag not found: %s/%s", username, tagName)
	}

	return nil
}

// List retrieves all tags (admin use only)
func (r *TagRepository) List(ctx context.Context) ([]*models.Tag, error) {
	query := `
		SELECT username, tag_name, target_kind, target_id, target_hash, version, created_by, moved_by, moved_at
		FROM tag
		ORDER BY username, tag_name ASC
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}
	defer rows.Close()

	var tags []*models.Tag
	for rows.Next() {
		tag := &models.Tag{}
		err := rows.Scan(
			&tag.Username,
			&tag.TagName,
			&tag.TargetKind,
			&tag.TargetID,
			&tag.TargetHash,
			&tag.Version,
			&tag.CreatedBy,
			&tag.MovedBy,
			&tag.MovedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan tag: %w", err)
		}
		tags = append(tags, tag)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tags: %w", err)
	}

	return tags, nil
}

// ListByUsername retrieves all tags for a specific user (EXACT MATCH - no LIKE query!)
// This is the secure replacement for ListByPrefix - uses exact username matching
// Returns all tags regardless of target_kind (dag_version or patch_set)
// Each tag represents one logical workflow, pointing to its latest version
func (r *TagRepository) ListByUsername(ctx context.Context, username string) ([]*models.Tag, error) {
	query := `
		SELECT username, tag_name, target_kind, target_id, target_hash, version, created_by, moved_by, moved_at
		FROM tag
		WHERE username = $1
		ORDER BY tag_name ASC
	`

	rows, err := r.db.Query(ctx, query, username)
	if err != nil {
		return nil, fmt.Errorf("failed to list tags by username: %w", err)
	}
	defer rows.Close()

	var tags []*models.Tag
	for rows.Next() {
		tag := &models.Tag{}
		err := rows.Scan(
			&tag.Username,
			&tag.TagName,
			&tag.TargetKind,
			&tag.TargetID,
			&tag.TargetHash,
			&tag.Version,
			&tag.CreatedBy,
			&tag.MovedBy,
			&tag.MovedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan tag: %w", err)
		}
		tags = append(tags, tag)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tags: %w", err)
	}

	return tags, nil
}

// Exists checks if a tag exists for a specific user
func (r *TagRepository) Exists(ctx context.Context, username, tagName string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM tag WHERE username = $1 AND tag_name = $2)`

	var exists bool
	err := r.db.QueryRow(ctx, query, username, tagName).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check tag existence: %w", err)
	}

	return exists, nil
}

// GetHistory retrieves the tag move history for a specific user's tag
func (r *TagRepository) GetHistory(ctx context.Context, username, tagName string, limit int) ([]*models.TagMove, error) {
	query := `
		SELECT id, username, tag_name, from_kind, from_id, to_kind, to_id, expected_hash, moved_by, moved_at
		FROM tag_move
		WHERE username = $1 AND tag_name = $2
		ORDER BY moved_at DESC
		LIMIT $3
	`

	rows, err := r.db.Query(ctx, query, username, tagName, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get tag history: %w", err)
	}
	defer rows.Close()

	var history []*models.TagMove
	for rows.Next() {
		move := &models.TagMove{}
		err := rows.Scan(
			&move.ID,
			&move.Username,
			&move.TagName,
			&move.FromKind,
			&move.FromID,
			&move.ToKind,
			&move.ToID,
			&move.ExpectedHash,
			&move.MovedBy,
			&move.MovedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan tag move: %w", err)
		}
		history = append(history, move)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tag history: %w", err)
	}

	return history, nil
}
