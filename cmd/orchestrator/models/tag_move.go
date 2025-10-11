package models

import (
	"time"

	"github.com/google/uuid"
)

// TagMove represents an audit log entry for tag movements (undo/redo history)
// Maps to: tag_move table
type TagMove struct {
	// Auto-incrementing ID
	ID int64 `db:"id" json:"id"`

	// Tag that was moved
	TagName string `db:"tag_name" json:"tag_name"`

	// Previous target
	FromKind *ArtifactKind `db:"from_kind" json:"from_kind,omitempty"`
	FromID   *uuid.UUID    `db:"from_id" json:"from_id,omitempty"`

	// New target
	ToKind ArtifactKind `db:"to_kind" json:"to_kind"`
	ToID   uuid.UUID    `db:"to_id" json:"to_id"`

	// Expected hash (for CAS validation)
	ExpectedHash *string `db:"expected_hash" json:"expected_hash,omitempty"`

	// Audit fields
	MovedBy *string   `db:"moved_by" json:"moved_by,omitempty"`
	MovedAt time.Time `db:"moved_at" json:"moved_at"`
}
