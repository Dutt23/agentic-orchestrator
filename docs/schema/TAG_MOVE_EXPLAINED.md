# Understanding `tag_move` Table

## What is `tag_move`?

`tag_move` is an **immutable audit log** that records every single movement of every tag. Think of it as a **Git reflog** - it tracks the complete history of where tags have pointed over time.

---

## Core Functions

### 1. **Audit Trail** (Who, What, When)

Every time a tag moves, we record:
- **Who**: `moved_by` - user or system
- **What**: `from_id` → `to_id` (which artifacts)
- **When**: `moved_at` - timestamp

```sql
SELECT * FROM tag_move WHERE tag_name = 'exp/quality';
```

| id | tag_name | from_id | to_id | moved_by | moved_at |
|----|----------|---------|-------|----------|----------|
| 1 | exp/quality | NULL | V1 | alice@co.com | 2024-01-10 10:00 |
| 2 | exp/quality | V1 | P1 | alice@co.com | 2024-01-10 10:15 |
| 3 | exp/quality | P1 | P2 | bob@co.com | 2024-01-10 11:30 |
| 4 | exp/quality | P2 | P1 | alice@co.com (undo) | 2024-01-10 12:00 |

**Insight**: Alice created the branch, Bob added a patch, Alice undid it.

---

### 2. **Undo/Redo** (Time Travel)

`tag_move` enables Git-like undo/redo by providing a **linked list** of tag positions.

#### Undo: Look Backward

```sql
-- Current position: tag → P2
-- Question: Where was tag before P2?

SELECT from_id, from_kind
FROM tag_move
WHERE tag_name = 'exp/quality'
  AND to_id = 'P2'  -- Current position
ORDER BY moved_at DESC
LIMIT 1;

-- Returns: from_id = P1
-- Action: Move tag from P2 → P1
```

#### Redo: Look Forward

```sql
-- Current position: tag → P1 (after undo)
-- Question: Where did tag go after P1?

SELECT to_id, to_kind
FROM tag_move
WHERE tag_name = 'exp/quality'
  AND from_id = 'P1'  -- Current position
ORDER BY moved_at ASC
LIMIT 1;

-- Returns: to_id = P2
-- Action: Move tag from P1 → P2
```

**Visual:**
```
tag_move history:
  V1 → P1 → P2 → P1(undo) → P2(redo)
       ↑    ↑     ↑          ↑
       │    │     │          └─ Redo: Look forward from P1
       │    │     └─ Undo: Look backward from P2
       │    └─ Normal move
       └─ First patch
```

---

### 3. **Complete History** (Forensics)

You can reconstruct the **entire timeline** of a tag:

```sql
SELECT
    id,
    from_kind || ':' || COALESCE(from_id::text, 'NULL') AS from_pos,
    to_kind || ':' || to_id AS to_pos,
    moved_by,
    moved_at
FROM tag_move
WHERE tag_name = 'exp/quality'
ORDER BY moved_at ASC;
```

| Step | From | To | Who | When |
|------|------|-------|---------|------------|
| 1 | NULL | dag_version:V1 | alice | 10:00 |
| 2 | dag_version:V1 | patch_set:P1 | alice | 10:15 |
| 3 | patch_set:P1 | patch_set:P2 | bob | 11:30 |
| 4 | patch_set:P2 | patch_set:P1 | alice (undo) | 12:00 |
| 5 | patch_set:P1 | patch_set:P2 | alice (redo) | 12:05 |

**Use case**: "What was the state of `exp/quality` at 11:00 AM yesterday?"

```sql
SELECT to_id, to_kind
FROM tag_move
WHERE tag_name = 'exp/quality'
  AND moved_at <= '2024-01-10 11:00:00'
ORDER BY moved_at DESC
LIMIT 1;

-- Returns: patch_set:P1 (Bob's P2 was added at 11:30)
```

---

### 4. **Conflict Detection** (Concurrent Edits)

When two users edit the same branch simultaneously, `tag_move` + optimistic locking prevent conflicts.

#### Scenario: Race Condition

```
Time  | Alice's Computer              | Bob's Computer
------|-------------------------------|--------------------------------
10:00 | GET tag (exp/quality → P1, v=5)| GET tag (exp/quality → P1, v=5)
10:01 | Create patch P2               | Create patch P3
10:02 | UPDATE tag SET id=P2, v=6     | (waiting...)
      | WHERE id=P1 AND v=5           |
      | ✅ Success (1 row updated)    |
10:03 |                               | UPDATE tag SET id=P3, v=6
      |                               | WHERE id=P1 AND v=5
      |                               | ❌ Conflict! (0 rows updated)
```

**Bob's transaction fails** because:
- Expected: `target_id = P1, version = 5`
- Actual: `target_id = P2, version = 6` (Alice updated it)

Bob must retry:
```sql
-- Bob retries based on latest state
GET tag → P2 (version=6)
Create P3 based on P2 (not P1)
UPDATE tag SET id=P3, v=7 WHERE id=P2 AND v=6
✅ Success
```

`tag_move` records both attempts:
```sql
INSERT INTO tag_move (...) VALUES ('exp/quality', P1, P2, 'alice', ...);
INSERT INTO tag_move (...) VALUES ('exp/quality', P2, P3, 'bob', ...);
```

---

### 5. **Compliance & Auditing** (Regulatory)

For regulated industries (finance, healthcare), `tag_move` provides:

- **Immutable log**: Cannot modify or delete history
- **Chain of custody**: Who approved what, when
- **Reproducibility**: Reconstruct exact state at any point

#### Example: FDA Audit

> "Show me the exact workflow used for batch #12345 on Jan 10, 2024."

```sql
-- 1. Find which tag was used for the run
SELECT base_ref FROM run WHERE batch_id = '12345';
-- Returns: 'prod'

-- 2. Find tag position at run time
SELECT to_id, to_kind
FROM tag_move
WHERE tag_name = 'prod'
  AND moved_at <= (SELECT submitted_at FROM run WHERE batch_id = '12345')
ORDER BY moved_at DESC
LIMIT 1;

-- Returns: dag_version:V5 (approved by QA)

-- 3. Reconstruct workflow from artifact
SELECT content FROM cas_blob
WHERE cas_id = (SELECT cas_id FROM artifact WHERE artifact_id = 'V5');
```

---

### 6. **Branching Analytics** (Insights)

`tag_move` enables analytics on workflow evolution:

#### Tag Churn (Instability)

```sql
-- Which branches change most frequently?
SELECT
    tag_name,
    COUNT(*) as moves,
    COUNT(*) FILTER (WHERE moved_by LIKE '%(undo)%') as undo_count
FROM tag_move
WHERE moved_at > now() - interval '7 days'
GROUP BY tag_name
ORDER BY moves DESC;
```

| tag_name | moves | undo_count |
|----------|-------|------------|
| exp/quality | 87 | 23 | ← High churn, many undos
| main | 12 | 0 | ← Stable
| prod | 3 | 0 | ← Very stable

**Insight**: `exp/quality` is unstable → needs review or compaction.

#### Collaboration Patterns

```sql
-- Who collaborates on which branches?
SELECT
    tag_name,
    moved_by,
    COUNT(*) as contributions
FROM tag_move
WHERE moved_at > now() - interval '30 days'
GROUP BY tag_name, moved_by
ORDER BY tag_name, contributions DESC;
```

| tag_name | moved_by | contributions |
|----------|----------|---------------|
| exp/quality | alice@co.com | 45 |
| exp/quality | bob@co.com | 32 |
| main | charlie@co.com | 8 |

---

## Schema Deep Dive

```sql
CREATE TABLE tag_move (
    -- Auto-incrementing ID (monotonic order)
    id BIGSERIAL PRIMARY KEY,

    -- Tag that was moved
    tag_name TEXT NOT NULL,

    -- Previous target (NULL for first move)
    from_kind TEXT,
    from_id UUID,

    -- New target
    to_kind TEXT NOT NULL,
    to_id UUID NOT NULL,

    -- Expected hash (for CAS validation, optional)
    expected_hash TEXT,

    -- Audit fields
    moved_by TEXT,
    moved_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Query tag history (most common)
CREATE INDEX idx_tag_move_name_time
    ON tag_move(tag_name, moved_at DESC);

-- Find moves involving specific artifacts
CREATE INDEX idx_tag_move_to
    ON tag_move(to_id, to_kind);

CREATE INDEX idx_tag_move_from
    ON tag_move(from_id, from_kind)
    WHERE from_id IS NOT NULL;
```

### Index Usage

```sql
-- Undo: Use idx_tag_move_name_time
SELECT from_id FROM tag_move
WHERE tag_name = 'exp/quality' AND to_id = 'P2'
ORDER BY moved_at DESC LIMIT 1;
-- Index scan: tag_name + moved_at

-- Redo: Same index
SELECT to_id FROM tag_move
WHERE tag_name = 'exp/quality' AND from_id = 'P1'
ORDER BY moved_at ASC LIMIT 1;

-- Find all runs using artifact P5
SELECT DISTINCT tag_name FROM tag_move
WHERE to_id = 'P5' AND to_kind = 'patch_set';
-- Index scan: idx_tag_move_to
```

---

## Comparison: `tag` vs `tag_move`

| Feature | `tag` | `tag_move` |
|---------|-------|-----------|
| **Size** | Tiny (~100 rows) | Large (grows forever) |
| **Mutability** | Mutable (UPDATE) | Immutable (INSERT only) |
| **Purpose** | Current state | Historical log |
| **Queries** | "Where is tag now?" | "Where was tag at time T?" |
| **Performance** | O(1) lookup | O(log n) with index |
| **Retention** | Forever | Forever (or archive) |

---

## Real-World Example: Git Reflog

Git's reflog is exactly `tag_move`:

```bash
# Git reflog (conceptually tag_move)
$ git reflog show exp/quality
a3f5c2e (HEAD -> exp/quality) exp/quality@{0}: commit: Add enrich node
91b7d4f exp/quality@{1}: commit: Add validate node
2c8e6a1 exp/quality@{2}: checkout: moving from main to exp/quality

# Undo (move HEAD back)
$ git reset --hard exp/quality@{1}

# This is like:
UPDATE tag SET target_id = '91b7d4f' WHERE tag_name = 'HEAD';
INSERT INTO tag_move (...) VALUES (..., 'a3f5c2e', '91b7d4f', ...);
```

---

## Performance Considerations

### Growth Rate

```
Assumptions:
- 50 branches
- 10 moves/branch/day
- Retention: 1 year

Rows/year = 50 × 10 × 365 = 182,500

At 200 bytes/row:
- Storage: ~35 MB/year
- Index: ~50 MB/year
- Total: <100 MB/year (negligible)
```

### Partitioning (Optional)

For high-churn systems, partition by time:

```sql
CREATE TABLE tag_move (...)
PARTITION BY RANGE (moved_at);

CREATE TABLE tag_move_2024_q1 PARTITION OF tag_move
    FOR VALUES FROM ('2024-01-01') TO ('2024-04-01');

CREATE TABLE tag_move_2024_q2 PARTITION OF tag_move
    FOR VALUES FROM ('2024-04-01') TO ('2024-07-01');
```

**Benefits**:
- Hot queries hit recent partition only
- Archive old partitions to cold storage
- Drop ancient partitions (if retention policy allows)

---

## Best Practices

### 1. Never Delete from `tag_move`

```sql
-- ❌ NEVER DO THIS
DELETE FROM tag_move WHERE moved_at < now() - interval '1 year';

-- ✅ Archive instead
INSERT INTO tag_move_archive
SELECT * FROM tag_move WHERE moved_at < now() - interval '1 year';

-- Then drop partition (if partitioned)
DROP TABLE tag_move_2023_q1;
```

### 2. Include Context in `moved_by`

```sql
-- Good: Include context
'user@example.com'
'user@example.com (undo)'
'user@example.com (redo)'
'system/compaction'
'system/automated-merge'

-- Helps distinguish user actions from system actions
```

### 3. Query Optimization

```sql
-- ✅ Good: Use index
SELECT from_id FROM tag_move
WHERE tag_name = 'exp/quality'
  AND to_id = 'P2'
ORDER BY moved_at DESC
LIMIT 1;

-- ❌ Bad: Missing WHERE clause
SELECT from_id FROM tag_move
WHERE to_id = 'P2'  -- Missing tag_name!
ORDER BY moved_at DESC
LIMIT 1;
-- Forces full table scan
```

---

## Summary

`tag_move` is a **critical audit table** that enables:

| Function | Benefit |
|----------|---------|
| **Undo/Redo** | Git-like time travel |
| **Audit Trail** | Who changed what, when |
| **Compliance** | Regulatory requirements |
| **Forensics** | Reconstruct history |
| **Analytics** | Branching patterns, churn |
| **Conflict Resolution** | Detect concurrent edits |

**Key Insight**: `tag` is the **current state** (snapshot), while `tag_move` is the **full history** (movie).

---

## Analogy

Think of `tag` and `tag_move` like a **GPS tracker**:

- **`tag`**: "Where is the car now?" (current location)
- **`tag_move`**: "Where has the car been?" (full route history)

You need both:
- `tag` for fast lookups (current state)
- `tag_move` for analysis and time travel (history)

---

**End of `tag_move` Explanation**