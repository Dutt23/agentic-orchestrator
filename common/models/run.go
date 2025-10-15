package models

import (
	"time"

	"github.com/google/uuid"
)

// RunStatus represents the status of a workflow run
type RunStatus string

const (
	StatusQueued              RunStatus = "QUEUED"
	StatusRunning             RunStatus = "RUNNING"
	StatusWaitingForApproval  RunStatus = "WAITING_FOR_APPROVAL"
	StatusCompleted           RunStatus = "COMPLETED"
	StatusFailed              RunStatus = "FAILED"
	StatusCancelled           RunStatus = "CANCELLED"
)

// BaseKind represents the type of base reference
type BaseKind string

const (
	BaseKindTag        BaseKind = "tag"
	BaseKindDAGVersion BaseKind = "dag_version"
	BaseKindPatchSet   BaseKind = "patch_set"
)

// Run represents a workflow run submission
// Maps to: run table (partitioned by submitted_at)
type Run struct {
	// Unique run ID (UUID v7)
	RunID uuid.UUID `db:"run_id" json:"run_id"`

	// Base reference type
	BaseKind BaseKind `db:"base_kind" json:"base_kind"`

	// Base reference value
	// - If base_kind='tag': tag name (e.g., 'main')
	// - If base_kind='dag_version': artifact_id::text
	// - If base_kind='patch_set': artifact_id::text
	BaseRef string `db:"base_ref" json:"base_ref"`

	// Optional run-specific patch (not shared)
	RunPatchID *uuid.UUID `db:"run_patch_id" json:"run_patch_id,omitempty"`

	// Snapshot of tag positions at submission time (JSONB)
	// Example: {"main": "V1", "exp/quality": "P5"}
	TagsSnapshot map[string]string `db:"tags_snapshot" json:"tags_snapshot"`

	// Run status
	Status RunStatus `db:"status" json:"status"`

	// Audit fields
	SubmittedBy *string   `db:"submitted_by" json:"submitted_by,omitempty"`
	SubmittedAt time.Time `db:"submitted_at" json:"submitted_at"`
}

// GetDefaultNodeStatus returns the expected node status based on run status
// This centralizes the logic for determining what status nodes should have
// when Redis context data is missing or expired
func (r *Run) GetDefaultNodeStatus() string {
	switch r.Status {
	case StatusCompleted:
		return "completed"
	case StatusFailed:
		return "failed"
	case StatusRunning:
		return "running"
	default:
		return "pending"
	}
}

// RunSnapshotIndex links runs to cached snapshots
// Maps to: run_snapshot_index table
type RunSnapshotIndex struct {
	// One-to-one with run
	RunID uuid.UUID `db:"run_id" json:"run_id"`

	// Reference to cached snapshot artifact
	SnapshotID uuid.UUID `db:"snapshot_id" json:"snapshot_id"`

	// Effective hash of the materialized graph
	VersionHash string `db:"version_hash" json:"version_hash"`

	// When snapshot was linked to this run
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
