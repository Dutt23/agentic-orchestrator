package models

import (
	"time"

	"github.com/google/uuid"
)

// WorkflowComponents holds all raw data needed to reconstruct a workflow
// This includes base DAG + optional patch chain
type WorkflowComponents struct {
	// Tag information
	TagName string `json:"tag_name"`

	// Artifact metadata
	ArtifactID uuid.UUID    `json:"artifact_id"`
	Kind       ArtifactKind `json:"kind"` // 'dag_version' or 'patch_set'

	// For patch_set only
	BaseVersion *uuid.UUID `json:"base_version,omitempty"`
	Depth       int        `json:"depth"`
	PatchCount  int        `json:"patch_count"`

	// Base DAG information
	BaseCASID      string `json:"base_cas_id"`
	BaseVersionHash string `json:"base_version_hash,omitempty"`

	// Base DAG content (always loaded)
	BaseContent []byte `json:"-"` // Don't serialize in JSON response by default

	// Patch chain (only for patch_set)
	PatchChain []PatchInfo `json:"patch_chain,omitempty"`

	// Timestamps
	CreatedAt time.Time  `json:"created_at"`
	CreatedBy *string    `json:"created_by,omitempty"`
}

// PatchInfo contains information about a single patch in the chain
type PatchInfo struct {
	Seq        int       `json:"seq"`         // Application order (1-indexed)
	ArtifactID uuid.UUID `json:"artifact_id"`
	CASID      string    `json:"cas_id"`
	OpCount    *int      `json:"op_count,omitempty"`
	Depth      int       `json:"depth"`

	// Patch operations content (loaded from CAS)
	Content []byte `json:"-"` // Don't serialize by default

	CreatedAt time.Time `json:"created_at"`
	CreatedBy *string   `json:"created_by,omitempty"`
}

// IsDAGVersion returns true if the workflow is a base DAG version
func (w *WorkflowComponents) IsDAGVersion() bool {
	return w.Kind == KindDAGVersion
}

// IsPatchSet returns true if the workflow is a patch set
func (w *WorkflowComponents) IsPatchSet() bool {
	return w.Kind == KindPatchSet
}

// RequiresMaterialization returns true if patches need to be applied
func (w *WorkflowComponents) RequiresMaterialization() bool {
	return w.IsPatchSet() && w.PatchCount > 0
}
