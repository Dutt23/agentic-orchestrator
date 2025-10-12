-- ============================================================================
-- Migration: Add compacted_from_id column for efficient compaction lookups
-- ============================================================================
-- Purpose: Replace JSONB lookup (meta->>'compacted_from') with indexed column
-- Performance: O(n) JSONB scan → O(log n) B-tree index lookup
-- Date: 2025-10-12
-- ============================================================================

BEGIN;

-- Add compacted_from_id column
ALTER TABLE artifact
ADD COLUMN compacted_from_id UUID REFERENCES artifact(artifact_id) ON DELETE RESTRICT;

COMMENT ON COLUMN artifact.compacted_from_id IS 'Original patch ID that was compacted into this base version';

-- Create index for fast reverse lookups (find compacted versions)
CREATE INDEX idx_artifact_compacted_from
    ON artifact(compacted_from_id)
    WHERE compacted_from_id IS NOT NULL;

-- Optional: Migrate existing data from JSONB meta to column
-- This is safe because meta->>'compacted_from' would be a UUID string
UPDATE artifact
SET compacted_from_id = (meta->>'compacted_from')::uuid
WHERE kind = 'dag_version'
  AND meta ? 'compacted_from'
  AND meta->>'compacted_from' ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$';

-- Optional: Remove redundant JSONB field after migration
-- Uncomment if you want to clean up:
-- UPDATE artifact
-- SET meta = meta - 'compacted_from'
-- WHERE kind = 'dag_version' AND meta ? 'compacted_from';

COMMIT;

-- ============================================================================
-- Verification Queries
-- ============================================================================

-- Check migration success
SELECT
    COUNT(*) FILTER (WHERE compacted_from_id IS NOT NULL) as with_compacted_from,
    COUNT(*) FILTER (WHERE kind = 'dag_version' AND meta ? 'compacted_from') as in_jsonb
FROM artifact;

-- Show indexed compaction relationships
SELECT
    a.artifact_id as compacted_version,
    a.compacted_from_id as original_patch,
    a.created_at,
    a.meta->>'original_depth' as original_depth
FROM artifact a
WHERE a.compacted_from_id IS NOT NULL
ORDER BY a.created_at DESC
LIMIT 10;
