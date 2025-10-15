# Schema Operations Guide

Complete guide for all database operations in the Agentic Orchestration Platform.

---

## Table of Contents

1. [Create Base DAG Version](#1-create-base-dag-version)
2. [Create Patch](#2-create-patch)
3. [Undo/Redo](#3-undoredo)
4. [Submit Run](#4-submit-run)
5. [Materialization](#5-materialization)
6. [Compaction](#6-compaction)
7. [Garbage Collection](#7-garbage-collection)
8. [Query Patterns](#8-query-patterns)

---

## 1. Create Base DAG Version

**Scenario:** Create the initial workflow definition (V1).

### Steps:

```sql
-- 1. Store content in CAS
INSERT INTO cas_blob (cas_id, media_type, size_bytes, content)
VALUES (
    'sha256:abc123...',  -- Compute from JSON
    'application/json;type=dag',
    12345,
    '{"nodes": [...], "edges": [...]}'::bytea
)
ON CONFLICT (cas_id) DO NOTHING;  -- Deduplication

-- 2. Create artifact
INSERT INTO artifact (
    artifact_id,
    kind,
    cas_id,
    name,
    version_hash,
    nodes_count,
    edges_count,
    created_by
)
VALUES (
    gen_random_uuid(),
    'dag_version',
    'sha256:abc123...',
    'Lead Processing v1.0',
    'vh:def456...',  -- Version hash
    10,  -- Node count
    12,  -- Edge count
    'user@example.com'
)
RETURNING artifact_id;

-- Store returned ID as V1_id

-- 3. Create or update tag 'main'
INSERT INTO tag (tag_name, target_kind, target_id, target_hash, moved_by)
VALUES ('main', 'dag_version', :V1_id, 'vh:def456...', 'user@example.com')
ON CONFLICT (tag_name) DO UPDATE SET
    target_kind = EXCLUDED.target_kind,
    target_id = EXCLUDED.target_id,
    target_hash = EXCLUDED.target_hash,
    version = tag.version + 1,
    moved_by = EXCLUDED.moved_by,
    moved_at = now();

-- 4. (Optional) Record tag move
INSERT INTO tag_move (tag_name, from_kind, from_id, to_kind, to_id, moved_by)
SELECT
    'main',
    t.target_kind,
    t.target_id,
    'dag_version',
    :V1_id,
    'user@example.com'
FROM tag t
WHERE t.tag_name = 'main';
```

---

## 2. Create Patch

**Scenario:** User edits a workflow; create patch P_k on branch `exp/quality`.

### Algorithm:

```sql
BEGIN;

-- 1. Lock parent artifact (serialization point)
SELECT artifact_id, kind, base_version, depth, version_hash
FROM artifact
WHERE artifact_id = :parent_id
FOR UPDATE;

-- Store result as parent_artifact

-- 2. Compute patch metadata
-- If parent is dag_version:
--   base_version = parent_id
--   depth = 1
-- If parent is patch_set:
--   base_version = parent.base_version
--   depth = parent.depth + 1

-- 3. Store patch ops in CAS
INSERT INTO cas_blob (cas_id, media_type, size_bytes, content)
VALUES (
    'sha256:patch_xyz...',
    'application/json;type=patch_ops',
    2048,
    '[{"op":"add", "path":"/nodes/-", "value":{...}}]'::bytea
);

-- 4. Create patch artifact
INSERT INTO artifact (
    artifact_id,
    kind,
    cas_id,
    name,
    base_version,
    depth,
    op_count,
    created_by
)
VALUES (
    gen_random_uuid(),
    'patch_set',
    'sha256:patch_xyz...',
    'Add retry logic',
    :base_version,  -- From step 2
    :depth,         -- From step 2
    3,              -- Operation count
    'user@example.com'
)
RETURNING artifact_id;

-- Store returned ID as new_patch_id

-- 5. Populate patch_chain_member
-- If parent was dag_version: start new chain
IF parent_kind = 'dag_version' THEN
    INSERT INTO patch_chain_member (head_id, seq, member_id)
    VALUES (:new_patch_id, 1, :new_patch_id);

-- If parent was patch_set: copy chain + append
ELSE
    -- Copy parent's chain
    INSERT INTO patch_chain_member (head_id, seq, member_id)
    SELECT :new_patch_id, seq, member_id
    FROM patch_chain_member
    WHERE head_id = :parent_id;

    -- Append new patch
    INSERT INTO patch_chain_member (head_id, seq, member_id)
    VALUES (
        :new_patch_id,
        (SELECT MAX(seq) + 1 FROM patch_chain_member WHERE head_id = :new_patch_id),
        :new_patch_id
    );
END IF;

-- 6. Move tag (with optimistic locking)
UPDATE tag
SET target_kind = 'patch_set',
    target_id = :new_patch_id,
    version = version + 1,
    moved_by = 'user@example.com',
    moved_at = now()
WHERE tag_name = 'exp/quality'
  AND target_id = :parent_id  -- CAS guard
  AND version = :expected_version;

-- Check: GET DIAGNOSTICS rows_updated = ROW_COUNT;
-- If rows_updated = 0: ROLLBACK; RAISE 'Conflict: tag was moved by another user';

-- 7. Record tag move
INSERT INTO tag_move (tag_name, from_kind, from_id, to_kind, to_id, moved_by)
VALUES ('exp/quality', :parent_kind, :parent_id, 'patch_set', :new_patch_id, 'user@example.com');

COMMIT;
```

---

## 3. Undo/Redo

**Scenario:** User wants to undo last patch on `exp/quality`.

### Undo (Move Tag Backward):

```sql
BEGIN;

-- 1. Get current tag position
SELECT target_id, target_kind, version
FROM tag
WHERE tag_name = 'exp/quality'
FOR UPDATE;

-- Store as current_id, current_kind, current_version

-- 2. Find previous position from tag_move history
SELECT from_id, from_kind
FROM tag_move
WHERE tag_name = 'exp/quality'
  AND to_id = :current_id
ORDER BY moved_at DESC
LIMIT 1;

-- Store as previous_id, previous_kind

-- 3. Move tag back
UPDATE tag
SET target_id = :previous_id,
    target_kind = :previous_kind,
    version = version + 1,
    moved_by = 'user@example.com (undo)',
    moved_at = now()
WHERE tag_name = 'exp/quality'
  AND version = :current_version;

-- 4. Record move
INSERT INTO tag_move (tag_name, from_kind, from_id, to_kind, to_id, moved_by)
VALUES ('exp/quality', :current_kind, :current_id, :previous_kind, :previous_id, 'user@example.com (undo)');

COMMIT;
```

### Redo (Move Tag Forward):

```sql
-- Similar to undo, but query next position:
SELECT to_id, to_kind
FROM tag_move
WHERE tag_name = 'exp/quality'
  AND from_id = :current_id
ORDER BY moved_at ASC
LIMIT 1;
```

---

## 4. Submit Run

**Scenario:** User submits a workflow run using tag `main`.

### Steps:

```sql
BEGIN;

-- 1. Resolve tag to artifact
SELECT target_kind, target_id, version_hash
FROM tag
WHERE tag_name = 'main';

-- Store as base_kind, base_id, base_hash

-- 2. If patch_set, get chain
IF base_kind = 'patch_set' THEN
    SELECT base_version, depth
    FROM artifact
    WHERE artifact_id = :base_id;

    SELECT member_id
    FROM patch_chain_member
    WHERE head_id = :base_id
    ORDER BY seq ASC;
    -- Store as chain_members[]
END IF;

-- 3. Compute plan hash
-- plan_hash = sha256(base_version + '\n' + join(chain_members, '\n') + '\n' + materializer_version + '\n' + options)
-- Example: 'sha256:plan_abc123...'

-- 4. Check snapshot cache
SELECT artifact_id, version_hash
FROM artifact
WHERE kind = 'run_snapshot'
  AND plan_hash = :computed_plan_hash
LIMIT 1;

-- If found: cache_hit = TRUE, snapshot_id = artifact_id
-- If not found: cache_hit = FALSE

-- 5. Create run manifest (small)
INSERT INTO cas_blob (cas_id, media_type, size_bytes, content)
VALUES (
    'sha256:manifest_xyz...',
    'application/json;type=run_manifest',
    512,
    jsonb_build_object(
        'base', :base_id,
        'chain', :chain_members,
        'plan_hash', :computed_plan_hash,
        'materializer_version', '1.0.0'
    )::text::bytea
);

INSERT INTO artifact (artifact_id, kind, cas_id, plan_hash, created_by)
VALUES (
    gen_random_uuid(),
    'run_manifest',
    'sha256:manifest_xyz...',
    :computed_plan_hash,
    'user@example.com'
);

-- 6. Create run record
INSERT INTO run (
    run_id,
    base_kind,
    base_ref,
    tags_snapshot,
    submitted_by,
    status
)
VALUES (
    gen_random_uuid(),
    'tag',
    'main',
    jsonb_build_object('main', :base_hash),
    'user@example.com',
    'QUEUED'
)
RETURNING run_id;

-- Store as new_run_id

-- 7. If cache hit, link snapshot immediately
IF cache_hit THEN
    INSERT INTO run_snapshot_index (run_id, snapshot_id, version_hash)
    VALUES (:new_run_id, :snapshot_id, :snapshot_version_hash);
ELSE
    -- Enqueue materialization job (external queue/service)
    -- Pass: run_id, base_id, chain_members[], plan_hash
END IF;

COMMIT;
```

---

## 5. Materialization

**Scenario:** Cache miss; materialize full DAG from base + patches.

### Algorithm (in application code):

```sql
-- 1. Load base DAG
SELECT content
FROM cas_blob
WHERE cas_id = (
    SELECT cas_id FROM artifact WHERE artifact_id = :base_id
);

-- Parse JSON into memory as dag_json

-- 2. Load and apply patches in order
FOR EACH member_id IN chain_members ORDER BY seq:
    SELECT content
    FROM cas_blob
    WHERE cas_id = (
        SELECT cas_id FROM artifact WHERE artifact_id = member_id
    );

    -- Parse patch ops JSON
    -- Apply each operation to dag_json (JSONPatch library)
END FOR;

-- 3. Canonicalize result
-- Sort nodes by ID, edges by (from,to), etc.
-- Compute effective_hash = sha256(canonical_json)

-- 4. Store snapshot
INSERT INTO cas_blob (cas_id, media_type, size_bytes, content)
VALUES (
    :effective_hash,
    'application/json;type=run_snapshot',
    length(dag_json),
    dag_json::bytea
);

INSERT INTO artifact (
    artifact_id,
    kind,
    cas_id,
    plan_hash,
    version_hash,
    nodes_count,
    edges_count
)
VALUES (
    gen_random_uuid(),
    'run_snapshot',
    :effective_hash,
    :plan_hash,
    :effective_hash,
    count_nodes(dag_json),
    count_edges(dag_json)
)
RETURNING artifact_id;

-- Store as snapshot_id

-- 5. Link to run
INSERT INTO run_snapshot_index (run_id, snapshot_id, version_hash)
VALUES (:run_id, :snapshot_id, :effective_hash);

-- 6. Update run status
UPDATE run SET status = 'RUNNING' WHERE run_id = :run_id;
```

---

## 6. Compaction

**Scenario:** Patch chain on `exp/quality` has depth=25; compact to new version.

### Trigger Conditions:

```sql
-- Find chains needing compaction
SELECT a.artifact_id, a.depth, a.base_version, t.tag_name
FROM artifact a
JOIN tag t ON t.target_id = a.artifact_id AND t.target_kind = 'patch_set'
WHERE a.kind = 'patch_set'
  AND a.depth > 20  -- Threshold
ORDER BY a.depth DESC;
```

### Compaction Process:

```sql
BEGIN;

-- 1. Materialize current head (reuse cached snapshot if available)
SELECT snapshot_id, version_hash
FROM run_snapshot_index rsi
JOIN run r ON r.run_id = rsi.run_id
WHERE r.base_ref = 'exp/quality'  -- or compute plan_hash
ORDER BY r.submitted_at DESC
LIMIT 1;

-- If found: reuse snapshot
-- Else: materialize (same as step 5)

-- 2. Create new dag_version from materialized snapshot
INSERT INTO artifact (
    artifact_id,
    kind,
    cas_id,
    name,
    version_hash,
    nodes_count,
    edges_count,
    meta,
    created_by
)
VALUES (
    gen_random_uuid(),
    'dag_version',
    :snapshot_cas_id,
    'exp/quality compacted at depth 25',
    :effective_hash,
    :nodes_count,
    :edges_count,
    jsonb_build_object('compacted_from', array[:old_head_id]),
    'system/compaction'
)
RETURNING artifact_id;

-- Store as new_version_id

-- 3. (Optional) Move branch tag to new version
-- Future patches will use new_version_id as base
UPDATE tag
SET target_kind = 'dag_version',
    target_id = :new_version_id,
    target_hash = :effective_hash,
    version = version + 1,
    moved_by = 'system/compaction',
    moved_at = now()
WHERE tag_name = 'exp/quality';

-- 4. Old patches remain for history (GC later if unreferenced)

COMMIT;
```

---

## 7. Garbage Collection

**Scenario:** Clean up unreferenced CAS blobs and artifacts.

### GC Strategy:

**Simplified Time-Based Cleanup (Postgres OLTP)**

Postgres handles only simple, time-based cleanup to avoid expensive recursive queries. Complex reachability analysis is delegated to Snowflake/OLAP.

**Retention Policy:**
- Keep artifacts created in last 7 days (safety window)
- Keep artifacts referenced by tags
- Keep artifacts used in runs from last 30 days
- Delegate complex graph traversal to Snowflake for deep analysis

### Simple Postgres Cleanup:

```sql
BEGIN;

-- 1. Delete old artifacts not referenced by recent runs or tags
-- Only delete artifacts older than safety window (7 days)
DELETE FROM artifact a
WHERE a.created_at < now() - interval '7 days'
  -- Not referenced by any tag
  AND NOT EXISTS (
    SELECT 1 FROM tag t WHERE t.target_id = a.artifact_id
  )
  -- Not used in recent runs (last 30 days)
  AND NOT EXISTS (
    SELECT 1 FROM run r
    WHERE r.submitted_at > now() - interval '30 days'
      AND (r.base_ref = a.artifact_id::text OR r.run_patch_id = a.artifact_id)
  )
  -- Not a snapshot from recent runs
  AND NOT EXISTS (
    SELECT 1 FROM run_snapshot_index rsi
    JOIN run r ON r.run_id = rsi.run_id
    WHERE rsi.snapshot_id = a.artifact_id
      AND r.submitted_at > now() - interval '30 days'
  );

-- 2. Clean up orphaned patch_chain_member rows
-- CASCADE delete handles this automatically, but verify:
DELETE FROM patch_chain_member
WHERE head_id NOT IN (SELECT artifact_id FROM artifact);

-- 3. Delete orphaned CAS blobs (not referenced by any artifact)
DELETE FROM cas_blob
WHERE cas_id NOT IN (SELECT DISTINCT cas_id FROM artifact)
  AND created_at < now() - interval '7 days';  -- Safety window

COMMIT;
```

### Export to Snowflake/OLAP for Deep Analysis:

For complex reachability analysis, export data to Snowflake where graph traversal queries are optimized:

```sql
-- Export artifacts with metadata (run nightly)
COPY (
  SELECT
    artifact_id,
    kind,
    cas_id,
    base_version,
    depth,
    plan_hash,
    version_hash,
    created_at,
    created_by
  FROM artifact
  WHERE created_at > now() - interval '90 days'
) TO '/tmp/artifacts_export.csv' CSV HEADER;

-- Export relationships
COPY (
  SELECT head_id, seq, member_id
  FROM patch_chain_member
) TO '/tmp/patch_chains_export.csv' CSV HEADER;

-- Export tags
COPY (
  SELECT tag_name, target_kind, target_id, moved_at
  FROM tag
) TO '/tmp/tags_export.csv' CSV HEADER;

-- Export run references
COPY (
  SELECT r.run_id, r.base_ref, r.submitted_at, rsi.snapshot_id
  FROM run r
  LEFT JOIN run_snapshot_index rsi ON rsi.run_id = r.run_id
  WHERE r.submitted_at > now() - interval '90 days'
) TO '/tmp/runs_export.csv' CSV HEADER;
```

### Snowflake Reachability Analysis:

In Snowflake, perform complex graph traversal to identify truly unreachable artifacts:

```sql
-- Run in Snowflake (supports recursive CTEs efficiently)
WITH RECURSIVE reachable AS (
  -- Seed 1: Tagged artifacts
  SELECT artifact_id, 'tag' as source
  FROM artifacts_staging a
  JOIN tags_staging t ON t.target_id = a.artifact_id

  UNION ALL

  -- Seed 2: Recent run artifacts
  SELECT snapshot_id as artifact_id, 'run' as source
  FROM runs_staging
  WHERE snapshot_id IS NOT NULL
    AND submitted_at > DATEADD(day, -30, CURRENT_TIMESTAMP())

  UNION ALL

  -- Recursive: Patch chain members
  SELECT pc.member_id as artifact_id, 'chain' as source
  FROM reachable r
  JOIN patch_chains_staging pc ON pc.head_id = r.artifact_id

  UNION ALL

  -- Recursive: Base versions
  SELECT a.base_version as artifact_id, 'base' as source
  FROM reachable r
  JOIN artifacts_staging a ON a.artifact_id = r.artifact_id
  WHERE a.base_version IS NOT NULL
)
SELECT artifact_id
FROM artifacts_staging
WHERE artifact_id NOT IN (SELECT DISTINCT artifact_id FROM reachable)
  AND created_at < DATEADD(day, -7, CURRENT_TIMESTAMP());
-- Result: IDs of artifacts safe to delete
```

### GC Workflow:

**Daily (Postgres):**
- Run simple time-based cleanup (fast, <1s)
- Delete obvious candidates (old + not in recent runs + not tagged)

**Weekly (Snowflake + Postgres):**
1. Export Postgres data to Snowflake (nightly batch job)
2. Run deep reachability analysis in Snowflake
3. Generate deletion list of truly unreachable artifact IDs
4. Import deletion list back to Postgres
5. Execute batch deletes in Postgres:

```sql
-- Import from Snowflake analysis
CREATE TEMP TABLE artifacts_to_delete (
  artifact_id UUID
);

COPY artifacts_to_delete FROM '/tmp/snowflake_gc_list.csv' CSV HEADER;

-- Batch delete (run during off-peak)
BEGIN;

DELETE FROM artifact
WHERE artifact_id IN (SELECT artifact_id FROM artifacts_to_delete);

DELETE FROM cas_blob
WHERE cas_id NOT IN (SELECT DISTINCT cas_id FROM artifact);

COMMIT;

DROP TABLE artifacts_to_delete;
```

### GC Statistics:

```sql
-- Storage breakdown (run before/after GC)
SELECT
    kind,
    COUNT(*) as count,
    SUM(size_bytes) / 1024 / 1024 / 1024 as size_gb,
    AVG(EXTRACT(EPOCH FROM (now() - created_at)) / 86400) as avg_age_days
FROM artifact a
JOIN cas_blob c ON c.cas_id = a.cas_id
GROUP BY kind
ORDER BY size_gb DESC;

-- Cleanup candidates (preview before delete)
SELECT
  kind,
  COUNT(*) as candidates,
  SUM(size_bytes) / 1024 / 1024 as potential_mb_freed
FROM artifact a
JOIN cas_blob c ON c.cas_id = a.cas_id
WHERE a.created_at < now() - interval '7 days'
  AND NOT EXISTS (SELECT 1 FROM tag t WHERE t.target_id = a.artifact_id)
  AND NOT EXISTS (
    SELECT 1 FROM run r
    WHERE r.submitted_at > now() - interval '30 days'
      AND r.run_patch_id = a.artifact_id
  )
GROUP BY kind;
```

### Benefits of OLAP Delegation:

- **Postgres stays fast**: No expensive recursive CTEs on OLTP database
- **Snowflake optimized**: Graph queries run 10-100x faster on columnar storage
- **Scalability**: Analyze billions of artifacts without impacting production
- **Safety**: Validate reachability offline before deleting anything
- **Cost-effective**: Run deep analysis weekly, not on every transaction

---

## 8. Query Patterns

### 8.1 Resolve Tag to Base + Chain

```sql
-- Get target artifact
SELECT a.artifact_id, a.kind, a.base_version, a.depth, a.version_hash
FROM tag t
JOIN artifact a ON a.artifact_id = t.target_id
WHERE t.tag_name = 'exp/quality';

-- If kind='patch_set', get chain
SELECT member_id
FROM patch_chain_member
WHERE head_id = :artifact_id
ORDER BY seq ASC;
```

### 8.2 Find Runs Using Same Snapshot

```sql
SELECT r.run_id, r.submitted_at, r.submitted_by, r.status
FROM run_snapshot_index rsi
JOIN run r ON r.run_id = rsi.run_id
WHERE rsi.version_hash = 'sha256:effective_abc...'
ORDER BY r.submitted_at DESC;
```

### 8.3 Find All Patches Based on Version

```sql
-- Direct children
SELECT artifact_id, name, depth, op_count
FROM artifact
WHERE kind = 'patch_set'
  AND base_version = :version_id
ORDER BY created_at;

-- All descendants (via chain members)
SELECT DISTINCT a.artifact_id, a.name, a.depth
FROM artifact a
JOIN patch_chain_member pcm ON pcm.member_id = a.artifact_id
JOIN artifact head ON head.artifact_id = pcm.head_id
WHERE head.base_version = :version_id
ORDER BY a.depth, a.created_at;
```

### 8.4 Check Snapshot Cache

```sql
SELECT artifact_id, version_hash, nodes_count, edges_count
FROM artifact
WHERE kind = 'run_snapshot'
  AND plan_hash = :computed_plan_hash
LIMIT 1;
```

### 8.5 Tag Movement History

```sql
SELECT
    moved_at,
    from_kind,
    from_id,
    to_kind,
    to_id,
    moved_by
FROM tag_move
WHERE tag_name = 'exp/quality'
ORDER BY moved_at DESC
LIMIT 20;
```

### 8.6 Compaction Candidates

```sql
-- Chains over depth threshold
SELECT
    a.artifact_id,
    a.depth,
    a.name,
    t.tag_name,
    COUNT(pcm.member_id) as member_count
FROM artifact a
JOIN tag t ON t.target_id = a.artifact_id
LEFT JOIN patch_chain_member pcm ON pcm.head_id = a.artifact_id
WHERE a.kind = 'patch_set'
  AND a.depth > 20
GROUP BY a.artifact_id, a.depth, a.name, t.tag_name
ORDER BY a.depth DESC;
```

---

## Performance Tips

### 1. Use Prepared Statements

```sql
PREPARE resolve_tag(text) AS
SELECT a.artifact_id, a.kind, a.base_version, a.depth
FROM tag t JOIN artifact a ON a.artifact_id = t.target_id
WHERE t.tag_name = $1;

EXECUTE resolve_tag('main');
```

### 2. Batch Chain Lookups

```sql
-- Fetch multiple chains at once
SELECT head_id, array_agg(member_id ORDER BY seq) as chain
FROM patch_chain_member
WHERE head_id = ANY(:head_ids)
GROUP BY head_id;
```

### 3. Use Connection Pooling

- Set `max_connections` appropriate to workload
- Use pgBouncer for connection pooling
- Configure `pool_mode = transaction` for stateless queries

### 4. Monitor Query Performance

```sql
-- Enable pg_stat_statements extension
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- Find slow queries
SELECT
    query,
    calls,
    mean_exec_time,
    max_exec_time
FROM pg_stat_statements
ORDER BY mean_exec_time DESC
LIMIT 10;
```

---

## Concurrency Notes

1. **Patch Creation:** Use `FOR UPDATE` on parent artifact to serialize branch head updates
2. **Tag Moves:** Use optimistic locking (`version` column) to detect conflicts
3. **Run Submission:** Read-only on artifacts; no locking needed
4. **Materialization:** Idempotent; multiple workers can safely cache same plan_hash
5. **Compaction:** Exclusive lock on tag during compaction (short duration)
6. **GC:** Run during off-peak hours; use `CONCURRENTLY` for index cleanup

---

## Monitoring Queries

```sql
-- Chain depth distribution
SELECT depth, COUNT(*) as patch_count
FROM artifact
WHERE kind = 'patch_set'
GROUP BY depth
ORDER BY depth;

-- Cache hit rate (approximate)
WITH snapshot_usage AS (
    SELECT snapshot_id, COUNT(*) as run_count
    FROM run_snapshot_index
    GROUP BY snapshot_id
)
SELECT
    COUNT(*) FILTER (WHERE run_count = 1) as cache_misses,
    COUNT(*) FILTER (WHERE run_count > 1) as cache_hits,
    ROUND(100.0 * COUNT(*) FILTER (WHERE run_count > 1) / COUNT(*), 2) as hit_rate_pct
FROM snapshot_usage;

-- Storage breakdown
SELECT
    kind,
    COUNT(*) as count,
    SUM(c.size_bytes) / 1024 / 1024 as size_mb
FROM artifact a
JOIN cas_blob c ON c.cas_id = a.cas_id
GROUP BY kind
ORDER BY size_mb DESC;

-- Tag churn
SELECT tag_name, COUNT(*) as move_count
FROM tag_move
WHERE moved_at > now() - interval '7 days'
GROUP BY tag_name
ORDER BY move_count DESC;
```

---

**End of Operations Guide**
