-- ============================================================================
-- Agentic Orchestration Platform - Final Schema
-- ============================================================================
-- Version: 1.0
-- Description: Content-addressable storage with Git-like versioning for DAGs
-- Features:
--   - Immutable artifacts (DAG versions, patches, snapshots)
--   - Tag-based branching (main, prod, exp/*)
--   - O(1) patch chain resolution
--   - Snapshot caching by plan hash
--   - Undo/redo via tag moves
--   - Bounded write amplification
-- ============================================================================

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS pgcrypto;  -- For gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;  -- For query performance monitoring

-- ============================================================================
-- UUID v7 GENERATION (Time-Ordered UUIDs)
-- ============================================================================
-- UUID v7 format: xxxxxxxx-xxxx-7xxx-xxxx-xxxxxxxxxxxx
-- First 48 bits: Unix timestamp in milliseconds
-- Provides time-ordered UUIDs for better index performance and sorting
-- ============================================================================

CREATE OR REPLACE FUNCTION uuid_generate_v7()
RETURNS UUID
AS $$
DECLARE
  unix_ts_ms BIGINT;
  uuid_bytes BYTEA;
BEGIN
  -- Get Unix timestamp in milliseconds (48 bits)
  unix_ts_ms := (EXTRACT(EPOCH FROM clock_timestamp()) * 1000)::BIGINT;

  -- Generate random bytes for the rest
  uuid_bytes := gen_random_bytes(16);

  -- Set timestamp in first 6 bytes (48 bits)
  uuid_bytes := SET_BYTE(uuid_bytes, 0, (unix_ts_ms >> 40)::INT);
  uuid_bytes := SET_BYTE(uuid_bytes, 1, (unix_ts_ms >> 32)::INT);
  uuid_bytes := SET_BYTE(uuid_bytes, 2, (unix_ts_ms >> 24)::INT);
  uuid_bytes := SET_BYTE(uuid_bytes, 3, (unix_ts_ms >> 16)::INT);
  uuid_bytes := SET_BYTE(uuid_bytes, 4, (unix_ts_ms >> 8)::INT);
  uuid_bytes := SET_BYTE(uuid_bytes, 5, unix_ts_ms::INT);

  -- Set version to 7 (0111 in bits 12-15 of time_hi_and_version)
  uuid_bytes := SET_BYTE(uuid_bytes, 6, (GET_BYTE(uuid_bytes, 6) & 15) | 112);

  -- Set variant to RFC 4122 (10 in bits 6-7 of clock_seq_hi_and_reserved)
  uuid_bytes := SET_BYTE(uuid_bytes, 8, (GET_BYTE(uuid_bytes, 8) & 63) | 128);

  RETURN encode(uuid_bytes, 'hex')::UUID;
END
$$ LANGUAGE plpgsql VOLATILE;

COMMENT ON FUNCTION uuid_generate_v7() IS 'Generate time-ordered UUID v7 (better for indexes and sorting)';

-- ============================================================================
-- 1. CAS BLOB (Content-Addressed Storage)
-- ============================================================================

CREATE TABLE cas_blob (
    -- Content hash (sha256, sha3, blake3, etc.)
    cas_id TEXT PRIMARY KEY,

    -- Media type with optional subtype
    -- Examples:
    --   'application/json;type=dag'
    --   'application/json;type=patch_ops'
    --   'application/json;type=run_manifest'
    --   'application/json;type=run_snapshot'
    media_type TEXT NOT NULL,

    -- Blob size in bytes
    size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),

    -- Creation timestamp
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Inline storage (NULL if stored externally)
    content BYTEA,

    -- External storage URL (S3, MinIO, etc.)
    storage_url TEXT,

    -- Either inline or external, not both
    CONSTRAINT cas_blob_storage_check
        CHECK ((content IS NOT NULL) <> (storage_url IS NOT NULL))
);

-- Index for cleanup queries
CREATE INDEX idx_cas_blob_media_created
    ON cas_blob(media_type, created_at DESC);

-- Index for size-based queries (monitoring)
CREATE INDEX idx_cas_blob_size
    ON cas_blob(size_bytes DESC)
    WHERE size_bytes > 1048576; -- Only large blobs (>1MB)

COMMENT ON TABLE cas_blob IS 'Content-addressed storage for all artifacts';
COMMENT ON COLUMN cas_blob.cas_id IS 'Content hash (e.g., sha256:abc123...)';
COMMENT ON COLUMN cas_blob.content IS 'Inline storage for small blobs (<1MB)';
COMMENT ON COLUMN cas_blob.storage_url IS 'External storage URL (S3, MinIO)';

-- ============================================================================
-- 2. ARTIFACT (Catalog of Logical Objects)
-- ============================================================================

CREATE TABLE artifact (
    -- Unique artifact ID (time-ordered for better index performance)
    artifact_id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),

    -- Artifact type
    -- Values: 'dag_version', 'patch_set', 'run_manifest', 'run_snapshot'
    kind TEXT NOT NULL CHECK (kind IN (
        'dag_version',
        'patch_set',
        'run_manifest',
        'run_snapshot'
    )),

    -- Reference to CAS blob
    cas_id TEXT NOT NULL REFERENCES cas_blob(cas_id) ON DELETE RESTRICT,

    -- Optional human-readable name
    name TEXT,

    -- ========================================================================
    -- EXTRACTED COLUMNS (formerly in JSONB meta)
    -- ========================================================================

    -- For snapshot cache lookups (run_snapshot)
    plan_hash TEXT,

    -- For integrity verification (dag_version, run_snapshot)
    version_hash TEXT,

    -- For patch chain traversal (patch_set)
    base_version UUID REFERENCES artifact(artifact_id) ON DELETE RESTRICT,

    -- Chain depth from base (patch_set)
    -- depth=1: first patch on a version
    -- depth=N: Nth patch in chain
    depth INT CHECK (depth >= 0),

    -- Operation count (patch_set)
    op_count INT CHECK (op_count >= 0),

    -- Node/edge counts (dag_version, run_snapshot)
    nodes_count INT CHECK (nodes_count >= 0),
    edges_count INT CHECK (edges_count >= 0),

    -- ========================================================================
    -- FLEXIBLE METADATA (rarely queried, extensions)
    -- ========================================================================

    -- Remaining flexible metadata
    -- Examples:
    --   {"author": "user@example.com", "message": "Add retry logic"}
    --   {"materializer_version": "1.0.0"}
    --   {"compacted_from": ["P1", "P2", "P3"]}
    meta JSONB NOT NULL DEFAULT '{}'::jsonb,

    -- Audit fields
    created_by TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Constraints based on kind
    CONSTRAINT artifact_plan_hash_check
        CHECK (kind != 'run_snapshot' OR plan_hash IS NOT NULL),

    CONSTRAINT artifact_version_hash_check
        CHECK (kind NOT IN ('dag_version', 'run_snapshot') OR version_hash IS NOT NULL),

    CONSTRAINT artifact_patch_depth_check
        CHECK (kind != 'patch_set' OR (base_version IS NOT NULL AND depth IS NOT NULL))
);

-- Primary indexes
CREATE INDEX idx_artifact_kind ON artifact(kind);
CREATE INDEX idx_artifact_created ON artifact(created_at DESC);
CREATE INDEX idx_artifact_cas ON artifact(cas_id);

-- Snapshot cache lookup (CRITICAL - hottest query)
CREATE UNIQUE INDEX idx_artifact_plan_hash
    ON artifact(plan_hash)
    WHERE kind = 'run_snapshot' AND plan_hash IS NOT NULL;

-- Version hash lookup
CREATE INDEX idx_artifact_version_hash
    ON artifact(version_hash)
    WHERE version_hash IS NOT NULL;

-- Patch chain queries
CREATE INDEX idx_artifact_base_version
    ON artifact(base_version, depth)
    WHERE kind = 'patch_set';

-- Compaction queries (find deep chains)
CREATE INDEX idx_artifact_depth
    ON artifact(depth DESC)
    WHERE kind = 'patch_set' AND depth > 10;

-- Flexible metadata (GIN index for rare queries)
CREATE INDEX idx_artifact_meta_gin
    ON artifact USING GIN(meta jsonb_path_ops);

COMMENT ON TABLE artifact IS 'Catalog of all logical artifacts';
COMMENT ON COLUMN artifact.plan_hash IS 'Snapshot cache key (run_snapshot only)';
COMMENT ON COLUMN artifact.version_hash IS 'Integrity hash (dag_version, run_snapshot)';
COMMENT ON COLUMN artifact.base_version IS 'Parent DAG version for patches';
COMMENT ON COLUMN artifact.depth IS 'Chain depth from base version';

-- ============================================================================
-- 3. TAG (Mutable Branch/Release Pointers)
-- ============================================================================

CREATE TABLE tag (
    -- Tag name (branch or release)
    -- Examples: 'main', 'prod', 'exp/quality', 'release/v1.0'
    tag_name TEXT PRIMARY KEY,

    -- Target artifact type
    target_kind TEXT NOT NULL CHECK (target_kind IN ('dag_version', 'patch_set')),

    -- Target artifact ID
    target_id UUID NOT NULL REFERENCES artifact(artifact_id) ON DELETE RESTRICT,

    -- Optional: target's version/content hash for optimistic locking
    target_hash TEXT,

    -- Optimistic locking version (for CAS updates)
    version BIGINT NOT NULL DEFAULT 1,

    -- Audit fields
    moved_by TEXT,
    moved_at TIMESTAMPTZ NOT NULL DEFAULT now()

    -- Note: target_kind validation enforced by trigger (see below)
    -- Cannot use CHECK constraint with subqueries in Postgres
);

-- Target lookups
CREATE INDEX idx_tag_target ON tag(target_kind, target_id);

-- Version for monitoring tag churn
CREATE INDEX idx_tag_version ON tag(version DESC);

COMMENT ON TABLE tag IS 'Mutable pointers to artifacts (Git-like branches)';
COMMENT ON COLUMN tag.version IS 'Optimistic locking version for CAS updates';
COMMENT ON COLUMN tag.target_hash IS 'Optional guard for compare-and-swap';

-- Trigger to enforce target_kind matches artifact.kind
CREATE OR REPLACE FUNCTION validate_tag_target_kind()
RETURNS TRIGGER AS $$
DECLARE
    actual_kind TEXT;
BEGIN
    -- Lookup actual kind of target artifact
    SELECT kind INTO actual_kind
    FROM artifact
    WHERE artifact_id = NEW.target_id;

    -- If artifact doesn't exist, fail
    IF NOT FOUND THEN
        RAISE EXCEPTION 'Tag target artifact % does not exist', NEW.target_id;
    END IF;

    -- If kind mismatch, fail
    IF actual_kind != NEW.target_kind THEN
        RAISE EXCEPTION 'Tag target_kind mismatch: expected %, got %', NEW.target_kind, actual_kind;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tag_validate_target_kind
    BEFORE INSERT OR UPDATE ON tag
    FOR EACH ROW
    EXECUTE FUNCTION validate_tag_target_kind();

-- ============================================================================
-- 4. TAG_MOVE (Audit Log for Tag History)
-- ============================================================================

CREATE TABLE tag_move (
    -- Auto-incrementing ID
    id BIGSERIAL PRIMARY KEY,

    -- Tag that was moved
    tag_name TEXT NOT NULL,

    -- Previous target
    from_kind TEXT,
    from_id UUID,

    -- New target
    to_kind TEXT NOT NULL,
    to_id UUID NOT NULL,

    -- Expected hash (for CAS validation)
    expected_hash TEXT,

    -- Audit fields
    moved_by TEXT,
    moved_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Query tag history
CREATE INDEX idx_tag_move_name_time ON tag_move(tag_name, moved_at DESC);

-- Find moves involving specific artifacts
CREATE INDEX idx_tag_move_to ON tag_move(to_id, to_kind);
CREATE INDEX idx_tag_move_from ON tag_move(from_id, from_kind)
    WHERE from_id IS NOT NULL;

COMMENT ON TABLE tag_move IS 'Audit log for tag movements (undo/redo history)';

-- ============================================================================
-- 5. PATCH_CHAIN_MEMBER (O(1) Chain Resolution)
-- ============================================================================

CREATE TABLE patch_chain_member (
    -- Head patch ID (the latest patch in chain)
    head_id UUID NOT NULL REFERENCES artifact(artifact_id) ON DELETE CASCADE,

    -- Application order (1 = first patch, N = last patch)
    seq INT NOT NULL CHECK (seq > 0),

    -- Member patch ID (can be same as head_id for last member)
    member_id UUID NOT NULL REFERENCES artifact(artifact_id) ON DELETE RESTRICT,

    -- Primary key: each head has unique sequence
    PRIMARY KEY (head_id, seq),

    -- Unique: no duplicate members in same chain
    UNIQUE (head_id, member_id)
);

-- Reverse lookup: find chains containing a specific patch
CREATE INDEX idx_patch_chain_member_member ON patch_chain_member(member_id);

-- Count members per head (for monitoring)
CREATE INDEX idx_patch_chain_member_head_count
    ON patch_chain_member(head_id, seq DESC);

COMMENT ON TABLE patch_chain_member IS 'Pre-materialized patch chains for O(1) resolution';
COMMENT ON COLUMN patch_chain_member.seq IS 'Application order (1-indexed)';

-- ============================================================================
-- 6. RUN (Workflow Submissions)
-- ============================================================================

CREATE TABLE run (
    -- Unique run ID (time-ordered for better index performance)
    run_id UUID DEFAULT uuid_generate_v7(),

    -- Base reference type
    base_kind TEXT NOT NULL CHECK (base_kind IN ('tag', 'dag_version', 'patch_set')),

    -- Base reference value
    -- - If base_kind='tag': tag name (e.g., 'main')
    -- - If base_kind='dag_version': artifact_id::text
    -- - If base_kind='patch_set': artifact_id::text
    base_ref TEXT NOT NULL,

    -- Optional run-specific patch (not shared)
    run_patch_id UUID REFERENCES artifact(artifact_id) ON DELETE RESTRICT,

    -- Snapshot of tag positions at submission time
    -- Example: {"main": "V1", "exp/quality": "P5"}
    tags_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,

    -- Run status
    status TEXT NOT NULL DEFAULT 'QUEUED' CHECK (status IN (
        'QUEUED',
        'RUNNING',
        'COMPLETED',
        'FAILED',
        'CANCELLED'
    )),

    -- Audit fields
    submitted_by TEXT,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Primary key must include partition key for partitioned tables
    PRIMARY KEY (run_id, submitted_at)
) PARTITION BY RANGE (submitted_at);

-- Create initial partitions (extend with pg_partman or cron job)
CREATE TABLE run_2024 PARTITION OF run
    FOR VALUES FROM ('2024-01-01') TO ('2025-01-01');

CREATE TABLE run_2025 PARTITION OF run
    FOR VALUES FROM ('2025-01-01') TO ('2026-01-01');

-- Indexes (on partitioned table)
CREATE INDEX idx_run_status_submitted ON run(status, submitted_at DESC);
CREATE INDEX idx_run_base ON run(base_kind, base_ref);
CREATE INDEX idx_run_submitted_by ON run(submitted_by, submitted_at DESC);

-- GIN index for tag snapshot queries
CREATE INDEX idx_run_tags_snapshot_gin ON run USING GIN(tags_snapshot jsonb_path_ops);

COMMENT ON TABLE run IS 'Workflow run submissions (partitioned by time)';
COMMENT ON COLUMN run.tags_snapshot IS 'Tag positions at submission time (for auditing)';

-- ============================================================================
-- 7. RUN_SNAPSHOT_INDEX (Run → Cached Snapshot)
-- ============================================================================

CREATE TABLE run_snapshot_index (
    -- One-to-one with run (must match run's composite PK)
    run_id UUID NOT NULL,
    run_submitted_at TIMESTAMPTZ NOT NULL,

    -- Reference to cached snapshot artifact
    snapshot_id UUID NOT NULL REFERENCES artifact(artifact_id) ON DELETE RESTRICT,

    -- Effective hash of the materialized graph
    version_hash TEXT NOT NULL,

    -- When snapshot was linked to this run
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Primary key and foreign key to run table
    PRIMARY KEY (run_id, run_submitted_at),
    FOREIGN KEY (run_id, run_submitted_at) REFERENCES run(run_id, submitted_at) ON DELETE CASCADE
);

-- Lookup by run_id (common query pattern)
CREATE INDEX idx_run_snapshot_index_run_id ON run_snapshot_index(run_id);

-- Reverse lookup: find runs using same snapshot
CREATE INDEX idx_run_snapshot_index_snapshot ON run_snapshot_index(snapshot_id);

-- Find runs by effective graph hash
CREATE INDEX idx_run_snapshot_index_hash ON run_snapshot_index(version_hash);

COMMENT ON TABLE run_snapshot_index IS 'Links runs to cached snapshots';
COMMENT ON COLUMN run_snapshot_index.version_hash IS 'Effective hash of materialized DAG';

-- ============================================================================
-- GRANTS (Adjust for your security model)
-- ============================================================================

-- Example: Read-only role for analytics
-- CREATE ROLE orchestrator_readonly;
-- GRANT SELECT ON ALL TABLES IN SCHEMA public TO orchestrator_readonly;

-- Example: Application role
-- CREATE ROLE orchestrator_app;
-- GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO orchestrator_app;
-- GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO orchestrator_app;

-- ============================================================================
-- STATISTICS (Help query planner)
-- ============================================================================

-- Increase statistics target for hot columns
ALTER TABLE artifact ALTER COLUMN kind SET STATISTICS 1000;
ALTER TABLE artifact ALTER COLUMN plan_hash SET STATISTICS 1000;
ALTER TABLE tag ALTER COLUMN tag_name SET STATISTICS 1000;

-- ============================================================================
-- END OF SCHEMA
-- ============================================================================
