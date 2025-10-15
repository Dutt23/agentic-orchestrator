package models

import "github.com/google/uuid"

// PatchChainMember represents a pre-materialized patch chain for O(1) resolution
// Maps to: patch_chain_member table
type PatchChainMember struct {
	// Head patch ID (the latest patch in chain)
	HeadID uuid.UUID `db:"head_id" json:"head_id"`

	// Application order (1 = first patch, N = last patch)
	Seq int `db:"seq" json:"seq"`

	// Member patch ID (can be same as head_id for last member)
	MemberID uuid.UUID `db:"member_id" json:"member_id"`
}
