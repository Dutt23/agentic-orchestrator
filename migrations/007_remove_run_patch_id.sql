-- ============================================================================
-- Remove run_patch_id from run table
-- ============================================================================
-- Version: 1.0
-- Description: Remove unused run_patch_id column - run patches now use
--              run_patches table with one-to-many relationship
-- ============================================================================

-- Drop the foreign key constraint first
ALTER TABLE run DROP CONSTRAINT IF EXISTS run_run_patch_id_fkey;

-- Drop the column
ALTER TABLE run DROP COLUMN IF EXISTS run_patch_id;

COMMENT ON TABLE run IS 'Workflow run submissions (partitioned by time). Run patches stored in run_patches table.';
