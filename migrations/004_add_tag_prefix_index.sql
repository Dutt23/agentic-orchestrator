-- ============================================================================
-- Migration: Add Index for Tag Prefix Queries
-- ============================================================================
-- Purpose: Optimize tag listing queries that filter by username prefix
-- Strategy: Add B-tree index to support efficient LIKE 'prefix%' queries
-- Date: 2025-10-12
-- ============================================================================

BEGIN;

-- ============================================================================
-- Add index for prefix-based tag queries
-- ============================================================================
-- This index optimizes queries like:
--   SELECT * FROM tag WHERE tag_name LIKE 'alice/%'
--   SELECT * FROM tag WHERE tag_name LIKE '_global_/%'
--
-- B-tree indexes support prefix matching efficiently:
-- - Pattern 'alice/%' → Uses index (O(log n))
-- - Pattern '%/main' → Cannot use index (full scan)
--
-- Performance:
-- - Before: Full table scan O(n)
-- - After: Index scan O(log n) + small range scan
--
-- Note: The PRIMARY KEY index on tag_name already supports exact lookups.
--       This index is specifically for range queries with prefix patterns.

CREATE INDEX IF NOT EXISTS idx_tag_prefix
    ON tag(tag_name text_pattern_ops);

-- text_pattern_ops is important for LIKE queries with patterns:
-- - Regular B-tree: Good for equality and comparison
-- - text_pattern_ops: Optimized for LIKE 'prefix%' patterns

COMMENT ON INDEX idx_tag_prefix IS 'Optimizes tag listing by username prefix (LIKE queries)';

COMMIT;

-- ============================================================================
-- Verification Query
-- ============================================================================
-- Test the index is being used:
--
-- EXPLAIN SELECT * FROM tag WHERE tag_name LIKE 'alice/%';
-- → Should show: Index Scan using idx_tag_prefix
--
-- EXPLAIN SELECT * FROM tag WHERE tag_name = 'alice/main';
-- → Should show: Index Scan using tag_pkey (PRIMARY KEY)

-- ============================================================================
-- Performance Notes
-- ============================================================================
--
-- Query patterns and index usage:
--
-- ✅ Uses idx_tag_prefix (fast):
--   - WHERE tag_name LIKE 'alice/%'
--   - WHERE tag_name LIKE '_global_/%'
--   - WHERE tag_name LIKE 'user123/%'
--
-- ✅ Uses tag_pkey (fastest):
--   - WHERE tag_name = 'alice/main'
--   - WHERE tag_name IN ('alice/main', 'bob/feature')
--
-- ❌ Cannot use index (slow):
--   - WHERE tag_name LIKE '%/main'       -- Suffix match
--   - WHERE tag_name LIKE '%alice%'      -- Contains match
--
-- Index size:
-- - Small overhead (~10KB per 1000 tags)
-- - Worth it for multi-user systems with many tags

-- ============================================================================
-- Rollback (if needed)
-- ============================================================================
-- If you need to remove the index:
--
-- DROP INDEX IF EXISTS idx_tag_prefix;
