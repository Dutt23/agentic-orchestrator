package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/lyzr/orchestrator/cmd/orchestrator/models"
	"github.com/lyzr/orchestrator/common/db"
)

// ArtifactRepository handles database operations for artifacts
type ArtifactRepository struct {
	db *db.DB
}

// NewArtifactRepository creates a new artifact repository
func NewArtifactRepository(db *db.DB) *ArtifactRepository {
	return &ArtifactRepository{db: db}
}

// Create inserts a new artifact
func (r *ArtifactRepository) Create(ctx context.Context, artifact *models.Artifact) error {
	query := `
		INSERT INTO artifact (
			artifact_id, kind, cas_id, name, plan_hash, version_hash,
			base_version, depth, op_count, nodes_count, edges_count,
			meta, created_by, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
		)
		RETURNING artifact_id
	`

	err := r.db.QueryRow(ctx, query,
		artifact.ArtifactID,
		artifact.Kind,
		artifact.CasID,
		artifact.Name,
		artifact.PlanHash,
		artifact.VersionHash,
		artifact.BaseVersion,
		artifact.Depth,
		artifact.OpCount,
		artifact.NodesCount,
		artifact.EdgesCount,
		artifact.Meta,
		artifact.CreatedBy,
		artifact.CreatedAt,
	).Scan(&artifact.ArtifactID)

	if err != nil {
		return fmt.Errorf("failed to create artifact: %w", err)
	}

	return nil
}

// GetByID retrieves an artifact by its ID
func (r *ArtifactRepository) GetByID(ctx context.Context, artifactID uuid.UUID) (*models.Artifact, error) {
	query := `
		SELECT
			artifact_id, kind, cas_id, name, plan_hash, version_hash,
			base_version, depth, op_count, nodes_count, edges_count,
			meta, created_by, created_at
		FROM artifact
		WHERE artifact_id = $1
	`

	artifact := &models.Artifact{}
	err := r.db.QueryRow(ctx, query, artifactID).Scan(
		&artifact.ArtifactID,
		&artifact.Kind,
		&artifact.CasID,
		&artifact.Name,
		&artifact.PlanHash,
		&artifact.VersionHash,
		&artifact.BaseVersion,
		&artifact.Depth,
		&artifact.OpCount,
		&artifact.NodesCount,
		&artifact.EdgesCount,
		&artifact.Meta,
		&artifact.CreatedBy,
		&artifact.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get artifact: %w", err)
	}

	return artifact, nil
}

// GetByVersionHash retrieves an artifact by its version hash
func (r *ArtifactRepository) GetByVersionHash(ctx context.Context, versionHash string) (*models.Artifact, error) {
	query := `
		SELECT
			artifact_id, kind, cas_id, name, plan_hash, version_hash,
			base_version, depth, op_count, nodes_count, edges_count,
			meta, created_by, created_at
		FROM artifact
		WHERE version_hash = $1
		LIMIT 1
	`

	artifact := &models.Artifact{}
	err := r.db.QueryRow(ctx, query, versionHash).Scan(
		&artifact.ArtifactID,
		&artifact.Kind,
		&artifact.CasID,
		&artifact.Name,
		&artifact.PlanHash,
		&artifact.VersionHash,
		&artifact.BaseVersion,
		&artifact.Depth,
		&artifact.OpCount,
		&artifact.NodesCount,
		&artifact.EdgesCount,
		&artifact.Meta,
		&artifact.CreatedBy,
		&artifact.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get artifact by version hash: %w", err)
	}

	return artifact, nil
}

// GetByPlanHash retrieves a snapshot artifact by its plan hash
func (r *ArtifactRepository) GetByPlanHash(ctx context.Context, planHash string) (*models.Artifact, error) {
	query := `
		SELECT
			artifact_id, kind, cas_id, name, plan_hash, version_hash,
			base_version, depth, op_count, nodes_count, edges_count,
			meta, created_by, created_at
		FROM artifact
		WHERE kind = 'run_snapshot' AND plan_hash = $1
		LIMIT 1
	`

	artifact := &models.Artifact{}
	err := r.db.QueryRow(ctx, query, planHash).Scan(
		&artifact.ArtifactID,
		&artifact.Kind,
		&artifact.CasID,
		&artifact.Name,
		&artifact.PlanHash,
		&artifact.VersionHash,
		&artifact.BaseVersion,
		&artifact.Depth,
		&artifact.OpCount,
		&artifact.NodesCount,
		&artifact.EdgesCount,
		&artifact.Meta,
		&artifact.CreatedBy,
		&artifact.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get artifact by plan hash: %w", err)
	}

	return artifact, nil
}

// ListByKind lists artifacts by kind
func (r *ArtifactRepository) ListByKind(ctx context.Context, kind string, limit int) ([]*models.Artifact, error) {
	query := `
		SELECT
			artifact_id, kind, cas_id, name, plan_hash, version_hash,
			base_version, depth, op_count, nodes_count, edges_count,
			meta, created_by, created_at
		FROM artifact
		WHERE kind = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := r.db.Query(ctx, query, kind, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list artifacts: %w", err)
	}
	defer rows.Close()

	var artifacts []*models.Artifact
	for rows.Next() {
		artifact := &models.Artifact{}
		err := rows.Scan(
			&artifact.ArtifactID,
			&artifact.Kind,
			&artifact.CasID,
			&artifact.Name,
			&artifact.PlanHash,
			&artifact.VersionHash,
			&artifact.BaseVersion,
			&artifact.Depth,
			&artifact.OpCount,
			&artifact.NodesCount,
			&artifact.EdgesCount,
			&artifact.Meta,
			&artifact.CreatedBy,
			&artifact.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan artifact: %w", err)
		}
		artifacts = append(artifacts, artifact)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating artifacts: %w", err)
	}

	return artifacts, nil
}

// GetPatchChain retrieves the full patch chain for a head artifact
func (r *ArtifactRepository) GetPatchChain(ctx context.Context, headID uuid.UUID) ([]*models.Artifact, error) {
	query := `
		SELECT
			a.artifact_id, a.kind, a.cas_id, a.name, a.plan_hash, a.version_hash,
			a.base_version, a.depth, a.op_count, a.nodes_count, a.edges_count,
			a.meta, a.created_by, a.created_at
		FROM artifact a
		INNER JOIN patch_chain_member pcm ON a.artifact_id = pcm.member_id
		WHERE pcm.head_id = $1
		ORDER BY pcm.seq ASC
	`

	rows, err := r.db.Query(ctx, query, headID)
	if err != nil {
		return nil, fmt.Errorf("failed to get patch chain: %w", err)
	}
	defer rows.Close()

	var artifacts []*models.Artifact
	for rows.Next() {
		artifact := &models.Artifact{}
		err := rows.Scan(
			&artifact.ArtifactID,
			&artifact.Kind,
			&artifact.CasID,
			&artifact.Name,
			&artifact.PlanHash,
			&artifact.VersionHash,
			&artifact.BaseVersion,
			&artifact.Depth,
			&artifact.OpCount,
			&artifact.NodesCount,
			&artifact.EdgesCount,
			&artifact.Meta,
			&artifact.CreatedBy,
			&artifact.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan artifact in patch chain: %w", err)
		}
		artifacts = append(artifacts, artifact)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating patch chain: %w", err)
	}

	return artifacts, nil
}
