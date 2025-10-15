# Redis Keys Reference

> **Hot-path execution state in Redis - complete key inventory**

## 📖 Document Overview

**Purpose:** Complete inventory of all Redis keys with usage patterns and lifecycle

**In this document:**
- [Overview](#overview) - Design principles
- [Key Categories](#key-categories) - Organized by purpose
  - [Workflow State](#1-workflow-state) - IR, context, counter, applied keys
  - [Work Queues](#2-work-queues-redis-streams) - Worker streams, completion signals
  - [HITL](#3-hitl-human-in-the-loop) - Approval tracking
  - [Real-Time Events](#4-real-time-events-pubsub) - WebSocket pub/sub
  - [Caching](#5-caching-optional) - Memoized results
- [Key Lifecycle](#key-lifecycle) - From start to completion
- [Lua Scripts](#lua-scripts-atomic-operations) - Idempotent operations
- [Performance](#performance-characteristics) - Throughput and memory
- [Monitoring](#monitoring--debugging) - Debug commands
- [Complete Inventory](#complete-redis-key-inventory) - Full table

---

## Overview

Redis stores **ephemeral execution state** for high-throughput access. All keys are scoped by `run_id` for isolation and cleanup.

**Design Principle:** Redis = hot path (fast), Postgres = cold path (durable)

---

## Key Categories

### 1. Workflow State

#### `ir:{run_id}` - Compiled Intermediate Representation

**Type:** String (JSON)

**Purpose:** Cached compiled workflow (IR) for fast coordinator access

**Value:**
```json
{
  "nodes": {
    "fetch": {"id": "fetch", "type": "http", "dependents": ["process"]},
    "process": {"id": "process", "type": "function", "dependents": ["save"]},
    "save": {"id": "save", "type": "function", "dependents": []}
  },
  "entry_nodes": ["fetch"],
  "version": "1.2"
}
```

**Operations:**
```bash
# Set IR (on run start or patch apply)
SET ir:run_7f3e4a "{json_ir}"

# Load IR (coordinator on every completion)
GET ir:run_7f3e4a

# Cleanup (on run completion)
DEL ir:run_7f3e4a
```

**Why important:** Coordinator loads this on EVERY completion signal. If patched, coordinator gets NEW topology immediately!

---

#### `context:{run_id}` - Node Outputs

**Type:** Hash

**Purpose:** Store outputs from completed nodes (for agents and downstream nodes)

**Fields:**
```
node_a:output          → "cas://sha256:abc..."
node_a:completed_at    → "2025-10-15T10:30:00Z"
node_b:output          → "cas://sha256:def..."
node_b:duration_ms     → "1234"
```

**Operations:**
```bash
# Store node output
HSET context:run_7f3e4a node_a:output "cas://sha256:abc..."
HSET context:run_7f3e4a node_a:completed_at "2025-10-15T10:30:00Z"

# Load all context (for agent LLM calls)
HGETALL context:run_7f3e4a

# Load specific node output
HGET context:run_7f3e4a node_a:output

# Cleanup
DEL context:run_7f3e4a
```

**Why important:** Agents load this to understand previous execution history before making decisions!

---

#### `counter:{run_id}` - Token Counter (Choreography)

**Type:** Integer

**Purpose:** Track active tokens in flight (completion detection)

**Value:** Integer (number of active tokens)

**Operations:**
```bash
# Initialize run (1 token for entry node)
SET counter:run_7f3e4a 1

# Consume token (-1)
INCRBY counter:run_7f3e4a -1

# Emit tokens to 3 next nodes (+3)
INCRBY counter:run_7f3e4a 3

# Check completion
GET counter:run_7f3e4a  # If 0 and no pending HITL → DONE!

# Cleanup
DEL counter:run_7f3e4a
```

**Why important:** Core of choreography - counter == 0 means all tokens consumed, workflow complete!

---

#### `applied:{run_id}` - Idempotency Keys

**Type:** Set

**Purpose:** Prevent duplicate token processing (idempotency)

**Members:** Operation keys like `consume:token_456`, `emit:token_789`

**Operations:**
```bash
# Check if already applied
SISMEMBER applied:run_7f3e4a "consume:token_456"
# Returns: 1 (already applied) or 0 (not applied)

# Mark as applied (idempotent)
SADD applied:run_7f3e4a "consume:token_456"

# Cleanup
DEL applied:run_7f3e4a
```

**Why important:** Prevents double-processing on Redis redelivery!

---

### 2. Work Queues (Redis Streams)

#### `wf.tasks.{type}` - Worker Streams

**Type:** Stream

**Purpose:** Route work to type-specific workers

**Streams:**
- `wf.tasks.agent` → Agent workers
- `wf.tasks.http` → HTTP workers
- `wf.tasks.hitl` → HITL workers
- `wf.tasks.function` → Function workers

**Operations:**
```bash
# Publish work (coordinator)
XADD wf.tasks.agent * token="{json_token}" run_id="run_7f3e4a" node_id="analyze"

# Consume work (worker)
XREAD BLOCK 5000 STREAMS wf.tasks.agent $

# Consumer groups (multiple workers)
XGROUP CREATE wf.tasks.agent workers $ MKSTREAM
XREADGROUP GROUP workers worker-1 STREAMS wf.tasks.agent >
```

**Why important:** Decouples coordinator from workers, enables horizontal scaling!

---

#### `completion_signals` - Completion Queue

**Type:** List (queue)

**Purpose:** Workers signal completion to coordinator

**Operations:**
```bash
# Worker publishes completion (fire and forget)
RPUSH completion_signals '{"run_id": "run_7f3e4a", "node_id": "fetch", "result_ref": "cas://..."}'

# Coordinator consumes (blocking)
BLPOP completion_signals 0  # Block forever

# Check queue depth
LLEN completion_signals
```

**Why important:** Async completion signaling - workers don't wait for coordinator!

---

### 3. HITL (Human-in-the-Loop)

#### `pending_approvals:{run_id}` - Pending Approval Set

**Type:** Set

**Purpose:** Track which approvals are pending (used in completion detection)

**Members:** Approval IDs

**Operations:**
```bash
# Add pending approval
SADD pending_approvals:run_7f3e4a "approval_456"

# Check count (completion detection)
SCARD pending_approvals:run_7f3e4a
# If > 0, workflow NOT complete (even if counter == 0)

# Remove on decision
SREM pending_approvals:run_7f3e4a "approval_456"

# Cleanup
DEL pending_approvals:run_7f3e4a
```

**Why important:** Prevents false completion detection when workflow is paused for approval!

---

#### `approval:{approval_id}` - Approval Details

**Type:** Hash

**Purpose:** Store approval metadata

**Fields:**
```
run_id              → "run_7f3e4a"
node_id             → "manager_approval"
status              → "pending"
created_at          → "2025-10-15T10:00:00Z"
timeout_at          → "2025-10-15T12:00:00Z"
approver            → "manager@company.com"
payload_cas_ref     → "cas://sha256:..."
```

**Operations:**
```bash
# Create approval
HMSET approval:456 run_id "run_7f3e4a" node_id "manager_approval" status "pending" ...

# Get approval
HGETALL approval:456

# Update status
HSET approval:456 status "approved" decided_at "2025-10-15T11:00:00Z"

# Cleanup (after workflow completes)
DEL approval:456
```

---

### 4. Real-Time Events (Pub/Sub)

#### `run:{run_id}` - Run-Specific Events

**Type:** Pub/Sub channel

**Purpose:** Real-time UI updates for specific run

**Messages:**
```json
{
  "type": "node_completed",
  "node_id": "fetch",
  "timestamp": "2025-10-15T10:30:00Z"
}
```

**Operations:**
```bash
# Publish event
PUBLISH run:run_7f3e4a '{"type": "node_completed", "node_id": "fetch"}'

# Subscribe (Fanout service)
SUBSCRIBE run:run_7f3e4a

# Pattern subscribe (all runs)
PSUBSCRIBE run:*
```

**Why important:** Powers real-time WebSocket updates in UI!

---

### 5. Caching (Optional)

#### `cache:{scope}:{key}` - Memoized Node Results

**Type:** String (CAS reference)

**Purpose:** Cache deterministic node outputs

**Example:**
```
cache:workflow:enrich_A:sha256:abc123 → "cas://sha256:xyz789"
```

**Operations:**
```bash
# Check cache
GET cache:workflow:enrich_A:sha256:abc123

# Store result
SET cache:workflow:enrich_A:sha256:abc123 "cas://sha256:xyz789" EX 3600

# Invalidate
DEL cache:workflow:enrich_A:sha256:abc123
```

---

## Key Lifecycle

### Run Start

```bash
# 1. Store IR
SET ir:run_7f3e4a "{ir_json}"

# 2. Initialize counter
SET counter:run_7f3e4a 1

# 3. Initialize context
# (empty, populated as nodes complete)

# 4. Initialize applied set
# (empty, populated as operations execute)
```

### During Execution

```bash
# Node completion
HSET context:run_7f3e4a node_a:output "cas://..."
SADD applied:run_7f3e4a "consume:token_123"
INCRBY counter:run_7f3e4a -1
INCRBY counter:run_7f3e4a 2  # Emit to 2 next nodes
```

### Agent Patch

```bash
# Patch applied
SET ir:run_7f3e4a "{new_ir_with_patch}"
# Next coordinator load gets NEW IR!
```

### HITL Pause

```bash
# Create approval
SADD pending_approvals:run_7f3e4a "approval_456"
HMSET approval:456 run_id "run_7f3e4a" status "pending" ...
```

### Run Completion

```bash
# Check completion
counter = GET counter:run_7f3e4a  # 0?
pending = SCARD pending_approvals:run_7f3e4a  # 0?
# Both 0 → COMPLETED!

# Cleanup
DEL ir:run_7f3e4a
DEL context:run_7f3e4a
DEL counter:run_7f3e4a
DEL applied:run_7f3e4a
DEL pending_approvals:run_7f3e4a
```

---

## Key Naming Conventions

### Pattern

```
{category}:{scope}:{identifier}
```

**Examples:**
- `ir:run_7f3e4a` - IR for run
- `context:run_7f3e4a` - Context for run
- `counter:run_7f3e4a` - Counter for run
- `approval:456` - Approval details
- `cache:workflow:enrich:abc` - Cached result

### Why Consistent Naming?

- Easy to find all keys for a run: `KEYS *:run_7f3e4a`
- Easy to cleanup: `DEL ir:run_7f3e4a context:run_7f3e4a counter:run_7f3e4a`
- Easy to monitor: `INFO keyspace` shows key distribution

---

## Memory Management

### Key Expiration

**Automatic cleanup:**
```bash
# Set TTL on run start (24 hours)
EXPIRE ir:run_7f3e4a 86400
EXPIRE context:run_7f3e4a 86400
EXPIRE counter:run_7f3e4a 86400

# On completion: explicit DEL (don't wait for expiry)
```

### Memory Monitoring

```bash
# Check memory usage
INFO memory

# Key count
DBSIZE

# Largest keys
redis-cli --bigkeys

# Memory per key type
redis-cli --memkeys
```

---

## Redis Configuration

### Recommended Settings

```
# redis.conf

# Memory
maxmemory 16gb
maxmemory-policy allkeys-lru  # Evict least recently used

# Persistence (for durability)
appendonly yes
appendfsync everysec

# Keyspace notifications (for expiry events)
notify-keyspace-events Ex

# Max clients
maxclients 10000
```

### Monitoring Commands

```bash
# Real-time operations
redis-cli MONITOR

# Stats
redis-cli INFO stats

# Slow queries
redis-cli SLOWLOG get 10

# Connected clients
redis-cli CLIENT LIST
```

---

## Complete Redis Key Inventory

| Key Pattern | Type | Purpose | Lifecycle |
|------------|------|---------|-----------|
| `ir:{run_id}` | String | Compiled workflow IR | Start → Complete |
| `context:{run_id}` | Hash | Node outputs | Start → Complete |
| `counter:{run_id}` | Integer | Token counter | Start → Complete |
| `applied:{run_id}` | Set | Idempotency keys | Start → Complete |
| `wf.tasks.{type}` | Stream | Worker queues | Persistent |
| `completion_signals` | List | Completion queue | Persistent |
| `pending_approvals:{run_id}` | Set | HITL tracking | HITL start → Decision |
| `approval:{id}` | Hash | Approval details | HITL start → Complete |
| `run:{run_id}` | Pub/Sub | Real-time events | Start → Complete |
| `cache:{scope}:{key}` | String | Memoized results | On cache, TTL 1h |

---

## Example: Complete Run Lifecycle

### Start (t=0)

```bash
SET ir:run_123 "{...compiled_ir...}"
SET counter:run_123 1
XADD wf.tasks.http * run_id "run_123" node_id "fetch"
```

### Execution (t=5s)

```bash
# fetch completes
HSET context:run_123 fetch:output "cas://sha256:abc"
SADD applied:run_123 "consume:token_001"
INCRBY counter:run_123 -1  # 1 → 0
INCRBY counter:run_123 1   # 0 → 1 (emit to process)
RPUSH completion_signals '{"run_id": "run_123", "node_id": "fetch"}'
```

### Agent Patch (t=10s)

```bash
# Agent adds email node
SET ir:run_123 "{...new_ir_with_email_node...}"
# Coordinator loads NEW IR on next completion!
```

### HITL (t=15s)

```bash
# HITL node hit
SADD pending_approvals:run_123 "approval_456"
HMSET approval:456 run_id "run_123" status "pending"
INCRBY counter:run_123 -1  # Counter → 0
# But workflow NOT complete (pending approval)
```

### Approval (t=20min)

```bash
# Human approves
SREM pending_approvals:run_123 "approval_456"
HSET approval:456 status "approved"
INCRBY counter:run_123 1  # Emit to next node
RPUSH completion_signals '{"run_id": "run_123", "node_id": "manager_approval"}'
```

### Completion (t=25min)

```bash
# Check
GET counter:run_123         # → 0
SCARD pending_approvals:run_123  # → 0
# COMPLETED!

# Cleanup
DEL ir:run_123
DEL context:run_123
DEL counter:run_123
DEL applied:run_123
DEL pending_approvals:run_123
```

---

## Lua Scripts (Atomic Operations)

### apply_delta.lua - Idempotent Counter Update

**Purpose:** Atomically apply counter delta with idempotency

```lua
-- Keys: applied_set, counter_key
-- Args: op_key, delta

local applied_set = KEYS[1]
local counter_key = KEYS[2]
local op_key = ARGV[1]
local delta = tonumber(ARGV[2])

-- Check if already applied
if redis.call('SISMEMBER', applied_set, op_key) == 1 then
    return 0  -- Already applied (idempotent)
end

-- Add to applied set
redis.call('SADD', applied_set, op_key)

-- Update counter
local new_value = redis.call('INCRBY', counter_key, delta)

return new_value
```

**Usage:**
```go
// Go code
result := redis.Eval(ctx, applyDeltaScript,
    []string{"applied:run_123", "counter:run_123"},
    "consume:token_456", -1)
```

**Why important:** Guarantees exactly-once counter updates even with Redis redelivery!

---

## Performance Characteristics

### Throughput

| Operation | Throughput | Latency |
|-----------|-----------|---------|
| GET (IR) | 100K ops/sec | <1ms |
| HSET (context) | 80K ops/sec | <1ms |
| INCRBY (counter) | 100K ops/sec | <1ms |
| SADD (applied) | 100K ops/sec | <1ms |
| Lua script (apply_delta) | 80K ops/sec | <2ms |
| XADD (stream) | 50K msgs/sec | <5ms |
| RPUSH (queue) | 100K ops/sec | <1ms |

### Memory Usage

**Per run (typical):**
- `ir:{run_id}`: 10-50KB (compiled workflow)
- `context:{run_id}`: 1-10KB (node outputs, CAS refs only)
- `counter:{run_id}`: 8 bytes
- `applied:{run_id}`: 100-1000 bytes (set of keys)
- Total: **~20-70KB per active run**

**1,000 active runs:** ~20-70MB in Redis

---

## Monitoring & Debugging

### Check All Keys for a Run

```bash
# List all keys for run_123
redis-cli KEYS *run_123*

# Output:
# ir:run_123
# context:run_123
# counter:run_123
# applied:run_123
# pending_approvals:run_123
```

### Debug Workflow State

```bash
# 1. Check IR
redis-cli GET ir:run_123 | jq .

# 2. Check counter
redis-cli GET counter:run_123

# 3. Check context (node outputs)
redis-cli HGETALL context:run_123

# 4. Check pending approvals
redis-cli SCARD pending_approvals:run_123

# 5. Check applied operations
redis-cli SCARD applied:run_123
redis-cli SMEMBERS applied:run_123 | head -10
```

### Monitor Queues

```bash
# Stream depth
redis-cli XLEN wf.tasks.agent
redis-cli XLEN wf.tasks.http

# Queue depth
redis-cli LLEN completion_signals

# Consumer groups
redis-cli XINFO GROUPS wf.tasks.agent
redis-cli XINFO CONSUMERS wf.tasks.agent workers
```

### Performance Monitoring

```bash
# Ops per second
redis-cli INFO stats | grep instantaneous_ops_per_sec

# Hit rate
redis-cli INFO stats | grep keyspace_hits

# Memory
redis-cli INFO memory | grep used_memory_human

# Connections
redis-cli INFO clients | grep connected_clients
```

---

## Cleanup Strategy

### Manual Cleanup

```bash
# Cleanup specific run
redis-cli DEL ir:run_123 context:run_123 counter:run_123 applied:run_123 pending_approvals:run_123

# Cleanup all runs (DANGEROUS!)
redis-cli KEYS "ir:*" | xargs redis-cli DEL
```

### Automatic Cleanup

```go
// On run completion
func (c *Coordinator) cleanup(runID string) {
    keys := []string{
        "ir:" + runID,
        "context:" + runID,
        "counter:" + runID,
        "applied:" + runID,
        "pending_approvals:" + runID,
    }

    for _, key := range keys {
        c.redis.Del(ctx, key)
    }
}
```

### TTL-Based Cleanup (Failsafe)

```bash
# Set TTL on all run keys (24 hours)
EXPIRE ir:run_123 86400
EXPIRE context:run_123 86400
EXPIRE counter:run_123 86400
```

---

## Redis Cluster Considerations

### Key Distribution

```
Keys are distributed by hash slot:
  ir:run_123 → hash("ir:run_123") → slot 1234 → node 1
  context:run_123 → hash("context:run_123") → slot 5678 → node 2

Problem: Keys for same run on different nodes!
```

### Solution: Hash Tags

```bash
# Use {run_id} hash tag
SET ir:{run_123} "..."
SET context:{run_123} "..."
SET counter:{run_123} 1

# All keys with same {run_123} go to same slot!
```

**Benefits:**
- All keys for a run on same Redis node
- Multi-key operations work (MGET, DEL multiple)
- Better locality

---

## Summary

### Hot Path (Redis)
✅ Compiled IR (fast coordinator access)
✅ Node outputs (for agent context)
✅ Token counter (choreography)
✅ Idempotency keys (prevent duplicates)
✅ Work queues (horizontal scaling)
✅ Completion signals (async coordination)
✅ HITL state (pause/resume)

### Cold Path (Postgres)
✅ Run metadata (durable)
✅ Patches (audit trail)
✅ Artifacts (versioning)
✅ Event log (replay)

**Redis = speed, Postgres = durability!**

---

**Complete key inventory documented for operations and debugging.**
