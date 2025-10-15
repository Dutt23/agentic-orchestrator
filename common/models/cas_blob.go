package models

import "time"

// CASBlob represents content-addressed storage for all artifacts
// Maps to: cas_blob table
type CASBlob struct {
	// Content hash (sha256:abc123...)
	CasID string `db:"cas_id" json:"cas_id"`

	// Media type with optional subtype
	// Examples:
	//   'application/json;type=dag'
	//   'application/json;type=patch_ops'
	//   'application/json;type=run_manifest'
	//   'application/json;type=run_snapshot'
	MediaType string `db:"media_type" json:"media_type"`

	// Blob size in bytes
	SizeBytes int64 `db:"size_bytes" json:"size_bytes"`

	// Creation timestamp
	CreatedAt time.Time `db:"created_at" json:"created_at"`

	// Inline storage (NULL if stored externally)
	Content []byte `db:"content" json:"content,omitempty"`

	// External storage URL (S3, MinIO, etc.)
	StorageURL *string `db:"storage_url" json:"storage_url,omitempty"`
}

// Media types for different artifact types
const (
	MediaTypeDAG         = "application/json;type=dag"
	MediaTypePatchOps    = "application/json;type=patch_ops"
	MediaTypeRunManifest = "application/json;type=run_manifest"
	MediaTypeRunSnapshot = "application/json;type=run_snapshot"
)