package repository

import (
	"context"
	"fmt"

	"github.com/lyzr/orchestrator/cmd/orchestrator/models"
	"github.com/lyzr/orchestrator/common/db"
)

// CASBlobRepository handles database operations for CAS blobs
type CASBlobRepository struct {
	db *db.DB
}

// NewCASBlobRepository creates a new CAS blob repository
func NewCASBlobRepository(db *db.DB) *CASBlobRepository {
	return &CASBlobRepository{db: db}
}

// Create inserts a new CAS blob
func (r *CASBlobRepository) Create(ctx context.Context, blob *models.CASBlob) error {
	query := `
		INSERT INTO cas_blob (cas_id, media_type, size_bytes, content, storage_url, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (cas_id) DO NOTHING
	`

	_, err := r.db.Exec(ctx, query,
		blob.CasID,
		blob.MediaType,
		blob.SizeBytes,
		blob.Content,
		blob.StorageURL,
		blob.CreatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create CAS blob: %w", err)
	}

	return nil
}

// GetByID retrieves a CAS blob by its ID
func (r *CASBlobRepository) GetByID(ctx context.Context, casID string) (*models.CASBlob, error) {
	query := `
		SELECT cas_id, media_type, size_bytes, content, storage_url, created_at
		FROM cas_blob
		WHERE cas_id = $1
	`

	blob := &models.CASBlob{}
	err := r.db.QueryRow(ctx, query, casID).Scan(
		&blob.CasID,
		&blob.MediaType,
		&blob.SizeBytes,
		&blob.Content,
		&blob.StorageURL,
		&blob.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get CAS blob: %w", err)
	}

	return blob, nil
}

// Exists checks if a CAS blob exists
func (r *CASBlobRepository) Exists(ctx context.Context, casID string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM cas_blob WHERE cas_id = $1)`

	var exists bool
	err := r.db.QueryRow(ctx, query, casID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check CAS blob existence: %w", err)
	}

	return exists, nil
}

// GetContentByID retrieves only the content of a CAS blob
func (r *CASBlobRepository) GetContentByID(ctx context.Context, casID string) ([]byte, error) {
	query := `SELECT content FROM cas_blob WHERE cas_id = $1`

	var content []byte
	err := r.db.QueryRow(ctx, query, casID).Scan(&content)
	if err != nil {
		return nil, fmt.Errorf("failed to get CAS blob content: %w", err)
	}

	return content, nil
}

// GetContentBulk retrieves content for multiple CAS blobs in a single query
func (r *CASBlobRepository) GetContentBulk(ctx context.Context, casIDs []string) (map[string][]byte, error) {
	if len(casIDs) == 0 {
		return make(map[string][]byte), nil
	}

	query := `
		SELECT cas_id, content
		FROM cas_blob
		WHERE cas_id = ANY($1)
	`

	rows, err := r.db.Query(ctx, query, casIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get CAS blob contents in bulk: %w", err)
	}
	defer rows.Close()

	results := make(map[string][]byte, len(casIDs))
	for rows.Next() {
		var casID string
		var content []byte
		if err := rows.Scan(&casID, &content); err != nil {
			return nil, fmt.Errorf("failed to scan CAS blob content: %w", err)
		}
		results[casID] = content
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating CAS blob contents: %w", err)
	}

	return results, nil
}

// ListByMediaType lists CAS blobs by media type
func (r *CASBlobRepository) ListByMediaType(ctx context.Context, mediaType string, limit int) ([]*models.CASBlob, error) {
	query := `
		SELECT cas_id, media_type, size_bytes, content, storage_url, created_at
		FROM cas_blob
		WHERE media_type = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := r.db.Query(ctx, query, mediaType, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list CAS blobs: %w", err)
	}
	defer rows.Close()

	var blobs []*models.CASBlob
	for rows.Next() {
		blob := &models.CASBlob{}
		err := rows.Scan(
			&blob.CasID,
			&blob.MediaType,
			&blob.SizeBytes,
			&blob.Content,
			&blob.StorageURL,
			&blob.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan CAS blob: %w", err)
		}
		blobs = append(blobs, blob)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating CAS blobs: %w", err)
	}

	return blobs, nil
}
