-- ============================================================================
-- Migration: Add Tag Namespacing
-- ============================================================================
-- Purpose: Enable multiple users to have tags with the same name
-- Strategy: Prefix existing tag names with owner's username
-- Example: "main" → "alice/main" or "_global_/main"
-- Date: 2025-10-12
-- ============================================================================
-- IMPORTANT: This migration modifies tag names but does NOT change schema.
--            The tag.tag_name column remains TEXT PRIMARY KEY.
-- ============================================================================

BEGIN;

-- ============================================================================
-- STEP 1: Backup existing tags (for rollback)
-- ============================================================================

CREATE TABLE IF NOT EXISTS tag_backup_20251012 AS
SELECT * FROM tag;

CREATE TABLE IF NOT EXISTS tag_move_backup_20251012 AS
SELECT * FROM tag_move;

COMMENT ON TABLE tag_backup_20251012 IS 'Backup before tag namespacing migration (2025-10-12)';
COMMENT ON TABLE tag_move_backup_20251012 IS 'Backup of tag_move before migration (2025-10-12)';

-- ============================================================================
-- STEP 2: Add username prefix to existing tags
-- ============================================================================

-- Update tags that have an owner (moved_by is not null)
-- Example: tag_name="main", moved_by="alice" → tag_name="alice/main"
UPDATE tag
SET tag_name = moved_by || '/' || tag_name
WHERE moved_by IS NOT NULL
  AND tag_name NOT LIKE '%/%'  -- Skip if already prefixed
  AND tag_name NOT LIKE '\_%';  -- Skip if starts with underscore

-- Update tags without owner (mark as global)
-- Example: tag_name="prod", moved_by=NULL → tag_name="_global_/prod"
UPDATE tag
SET tag_name = '_global_/' || tag_name
WHERE moved_by IS NULL
  AND tag_name NOT LIKE '%/%'  -- Skip if already prefixed
  AND tag_name NOT LIKE '\_%';  -- Skip if starts with underscore

-- ============================================================================
-- STEP 3: Update tag_move history to match new tag names
-- ============================================================================

-- Create temporary table with new tag names mapping
CREATE TEMPORARY TABLE tag_name_mapping AS
SELECT
    t_backup.tag_name as old_name,
    t.tag_name as new_name
FROM tag_backup_20251012 t_backup
INNER JOIN tag t ON t.target_id = t_backup.target_id
WHERE t_backup.tag_name != t.tag_name;

-- Update tag_move.tag_name to match new format
UPDATE tag_move tm
SET tag_name = (
    SELECT new_name FROM tag_name_mapping
    WHERE old_name = tm.tag_name
)
WHERE EXISTS (
    SELECT 1 FROM tag_name_mapping
    WHERE old_name = tm.tag_name
);

-- Note: from_id and to_id in tag_move remain unchanged (they reference artifact IDs, not tag names)

COMMIT;

-- ============================================================================
-- Verification Queries
-- ============================================================================

-- Show migration results
\echo ''
\echo 'Migration Results:'
\echo '==================='

-- Count tags by type
SELECT
    CASE
        WHEN tag_name LIKE '\\_global\\_/%' THEN 'Global Tags'
        WHEN tag_name LIKE '%/%' THEN 'User Tags'
        ELSE 'Legacy Tags (no prefix)'
    END as tag_type,
    COUNT(*) as count
FROM tag
GROUP BY 1
ORDER BY 1;

-- Show sample of migrated tags
\echo ''
\echo 'Sample Migrated Tags:'
SELECT
    t_backup.tag_name as old_name,
    t.tag_name as new_name,
    t.moved_by as owner
FROM tag_backup_20251012 t_backup
INNER JOIN tag t ON t.target_id = t_backup.target_id
WHERE t_backup.tag_name != t.tag_name
LIMIT 10;

-- Show any tags that failed migration (shouldn't be any)
\echo ''
\echo 'Tags that failed migration (should be empty):'
SELECT tag_name, moved_by
FROM tag
WHERE tag_name NOT LIKE '%/%'
  AND tag_name NOT LIKE '\\_global\\_/%'
LIMIT 10;

-- ============================================================================
-- Rollback Instructions (if needed)
-- ============================================================================

-- If migration fails or needs to be rolled back, run:
--
-- BEGIN;
--
-- -- Restore tag table
-- DROP TABLE tag;
-- ALTER TABLE tag_backup_20251012 RENAME TO tag;
--
-- -- Restore tag_move table
-- DROP TABLE tag_move;
-- ALTER TABLE tag_move_backup_20251012 RENAME TO tag_move;
--
-- COMMIT;
--
-- After successful rollback, verify:
-- SELECT COUNT(*) FROM tag;
-- SELECT COUNT(*) FROM tag_move;

-- ============================================================================
-- Cleanup (after verifying migration success - run manually)
-- ============================================================================

-- After 1-2 weeks of successful operation, clean up backup tables:
--
-- DROP TABLE IF EXISTS tag_backup_20251012;
-- DROP TABLE IF EXISTS tag_move_backup_20251012;

-- ============================================================================
-- Post-Migration Notes
-- ============================================================================

-- 1. API Changes Required:
--    - Update handlers to use TagService.CreateTagWithNamespace()
--    - Update handlers to extract X-User-ID header
--    - Update responses to strip username prefix for display
--
-- 2. User Impact:
--    - Users must specify X-User-ID header in API requests
--    - Tag names in responses remain unchanged (prefix stripped)
--    - Multiple users can now create tags with same name
--
-- 3. Global Tags:
--    - Prefixed with "_global_/"
--    - Accessible to all users (read-only unless admin)
--    - Use for system-wide tags like "prod", "staging"
--
-- 4. Query Patterns:
--    - Find user's tags: WHERE tag_name LIKE 'alice/%'
--    - Find global tags: WHERE tag_name LIKE '_global_/%'
--    - Get specific tag: WHERE tag_name = 'alice/main'

-- ============================================================================
-- END OF MIGRATION
-- ============================================================================
