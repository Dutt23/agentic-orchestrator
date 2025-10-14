-- Migration: Add node_id to run_patches table
-- Description: Track which workflow node generated each patch for analytics and debugging

-- Add node_id column to track patch origin
ALTER TABLE run_patches
ADD COLUMN node_id VARCHAR(255);

-- Add index for querying patches by node
CREATE INDEX IF NOT EXISTS idx_run_patches_node_id ON run_patches(node_id);

-- Add comment for documentation
COMMENT ON COLUMN run_patches.node_id IS 'ID of the workflow node that generated this patch (e.g., agent_node_1)';
