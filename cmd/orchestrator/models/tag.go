package models

import (
	"time"

	"github.com/google/uuid"
)

// Tag represents a mutable pointer to an artifact (Git-like branch)
// Maps to: tag table
type Tag struct {
	// Tag name (branch or release)
	// Examples: 'main', 'prod', 'exp/quality', 'release/v1.0'
	TagName string `db:"tag_name" json:"tag_name"`

	// Target artifact type
	TargetKind ArtifactKind `db:"target_kind" json:"target_kind"`

	// Target artifact ID
	TargetID uuid.UUID `db:"target_id" json:"target_id"`

	// Optional: target's version/content hash for optimistic locking
	TargetHash *string `db:"target_hash" json:"target_hash,omitempty"`

	// Optimistic locking version (for CAS updates)
	Version int64 `db:"version" json:"version"`

	// Audit fields
	MovedBy *string   `db:"moved_by" json:"moved_by,omitempty"`
	MovedAt time.Time `db:"moved_at" json:"moved_at"`
}
