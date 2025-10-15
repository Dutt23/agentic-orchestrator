# Table Relationships and Architecture

## Overview

This document explains how all tables in the orchestrator schema relate to each other, with concrete examples and visual diagrams.

---

## Table Architecture Layers

```
┌─────────────────────────────────────────────────────────────────────┐
│                        STORAGE LAYER                                 │
│  Immutable content-addressed blobs                                   │
└─────────────────────────────────────────────────────────────────────┘

    ┌──────────────┐
    │  cas_blob    │  ← Content-addressed storage (immutable)
    │──────────────│
    │ cas_id (PK)  │  sha256:abc123...
    │ media_type   │  application/json;type=dag
    │ size_bytes   │  1024
    │ content      │  {...workflow JSON...}
    │ storage_url  │  s3://bucket/... (for large files)
    └──────┬───────┘
           │
           │ Referenced by (FK)
           ↓

┌─────────────────────────────────────────────────────────────────────┐
│                        ARTIFACT LAYER                                │
│  Catalog of logical objects (versions, patches, snapshots)           │
└─────────────────────────────────────────────────────────────────────┘

    ┌─────────────────┐
    │   artifact      │  ← Catalog of all artifacts
    │─────────────────│
    │ artifact_id(PK) │  V1, P1, P2, etc. (UUID)
    │ kind            │  'dag_version' or 'patch_set'
    │ cas_id (FK)─────┼──→ Points to cas_blob
    │ base_version(FK)│  For patches: points to parent DAG version
    │ depth           │  Patch chain depth (0 for base versions)
    │ version_hash    │  Integrity hash
    │ plan_hash       │  For run_snapshot: cache key
    │ name            │  Human-readable name
    │ meta            │  Flexible JSONB metadata
    └────┬────────────┘
         │
         │ Forms a tree structure
         ↓

    Example Chain:
    V1 (depth=0, base=NULL)
     ↑
     │ base_version
     │
    P1 (depth=1, base=V1)
     ↑
     │ base_version
     │
    P2 (depth=2, base=V1)  ← Note: base is V1, not P1!
     ↑
     │
    P3 (depth=3, base=V1)


┌─────────────────────────────────────────────────────────────────────┐
│                      BRANCHING LAYER (Mutable)                       │
│  Git-like branch pointers that move over time                        │
└─────────────────────────────────────────────────────────────────────┘

    ┌──────────────┐
    │     tag      │  ← Current branch pointers (like Git refs)
    │──────────────│
    │ tag_name(PK) │  'main', 'prod', 'exp/quality'
    │ target_kind  │  'dag_version' or 'patch_set'
    │ target_id(FK)├──→ Points to artifact (current position)
    │ target_hash  │  Optional: for CAS validation
    │ version      │  Optimistic locking version
    │ moved_by     │  Who moved it last
    │ moved_at     │  When it was last moved
    └──────────────┘

    Example:
    ┌─────────┬─────────────┬───────────┬─────────┐
    │tag_name │ target_kind │ target_id │ version │
    ├─────────┼─────────────┼───────────┼─────────┤
    │ main    │ patch_set   │ P3        │ 8       │  ← Current state
    │ prod    │ dag_version │ V1        │ 2       │
    │ exp/qa  │ patch_set   │ P5        │ 12      │
    └─────────┴─────────────┴───────────┴─────────┘


┌─────────────────────────────────────────────────────────────────────┐
│                      HISTORY LAYER (Immutable)                       │
│  Audit log of all tag movements (like Git reflog)                   │
└─────────────────────────────────────────────────────────────────────┘

    ┌──────────────┐
    │  tag_move    │  ← Audit log (append-only, INSERT only)
    │──────────────│
    │ id (PK)      │  Auto-increment (BIGSERIAL)
    │ tag_name     │  Which tag moved
    │ from_kind    │  Previous artifact kind (NULL for first move)
    │ from_id      │  Previous artifact ID
    │ to_kind      │  New artifact kind
    │ to_id        │  New artifact ID
    │ expected_hash│  For CAS validation
    │ moved_by     │  Who/what moved it
    │ moved_at     │  Timestamp
    └──────────────┘

    Example for 'main' tag:
    ┌────┬──────────┬─────────┬───────┬──────────────┬──────────┐
    │ id │ tag_name │ from_id │ to_id │ moved_by     │ moved_at │
    ├────┼──────────┼─────────┼───────┼──────────────┼──────────┤
    │ 1  │ main     │ NULL    │ V1    │ alice        │ 10:00    │
    │ 2  │ main     │ V1      │ P1    │ alice        │ 10:15    │
    │ 3  │ main     │ P1      │ P2    │ alice        │ 10:30    │
    │ 4  │ main     │ P2      │ P3    │ bob          │ 11:00    │
    │ 5  │ main     │ P3      │ P2    │ alice (undo) │ 11:30    │
    │ 6  │ main     │ P2      │ P3    │ alice (redo) │ 11:35    │
    └────┴──────────┴─────────┴───────┴──────────────┴──────────┘
                                 ↑
                                 │
                      Forms a linked list (movement history)


┌─────────────────────────────────────────────────────────────────────┐
│                   OPTIMIZATION LAYER (O(1) Lookup)                   │
│  Pre-computed patch chains to avoid recursive queries                │
└─────────────────────────────────────────────────────────────────────┘

    ┌───────────────────────┐
    │ patch_chain_member    │  ← Pre-materialized patch chains
    │───────────────────────│
    │ head_id (PK)          │  Latest patch in chain
    │ seq (PK)              │  Application order (1, 2, 3...)
    │ member_id             │  Patch at this position
    └───────────────────────┘

    Example for P3 chain:
    ┌─────────┬─────┬───────────┐
    │ head_id │ seq │ member_id │  ← Meaning
    ├─────────┼─────┼───────────┤
    │ P3      │ 1   │ P1        │  Apply P1 first
    │ P3      │ 2   │ P2        │  Then apply P2
    │ P3      │ 3   │ P3        │  Then apply P3
    └─────────┴─────┴───────────┘

    Query to materialize P3 (O(1) lookup!):
    SELECT member_id FROM patch_chain_member
    WHERE head_id = 'P3' ORDER BY seq;
    → Returns: [P1, P2, P3]


┌─────────────────────────────────────────────────────────────────────┐
│                       EXECUTION LAYER                                │
│  Workflow runs and their snapshots                                   │
└─────────────────────────────────────────────────────────────────────┘

    ┌──────────────────┐
    │      run         │  ← Workflow submissions (partitioned by time)
    │──────────────────│
    │ run_id (PK)      │  UUID v7
    │ submitted_at(PK) │  Partition key
    │ base_kind        │  'tag', 'dag_version', 'patch_set'
    │ base_ref         │  Tag name or artifact ID
    │ run_patch_id(FK) │  Optional run-specific patch
    │ tags_snapshot    │  JSONB: tag positions at submission
    │ status           │  QUEUED, RUNNING, COMPLETED, FAILED
    │ submitted_by     │  User ID
    └────┬─────────────┘
         │
         │ One-to-one
         ↓
    ┌──────────────────────┐
    │ run_snapshot_index   │  ← Links runs to cached snapshots
    │──────────────────────│
    │ run_id (PK, FK)      │  References run
    │ run_submitted_at(PK) │  Partition key
    │ snapshot_id (FK)─────┼──→ Points to artifact (run_snapshot)
    │ version_hash         │  Effective hash of materialized DAG
    └──────────────────────┘
```

---

## Complete Flow: From Storage to Tag

### Scenario: User requests workflow at tag 'main'

```
USER REQUEST: GET /api/v1/workflows/main?materialize=true
    ↓
┌────────────────────────────────────────┐
│ STEP 1: LOOKUP TAG                     │
│ SELECT target_id, target_kind          │
│ FROM tag WHERE tag_name = 'main'       │
│                                         │
│ Result: target_id=P3, target_kind=patch_set
└───────────────┬────────────────────────┘
                ↓
┌────────────────────────────────────────┐
│ STEP 2: GET ARTIFACT METADATA          │
│ SELECT base_version, depth, cas_id     │
│ FROM artifact WHERE artifact_id='P3'   │
│                                         │
│ Result: base=V1, depth=3, cas_id=sha256:patch3...
└───────────────┬────────────────────────┘
                ↓
┌────────────────────────────────────────┐
│ STEP 3: RESOLVE PATCH CHAIN (O(1))     │
│ SELECT member_id                       │
│ FROM patch_chain_member                │
│ WHERE head_id = 'P3' ORDER BY seq      │
│                                         │
│ Result: [P1, P2, P3]                   │
└───────────────┬────────────────────────┘
                ↓
┌────────────────────────────────────────┐
│ STEP 4: FETCH BASE VERSION             │
│ SELECT cas_id FROM artifact            │
│ WHERE artifact_id = 'V1'               │
│                                         │
│ Result: sha256:base123                 │
└───────────────┬────────────────────────┘
                ↓
┌────────────────────────────────────────┐
│ STEP 5: FETCH CONTENT FROM CAS         │
│ SELECT content FROM cas_blob           │
│ WHERE cas_id IN (                      │
│   'sha256:base123',  ← V1              │
│   'sha256:patch1',   ← P1              │
│   'sha256:patch2',   ← P2              │
│   'sha256:patch3'    ← P3              │
│ )                                       │
│                                         │
│ Result: 4 JSON blobs                   │
└───────────────┬────────────────────────┘
                ↓
┌────────────────────────────────────────┐
│ STEP 6: MATERIALIZE (Apply Patches)    │
│ workflow = parse_json(V1.content)      │
│ workflow = apply_patch(workflow, P1)   │
│ workflow = apply_patch(workflow, P2)   │
│ workflow = apply_patch(workflow, P3)   │
│                                         │
│ Result: Final materialized workflow    │
└────────────────────────────────────────┘
```

**Total Queries: 4-5 (depending on caching)**

---

## Concrete Example: Complete Lifecycle

### Step 1: Create Base Workflow

**User Action:** `POST /api/v1/workflows` with `tag_name: "main"`

**Tables After:**

**cas_blob:**
| cas_id | media_type | content |
|--------|------------|---------|
| sha256:abc123 | application/json;type=dag | `{"nodes":[...], "edges":[...]}` |

**artifact:**
| artifact_id | kind | cas_id | base_version | depth |
|-------------|------|--------|--------------|-------|
| V1 | dag_version | sha256:abc123 | NULL | 0 |

**tag:**
| tag_name | target_kind | target_id | version | moved_by |
|----------|-------------|-----------|---------|----------|
| main | dag_version | V1 | 1 | alice@co.com |

**tag_move:**
| id | tag_name | from_id | to_id | moved_by | moved_at |
|----|----------|---------|-------|----------|----------|
| 1 | main | NULL | V1 | alice@co.com | 10:00:00 |

**patch_chain_member:**
```
(empty - V1 is a base version, not a patch)
```

---

### Step 2: Create Patch P1

**User Action:** Create patch that adds a node

**Tables After:**

**cas_blob:** (new entry)
| cas_id | media_type | content |
|--------|------------|---------|
| sha256:abc123 | application/json;type=dag | `{...}` (existing) |
| sha256:patch1 | application/json;type=patch_ops | `[{"op":"add",...}]` |

**artifact:** (new entry)
| artifact_id | kind | cas_id | base_version | depth |
|-------------|------|--------|--------------|-------|
| V1 | dag_version | sha256:abc123 | NULL | 0 |
| P1 | patch_set | sha256:patch1 | V1 | 1 |

**tag:** (updated)
| tag_name | target_kind | target_id | version | moved_by |
|----------|-------------|-----------|---------|----------|
| main | patch_set | P1 | 2 | alice@co.com |

**tag_move:** (new entry)
| id | tag_name | from_id | to_id | moved_by | moved_at |
|----|----------|---------|-------|----------|----------|
| 1 | main | NULL | V1 | alice@co.com | 10:00:00 |
| 2 | main | V1 | P1 | alice@co.com | 10:15:00 |

**patch_chain_member:** (new entries)
| head_id | seq | member_id |
|---------|-----|-----------|
| P1 | 1 | P1 |

---

### Step 3: Create Patch P2

**Tables After:**

**artifact:**
| artifact_id | kind | cas_id | base_version | depth |
|-------------|------|--------|--------------|-------|
| V1 | dag_version | sha256:abc123 | NULL | 0 |
| P1 | patch_set | sha256:patch1 | V1 | 1 |
| P2 | patch_set | sha256:patch2 | V1 | 2 |

**tag:**
| tag_name | target_kind | target_id | version |
|----------|-------------|-----------|---------|
| main | patch_set | P2 | 3 |

**tag_move:**
| id | tag_name | from_id | to_id | moved_at |
|----|----------|---------|-------|----------|
| 1 | main | NULL | V1 | 10:00:00 |
| 2 | main | V1 | P1 | 10:15:00 |
| 3 | main | P1 | P2 | 10:30:00 |

**patch_chain_member:**
| head_id | seq | member_id | Meaning |
|---------|-----|-----------|---------|
| P1 | 1 | P1 | P1's chain: [P1] |
| P2 | 1 | P1 | P2's chain: [P1, P2] |
| P2 | 2 | P2 | ↑ |

---

### Step 4: Undo (P2 → P1)

**User Action:** `POST /api/v1/tags/main/undo`

**Algorithm:**
```sql
-- 1. Get current position
SELECT target_id FROM tag WHERE tag_name = 'main';
-- Returns: P2

-- 2. Find previous position
SELECT from_id FROM tag_move
WHERE tag_name = 'main' AND to_id = 'P2'
ORDER BY moved_at DESC LIMIT 1;
-- Returns: P1

-- 3. Move tag backward
UPDATE tag
SET target_id = 'P1', version = version + 1, moved_by = 'alice (undo)'
WHERE tag_name = 'main';

-- 4. Record undo
INSERT INTO tag_move (tag_name, from_id, to_id, moved_by)
VALUES ('main', 'P2', 'P1', 'alice (undo)');
```

**tag:** (after undo)
| tag_name | target_kind | target_id | version |
|----------|-------------|-----------|---------|
| main | patch_set | P1 | 4 |

**tag_move:** (after undo)
| id | tag_name | from_id | to_id | moved_by | moved_at |
|----|----------|---------|-------|----------|----------|
| 1 | main | NULL | V1 | alice | 10:00:00 |
| 2 | main | V1 | P1 | alice | 10:15:00 |
| 3 | main | P1 | P2 | alice | 10:30:00 |
| 4 | main | P2 | P1 | alice (undo) | 11:00:00 |

---

## Key Relationships Summary

### 1. cas_blob ← artifact
- **Type:** One-to-many (many artifacts can share same content via deduplication)
- **Purpose:** Content-addressed storage
- **Example:** If two patches have identical operations, they share one cas_blob

### 2. artifact ← artifact (self-reference via base_version)
- **Type:** Tree structure (parent-child)
- **Purpose:** Track patch lineage
- **Example:** P1, P2, P3 all have `base_version = V1`

### 3. artifact ← tag
- **Type:** Many-to-one (many tags can point to same artifact)
- **Purpose:** Git-like branch pointers
- **Example:** 'main' → P3, 'staging' → P3 (same artifact)

### 4. tag → tag_move
- **Type:** One-to-many (one tag has many historical moves)
- **Purpose:** Audit trail for undo/redo
- **Example:** 'main' has 10 moves in tag_move history

### 5. artifact ← patch_chain_member
- **Type:** One-to-many (one patch head has many members)
- **Purpose:** Pre-computed patch application order
- **Example:** P3 has 3 entries in patch_chain_member: [P1, P2, P3]

### 6. artifact ← run
- **Type:** One-to-many (many runs can use same workflow)
- **Purpose:** Track workflow executions
- **Example:** 100 runs all executed 'main' tag

### 7. run ← run_snapshot_index
- **Type:** One-to-one (each run has one snapshot)
- **Purpose:** Link runs to cached materialized workflows
- **Example:** Run R1 → Snapshot S1 (cached materialization)

---

## Tag vs Tag_move: Key Difference

```
┌────────────────────────────────────────────────────────────┐
│                         tag                                 │
│  Purpose: Current state ("Where is the tag NOW?")          │
│  Size: ~100 rows (one per branch)                          │
│  Mutability: MUTABLE (UPDATE on every move)                │
│  Query: "What does 'main' point to?"                       │
│  Lookup: O(1) - indexed by tag_name                        │
└────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────┐
│                      tag_move                               │
│  Purpose: History ("Where has the tag BEEN?")              │
│  Size: Growing (one per tag movement)                      │
│  Mutability: IMMUTABLE (INSERT only, never UPDATE/DELETE)  │
│  Query: "Where was 'main' at 10:00 AM yesterday?"          │
│  Lookup: O(log n) - indexed by (tag_name, moved_at)        │
└────────────────────────────────────────────────────────────┘

Analogy:
  tag      = GPS showing "You are here"
  tag_move = GPS showing "Your route history"
```

---

## Performance Characteristics

### Read Performance

| Operation | Tables Accessed | Queries | Latency |
|-----------|----------------|---------|---------|
| Get tag current position | tag | 1 | <1ms |
| Get tag history | tag_move | 1 | 2-5ms |
| Materialize workflow (depth 10) | artifact, patch_chain_member, cas_blob | 3 | 10-50ms |
| Undo tag | tag, tag_move | 2 | 2-5ms |
| List all workflows | tag | 1 | 1-5ms |

### Write Performance

| Operation | Tables Modified | Latency |
|-----------|----------------|---------|
| Create base version | cas_blob, artifact, tag, tag_move | 5-10ms |
| Create patch | cas_blob, artifact, patch_chain_member, tag, tag_move | 10-20ms |
| Move tag | tag, tag_move | 2-5ms |
| Undo/Redo | tag, tag_move | 2-5ms |

---

## Storage Growth Rates

### For a typical deployment:

**Assumptions:**
- 50 workflows (tags)
- 10 patches per workflow per week
- Average patch size: 5KB
- Average base version size: 50KB
- 1 year retention

**Growth:**

```
cas_blob:
  - Base versions: 50 × 50KB = 2.5MB
  - Patches: 50 × 10 × 52 × 5KB = 130MB/year
  - Deduplication: ~50% savings = 65MB/year
  - Total: ~68MB/year

artifact:
  - Rows: 50 + (50 × 10 × 52) = 26,050 rows/year
  - Storage: ~10MB/year (metadata only)

tag:
  - Rows: 50 (constant)
  - Storage: <10KB

tag_move:
  - Rows: 50 × 10 × 52 = 26,000 rows/year
  - Storage: ~5MB/year

patch_chain_member:
  - Without compaction: n(n+1)/2 per chain
  - With compaction (depth < 10): manageable
  - Worst case: 50 × (10 × 11 / 2) = 2,750 rows
  - Realistic: ~1,000 rows (with compaction)
  - Storage: <1MB
```

**Total: ~100MB/year (including indexes)**

---

## Best Practices

### 1. When to Use Each Table

**Use `tag` when:**
- Getting current workflow for a branch
- Fast O(1) lookups needed
- Want to know "what's deployed to production now?"

**Use `tag_move` when:**
- Implementing undo/redo
- Auditing tag movements
- Reconstructing history
- Compliance/regulatory requirements

**Use `patch_chain_member` when:**
- Materializing patches (always)
- Need O(1) chain resolution
- Avoiding recursive queries

**Use `cas_blob` directly when:**
- Downloading raw workflow JSON
- Content deduplication checks
- Storage size analysis

### 2. Query Optimization

```sql
-- ✅ GOOD: Use tag for current state
SELECT target_id FROM tag WHERE tag_name = 'main';

-- ❌ BAD: Don't use tag_move for current state
SELECT to_id FROM tag_move
WHERE tag_name = 'main'
ORDER BY moved_at DESC LIMIT 1;
-- Slower and unnecessary!

-- ✅ GOOD: Use patch_chain_member for materialization
SELECT member_id FROM patch_chain_member
WHERE head_id = 'P10' ORDER BY seq;

-- ❌ BAD: Recursive query
WITH RECURSIVE chain AS (...)
-- O(n) instead of O(1)
```

### 3. Transaction Boundaries

```go
// Moving a tag requires updating TWO tables atomically:
tx.Begin()
  // 1. Update tag (current state)
  tx.Exec("UPDATE tag SET target_id = $1 WHERE tag_name = $2")

  // 2. Insert into tag_move (history)
  tx.Exec("INSERT INTO tag_move (...) VALUES (...)")
tx.Commit()

// NEVER update one without the other!
```

---

## Troubleshooting

### Issue: Tag points to non-existent artifact

**Symptom:** `tag.target_id` not found in `artifact` table

**Cause:** Orphaned reference (artifact deleted but tag not updated)

**Fix:**
```sql
-- Find orphaned tags
SELECT t.tag_name, t.target_id
FROM tag t
LEFT JOIN artifact a ON a.artifact_id = t.target_id
WHERE a.artifact_id IS NULL;

-- Fix: Move to previous position
UPDATE tag
SET target_id = (
  SELECT from_id FROM tag_move
  WHERE tag_name = t.tag_name
  ORDER BY moved_at DESC LIMIT 1
)
FROM tag t
WHERE t.target_id NOT IN (SELECT artifact_id FROM artifact);
```

### Issue: Missing patch_chain_member entries

**Symptom:** Cannot materialize patch (chain incomplete)

**Cause:** Bug in patch creation logic

**Fix:**
```sql
-- Rebuild chain for P3
DELETE FROM patch_chain_member WHERE head_id = 'P3';

-- Rebuild by depth
INSERT INTO patch_chain_member (head_id, seq, member_id)
SELECT 'P3', depth, artifact_id
FROM artifact
WHERE base_version = (SELECT base_version FROM artifact WHERE artifact_id = 'P3')
  AND depth <= (SELECT depth FROM artifact WHERE artifact_id = 'P3')
ORDER BY depth;
```

---

## Summary

**Table Purpose Matrix:**

| Table | Purpose | Mutability | Size | Access Pattern |
|-------|---------|------------|------|----------------|
| cas_blob | Content storage | Immutable | Large | Bulk reads |
| artifact | Catalog | Append-only | Medium | Indexed lookups |
| tag | Current branch state | Mutable | Tiny | O(1) lookups |
| tag_move | Tag history | Append-only | Large | Range scans |
| patch_chain_member | Patch order cache | Append-only | Medium | O(1) lookups |
| run | Execution tracking | Append-only | Large | Partitioned |
| run_snapshot_index | Snapshot cache | Append-only | Medium | Indexed |

---

**End of Table Relationships Documentation**
