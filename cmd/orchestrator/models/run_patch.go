package models

import (
	"time"

	"github.com/google/uuid"
)

// RunPatch represents a patch applied during a specific workflow run
// These are separate from the workflow's main patch chain
type RunPatch struct {
	ID          uuid.UUID  `db:"id" json:"id"`
	RunID       string     `db:"run_id" json:"run_id"`
	ArtifactID  uuid.UUID  `db:"artifact_id" json:"artifact_id"`
	Seq         int        `db:"seq" json:"seq"`
	NodeID      *string    `db:"node_id" json:"node_id,omitempty"` // Which node generated this patch
	Description *string    `db:"description" json:"description,omitempty"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	CreatedBy   *string    `db:"created_by" json:"created_by,omitempty"`
}

// RunPatchWithDetails includes artifact and CAS details
type RunPatchWithDetails struct {
	RunPatch
	CASID   string `json:"cas_id"`
	OpCount *int   `json:"op_count,omitempty"`
	Depth   int    `json:"depth"`
}
