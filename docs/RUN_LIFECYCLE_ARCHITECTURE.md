# Run Lifecycle Architecture

## Overview

This document describes the complete lifecycle of a workflow run in the orchestrator system, from creation to completion.

**Key Principle**: Workflows are **frozen at run creation time** by materializing and storing them as immutable artifacts. This ensures reproducibility and simplifies patch management.

---

## 1. Database Schema

### Run Table
```sql
CREATE TABLE run (
    run_id UUID PRIMARY KEY,                 -- UUID v7 (time-ordered)
    base_kind TEXT NOT NULL,                  -- 'dag_version' (always for runs)
    base_ref TEXT NOT NULL,                   -- artifact_id of materialized workflow
    tags_snapshot JSONB NOT NULL,             -- Tag positions at submission time
    status TEXT NOT NULL DEFAULT 'QUEUED',    -- QUEUED, RUNNING, COMPLETED, FAILED, CANCELLED
    submitted_by TEXT,
    submitted_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (run_id, submitted_at)
) PARTITION BY RANGE (submitted_at);
```

**Key Fields**:
- `base_ref`: Points to artifact containing **frozen, materialized workflow**
- `tags_snapshot`: Audit trail showing all tag positions at submission (e.g., `{"main": "artifact_id_1", "exp/feature": "artifact_id_2"}`)
- No `run_patch_id` field - patches stored in separate `run_patches` table

### Run Patches Table
```sql
CREATE TABLE run_patches (
    id UUID PRIMARY KEY,
    run_id VARCHAR(255) NOT NULL,
    artifact_id UUID NOT NULL,              -- Artifact containing patch operations
    seq INTEGER NOT NULL,                   -- Application order (1, 2, 3, ...)
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(255),
    UNIQUE(run_id, seq)
);
```

**One-to-Many Relationship**: `run (1) → run_patches (*)`

---

## 2. Run Creation Flow

### Step-by-Step Process

#### **A. User Submits Run Request**
```http
POST /api/v1/workflows/:tag/execute
X-User-ID: username

{
  "inputs": {...}
}
```

#### **B. Orchestrator Materializes Workflow**
`cmd/orchestrator/service/run.go:CreateRun()`

1. **Fetch workflow by tag** (from tag table)
2. **Resolve all patches** in the patch chain
3. **Combine base + patches** to create final workflow
4. **Store in CAS** as JSON blob
5. **Create artifact** pointing to CAS blob:
   ```go
   artifact := &Artifact{
       Kind:   "dag_version",
       CasID:  "sha256:abc123...",
       ...
   }
   ```

#### **C. Create Run Entry**
```go
run := &Run{
    RunID:        uuid.New(),
    BaseKind:     "dag_version",
    BaseRef:      artifact.ArtifactID.String(),
    TagsSnapshot: {"main": "artifact_xyz", ...},
    Status:       "QUEUED",
    SubmittedBy:  username,
    SubmittedAt:  time.Now(),
}
```

#### **D. Publish to Redis Stream**
```go
runRequest := {
    "run_id":      runID.String(),
    "artifact_id": artifact.ArtifactID.String(),
    "tag":         tagName,
    "username":    username,
    "inputs":      inputs,
}
redis.XAdd("wf.run.requests", runRequest)
```

---

## 3. Workflow Fetching (workflow-runner)

### Old Approach (Before)
- workflow-runner called `/api/v1/workflows/:tag?materialize=true`
- Materialization happened **every run**
- Patch chain resolution on-demand

### New Approach (After)
`cmd/workflow-runner/executor/run_request_consumer.go:fetchWorkflowFromArtifact()`

1. **Receive run request** from Redis stream with `artifact_id`
2. **Fetch frozen workflow**:
   ```http
   GET /api/v1/artifacts/:artifact_id

   Response:
   {
       "artifact_id": "...",
       "kind": "dag_version",
       "content": {...}  // Full workflow JSON
   }
   ```
3. **Compile to IR** and proceed with execution

**Benefits**:
- ✅ No re-materialization needed
- ✅ Workflow is immutable (frozen at run creation)
- ✅ Faster startup
- ✅ Reproducible executions

---

## 4. Run Status Management

### Architecture: Dual Storage

| Aspect | Redis (Hot) | Database (Cold) |
|--------|-------------|-----------------|
| **Purpose** | Operational queries | Audit & history |
| **Update Time** | Immediate | Async (queued) |
| **Read Speed** | Microseconds | Milliseconds |
| **Durability** | AOF + RDB | Fully durable |
| **Query From** | Active runs | Historical analysis |

### Status Update Flow

#### **Hot Path (Redis) - Immediate**
`cmd/workflow-runner/coordinator/coordinator.go:updateRunStatusInRedis()`

```go
// On workflow completion or failure
c.updateRunStatusInRedis(ctx, runID, "COMPLETED")

// Stores in Redis:
// Key: run:status:{run_id}
// Value: "COMPLETED"
// TTL: 24 hours
```

**When**: On every status change
**Speed**: <1ms
**Purpose**: Fast status checks for active runs

#### **Cold Path (Database) - Async**
`cmd/workflow-runner/coordinator/coordinator.go:queueRunStatusUpdate()`

```go
// Queue for DB write
c.queueRunStatusUpdate(ctx, runID, "COMPLETED")

// Publishes to Redis stream:
// Stream: run.status.updates
// Payload: {"run_id": "...", "status": "COMPLETED", "timestamp": ...}
```

**When**: Only on terminal states (COMPLETED, FAILED)
**Processing**: Background worker (can be added later)
**Purpose**: Durable audit trail

### Reading Status

**For Active Runs**:
```go
// Check Redis first
status := redis.Get("run:status:{run_id}")
if status != "" {
    return status  // Fast path
}

// Fallback to DB
run := db.GetRun(runID)
return run.Status
```

**For Historical Queries**:
```sql
-- Query DB directly
SELECT * FROM run WHERE submitted_by = 'user' ORDER BY submitted_at DESC;
```

---

## 5. Run Patches (Self-Modifying Workflows)

### Agent Creates Patch
`cmd/agent-runner-py/workflow/patch_client.py:apply_run_patch()`

1. Agent decides to add new nodes during execution
2. Agent calls orchestrator:
   ```http
   POST /api/v1/runs/{run_id}/patches
   X-User-ID: username

   {
       "operations": [
           {"op": "add", "path": "/nodes/-", "value": {...}},
           {"op": "add", "path": "/edges/-", "value": {...}}
       ],
       "description": "Added validation step"
   }
   ```
3. Orchestrator stores patch in `run_patches` table with `seq`

### Coordinator Applies Patches
`cmd/workflow-runner/coordinator/coordinator.go:reloadIRIfPatched()`

1. **Detect agent completion**
2. **Fetch patches** from orchestrator API
3. **Materialize workflow**:
   - Start with base workflow (from artifact)
   - Apply patches in sequence order (seq: 1, 2, 3...)
   - Generate new workflow JSON
4. **Update IR in Redis** with patched workflow
5. **Continue routing** to newly added nodes

---

## 6. Data Flow Diagram

```
┌──────────────────────────────────────────────────────────────────┐
│                         RUN CREATION                              │
└──────────────────────────────────────────────────────────────────┘

User Request
    ↓
┌─────────────────────────────────────┐
│ POST /workflows/:tag/execute        │
│ (Orchestrator)                      │
└─────────────────────────────────────┘
    ↓
┌─────────────────────────────────────┐
│ 1. Materialize Workflow             │
│    - Fetch tag → base workflow      │
│    - Resolve patch chain            │
│    - Combine base + patches         │
└─────────────────────────────────────┘
    ↓
┌─────────────────────────────────────┐
│ 2. Store as Artifact                │
│    - Hash content → CAS ID          │
│    - Create artifact record         │
│    - Store in cas_blob table        │
└─────────────────────────────────────┘
    ↓
┌─────────────────────────────────────┐
│ 3. Create Run Entry (DB)            │
│    - Generate UUID                  │
│    - base_ref = artifact_id         │
│    - tags_snapshot = current tags   │
│    - status = QUEUED                │
└─────────────────────────────────────┘
    ↓
┌─────────────────────────────────────┐
│ 4. Publish to Redis Stream          │
│    Stream: wf.run.requests          │
│    Payload: {run_id, artifact_id}   │
└─────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────┐
│                      RUN EXECUTION                                │
└──────────────────────────────────────────────────────────────────┘

workflow-runner consumes stream
    ↓
┌─────────────────────────────────────┐
│ GET /artifacts/:artifact_id         │
│ (Fetch frozen workflow)             │
└─────────────────────────────────────┘
    ↓
┌─────────────────────────────────────┐
│ Compile to IR & Store in Redis      │
│ Key: ir:{run_id}                    │
└─────────────────────────────────────┘
    ↓
┌─────────────────────────────────────┐
│ Execute Nodes                       │
│ (Workers process tasks)             │
└─────────────────────────────────────┘
    ↓
┌─────────────────────────────────────┐
│ On Agent Completion:                │
│ - Check for patches                 │
│ - Materialize patched workflow      │
│ - Update IR in Redis                │
│ - Route to new nodes                │
└─────────────────────────────────────┘
    ↓
┌─────────────────────────────────────┐
│ On Terminal State:                  │
│ - Update Redis (immediate)          │
│ - Queue DB update (async)           │
│ - Publish events                    │
└─────────────────────────────────────┘
```

---

## 7. Redis Persistence Configuration

### Recommended Configuration

```conf
# Append-Only File (AOF) - Log every write
appendonly yes
appendfsync everysec  # Fsync every second (good balance)

# RDB Snapshots (baseline backups)
save 900 1      # Save if 1 key changed in 15 minutes
save 300 10     # Save if 10 keys changed in 5 minutes
save 60 10000   # Save if 10,000 keys changed in 1 minute

# Hybrid persistence
aof-use-rdb-preamble yes  # Use RDB for base + AOF for recent changes
```

### Recovery Strategy

1. **Normal shutdown**: Both RDB and AOF saved
2. **Crash recovery**: Replay AOF from last RDB snapshot
3. **Background job**: Reconcile Redis → DB for any missing updates

---

## 8. Trade-offs & Design Decisions

### ✅ Benefits

| Decision | Benefit |
|----------|---------|
| Materialize at run creation | Immutable workflows, reproducible |
| Store as artifact | Content-addressed deduplication |
| Frozen workflows | No re-materialization overhead |
| Run patches separate | Clear audit trail, diff with base |
| Redis + DB dual storage | Fast operational queries + durable audit |

### ⚠️ Trade-offs

| Aspect | Trade-off | Mitigation |
|--------|-----------|------------|
| **Storage** | Each run creates artifact | CAS deduplication (same content → same artifact) |
| **Eventual Consistency** | DB lags behind Redis | Background reconciliation job |
| **Lost Updates** | Crash between Redis & DB | Redis AOF + reconciliation on startup |
| **Complexity** | Two sources of truth | Clear ownership: Redis=operational, DB=audit |

---

## 9. API Endpoints

### Run Management

```http
# Create run (materialize & execute)
POST /api/v1/workflows/:tag/execute
X-User-ID: username
Body: {"inputs": {...}}
Response: {"run_id": "...", "artifact_id": "...", "status": "queued"}

# Get run status
GET /api/v1/runs/:run_id
Response: {"run_id": "...", "status": "...", "submitted_at": "..."}

# List user's runs
GET /api/v1/runs?user=username&limit=50
Response: {"runs": [...]}
```

### Artifact Management

```http
# Get artifact with content
GET /api/v1/artifacts/:artifact_id
Response: {
    "artifact_id": "...",
    "kind": "dag_version",
    "cas_id": "sha256:...",
    "content": {...}  // Full workflow JSON
}
```

### Run Patches

```http
# Create run patch
POST /api/v1/runs/:run_id/patches
X-User-ID: username
Body: {"operations": [...], "description": "..."}
Response: {"id": "...", "seq": 1, "artifact_id": "..."}

# List run patches
GET /api/v1/runs/:run_id/patches
Response: {"patches": [...], "count": 3}

# Get patch operations
GET /api/v1/runs/:run_id/patches/:cas_id/operations
Response: {"operations": [...]}
```

---

## 10. Future Enhancements

### A. Cold Storage Archival
- Move artifacts older than 90 days to S3 Glacier
- Keep metadata in DB, update `storage_url` in cas_blob table
- Lazy rehydration on access

### B. Background Status Updater Worker
```go
// Consumer for run.status.updates stream
func (w *StatusUpdater) Start(ctx context.Context) {
    for {
        updates := redis.XReadGroup("status_updaters", "run.status.updates")
        for _, update := range updates {
            db.UpdateRunStatus(update.RunID, update.Status)
            redis.XAck("run.status.updates", update.ID)
        }
    }
}
```

### C. Reconciliation Job
- Periodic (every hour) scan of Redis `run:status:*` keys
- Compare with DB status
- Backfill missing updates to DB

### D. Workflow Deduplication Metrics
- Track artifact reuse rate
- Report: "80% of runs reused existing artifacts"
- Optimize storage costs

---

## 11. Monitoring & Observability

### Key Metrics

| Metric | Purpose |
|--------|---------|
| `run_creation_duration_ms` | Time to materialize + store |
| `artifact_reuse_rate` | % of runs reusing existing artifacts |
| `redis_status_update_latency` | Hot path performance |
| `db_status_update_lag_seconds` | Cold path lag |
| `run_patches_per_run` | Self-modification activity |

### Alerts

- ⚠️ Redis memory usage > 80%
- ⚠️ DB status lag > 5 minutes
- ⚠️ Artifact fetch failures > 1%
- ⚠️ Run creation failures > 0.1%

---

## 12. Summary

**Run Lifecycle in 5 Steps**:

1. **Create**: Materialize workflow → Store as artifact → Create run entry → Publish to stream
2. **Fetch**: workflow-runner fetches frozen workflow from artifact
3. **Execute**: Compile to IR → Execute nodes → Handle patches
4. **Update**: Status updates to Redis (hot) + Queue for DB (cold)
5. **Complete**: Terminal state → Update both Redis & DB → Publish events

**Key Takeaway**: **Workflows are immutable artifacts**. This simplifies everything: no re-materialization, clear lineage, reproducible runs, and easy diffing.

---

**Related Files**:
- `cmd/orchestrator/service/run.go` - Run creation logic
- `cmd/orchestrator/handlers/run.go` - Run API endpoints
- `cmd/orchestrator/handlers/artifact.go` - Artifact fetch endpoint
- `cmd/workflow-runner/executor/run_request_consumer.go` - Run execution
- `cmd/workflow-runner/coordinator/coordinator.go` - Status management
- `migrations/007_remove_run_patch_id.sql` - Schema update
