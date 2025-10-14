-- Migration: Add run_patches table for tracking workflow patches applied during execution
-- Run-specific patches are separate from the workflow's main patch chain

-- Create run_patches table
CREATE TABLE IF NOT EXISTS run_patches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id VARCHAR(255) NOT NULL,
    artifact_id UUID NOT NULL REFERENCES artifact(artifact_id) ON DELETE CASCADE,
    seq INTEGER NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(255),

    -- Ensure patches are applied in order for each run
    UNIQUE(run_id, seq)
);

-- Index for querying patches by run_id
CREATE INDEX IF NOT EXISTS idx_run_patches_run_id ON run_patches(run_id);

-- Index for querying patches with artifact details
CREATE INDEX IF NOT EXISTS idx_run_patches_artifact_id ON run_patches(artifact_id);

-- Index for ordering patches by sequence
CREATE INDEX IF NOT EXISTS idx_run_patches_run_seq ON run_patches(run_id, seq);

-- Comments
COMMENT ON TABLE run_patches IS 'Tracks workflow patches applied during specific workflow runs (separate from main patch chain)';
COMMENT ON COLUMN run_patches.run_id IS 'Workflow run identifier';
COMMENT ON COLUMN run_patches.artifact_id IS 'Reference to the patch artifact';
COMMENT ON COLUMN run_patches.seq IS 'Sequence number for patch application order (1, 2, 3...)';
COMMENT ON COLUMN run_patches.description IS 'Description of the patch (e.g., "Agent added branch node")';
COMMENT ON COLUMN run_patches.created_by IS 'Username or agent that created the patch';
