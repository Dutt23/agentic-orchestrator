# Database Schema

> **Postgres schema for durable workflow state and metadata**

## 📖 Document Overview

**Purpose:** Complete Postgres schema reference with examples and best practices

**In this document:**
- [Overview](#overview) - Design principles
- [Core Tables](#core-tables) - 7 essential tables
  - [runs](#1-runs---workflow-instances) - Workflow execution instances
  - [artifact](#2-artifact---workflow-versions-cas-metadata) - Workflow versions
  - [cas_blob](#3-cas_blob---content-addressed-storage-metadata) - CAS metadata
  - [tag](#4-tag---git-like-workflow-versioning) - Named pointers
  - [tag_move](#5-tag_move---tag-movement-history-undoredo) - Undo/redo history
  - [patch_chain_member](#6-patch_chain_member---patch-relationships) - Patch tracking
  - [patches](#7-patches---agentoptimizer-modifications) - Agent patches
- [Schema Migration](#schema-migration-strategy) - Versioning approach
- [Performance Tips](#performance-tips) - Optimization techniques

---

## Overview

The database stores durable workflow metadata, run information, patches, and artifacts. Redis handles hot-path execution state, while Postgres provides durability and queryability.

**Design Principle:** Keep writes append-only for audit trail and replay capability.

---

## Core Tables

### 1. runs - Workflow Instances

**Purpose:** Track workflow execution instances

```sql
CREATE TABLE runs (
    run_id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),

    -- Workflow reference
    workflow_id UUID NOT NULL,
    artifact_id UUID NOT NULL,        -- Workflow version used
    tag_name TEXT,                    -- Tag used to start (e.g., "main")

    -- Status
    status TEXT NOT NULL DEFAULT 'RUNNING',
    -- Values: RUNNING, COMPLETED, FAILED, CANCELLED, PAUSED

    -- Timestamps
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    ended_at TIMESTAMPTZ,

    -- Metadata
    submitted_by TEXT,                -- User who submitted
    inputs JSONB,                     -- Input parameters
    outputs JSONB,                    -- Final outputs

    -- Tracking
    ir_cas_ref TEXT,                  -- Compiled IR reference
    final_counter_value INT,          -- Token counter at completion

    -- Foreign keys
    FOREIGN KEY (artifact_id) REFERENCES artifact(artifact_id)
);

CREATE INDEX idx_runs_status ON runs(status);
CREATE INDEX idx_runs_workflow_id ON runs(workflow_id);
CREATE INDEX idx_runs_submitted_at ON runs(submitted_at DESC);
```

**Query examples:**
```sql
-- Get active runs
SELECT run_id, workflow_id, started_at
FROM runs
WHERE status = 'RUNNING';

-- Get runs for a workflow
SELECT * FROM runs
WHERE workflow_id = '...'
ORDER BY submitted_at DESC;
```

---

### 2. artifact - Workflow Versions (CAS Metadata)

**Purpose:** Track workflow versions and patches (content stored in CAS)

```sql
CREATE TABLE artifact (
    artifact_id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),

    -- Content reference
    cas_id TEXT NOT NULL,             -- sha256 hash of content

    -- Type
    kind TEXT NOT NULL,
    -- Values:
    --   'dag_version' - Base workflow version
    --   'patch_set' - Agent/optimizer patch
    --   'run_snapshot' - Run state snapshot

    -- Metadata
    version_hash TEXT,                -- Hash of (base + patches)
    media_type TEXT,                  -- 'application/json', etc.
    size_bytes BIGINT,

    -- Lineage
    base_artifact_id UUID,            -- Parent artifact (for patches)
    created_by TEXT,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    UNIQUE(cas_id, kind),
    FOREIGN KEY (base_artifact_id) REFERENCES artifact(artifact_id)
);

CREATE INDEX idx_artifact_cas_id ON artifact(cas_id);
CREATE INDEX idx_artifact_kind ON artifact(kind);
CREATE INDEX idx_artifact_version_hash ON artifact(version_hash);
```

**Query examples:**
```sql
-- Get workflow version
SELECT * FROM artifact
WHERE kind = 'dag_version'
  AND version_hash = '...';

-- Get patch chain
WITH RECURSIVE patch_chain AS (
    SELECT * FROM artifact WHERE artifact_id = '...'
    UNION ALL
    SELECT a.* FROM artifact a
    JOIN patch_chain pc ON a.artifact_id = pc.base_artifact_id
)
SELECT * FROM patch_chain;
```

---

### 3. cas_blob - Content-Addressed Storage Metadata

**Purpose:** Track CAS blob metadata (actual content in S3/MinIO)

```sql
CREATE TABLE cas_blob (
    cas_id TEXT PRIMARY KEY,          -- sha256 hash

    -- Storage
    size_bytes BIGINT NOT NULL,
    media_type TEXT NOT NULL,
    storage_backend TEXT DEFAULT 's3',  -- 's3', 'minio', 'local'

    -- Metadata
    compression TEXT,                 -- 'gzip', 'zstd', null
    encryption TEXT,                  -- 'aes256', null

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_accessed_at TIMESTAMPTZ,
    access_count BIGINT DEFAULT 0,

    -- Retention
    retain_until TIMESTAMPTZ,
    status TEXT DEFAULT 'active'      -- 'active', 'archived', 'deleted'
);

CREATE INDEX idx_cas_blob_created_at ON cas_blob(created_at);
CREATE INDEX idx_cas_blob_status ON cas_blob(status);
```

**Usage:**
```go
// Store content
hash := sha256(content)
db.Exec(`
    INSERT INTO cas_blob (cas_id, size_bytes, media_type)
    VALUES ($1, $2, $3)
    ON CONFLICT (cas_id) DO NOTHING
`, hash, len(content), "application/json")

// Access tracking
db.Exec(`
    UPDATE cas_blob
    SET last_accessed_at = NOW(), access_count = access_count + 1
    WHERE cas_id = $1
`, hash)
```

---

### 4. tag - Git-Like Workflow Versioning

**Purpose:** Named pointers to artifacts (like Git tags/branches)

```sql
CREATE TABLE tag (
    tag_name TEXT PRIMARY KEY,

    -- Target
    target_kind TEXT NOT NULL,        -- 'artifact', 'run'
    target_id UUID NOT NULL,

    -- Version tracking
    version_seq BIGINT NOT NULL DEFAULT 1,

    -- Metadata
    created_by TEXT,
    description TEXT,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tag_target ON tag(target_id);
CREATE INDEX idx_tag_kind ON tag(target_kind);
```

**Usage:**
```sql
-- Resolve tag
SELECT target_id FROM tag WHERE tag_name = 'main';

-- Move tag (like git tag -f)
UPDATE tag
SET target_id = '...', version_seq = version_seq + 1, updated_at = NOW()
WHERE tag_name = 'main';
```

---

### 5. tag_move - Tag Movement History (Undo/Redo)

**Purpose:** Track tag movements for undo/redo

```sql
CREATE TABLE tag_move (
    move_id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    tag_name TEXT NOT NULL,

    -- Movement
    from_artifact_id UUID,
    to_artifact_id UUID NOT NULL,

    -- Undo/Redo tracking
    move_seq BIGINT NOT NULL,         -- Sequential move number
    undone BOOLEAN DEFAULT FALSE,

    -- Metadata
    moved_by TEXT,
    reason TEXT,

    -- Timestamp
    moved_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    FOREIGN KEY (tag_name) REFERENCES tag(tag_name),
    FOREIGN KEY (from_artifact_id) REFERENCES artifact(artifact_id),
    FOREIGN KEY (to_artifact_id) REFERENCES artifact(artifact_id)
);

CREATE INDEX idx_tag_move_tag_name ON tag_move(tag_name, move_seq DESC);
```

**Usage:**
```sql
-- Get tag history
SELECT * FROM tag_move
WHERE tag_name = 'main'
ORDER BY move_seq DESC;

-- Undo last move
UPDATE tag
SET target_id = (
    SELECT from_artifact_id FROM tag_move
    WHERE tag_name = 'main' AND undone = FALSE
    ORDER BY move_seq DESC LIMIT 1
)
WHERE tag_name = 'main';

UPDATE tag_move
SET undone = TRUE
WHERE tag_name = 'main' AND move_seq = (
    SELECT MAX(move_seq) FROM tag_move WHERE tag_name = 'main'
);
```

---

### 6. patch_chain_member - Patch Relationships

**Purpose:** Track which patches apply to which workflows

```sql
CREATE TABLE patch_chain_member (
    member_id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),

    -- Chain
    chain_head_id UUID NOT NULL,      -- Head of patch chain
    patch_artifact_id UUID NOT NULL,  -- Individual patch

    -- Position
    seq_in_chain INT NOT NULL,        -- Order in chain

    -- Status
    applied BOOLEAN DEFAULT FALSE,

    -- Metadata
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    FOREIGN KEY (chain_head_id) REFERENCES artifact(artifact_id),
    FOREIGN KEY (patch_artifact_id) REFERENCES artifact(artifact_id),
    UNIQUE(chain_head_id, seq_in_chain)
);

CREATE INDEX idx_patch_chain_head ON patch_chain_member(chain_head_id, seq_in_chain);
```

**Usage:**
```sql
-- Get patches for a workflow
SELECT p.*, a.cas_id
FROM patch_chain_member p
JOIN artifact a ON p.patch_artifact_id = a.artifact_id
WHERE p.chain_head_id = '...'
ORDER BY p.seq_in_chain;
```

---

### 7. patches - Agent/Optimizer Modifications

**Purpose:** Store patch operations applied to workflows

```sql
CREATE TABLE patches (
    patch_id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),

    -- Associated run
    run_id UUID NOT NULL,

    -- Patch content
    patch_jsonb JSONB NOT NULL,       -- JSON Patch operations
    artifact_id UUID,                 -- Stored as artifact

    -- Status
    status TEXT DEFAULT 'pending',    -- 'pending', 'applied', 'rejected'

    -- Metadata
    created_by TEXT,                  -- 'agent', 'optimizer', 'user'
    justification TEXT,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    applied_at TIMESTAMPTZ,

    FOREIGN KEY (run_id) REFERENCES runs(run_id),
    FOREIGN KEY (artifact_id) REFERENCES artifact(artifact_id)
);

CREATE INDEX idx_patches_run_id ON patches(run_id);
CREATE INDEX idx_patches_status ON patches(status);
CREATE INDEX idx_patches_created_at ON patches(created_at DESC);
```

**Example patch:**
```json
{
  "patch_id": "patch_abc123",
  "run_id": "run_7f3e4a",
  "patch_jsonb": {
    "operations": [
      {"op": "add", "path": "/nodes/-", "value": {"id": "email", "type": "task"}},
      {"op": "add", "path": "/edges/-", "value": {"from": "process", "to": "email"}}
    ]
  },
  "created_by": "agent",
  "justification": "Add email notification for processed data",
  "status": "applied"
}
```

---

## Optional Tables (Future)

### approvals - HITL Approval Records

```sql
CREATE TABLE approvals (
    approval_id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    run_id UUID NOT NULL,
    node_id TEXT NOT NULL,

    -- Status
    status TEXT DEFAULT 'pending',    -- 'pending', 'approved', 'rejected', 'timeout'

    -- Request
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    timeout_at TIMESTAMPTZ,
    requester TEXT,

    -- Decision
    decision TEXT,                    -- 'approve', 'reject'
    decided_by TEXT,
    decided_at TIMESTAMPTZ,
    reason TEXT,

    FOREIGN KEY (run_id) REFERENCES runs(run_id)
);

CREATE INDEX idx_approvals_run_id ON approvals(run_id);
CREATE INDEX idx_approvals_status ON approvals(status) WHERE status = 'pending';
```

### event_log - Event Sourcing (Optional)

```sql
CREATE TABLE event_log (
    event_id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    run_id UUID NOT NULL,

    -- Event
    event_type TEXT NOT NULL,         -- 'node.started', 'node.completed', etc.
    sequence_num BIGINT NOT NULL,     -- Strict ordering per run

    -- Payload
    event_data JSONB NOT NULL,

    -- Timestamp
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (run_id, sequence_num),
    FOREIGN KEY (run_id) REFERENCES runs(run_id)
);

CREATE INDEX idx_event_log_run ON event_log(run_id, sequence_num);
CREATE INDEX idx_event_log_type ON event_log(event_type);
```

**Use case:** Full replay and audit trail

---

## Schema Migration Strategy

### Adding New Column (Backward Compatible)

```sql
-- Add optional column
ALTER TABLE runs ADD COLUMN cost_cents BIGINT DEFAULT 0;

-- Update existing rows (if needed)
UPDATE runs SET cost_cents = 0 WHERE cost_cents IS NULL;
```

### Adding New Table

```sql
-- Create new table
CREATE TABLE new_feature (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    run_id UUID NOT NULL,
    data JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    FOREIGN KEY (run_id) REFERENCES runs(run_id)
);
```

### Schema Versioning

**Approach:** Sequential migration files

```
migrations/
├── 001_initial_schema.sql
├── 002_add_patches_table.sql
├── 003_add_cas_blob_table.sql
└── 004_add_tag_move_history.sql
```

---

## Indexes Strategy

### Primary Indexes (Already Listed Above)

**Query patterns optimized:**
- List runs by status
- Find runs for workflow
- Get recent runs
- Resolve tags
- Get patches for run
- CAS blob lookup

### Monitoring Index Usage

```sql
-- Check index usage
SELECT schemaname, tablename, indexname, idx_scan
FROM pg_stat_user_indexes
WHERE schemaname = 'public'
ORDER BY idx_scan ASC;

-- Unused indexes (candidates for removal)
SELECT * FROM pg_stat_user_indexes
WHERE idx_scan = 0
  AND schemaname = 'public';
```

---

## Data Retention

### CAS Blob Cleanup

```sql
-- Mark blobs for deletion (30 days after last access)
UPDATE cas_blob
SET status = 'archived'
WHERE last_accessed_at < NOW() - INTERVAL '30 days'
  AND status = 'active';

-- Delete archived blobs (after grace period)
DELETE FROM cas_blob
WHERE status = 'archived'
  AND retain_until < NOW();
```

### Event Log Archival

```sql
-- Archive old runs (90 days)
DELETE FROM event_log
WHERE run_id IN (
    SELECT run_id FROM runs
    WHERE ended_at < NOW() - INTERVAL '90 days'
);
```

---

## Connection Pooling

**Configuration:**

```go
// pgx pool
config, _ := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
config.MaxConns = 100
config.MinConns = 10
config.MaxConnLifetime = time.Hour
config.MaxConnIdleTime = 30 * time.Minute
config.HealthCheckPeriod = time.Minute

pool, _ := pgxpool.ConnectConfig(ctx, config)
```

**Monitoring:**

```sql
-- Active connections
SELECT count(*) FROM pg_stat_activity;

-- Long-running queries
SELECT pid, now() - query_start AS duration, query
FROM pg_stat_activity
WHERE state = 'active'
  AND now() - query_start > interval '5 seconds';
```

---

## Performance Tips

### 1. Use JSONB (not JSON)

```sql
-- Faster queries, supports indexing
ALTER TABLE runs ALTER COLUMN inputs TYPE JSONB;

-- JSONB index (for specific field queries)
CREATE INDEX idx_runs_inputs_priority
ON runs USING GIN ((inputs->'priority'));
```

### 2. Partial Indexes

```sql
-- Only index pending approvals (smaller index)
CREATE INDEX idx_approvals_pending
ON approvals(created_at)
WHERE status = 'pending';
```

### 3. Avoid SELECT *

```sql
-- Bad
SELECT * FROM runs WHERE run_id = '...';

-- Good
SELECT run_id, status, started_at FROM runs WHERE run_id = '...';
```

---

## Complete Schema Documentation

**Detailed specs:**
- [../../docs/schema/DESIGN.md](../../docs/schema/DESIGN.md) - Full schema design
- [../../docs/schema/TABLE_RELATIONSHIPS.md](../../docs/schema/TABLE_RELATIONSHIPS.md) - Table relationships
- [../../docs/schema/OPERATIONS.md](../../docs/schema/OPERATIONS.md) - Common operations
- [../references/SCHEMA_SETUP.md](../references/SCHEMA_SETUP.md) - Setup instructions

---

**All tables designed for append-only operations, audit trails, and horizontal scaling.**
