package models

import (
	"time"

	"github.com/google/uuid"
)

// ArtifactKind represents the type of artifact
type ArtifactKind string

const (
	KindDAGVersion   ArtifactKind = "dag_version"
	KindPatchSet     ArtifactKind = "patch_set"
	KindRunManifest  ArtifactKind = "run_manifest"
	KindRunSnapshot  ArtifactKind = "run_snapshot"
)

// Artifact represents an entry in the artifact catalog
// Maps to: artifact table
type Artifact struct {
	// Unique artifact ID (UUID v7)
	ArtifactID uuid.UUID `db:"artifact_id" json:"artifact_id"`

	// Artifact type: 'dag_version', 'patch_set', 'run_manifest', 'run_snapshot'
	Kind ArtifactKind `db:"kind" json:"kind"`

	// Reference to CAS blob
	CasID string `db:"cas_id" json:"cas_id"`

	// Optional human-readable name
	Name *string `db:"name" json:"name,omitempty"`

	// ========================================================================
	// EXTRACTED COLUMNS (hot columns for performance)
	// ========================================================================

	// For snapshot cache lookups (run_snapshot only)
	PlanHash *string `db:"plan_hash" json:"plan_hash,omitempty"`

	// For integrity verification (dag_version, run_snapshot)
	VersionHash *string `db:"version_hash" json:"version_hash,omitempty"`

	// For patch chain traversal (patch_set)
	BaseVersion *uuid.UUID `db:"base_version" json:"base_version,omitempty"`

	// Chain depth from base (patch_set)
	// depth=1: first patch on a version
	// depth=N: Nth patch in chain
	Depth *int `db:"depth" json:"depth,omitempty"`

	// Operation count (patch_set)
	OpCount *int `db:"op_count" json:"op_count,omitempty"`

	// Node/edge counts (dag_version, run_snapshot)
	NodesCount *int `db:"nodes_count" json:"nodes_count,omitempty"`
	EdgesCount *int `db:"edges_count" json:"edges_count,omitempty"`

	// For compaction tracking (dag_version only)
	// Points to the patch that was compacted into this base version
	// Example: V2 (new base) compacted_from_id → P20 (old head)
	CompactedFromID *uuid.UUID `db:"compacted_from_id" json:"compacted_from_id,omitempty"`

	// ========================================================================
	// FLEXIBLE METADATA (rarely queried)
	// ========================================================================

	// Remaining flexible metadata (JSONB)
	// Examples:
	//   {"author": "user@example.com", "message": "Add retry logic"}
	//   {"materializer_version": "1.0.0"}
	//   {"original_depth": 20, "compacted_at": "2025-10-12T..."}
	Meta map[string]interface{} `db:"meta" json:"meta,omitempty"`

	// Audit fields
	CreatedBy string    `db:"created_by" json:"created_by"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// IsDAGVersion checks if artifact is a DAG version
func (a *Artifact) IsDAGVersion() bool {
	return a.Kind == KindDAGVersion
}

// IsPatchSet checks if artifact is a patch set
func (a *Artifact) IsPatchSet() bool {
	return a.Kind == KindPatchSet
}

// IsRunManifest checks if artifact is a run manifest
func (a *Artifact) IsRunManifest() bool {
	return a.Kind == KindRunManifest
}

// IsRunSnapshot checks if artifact is a run snapshot
func (a *Artifact) IsRunSnapshot() bool {
	return a.Kind == KindRunSnapshot
}