-- ============================================================================
-- Migration 002: Separate Username Column for Proper User Isolation
-- ============================================================================
-- Purpose: Fix tag namespace security vulnerability
-- Problem: LIKE queries on composite tag_name (username/tagname) allow
--          username prefix collision (e.g., 'sdutt/' matches 'sdutt-1223/')
-- Solution: Store username in separate column for exact matching
-- ============================================================================

-- ============================================================================
-- 1. ADD USERNAME COLUMN TO TAG TABLE
-- ============================================================================

-- Add username column (defaults to 'sdutt' for existing rows)
ALTER TABLE tag
ADD COLUMN username TEXT NOT NULL DEFAULT 'sdutt';

-- Add created_by column for audit trail (tracks original creator)
ALTER TABLE tag
ADD COLUMN created_by TEXT NOT NULL DEFAULT 'sdutt';

-- ============================================================================
-- 2. MIGRATE EXISTING DATA
-- ============================================================================
-- Extract username from existing tag_name patterns:
-- - "username/tagname" → username='username', tag_name='tagname'
-- - "_global_/tagname" → username='_global_', tag_name='tagname'
-- - "tagname" (no prefix) → username='sdutt', tag_name='tagname'
-- ============================================================================

-- Update existing rows to extract username from tag_name
UPDATE tag
SET username = CASE
    WHEN tag_name LIKE '%/%' THEN split_part(tag_name, '/', 1)
    ELSE 'sdutt'
END,
tag_name = CASE
    WHEN tag_name LIKE '%/%' THEN split_part(tag_name, '/', 2)
    ELSE tag_name
END;

-- ============================================================================
-- 3. UPDATE TAG CONSTRAINTS
-- ============================================================================

-- Drop old primary key (tag_name only)
ALTER TABLE tag DROP CONSTRAINT tag_pkey;

-- Add new primary key (username + tag_name)
ALTER TABLE tag ADD PRIMARY KEY (username, tag_name);

-- Add index for efficient username lookups (exact match)
CREATE INDEX idx_tag_username ON tag(username, tag_name);

-- Add index for reverse lookups (find all tags pointing to an artifact)
DROP INDEX IF EXISTS idx_tag_target;
CREATE INDEX idx_tag_target ON tag(target_kind, target_id);

COMMENT ON COLUMN tag.username IS 'Tag owner (user namespace) - exact match for user isolation';
COMMENT ON COLUMN tag.tag_name IS 'Tag name within user namespace (e.g., "main", "dev")';
COMMENT ON COLUMN tag.created_by IS 'User who originally created this tag';

-- ============================================================================
-- 4. UPDATE TAG_MOVE TABLE (Audit Log)
-- ============================================================================

-- Add username column to tag_move for proper history tracking
ALTER TABLE tag_move
ADD COLUMN username TEXT NOT NULL DEFAULT 'sdutt';

-- Update existing rows to extract username from tag_name
UPDATE tag_move
SET username = CASE
    WHEN tag_name LIKE '%/%' THEN split_part(tag_name, '/', 1)
    ELSE 'sdutt'
END,
tag_name = CASE
    WHEN tag_name LIKE '%/%' THEN split_part(tag_name, '/', 2)
    ELSE tag_name
END;

-- Add composite index for tag history queries
DROP INDEX IF EXISTS idx_tag_move_name_time;
CREATE INDEX idx_tag_move_name_time ON tag_move(username, tag_name, moved_at DESC);

COMMENT ON COLUMN tag_move.username IS 'Tag owner (matches tag.username)';

-- ============================================================================
-- 5. UPDATE ARTIFACT TABLE (Make created_by NOT NULL)
-- ============================================================================

-- Set default for existing NULL values
UPDATE artifact
SET created_by = 'sdutt'
WHERE created_by IS NULL;

-- Make created_by NOT NULL
ALTER TABLE artifact
ALTER COLUMN created_by SET NOT NULL;

-- Set default for future inserts
ALTER TABLE artifact
ALTER COLUMN created_by SET DEFAULT 'sdutt';

COMMENT ON COLUMN artifact.created_by IS 'User who created this artifact (required for ownership tracking)';

-- ============================================================================
-- 6. UPDATE TAG VALIDATION TRIGGER
-- ============================================================================

-- Update the validate_tag_target_kind trigger to handle new schema
-- (No changes needed - trigger logic remains the same)

-- ============================================================================
-- 7. UPDATE STATISTICS
-- ============================================================================

-- Help query planner with new username column
ALTER TABLE tag ALTER COLUMN username SET STATISTICS 1000;
ALTER TABLE tag ALTER COLUMN tag_name SET STATISTICS 1000;

-- ============================================================================
-- VERIFICATION QUERIES (for testing)
-- ============================================================================

-- Verify all tags have proper username
-- SELECT username, tag_name, target_kind, target_id FROM tag ORDER BY username, tag_name;

-- Verify no NULL created_by in artifacts
-- SELECT COUNT(*) FROM artifact WHERE created_by IS NULL;  -- Should be 0

-- Verify tag uniqueness (should return no rows)
-- SELECT username, tag_name, COUNT(*) FROM tag GROUP BY username, tag_name HAVING COUNT(*) > 1;

-- ============================================================================
-- END OF MIGRATION
-- ============================================================================
