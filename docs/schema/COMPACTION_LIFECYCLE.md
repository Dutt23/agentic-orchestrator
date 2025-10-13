# Compaction Lifecycle: From Creation to Garbage Collection

## Overview

Compaction is the process of "squashing" a long chain of patches into a new base version to improve materialization performance. This document explains the complete lifecycle, from triggering compaction to eventual cleanup.

---

## The Compaction Problem

### Before Compaction

```
Timeline: 3 months of development
V1 (base, Jan 1)
 ↓
P1 (Jan 5)
 ↓
P2 (Jan 12)
 ↓
...
 ↓
P20 (Mar 30)

Current state: tag "main" → P20

Materialization cost:
- Fetch 20 patch blobs from CAS
- Apply 20 JSON patches sequentially
- O(20) operations
- Latency: 200-500ms ❌ TOO SLOW

patch_chain_member:
- P20 has 20 entries
- Total: 1+2+...+20 = 210 rows
- Storage: ~12KB

Problem: Deep chains are slow to materialize!
```

### After Compaction

```
V2 = materialize(V1 + P1 + P2 + ... + P20)
    (new base version, fully materialized)

Future patches can build on V2:
V2 (base, Mar 31) ← Fast!
 ↓
P21 (Apr 1)
 ↓
P22 (Apr 5)

Materialization cost:
- Fetch 1 base + 2 patches
- Apply 2 JSON patches
- O(2) operations
- Latency: 20-50ms ✅ MUCH FASTER

Storage saved:
- Old chain: 210 rows
- New chain: 1+2 = 3 rows
- Savings: 207 rows (98%!)
```

---

## Phase 1: Compaction Creation

### Trigger Conditions

**When to compact:**

1. **Depth threshold (primary)**
   ```
   IF patch_depth > 10 THEN compact
   ```

2. **Time-based (secondary)**
   ```
   IF days_since_last_compaction > 30 THEN compact
   ```

3. **Performance-based (advanced)**
   ```
   IF avg_materialization_time > 100ms THEN compact
   ```

4. **Manual (admin)**
   ```
   POST /api/v1/admin/compact?tag=main
   ```

### Compaction Algorithm

```go
func CompactWorkflow(tagName string) (uuid.UUID, error) {
    // Step 1: Get current tag state
    tag := GetTag(tagName) // main → P20
    if tag.TargetKind != "patch_set" {
        return nil, errors.New("cannot compact base version")
    }

    // Step 2: Check if compaction needed
    patch := GetArtifact(tag.TargetID) // P20
    if patch.Depth < 10 {
        return nil, errors.New("compaction not needed (depth < 10)")
    }

    // Step 3: Materialize full chain
    workflow := MaterializeWorkflow(patch.BaseVersion, patch.ID)
    // workflow = V1 + P1 + P2 + ... + P20 (fully materialized)

    // Step 4: Create new base version
    newBaseCASID := HashContent(workflow)
    InsertCASBlob(newBaseCASID, workflow)

    newBase := Artifact{
        ID:          GenerateUUID(), // V2
        Kind:        "dag_version",
        CASID:       newBaseCASID,
        BaseVersion: nil,            // Base version, not a patch
        Depth:       0,
        VersionHash: HashWorkflow(workflow),
        Meta: {
            "compacted_from": patch.ID,
            "compacted_at": time.Now(),
            "original_depth": patch.Depth,
        },
    }
    InsertArtifact(newBase)

    // Step 5: DO NOT delete old chain (yet!)
    // Step 6: DO NOT move tags (yet!)

    return newBase.ID, nil
}
```

### State After Compaction

**artifact table:**
```
┌────────────┬──────────┬──────────┬────────┬─────────────────┐
│artifact_id │ kind     │ base     │ depth  │ meta            │
├────────────┼──────────┼──────────┼────────┼─────────────────┤
│ V1         │dag_vers  │ NULL     │ 0      │ {}              │
│ P1         │patch_set │ V1       │ 1      │ {}              │ ← Still exist!
│ P2         │patch_set │ V1       │ 2      │ {}              │
│ ...        │...       │ ...      │ ...    │ ...             │
│ P20        │patch_set │ V1       │ 20     │ {}              │
│ V2         │dag_vers  │ NULL     │ 0      │ {compacted_from:P20} ← NEW!
└────────────┴──────────┴──────────┴────────┴─────────────────┘
```

**tag table:**
```
┌──────────┬─────────────┬───────────┬─────────┐
│tag_name  │ target_kind │ target_id │ version │
├──────────┼─────────────┼───────────┼─────────┤
│ main     │ patch_set   │ P20       │ 25      │ ← UNCHANGED!
└──────────┴─────────────┴───────────┴─────────┘
```

**patch_chain_member:**
```
All old entries still exist:
  P1: [P1]
  P2: [P1, P2]
  ...
  P20: [P1, P2, ..., P20]  (210 rows)

No new entries yet (V2 has no patches)
```

**Key Points:**
- ✅ V2 created
- ✅ Old chain (V1+P1-P20) preserved
- ✅ Tags unchanged (still point to P20)
- ✅ Undo/redo still works
- ✅ No breaking changes!

---

## Phase 2: Tag Migration

### Migration Timeline

```
Day 0: Compaction Complete
├─ V2 created
├─ Old chain intact
├─ Tags unchanged
└─ System notifies users: "New optimized base available"

Day 0-7: Grace Period (User Choice)
├─ New patches: Users choose base
│  ├─ Option A: P21 based on P20 (old chain, depth=21) ❌
│  └─ Option B: P21 based on V2 (new base, depth=1) ✅
├─ No forced migration
└─ Both paths coexist

Day 7-30: Soft Migration (Automatic)
├─ System migrates inactive tags to V2
├─ Records migration in tag_move
├─ Still allows undo to old positions
└─ UI shows "Migrated to optimized base"

Day 30-90: Retention Period
├─ Old chain still accessible
├─ Undo/redo works across migration
├─ Monitor: Are old chains still used?
└─ Prepare for archival

Day 90+: Archival (Cleanup)
├─ Archive old patch_chain_member to cold storage
├─ Keep metadata for audit
└─ Undo limited to retention window
```

### Strategy 1: Lazy Migration (User-Driven)

**When creating new patch, suggest V2:**

```go
func CreatePatch(parentID uuid.UUID, operations []JSONPatch) (uuid.UUID, error) {
    parent := GetArtifact(parentID)

    // Check if parent chain is deep
    if parent.Depth > 10 {
        // Look for compacted version
        compacted := FindCompactedBase(parent)
        if compacted != nil {
            // Suggest rebasing on V2
            return nil, &CompactionSuggestion{
                Message: "Parent chain is deep. Rebase on optimized version?",
                OldBase: parent.ID,
                NewBase: compacted.ID,
                Savings: fmt.Sprintf("%dms faster materialization", estimateSavings(parent, compacted)),
            }
        }
    }

    // User chose old chain or no compaction available
    newPatch := Artifact{
        BaseVersion: parent.BaseVersion, // Uses V1 (old)
        Depth:       parent.Depth + 1,   // depth=21 (slow)
    }
    InsertArtifact(newPatch)

    return newPatch.ID, nil
}

func FindCompactedBase(patch Artifact) *Artifact {
    // Look for dag_version with compacted_from=patch.ID
    var compacted Artifact
    err := db.QueryRow(`
        SELECT artifact_id, cas_id, version_hash
        FROM artifact
        WHERE kind = 'dag_version'
          AND meta->>'compacted_from' = $1
    `, patch.ID).Scan(&compacted.ID, &compacted.CASID, &compacted.VersionHash)

    if err != nil {
        return nil // No compacted version found
    }
    return &compacted
}
```

### Strategy 2: Automatic Migration

**Migrate tags automatically after grace period:**

```go
func MigrateTagsToCompactedBase() error {
    // Find tags pointing to old chains
    tags := db.Query(`
        SELECT t.tag_name, t.target_id, a.depth
        FROM tag t
        JOIN artifact a ON a.artifact_id = t.target_id
        WHERE a.kind = 'patch_set'
          AND a.depth > 10
          AND t.moved_at < NOW() - INTERVAL '7 days'  -- Inactive for 7 days
    `)

    for _, tag := range tags {
        // Find compacted version
        compacted := FindCompactedBase(GetArtifact(tag.TargetID))
        if compacted == nil {
            continue // No compacted version
        }

        // Move tag to V2
        tx.Begin()
        defer tx.Rollback()

        // Get current position for tag_move
        oldTarget := tag.TargetID
        oldKind := "patch_set"

        // Update tag
        tx.Exec(`
            UPDATE tag
            SET target_id = $1,
                target_kind = 'dag_version',
                version = version + 1,
                moved_by = 'system/compaction',
                moved_at = NOW()
            WHERE tag_name = $2
        `, compacted.ID, tag.Name)

        // Record in tag_move (preserves undo history!)
        tx.Exec(`
            INSERT INTO tag_move (tag_name, from_kind, from_id, to_kind, to_id, moved_by)
            VALUES ($1, $2, $3, $4, $5, $6)
        `, tag.Name, oldKind, oldTarget, "dag_version", compacted.ID, "system/compaction")

        tx.Commit()

        log.Printf("Migrated tag '%s' from %s (depth=%d) to %s (optimized)",
            tag.Name, oldTarget, tag.Depth, compacted.ID)
    }

    return nil
}
```

**Result after migration:**

**tag table:**
```
Before migration:
│ main     │ patch_set   │ P20       │ 25      │

After migration:
│ main     │ dag_version │ V2        │ 26      │ ← Moved to V2
```

**tag_move table:**
```
│ ... │ main     │ P20      │ V2       │ system/compaction │ Day 7  │
                    ↑           ↑
              Old position  New position
```

---

## Undo/Redo After Migration

### Scenario: Undo After Tag Migration

```
Timeline:
Day 0:  Compaction creates V2
Day 7:  Tag migrated: main → P20 → V2 (automatic)
Day 10: User undos: main → V2 → P20 (restore previous state)

Question: Does P20 still work?
Answer: YES! ✅
```

**Undo implementation:**

```go
func UndoTag(tagName string) error {
    // Step 1: Get current position
    tag := GetTag(tagName) // main → V2

    // Step 2: Find previous position from tag_move
    var prevID uuid.UUID
    var prevKind string
    db.QueryRow(`
        SELECT from_id, from_kind
        FROM tag_move
        WHERE tag_name = $1 AND to_id = $2
        ORDER BY moved_at DESC LIMIT 1
    `, tagName, tag.TargetID).Scan(&prevID, &prevKind)
    // Returns: prevID=P20, prevKind=patch_set

    // Step 3: Move tag backward
    UpdateTag(tagName, prevID, prevKind)

    // Step 4: Record undo
    RecordTagMove(tagName, tag.TargetID, tag.TargetKind, prevID, prevKind, "user (undo)")

    return nil
}
```

**Can we materialize P20 after migration?**

```sql
-- Get P20's chain
SELECT member_id FROM patch_chain_member
WHERE head_id = 'P20' ORDER BY seq;
-- Returns: [P1, P2, ..., P20] ✅ Still exists!

-- Materialize P20
result = V1  ← Base still exists
for patch in [P1, ..., P20]:  ← Patches still exist
  result = apply_patch(result, patch)
return result
```

**✅ Yes! Everything still works because we didn't delete anything.**

---

## Phase 3: Garbage Collection

### Safety Checks Before Deletion

**Never delete without checking:**

```sql
-- Check 1: No active tags pointing to old chain
SELECT COUNT(*) FROM tag
WHERE target_id IN ('P1', 'P2', ..., 'P20');
-- Must be 0

-- Check 2: No recent tag_move references
SELECT COUNT(*) FROM tag_move
WHERE (to_id IN ('P1', ..., 'P20') OR from_id IN ('P1', ..., 'P20'))
  AND moved_at > NOW() - INTERVAL '90 days';
-- Must be 0 (or acceptable loss)

-- Check 3: No recent runs using old chain
SELECT COUNT(*) FROM run
WHERE base_ref IN ('P1', ..., 'P20')
  AND submitted_at > NOW() - INTERVAL '90 days';
-- Must be 0

-- Check 4: No recent materializations
SELECT COUNT(*) FROM run_snapshot_index rsi
JOIN artifact a ON a.artifact_id = rsi.snapshot_id
WHERE a.meta->>'compacted_from' IN ('P1', ..., 'P20')
  AND rsi.created_at > NOW() - INTERVAL '90 days';
-- Must be 0
```

### Archival Process

**Don't delete immediately - archive first:**

```sql
-- Step 1: Mark artifacts as archived (soft delete)
UPDATE artifact
SET meta = jsonb_set(meta, '{archived}', 'true'),
    meta = jsonb_set(meta, '{archived_at}', to_jsonb(NOW()))
WHERE artifact_id IN ('P1', 'P2', ..., 'P20');

-- Step 2: Archive patch_chain_member to cold storage
INSERT INTO patch_chain_member_archive
SELECT *, NOW() as archived_at
FROM patch_chain_member
WHERE head_id IN ('P1', 'P2', ..., 'P20');

-- Step 3: Delete from hot storage
DELETE FROM patch_chain_member
WHERE head_id IN ('P1', 'P2', ..., 'P20');

-- Step 4: Archive CAS blobs (optional)
-- Move to S3 Glacier or delete if not needed
UPDATE cas_blob
SET storage_url = 's3://archive-bucket/' || cas_id,
    content = NULL  -- Free inline storage
WHERE cas_id IN (
  SELECT cas_id FROM artifact
  WHERE artifact_id IN ('P1', ..., 'P20')
);
```

### Restoration (If Needed)

**If user needs to materialize archived patch:**

```sql
-- Step 1: Check if archived
SELECT meta->>'archived' as archived
FROM artifact
WHERE artifact_id = 'P15';
-- Returns: 'true'

-- Step 2: Restore from archive (temporary)
INSERT INTO patch_chain_member (head_id, seq, member_id)
SELECT head_id, seq, member_id
FROM patch_chain_member_archive
WHERE head_id = 'P15';

-- Step 3: Restore CAS content (if needed)
UPDATE cas_blob
SET content = fetch_from_s3(storage_url)
WHERE cas_id = (SELECT cas_id FROM artifact WHERE artifact_id = 'P15');

-- Step 4: Materialize
-- Now works normally

-- Step 5: Re-archive after use (optional)
-- Clean up temporary restoration
```

---

## Edge Cases and Solutions

### Edge Case 1: Undo After Garbage Collection

**Problem:**

```
Day 0:   Compaction creates V2
Day 7:   Tag migrated: main → P20 → V2
Day 90:  GC deletes P1-P20 chains from patch_chain_member
Day 91:  User tries undo: main → V2 → P20 ❌ Can't materialize!
```

**Solution 1: Prevent Deep Undo**

```go
func UndoTag(tagName string) error {
    prevID := GetPreviousPosition(tagName)

    // Check if previous position is archived
    artifact := GetArtifact(prevID)
    if artifact.Meta["archived"] == "true" {
        archivedAt := artifact.Meta["archived_at"]
        return &ArchivedError{
            Message: fmt.Sprintf("Cannot undo to archived position (archived %s)", archivedAt),
            Suggestion: "This position is older than retention window (90 days). Create new patch instead.",
        }
    }

    // Proceed with undo
    MoveTag(tagName, prevID)
}
```

**Solution 2: Restore from Archive**

```go
func UndoTag(tagName string) error {
    prevID := GetPreviousPosition(tagName)
    artifact := GetArtifact(prevID)

    if artifact.Meta["archived"] == "true" {
        // Offer restoration (slow but possible)
        return &RestorationRequired{
            Message: "Position archived. Restore from cold storage?",
            EstimatedTime: "30-60 seconds",
            Cost: "$0.01",
            OnConfirm: func() {
                RestoreFromArchive(prevID)
                MoveTag(tagName, prevID)
            },
        }
    }

    MoveTag(tagName, prevID)
}
```

**Solution 3: Synthetic Undo**

```go
func SyntheticUndo(tagName string, targetPosition uuid.UUID) error {
    // Instead of going back to P20, create "reverse patch" on V2
    current := MaterializeTag(tagName)      // V2
    target := MaterializeTag(targetPosition) // P20 (from archive)

    // Compute diff
    reversePatch := ComputeJSONPatch(current, target)

    // Create new patch on V2
    newPatch := CreatePatch(GetTag(tagName).TargetID, reversePatch)

    // Move tag
    MoveTag(tagName, newPatch.ID)

    return nil
}
```

---

### Edge Case 2: Concurrent Patches During Compaction

**Problem:**

```
10:00 - Compaction starts (V2 = V1+P1...P20)
10:05 - User creates P21 (based on P20, depth=21)
10:10 - Compaction finishes (V2 ready)

Result: P21 based on old chain (not optimal)
```

**Solution: Compaction Lock**

```go
var compactionLock sync.RWMutex
var compactingWorkflows = make(map[uuid.UUID]bool)

func CreatePatch(parentID uuid.UUID, operations []JSONPatch) (uuid.UUID, error) {
    parent := GetArtifact(parentID)

    // Check if base is being compacted
    compactionLock.RLock()
    if compactingWorkflows[parent.BaseVersion] {
        compactionLock.RUnlock()
        return nil, &CompactionInProgress{
            Message: "Base version is being compacted. Please retry in 30 seconds.",
            RetryAfter: time.Now().Add(30 * time.Second),
        }
    }
    compactionLock.RUnlock()

    // Create patch normally
    newPatch := CreatePatchUnsafe(parentID, operations)
    return newPatch.ID, nil
}

func CompactWorkflow(baseID uuid.UUID) (uuid.UUID, error) {
    compactionLock.Lock()
    compactingWorkflows[baseID] = true
    compactionLock.Unlock()

    defer func() {
        compactionLock.Lock()
        delete(compactingWorkflows, baseID)
        compactionLock.Unlock()
    }()

    // Perform compaction
    newBase := CompactUnsafe(baseID)
    return newBase.ID, nil
}
```

---

### Edge Case 3: Multiple Branches Compacted

**Problem:**

```
Topology:
V1 → P1 → P2 → P3 → P4 → P5      (main branch)
          ↓
          P6 → P7 → P8 → P9 → P10 (exp branch)

Both deep! Need to compact both.
```

**Solution 1: Independent Compaction**

```
Result:
V2_main = V1+P1+P2+P3+P4+P5
V2_exp  = V1+P1+P2+P6+P7+P8+P9+P10

Storage cost:
- Shared prefix (P1, P2) duplicated in both
- Trade-off: Storage vs complexity

Benefit:
- Each branch has optimized base
- No cross-branch dependencies
```

**Solution 2: Shared Base Compaction**

```
Result:
V2 = V1+P1+P2 (common ancestor)

main: V2 → P3 → P4 → P5
exp:  V2 → P6 → P7 → P8 → P9 → P10

Storage:
- Shared prefix compacted once
- Better storage efficiency

Drawback:
- exp branch still has depth=5
- May need another compaction later
```

---

## Recommended Production Strategy

### Configuration

```yaml
compaction:
  triggers:
    depth_threshold: 10           # Compact when depth > 10
    time_interval_days: 30        # Compact if no compaction in 30 days
    performance_threshold_ms: 100 # Compact if avg materialization > 100ms

  migration:
    grace_period_days: 7          # Users can choose old/new base
    auto_migrate_after_days: 30   # Force migration for inactive tags

  retention:
    hot_storage_days: 90          # Keep chains in hot storage
    archive_retention_years: 7    # Keep archives for compliance
    undo_window_days: 90          # Allow undo within 90 days

  garbage_collection:
    check_interval_hours: 24      # Run GC checks daily
    min_age_days: 90              # Don't delete chains < 90 days old
    require_zero_references: true # Safety: must have no references
```

### Implementation Schedule

```
Phase 1: Compaction Creation (Week 1)
├─ Implement compaction trigger logic
├─ Create new base versions
├─ No deletion, no migration
└─ Test: Verify V2 creation

Phase 2: Lazy Migration (Week 2)
├─ Add UI prompts for new patches
├─ "Rebase on optimized version?"
├─ Track migration metrics
└─ Test: User acceptance

Phase 3: Auto Migration (Week 3-4)
├─ Implement automatic migration
├─ 7-day grace period
├─ Preserve undo history
└─ Test: No breaking changes

Phase 4: Monitoring (Week 5-8)
├─ Monitor old chain usage
├─ Track undo attempts
├─ Measure performance gains
└─ Adjust thresholds

Phase 5: Archival (Month 3+)
├─ Implement safety checks
├─ Archive to cold storage
├─ Restoration process
└─ Test: Emergency recovery

Phase 6: GC (Month 6+)
├─ Implement automated GC
├─ Compliance logging
├─ Metrics dashboard
└─ Production rollout
```

---

## Metrics and Monitoring

### Key Metrics

```sql
-- Compaction candidates
SELECT
    a.artifact_id,
    a.depth,
    t.tag_name,
    t.moved_at
FROM artifact a
JOIN tag t ON t.target_id = a.artifact_id
WHERE a.kind = 'patch_set'
  AND a.depth > 10
ORDER BY a.depth DESC;

-- Compaction history
SELECT
    artifact_id,
    meta->>'compacted_from' as from_patch,
    meta->>'original_depth' as depth,
    created_at
FROM artifact
WHERE kind = 'dag_version'
  AND meta ? 'compacted_from'
ORDER BY created_at DESC;

-- Migration progress
SELECT
    tag_name,
    to_kind as new_kind,
    moved_by,
    moved_at
FROM tag_move
WHERE moved_by = 'system/compaction'
  AND moved_at > NOW() - INTERVAL '30 days'
ORDER BY moved_at DESC;

-- Archived chains
SELECT
    COUNT(*) as archived_count,
    SUM(depth) as total_depth,
    MIN(meta->>'archived_at') as oldest_archive
FROM artifact
WHERE meta->>'archived' = 'true';
```

### Dashboard Panels

```
Compaction Health:
├─ Compaction candidates (depth > 10): 5 workflows
├─ Compactions last 30 days: 12
├─ Avg depth reduction: 15 → 2 (87% improvement)
└─ Avg materialization speedup: 250ms → 30ms (8.3x)

Migration Status:
├─ Auto-migrated tags: 45/50 (90%)
├─ User-migrated tags: 3/50 (6%)
├─ Still on old chains: 2/50 (4%)
└─ Migration grace period: 2 tags remaining

Storage:
├─ Hot storage: 1,250 patch_chain_member rows
├─ Archived: 8,500 rows (cold storage)
├─ Storage saved: 92%
└─ Undo success rate: 99.2% (within retention)
```

---

## Summary

### Compaction Lifecycle Phases

| Phase | Timeline | Actions | Data State |
|-------|----------|---------|------------|
| **Creation** | Day 0 | Create V2, keep old chain | V2 created, P1-P20 intact |
| **Grace Period** | Day 0-7 | Users choose base | Both paths coexist |
| **Migration** | Day 7-30 | Auto-migrate inactive tags | Tags move to V2 |
| **Retention** | Day 30-90 | Monitor usage, prepare archive | Old chain still accessible |
| **Archival** | Day 90+ | Move to cold storage | Hot storage freed |
| **GC** | Year 1+ | Optional hard delete | Compliance archives remain |

### Key Principles

1. **Never Delete Immediately** - Always preserve for undo/redo
2. **Grace Periods** - Give users time to adapt
3. **Safety Checks** - Verify no references before deletion
4. **Archival First** - Cold storage before hard delete
5. **Monitoring** - Track metrics, adjust thresholds

### Decision Tree

```
Should I compact?
├─ Depth > 10? ─────────────────────── YES → Compact
│
├─ Materialization > 100ms? ────────── YES → Compact
│
├─ 30 days since last compaction? ──── YES → Consider compaction
│
└─ Otherwise ──────────────────────── NO → Wait

After compaction, how long to keep old chain?
├─ Active tags pointing to it? ─────── Keep indefinitely
├─ Recent undos (< 30 days)? ────────── Keep for 90 days
├─ Compliance requirements? ─────────── Archive for 7 years
└─ No references + > 90 days ────────── Safe to archive/delete
```

---

**End of Compaction Lifecycle Documentation**
