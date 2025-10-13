# Choreographed Workflow Execution Engine - Complete Design

**Version**: 1.0
**Date**: 2025-10-13
**Status**: Design Review

---

## Table of Contents

1. [Overview](#overview)
2. [Core Directives & Design Validation](#core-directives--design-validation)
3. [Architecture](#architecture)
4. [Token Counter System](#token-counter-system)
5. [Data Structures](#data-structures)
6. [Node Types](#node-types)
7. [HITL (Human-in-the-Loop)](#hitl-human-in-the-loop)
8. [Event Sourcing & Replay](#event-sourcing--replay)
9. [Execution Flow Examples](#execution-flow-examples)
10. [Scalability & Performance](#scalability--performance)
11. [Implementation Phases](#implementation-phases)
12. [Open Questions](#open-questions)

---

## Overview

### What We're Building

A distributed, choreographed workflow execution engine that supports:
- **Deterministic DAG execution** via token-based choreography
- **Agentic routing** where LLMs make runtime decisions
- **Human-in-the-loop** approvals with pause/resume
- **Event-driven architecture** for all control flow
- **Persistent, replayable state** via event sourcing
- **Visual orchestration** through DSL + patches
- **Horizontal scalability** with Redis hot path

### Key Principles

1. **No central orchestrator** - workers autonomous, tokens flow peer-to-peer
2. **Counter-based completion** - active token tracking with idempotent operations
3. **CAS for data** - content-addressable storage keeps platform lightweight
4. **Redis hot path** - 100K+ ops/sec, Postgres for durable state
5. **Node autonomy** - nodes own their config schemas, platform agnostic

---

## Core Directives & Design Validation

| Directive | Design Solution | Status |
|-----------|----------------|--------|
| **Deterministic DAG execution** | Token choreography with edges, idempotent consume/emit | ✅ Covered |
| **Agentic, context-driven handoffs** | Agent node type with LLM routing, context store in Redis | ✅ Designed |
| **Events as control plane** | Redis Streams for messages, event log for state changes | ✅ Covered |
| **Persistent, replayable state** | Event sourcing table + CAS, reconstruct from event log | ✅ Designed |
| **Human review/intervention** | HITL node type, approval system, pending tracking | ✅ Designed |
| **Visual orchestration** | DSL → IR compiler, patches for live editing | ✅ Covered |
| **Scalability** | Redis hot path, horizontal workers, no DB hot row | ✅ Covered |
| **Resilience** | Idempotent tokens, CAS immutability, applied_keys dedup | ✅ Covered |
| **Extensibility** | Node type registry, plug-in worker discovery | ✅ Designed |

---

## Architecture

### System Components

```
┌─────────────────────────────────────────────────────────────────┐
│                         Orchestration Platform                   │
├─────────────────────────────────────────────────────────────────┤
│                                                                   │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │    User      │  │   UI/API     │  │  HITL Service│          │
│  │  (submit)    │→ │  (Gateway)   │← │  (external)  │          │
│  └──────────────┘  └──────────────┘  └──────────────┘          │
│                           ↓                                       │
│                    ┌──────────────┐                              │
│                    │ Run Supervisor│                             │
│                    │ (completion   │                             │
│                    │  detection)   │                             │
│                    └──────────────┘                              │
│                           ↓                                       │
│         ┌─────────────────┴─────────────────┐                   │
│         ↓                                     ↓                   │
│  ┌──────────────┐                    ┌──────────────┐           │
│  │    Redis     │←─────tokens───────→│   Workers    │           │
│  │   Streams    │                    │  (task/agent │           │
│  │              │                    │   /human)    │           │
│  │  Counters    │                    └──────────────┘           │
│  │  Applied Keys│                           ↓                    │
│  │  Context     │                    ┌──────────────┐           │
│  └──────────────┘                    │     CAS      │           │
│         ↓                             │  (S3/MinIO) │           │
│  ┌──────────────┐                    │  Payloads    │           │
│  │  Postgres    │                    │  Configs     │           │
│  │  (durable    │                    └──────────────┘           │
│  │   state)     │                                                │
│  │  - runs      │                                                │
│  │  - event_log │                                                │
│  │  - approvals │                                                │
│  └──────────────┘                                                │
└─────────────────────────────────────────────────────────────────┘
```

### Data Flow

```
1. Submit Run
   User → API: POST /runs {tag: "main", inputs: {...}}

2. Resolve & Compile
   API → Artifact: Resolve tag → artifact_id
   API → CAS: Materialize workflow (base + patches)
   API → IR Compiler: DSL → Executable IR
   API → CAS: Store IR → ir_cas_ref

3. Initialize Run
   API → Postgres: INSERT INTO runs (artifact_id, ir_cas_ref, ...)
   API → Redis: SET counter:run_123 1
   API → Redis: Publish seed token to entry nodes

4. Execution (Choreography)
   Worker → Redis: Consume token from stream
   Worker → SDK: Consume() → apply -1 to counter (idempotent)
   Worker → CAS: Load config, load payload
   Worker → Business Logic: Execute node
   Worker → SDK: Emit(next_nodes) → apply +N to counter
   Worker → Redis: Publish N tokens to next nodes

5. Completion Detection
   Supervisor → Redis: GET counter:run_123 → 0?
   Supervisor → Redis: SCARD pending_approvals:run_123 → 0?
   Supervisor → Postgres: UPDATE runs SET status='COMPLETED'
   Supervisor → Cleanup: DEL counter:run_123, applied:run_123
```

---

## Coordinator Architecture (MVP)

### Overview

For MVP, we use a **hybrid choreography model** where a coordinator handles routing logic while workers execute business logic. This provides a pragmatic balance between simplicity and performance.

**Key Insight**: The coordinator is in the hot path for MVP, but the architecture supports migration to pure choreography when needed.

### Worker-Coordinator Flow

```
┌──────────────────────────────────────────────────────────────┐
│                    Worker (Any Language)                      │
│                                                               │
│  1. Consume token from stream (wf.tasks.{type})              │
│  2. Execute business logic                                   │
│  3. Store result in CAS                                      │
│  4. Signal completion:                                       │
│     RPUSH completion_signals {run_id, node_id, result_ref}  │
│                                                               │
└──────────────────┬───────────────────────────────────────────┘
                   │
                   ↓ (fire and forget, async)
┌──────────────────────────────────────────────────────────────┐
│                   Coordinator (Go)                            │
│                                                               │
│  1. BLPOP completion_signals                                 │
│  2. Load latest IR from Redis: ir:{run_id}                   │
│  3. Choreography:                                            │
│     - ApplyDelta(run_id, "consume:...", -1)                  │
│     - Determine next nodes from IR                           │
│     - Route to appropriate streams by node type              │
│     - ApplyDelta(run_id, "emit:...", +N)                     │
│     - Publish tokens to worker streams                       │
│  4. If terminal node: check counter == 0 → mark COMPLETED    │
│                                                               │
└──────────────────────────────────────────────────────────────┘
```

### Stream-Based Routing

Workers consume from type-specific streams:

```redis
# Stream routing by node type
wf.tasks.agent       → Agent-Runner-Py workers
wf.tasks.classifier  → Classifier workers
wf.tasks.search      → Search workers
wf.tasks.function    → Function workers
wf.tasks.http        → HTTP workers
```

**Coordinator determines stream name:**

```go
func (c *Coordinator) getStreamForNodeType(nodeType string) string {
    switch nodeType {
    case "agent":      return "wf.tasks.agent"
    case "classifier": return "wf.tasks.classifier"
    case "search":     return "wf.tasks.search"
    case "function":   return "wf.tasks.function"
    case "http":       return "wf.tasks.http"
    default:           return "wf.tasks.default"
    }
}
```

### Worker Interface

All workers follow the same simple interface:

**1. Consume from stream:**
```go
token := redis.XREAD("wf.tasks.{type}")
```

**2. Execute business logic:**
```go
result := executeBusiness(token.payload)
```

**3. Store result:**
```go
resultRef := cas.Put(result)
```

**4. Signal completion:**
```go
redis.RPUSH("completion_signals", {
    run_id: token.run_id,
    node_id: token.node_id,
    result_ref: resultRef,
    status: "completed"
})
```

**No choreography logic in workers!** They just execute and signal.

### Completion Signaling

**Signal format:**
```json
{
  "run_id": "run_123",
  "node_id": "process_data",
  "result_ref": "cas://sha256:abc...",
  "status": "completed",
  "metadata": {
    "execution_time_ms": 1234,
    "retries": 0
  }
}
```

### Patch Workflow Integration

**Mid-flight patches work seamlessly:**

```
1. Agent worker calls LLM
2. LLM decides to patch workflow
3. Agent forwards to Orchestrator: POST /runs/{run_id}/patch
4. Orchestrator:
   - Applies JSON Patch operations
   - Recompiles IR
   - Updates Redis: SET ir:{run_id} new_ir
5. Agent signals completion
6. Coordinator loads IR (gets NEW version with added nodes)
7. Coordinator emits to new nodes from patched IR
```

**Key**: Coordinator always loads latest IR (no caching!) so patches apply immediately.

### Performance Characteristics

**MVP (Coordinator in hot path):**
- Latency: 5-10ms per hop
- Throughput: ~1000 tokens/sec (single coordinator)
- Bottleneck: Coordinator (but can scale with consumer groups)

**Benefits:**
- Simple to debug (centralized logic)
- Easy to add new worker types (just add stream)
- Fast to implement (1 day)

**Trade-offs accepted:**
- Coordinator in hot path (acceptable for MVP)
- Single point of coordination (mitigated by consumer groups)

### Migration Path to Pure Choreography

**When to migrate:**
- Coordinator becomes bottleneck (>1000 workflows/sec)
- Latency requirements tighten (<5ms per hop)
- Need geo-distributed workers

**How to migrate:**

**Phase 1 (MVP - Now):** Coordinator does routing
```
Worker → Signal → Coordinator → Route → Next worker
         (2ms)    (3ms)        (2ms)
Total: 7ms per hop
```

**Phase 2 (Hybrid):** Workers handle simple routing
```
Worker → SDK (check if simple) → If simple: Route directly
                                → If complex: Signal coordinator
```

**Phase 3 (Pure choreography):** Workers fully autonomous
```
Worker → SDK (full routing) → Publish directly to next stream
         (1ms)
Total: 2ms per hop
```

**Migration is incremental**: Add SDK to workers gradually, coordinator becomes optional monitoring service.

### Redis Data Structures

```redis
# IR storage (latest compiled workflow)
SET ir:{run_id} {json_ir}

# Completion signals (queue)
RPUSH completion_signals {signal_json}
BLPOP completion_signals 5  # Coordinator consumes

# Counter (choreography state)
SET counter:{run_id} {value}

# Applied keys (idempotency)
SADD applied:{run_id} "consume:..."

# Context (node outputs)
HSET context:{run_id} {node_id}:output {cas_ref}

# Pending tokens (join pattern)
SADD pending_tokens:{run_id}:{node_id} {from_node}
```

### Coordinator Components

**1. Main Loop (completion_handler.go):**
```go
func (c *Coordinator) Start() {
    for {
        // Block waiting for completion signals
        signal := c.redis.BLPop("completion_signals", 0)

        // Handle in goroutine (parallel processing)
        go c.handleCompletion(signal)
    }
}
```

**2. Choreography Handler:**
```go
func (c *Coordinator) handleCompletion(signal CompletionSignal) {
    // Load latest IR (might be patched!)
    ir := c.loadIR(signal.RunID)
    node := ir.Nodes[signal.NodeID]

    // Consume token
    c.sdk.Consume(signal.RunID, signal.NodeID)

    // Determine next nodes (branches, loops, etc.)
    nextNodes := c.determineNextNodes(node, signal.ResultRef)

    // Emit to appropriate streams
    for _, nextNode := range nextNodes {
        stream := c.getStreamForNodeType(nextNode.Type)
        c.publishToken(stream, signal.RunID, signal.NodeID, nextNode.ID)
    }

    // Emit counter update
    c.sdk.Emit(signal.RunID, signal.NodeID, nextNodes)

    // Terminal check
    if node.IsTerminal {
        c.checkCompletion(signal.RunID)
    }
}
```

**3. Stream Router (router.go):**
```go
func (c *Coordinator) publishToken(stream, runID, fromNode, toNode string) {
    token := Token{
        ID:       uuid.New().String(),
        RunID:    runID,
        FromNode: fromNode,
        ToNode:   toNode,
        Hop:      token.Hop + 1,
    }

    c.redis.XAdd(stream, "*",
        "token", token.ToJSON(),
        "run_id", runID,
        "node_id", toNode)
}
```

**4. Completion Supervisor (completion.go):**
```go
func (c *Coordinator) checkCompletion(runID string) {
    counter := c.redis.Get("counter:" + runID).Int()

    if counter == 0 {
        // Verify no pending work
        noPending := c.verifyNoPendingWork(runID)

        if noPending {
            // Mark as COMPLETED
            c.db.Exec("UPDATE run SET status='COMPLETED' WHERE run_id=?", runID)

            // Cleanup Redis
            c.cleanup(runID)
        }
    }
}
```

### Example: Agent Node Flow

```
1. Coordinator receives completion from previous node
2. Coordinator checks IR: next node type = "agent"
3. Coordinator publishes to wf.tasks.agent stream
4. Agent-Runner-Py consumes from wf.tasks.agent
5. Agent calls LLM, executes tools
6. Agent stores result in CAS
7. Agent signals: RPUSH completion_signals {...}
8. Coordinator receives signal
9. Coordinator loads IR (might be patched by agent!)
10. Coordinator determines next nodes from (possibly new) IR
11. Coordinator publishes to appropriate streams
12. Process continues...
```

### Scalability

**Coordinator scaling:**
- Single coordinator: ~1000 tokens/sec
- Consumer group (3 coordinators): ~3000 tokens/sec
- Add more coordinators as needed (linear scaling)

**Worker scaling:**
- Independent per type
- Add agent workers ≠ add function workers
- Scale based on load per type

---

## Token Counter System

### The Core Invariant

```
Counter = Number of active tokens in flight

Counter = Σ(all emits) - Σ(all consumes)

When counter = 0 AND no pending work:
  → All tokens emitted have been consumed
  → No node is waiting to emit
  → Workflow complete!
```

### Idempotent Operations

Every counter operation has a unique `op_key` and is applied exactly once:

```lua
-- apply_delta.lua (Redis Lua script - atomic)
local applied_set = KEYS[1]       -- "applied:run_123"
local counter_key = KEYS[2]       -- "counter:run_123"
local op_key = ARGV[1]            -- "consume:token_456" or "emit:token_456"
local delta = tonumber(ARGV[2])   -- -1 for consume, +N for emit

-- 1. Check if already applied
if redis.call('SISMEMBER', applied_set, op_key) == 1 then
    return 0  -- Already applied, skip (idempotency)
end

-- 2. Add to applied set (prevent future duplicates)
redis.call('SADD', applied_set, op_key)

-- 3. Update counter
local new_value = redis.call('INCRBY', counter_key, delta)

return new_value
```

### Example Counter Evolution

```
Workflow: A → (B, C) → D

Step  Event                           Counter  Applied Keys
─────────────────────────────────────────────────────────────────
 0    Initialize run                     1     []

 1    A consumes                         0     [consume:token_001]
 2    A emits to B,C                     2     [..., emit:token_001]

 3    B consumes                         1     [..., consume:token_002]
 4    B emits to D                       2     [..., emit:token_002]

 5    C consumes                         1     [..., consume:token_003]
 6    C emits to D                       2     [..., emit:token_003]

 7    D consumes first token             1     [..., consume:token_004]
 8    D wait for second token...         1     (wait_for_all: true)

 9    D consumes second token            0     [..., consume:token_005]
10    D completes (no emit)              0

Supervisor: counter=0, no pending work → COMPLETED!
```

### Handling Duplicate Deliveries

Redis redelivers token_002 due to network hiccup:

```
T1: Worker A receives token_002
    → SDK calls apply("consume:token_002", -1)
    → SADD applied:run_123 "consume:token_002" (success)
    → INCRBY counter:run_123 -1
    → Business logic executes

T2: Redis redelivers token_002
    → Worker B receives same token_002
    → SDK calls apply("consume:token_002", -1)
    → SADD fails (already in set!)
    → INCRBY not called
    → Business logic NOT executed
    → Returns "already processed"
```

---

## Terminal Node Optimization

### The Problem: Unnecessary Completion Checks

Without optimization, we'd check completion after EVERY node execution:

```
Workflow: A → B → C → D → E (terminal)

A executes → check counter=0? (unnecessary)
B executes → check counter=0? (unnecessary)
C executes → check counter=0? (unnecessary)
D executes → check counter=0? (unnecessary)
E executes → check counter=0? DONE! ✓
```

**Waste:** 4 unnecessary checks for a 5-node workflow.

### The Solution: Pre-compute + Runtime Check

**Compile time:**
1. Traverse IR graph
2. Find nodes with zero outgoing edges
3. Mark them: `"is_terminal": true`

**Runtime:**
- Only terminal nodes check completion
- Non-terminal nodes skip check entirely

### Terminal Node Detection Algorithm

```go
func computeTerminalNodes(ir *IR) {
    for nodeID, node := range ir.Nodes {
        // Static check: no dependents
        hasStaticDependents := len(node.Dependents) > 0

        // Dynamic check: no branch/loop that emits
        hasBranch := node.Branch != nil && node.Branch.Enabled
        hasLoop := node.Loop != nil && node.Loop.Enabled

        // Terminal if:
        // - No static dependents
        // - No branch config (or all branch paths empty)
        // - No loop config (or loop break_path is empty)

        if !hasStaticDependents && !hasBranch && !hasLoop {
            node.IsTerminal = true
        } else if hasBranch {
            // Check if all branch paths are empty
            allPathsEmpty := true
            for _, rule := range node.Branch.Rules {
                if len(rule.NextNodes) > 0 {
                    allPathsEmpty = false
                    break
                }
            }
            if allPathsEmpty && len(node.Branch.Default) == 0 {
                node.IsTerminal = true
            }
        } else if hasLoop {
            // Check if break_path is empty (loop exits to nowhere)
            if len(node.Loop.BreakPath) == 0 && len(node.Loop.TimeoutPath) == 0 {
                node.IsTerminal = true
            }
        }
    }
}
```

### Runtime Terminal Check

```go
func (w *Worker) Execute(token Token) error {
    ctx := w.loadContext(token)

    // 1. Consume token
    w.sdk.Consume(token)

    // 2. Execute business logic
    output := w.executeNode(ctx)

    // 3. Emit to next nodes
    w.sdk.Emit(ctx.Node.Dependents, output)

    // 4. *** Terminal node completion check ***
    if ctx.Node.IsTerminal {
        counter := w.redis.Get("counter:" + token.RunID).Int()
        if counter == 0 {
            w.markCompleted(token.RunID)
            w.log.Info("workflow completed", "run_id", token.RunID)
        }
    }

    return nil
}
```

### How It Works With Branching

```
Workflow: A → Branch → (high_value ⓣ, low_value ⓣ, medium_value ⓣ)

Pre-computed terminals: [high_value, low_value, medium_value]

Runtime (branch chooses high_value):
1. A executes → not terminal → no check
2. Branch executes → not terminal → no check
3. Branch emits to high_value only
4. high_value executes → IS TERMINAL → check counter=0 → DONE ✅

low_value and medium_value never execute (that's fine!)
```

### Dual Completion Strategy

We use **both** terminal node checks AND event-driven completion:

| Mechanism | Trigger | Purpose |
|-----------|---------|---------|
| **Terminal node check** | Node with `is_terminal=true` completes | Normal case (99% of workflows) |
| **Event-driven (Lua pub)** | Counter hits 0 | Backup for edge cases |
| **Timeout detector** | No activity for 5 min | Detect hanging workflows |

**Why both?**
- Terminal check: Fast, no pub/sub overhead, catches 99% of completions
- Event-driven: Safety net for complex scenarios (multiple terminals racing, loops, etc.)
- Timeout: Last resort for truly stuck workflows

### Performance Impact

**Before optimization:**
- Completion checks per workflow: O(N) where N = total nodes
- Example (10-node workflow): 10 checks

**After optimization:**
- Completion checks per workflow: O(T) where T = terminal nodes
- Example (10-node workflow, 2 terminals): 2 checks
- **Reduction: 80%** for typical workflows

### Edge Case: No Terminal Nodes

What if a workflow has no terminal nodes? (e.g., infinite loop)

```go
// IR validation
func (ir *IR) Validate() error {
    terminals := ir.CountTerminalNodes()

    if terminals == 0 && !ir.HasInfiniteLoopFlag {
        return fmt.Errorf("workflow has no terminal nodes (would run forever)")
    }

    return nil
}
```

---

## Join Pattern & Path Tracking

### The Problem: Parallel Fan-In

```
fetch_user (A) ──┐
                  ├──> merge_data (D) ──> send_email (E)
fetch_orders (B) ─┘
```

**Node D must wait for BOTH A and B** before executing.

### Token Structure with Path Tracking

Every token carries execution metadata:

```go
type Token struct {
    ID          string    // Unique token ID
    RunID       string    // Workflow instance
    FromNode    string    // Sender (path tracking!)
    ToNode      string    // Receiver
    PayloadRef  string    // CAS reference
    Hop         int       // Hop count
    CreatedAt   time.Time
}
```

**The `FromNode` field serves multiple purposes:**
1. **Join pattern** - Know which dependencies have sent tokens
2. **Path tracking** - Record execution path per run
3. **Observability** - Query "which path did run X take?"
4. **Debugging** - "Why didn't node Y execute?"
5. **Idempotency** - Unique keys per sender→receiver pair

### Node Configuration

```json
{
  "id": "merge_data",
  "type": "task",
  "dependencies": ["fetch_user", "fetch_orders"],
  "wait_for_all": true
}
```

### Execution Flow

```
1. Init: counter = 2

2. fetch_user consumes: counter = 1
3. fetch_user emits: counter = 2
   Token: {from: "fetch_user", to: "merge_data"}

4. fetch_orders consumes: counter = 1
5. fetch_orders emits: counter = 2
   Token: {from: "fetch_orders", to: "merge_data"}

6. merge_data receives first token (from fetch_user):
   - Check: wait_for_all=true, dependencies=["fetch_user", "fetch_orders"]
   - SADD pending_tokens:run_123:merge_data "fetch_user"
   - SCARD → 1 (need 2)
   - Don't consume yet, counter stays at 2

7. merge_data receives second token (from fetch_orders):
   - SADD pending_tokens:run_123:merge_data "fetch_orders"
   - SCARD → 2 (all satisfied!)
   - Consume BOTH tokens: counter = 2 - 2 = 0
   - Execute with merged payloads
   - Emit to send_email: counter = 1

8. send_email executes: counter = 0 → DONE
```

### Implementation

```go
func (w *TaskWorker) handleJoin(ctx context.Context, token *Token, node *Node) error {
    pendingKey := fmt.Sprintf("pending_tokens:%s:%s", token.RunID, token.ToNode)

    // 1. Store this token
    w.redis.SAdd(ctx, pendingKey, token.FromNode)

    // 2. Check if all dependencies satisfied
    received := w.redis.SCard(ctx, pendingKey).Val()
    if received < int64(len(node.Dependencies)) {
        // Still waiting
        w.logger.Info("join waiting",
            "node", token.ToNode,
            "received", received,
            "expected", len(node.Dependencies))
        return nil
    }

    // 3. All tokens received! Consume them all
    for _, dep := range node.Dependencies {
        opKey := fmt.Sprintf("consume:%s:%s->%s", token.RunID, dep, token.ToNode)
        w.sdk.ApplyDelta(ctx, token.RunID, opKey, -1)
    }

    // 4. Load all dependency outputs
    payloads := make(map[string]interface{})
    for _, dep := range node.Dependencies {
        outputRef := w.redis.HGet(ctx, "context:"+token.RunID, dep+":output").Val()
        payloads[dep] = w.sdk.LoadPayload(outputRef)
    }

    // 5. Execute business logic with merged data
    output := w.executeBusiness(node, payloads)

    // 6. Emit to next nodes
    outputRef := w.sdk.StoreOutput(output)
    w.sdk.Emit(ctx, token.RunID, token.ToNode, node.Dependents, outputRef)

    // 7. Clean up
    w.redis.Del(ctx, pendingKey)

    // 8. Terminal check
    if node.IsTerminal {
        w.checkCompletion(ctx, token.RunID)
    }

    return nil
}
```

### Path Tracking Benefits

**1. Observability - Which path did run X take?**

```sql
SELECT from_node, to_node, created_at
FROM event_log
WHERE run_id = 'run_123'
  AND event_type = 'token.emitted'
ORDER BY created_at;

-- Shows exact execution path per run
```

**2. Debugging - Why didn't node execute?**

```redis
SMEMBERS pending_tokens:run_456:merge_data
→ ["fetch_user"]  # Never got fetch_orders!
```

**3. Idempotency - Unique keys per sender→receiver**

```go
opKey := "consume:run_123:fetch_user->merge_data"
// Different from "consume:run_123:fetch_orders->merge_data"
```

### Redis Data Structures

```redis
# Pending tokens (join pattern)
SADD pending_tokens:run_123:merge_data "fetch_user"
SADD pending_tokens:run_123:merge_data "fetch_orders"
SCARD pending_tokens:run_123:merge_data  # → 2

# Applied keys (idempotency)
SADD applied:run_123 "consume:run_123:fetch_user->merge_data"
SADD applied:run_123 "consume:run_123:fetch_orders->merge_data"

# Context (outputs for merging)
HSET context:run_123 "fetch_user:output" "cas://sha256:abc"
HSET context:run_123 "fetch_orders:output" "cas://sha256:def"
```

### Example: Triple Fan-In

```
A ──┐
B ──┼──> E (wait_for_all: true)
C ──┘
```

E only executes when it receives tokens from A, B, AND C:

```
Token 1: {from: "A", to: "E"} → pending_tokens = ["A"]
Token 2: {from: "B", to: "E"} → pending_tokens = ["A", "B"]
Token 3: {from: "C", to: "E"} → pending_tokens = ["A", "B", "C"] ✓
Execute!
```

### Real-World Example: Data Enrichment

```
                  ┌──> fetch_weather (B) ──┐
                  │                         │
start (A) ───────┼──> fetch_traffic (C) ───┼──> combine (E) ──> display (F)
                  │                         │
                  └──> fetch_news (D) ──────┘
```

**Run execution trace:**

```
Token 1: {from: "A", to: "B", payload: "NYC"}
Token 2: {from: "A", to: "C", payload: "NYC"}
Token 3: {from: "A", to: "D", payload: "NYC"}

Token 4: {from: "B", to: "E", payload: "cas://weather_data"}
Token 5: {from: "C", to: "E", payload: "cas://traffic_data"}
Token 6: {from: "D", to: "E", payload: "cas://news_data"}

E receives all 3 tokens:
  - SADD pending_tokens:run_123:E "B"
  - SADD pending_tokens:run_123:E "C"
  - SADD pending_tokens:run_123:E "D"
  - SCARD → 3 ✓ All received!

E executes with merged data:
  {
    "weather": <load from B's payload>,
    "traffic": <load from C's payload>,
    "news": <load from D's payload>
  }
```

---

## Data Structures

### Redis Structures (Hot Path)

```redis
# 1. Counter (per run)
SET counter:run_123 1
INCRBY counter:run_123 -1  # consume
INCRBY counter:run_123 2   # emit 2 tokens

# 2. Applied keys (idempotency)
SADD applied:run_123 "consume:token_456"
SADD applied:run_123 "emit:token_456"
SISMEMBER applied:run_123 "consume:token_456"  # → 1 (already applied)

# 3. Execution context (for agents)
HSET context:run_123
    "validate_customer:output" "cas://sha256:abc"
    "enrich_data:output" "cas://sha256:def"
    "enrich_data:duration_ms" "1234"

# 4. Pending approvals (HITL tracking)
SADD pending_approvals:run_123 "approval_789"
SCARD pending_approvals:run_123  # → 1 (workflow not complete)

# 5. Approval details
HSET approval:789
    run_id "run_123"
    node_id "manager_approval"
    status "pending"
    created_at "2025-10-13T10:00:00Z"
    timeout_at "2025-10-13T12:00:00Z"
    approver "manager@example.com"
    payload_cas_ref "cas://sha256:ghi"

# 6. Message bus (Redis Streams)
XADD wf.data *
    to "validate_customer"
    run_id "run_123"
    message "{...token json...}"
```

### Postgres Tables (Durable State)

#### 1. Core Execution

```sql
-- Runs table
CREATE TABLE runs (
    run_id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),

    -- Workflow reference
    artifact_id UUID NOT NULL,            -- Points to existing artifact
    tag_name TEXT,                        -- Which tag was used ("main")
    ir_cas_ref TEXT NOT NULL,             -- Compiled IR in CAS

    -- Run metadata
    status TEXT DEFAULT 'RUNNING',        -- RUNNING|COMPLETED|FAILED|CANCELLED
    submitted_by TEXT,
    submitted_at TIMESTAMPTZ DEFAULT now(),
    started_at TIMESTAMPTZ,
    ended_at TIMESTAMPTZ,

    -- I/O (CAS refs)
    inputs_cas_ref TEXT,
    outputs_cas_ref TEXT,

    -- Tracking
    final_counter_value INT,              -- Flushed on completion
    dlq_count INT DEFAULT 0,
    last_event_at TIMESTAMPTZ,

    -- Metadata
    tags JSONB DEFAULT '{}',

    FOREIGN KEY (artifact_id) REFERENCES artifact(artifact_id)
);

CREATE INDEX idx_runs_artifact ON runs(artifact_id);
CREATE INDEX idx_runs_status ON runs(status);
CREATE INDEX idx_runs_submitted_at ON runs(submitted_at DESC);
```

#### 2. Event Sourcing (Replayability)

```sql
-- Event log (append-only, ordered)
CREATE TABLE event_log (
    event_id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    run_id UUID NOT NULL,

    event_type TEXT NOT NULL,             -- "node.consumed", "node.emitted", etc.
    sequence_num BIGINT NOT NULL,         -- Strict ordering per run
    timestamp TIMESTAMPTZ DEFAULT now(),

    -- Event payload (small, no large data)
    event_data JSONB NOT NULL,

    -- Causality
    parent_event_id UUID,
    correlation_id UUID,

    UNIQUE (run_id, sequence_num),
    FOREIGN KEY (run_id) REFERENCES runs(run_id) ON DELETE CASCADE
);

CREATE INDEX idx_event_log_run ON event_log(run_id, sequence_num);
CREATE INDEX idx_event_log_type ON event_log(event_type);
```

**Example events:**

```json
// Event 1
{
  "event_type": "node.consumed",
  "sequence_num": 1,
  "node_id": "validate_customer",
  "token_id": "018d-token-002",
  "counter_before": 1,
  "counter_after": 0,
  "timestamp": "2025-10-13T10:00:01.123Z"
}

// Event 2
{
  "event_type": "node.emitted",
  "sequence_num": 2,
  "node_id": "validate_customer",
  "to_nodes": ["kyc_check", "credit_check"],
  "counter_before": 0,
  "counter_after": 2,
  "output_cas_ref": "cas://sha256:abc123",
  "timestamp": "2025-10-13T10:00:02.456Z"
}
```

#### 3. Applied Keys (Audit Trail)

```sql
-- Applied keys (flushed from Redis on completion)
CREATE TABLE applied_keys (
    run_id UUID NOT NULL,
    op_key TEXT NOT NULL,                 -- "consume:token_456"
    delta INT NOT NULL,                   -- -1 or +N
    applied_at TIMESTAMPTZ DEFAULT now(),

    PRIMARY KEY (run_id, op_key),
    FOREIGN KEY (run_id) REFERENCES runs(run_id) ON DELETE CASCADE
);

CREATE INDEX idx_applied_keys_run ON applied_keys(run_id);
```

#### 4. HITL (Approvals)

```sql
-- Approvals (human-in-the-loop)
CREATE TABLE approvals (
    approval_id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    run_id UUID NOT NULL,
    node_id TEXT NOT NULL,

    -- Status
    status TEXT DEFAULT 'pending',        -- pending|decided|timeout|cancelled

    -- Request
    created_at TIMESTAMPTZ DEFAULT now(),
    timeout_at TIMESTAMPTZ,
    requester TEXT,
    approver TEXT,

    -- Decision
    decision TEXT,                        -- approve|reject|timeout
    decision_cas_ref TEXT,                -- Full decision data in CAS
    decided_at TIMESTAMPTZ,
    decided_by TEXT,

    -- References
    consumed_token_id UUID,
    payload_cas_ref TEXT,

    metadata JSONB DEFAULT '{}',

    FOREIGN KEY (run_id) REFERENCES runs(run_id) ON DELETE CASCADE
);

CREATE INDEX idx_approvals_run ON approvals(run_id);
CREATE INDEX idx_approvals_status ON approvals(status) WHERE status = 'pending';
CREATE INDEX idx_approvals_timeout ON approvals(timeout_at) WHERE status = 'pending';
```

#### 5. Counter Snapshots (Recovery)

```sql
-- Periodic snapshots (every 10s)
CREATE TABLE run_counter_snapshots (
    run_id UUID NOT NULL,
    value INT NOT NULL,
    snapshot_at TIMESTAMPTZ DEFAULT now(),

    PRIMARY KEY (run_id, snapshot_at)
);
```

#### 6. Node Type Registry (Extensibility)

```sql
-- Node type registry (for UI and validation)
CREATE TABLE node_type_registry (
    type_name TEXT PRIMARY KEY,
    version TEXT NOT NULL,

    -- Schema references
    config_schema_ref TEXT,               -- JSON Schema in CAS
    input_schema_ref TEXT,
    output_schema_ref TEXT,

    -- Deployment
    worker_image TEXT,                    -- Docker image
    worker_queue TEXT,                    -- Redis stream name

    -- Capabilities
    capabilities JSONB DEFAULT '{}',      -- {"supports_retry": true, ...}

    -- Metadata
    display_name TEXT,
    description TEXT,
    icon_url TEXT,
    created_at TIMESTAMPTZ DEFAULT now(),

    UNIQUE (type_name, version)
);
```

**Example entries:**

```sql
INSERT INTO node_type_registry VALUES
('task', 'v1', 'schema://task_config', 'wf.task.stream',
 '{"supports_retry": true, "max_concurrency": 100}',
 'Task Node', 'Standard execution node', 'task-icon.svg'),

('agent', 'v1', 'schema://agent_config', 'wf.agent.stream',
 '{"requires_context": true, "supports_dynamic_routing": true}',
 'AI Agent', 'LLM-driven decision node', 'agent-icon.svg'),

('human', 'v1', 'schema://human_config', 'wf.human.stream',
 '{"creates_approval": true, "supports_timeout": true}',
 'Human Approval', 'Human-in-the-loop node', 'human-icon.svg');
```

---

## Node Types

**Overview**: Three core node types with flexible loop/branch configuration:
- **Task**: Standard execution with optional loop/branch
- **Agent**: LLM-driven routing with optional loop/branch (see [Agentic Service Implementation](#agentic-service-implementation))
- **Human**: Human-in-the-loop approval

**Key Design**: Loop and branching are **node configuration**, not separate node types. Any node can have `loop` and/or `branch` configs.

### 1. Task Node (Deterministic)

**Purpose**: Standard execution, deterministic routing.

**Config:**

```json
{
  "node_id": "validate_customer",
  "type": "task",
  "config_ref": {
    "type_url": "svc.validate/Config@1",
    "cas_ref": "sha256:cfg001"
  },
  "dependencies": [],
  "dependents": ["kyc_check", "credit_check"],
  "retry": {
    "max_attempts": 3,
    "backoff_ms": 1000
  },
  "limits": {
    "deadline_s": 30,
    "max_hops": 100
  }
}
```

**Execution:**

```go
func (w *TaskWorker) Execute(ctx *NodeContext) error {
    // Load config from CAS
    config := ctx.LoadConfig()

    // Load payload from CAS
    payload := ctx.LoadPayload()

    // Business logic
    result := doValidation(payload, config)

    // Store output in CAS
    ctx.StoreOutput(result)

    // Emit to all dependents
    for _, nextNode := range ctx.Node.Dependents {
        ctx.Emit(nextNode, result)
    }

    return nil
}
```

### 2. Loop Configuration (Retry, Iteration)

**Purpose**: Any node can loop with iteration tracking.

**Example: Retry API call until success**

```json
{
  "node_id": "call_external_api",
  "type": "task",
  "config_ref": {"type_url": "svc.http/Config@1", "cas_ref": "sha256:cfg002"},
  "dependencies": ["prepare_request"],
  "dependents": ["process_response"],

  "loop": {
    "enabled": true,
    "condition": {
      "type": "cel",
      "expression": "output.status != 200"
    },
    "max_iterations": 5,
    "loop_back_to": "call_external_api",  // Can loop to self or earlier node
    "break_path": ["process_response"],
    "timeout_path": ["error_dlq"]
  }
}
```

**Redis Iteration Tracking:**

```redis
# Track loop iterations per node
HSET loop:run_123:call_external_api
  "current_iteration" 3
  "max_iterations" 5
  "last_output_ref" "cas://sha256:abc"
```

**SDK handles loop logic:**

```go
func (sdk *SDK) handleLoop(ctx *NodeContext, output interface{}) error {
    loopKey := fmt.Sprintf("loop:%s:%s", ctx.RunID, ctx.NodeID)

    // Get current iteration
    iteration := sdk.redis.HIncrBy(loopKey, "current_iteration", 1).Val()

    // Check max iterations
    if iteration >= ctx.Node.Loop.MaxIterations {
        // Exit to timeout_path
        for _, node := range ctx.Node.Loop.TimeoutPath {
            sdk.Emit(node, output)
        }
        sdk.redis.Del(loopKey)
        return nil
    }

    // Evaluate condition using Condition Evaluator
    conditionMet, _ := sdk.conditionEvaluator.Evaluate(
        ctx.Node.Loop.Condition,
        output,
        sdk.LoadContext(ctx.RunID),
    )

    if conditionMet {
        // Continue looping
        sdk.Emit(ctx.Node.Loop.LoopBackTo, output)
    } else {
        // Break loop, go to break_path
        for _, node := range ctx.Node.Loop.BreakPath {
            sdk.Emit(node, output)
        }
        sdk.redis.Del(loopKey)
    }

    return nil
}
```

### 3. Branch Configuration (Conditional Routing)

**Purpose**: Dynamic routing based on conditions or agent decisions.

**Example: Conditional branching**

```json
{
  "node_id": "score_customer",
  "type": "task",
  "config_ref": {"type_url": "svc.scoring/Config@1", "cas_ref": "sha256:cfg003"},
  "dependencies": ["enrich_data"],
  "dependents": [],  // ← Overridden by branch

  "branch": {
    "enabled": true,
    "type": "conditional",
    "rules": [
      {
        "condition": {
          "type": "cel",
          "expression": "output.score >= 80 && ctx.enrich_data.output.revenue > 100000"
        },
        "next_nodes": ["enterprise_sales", "send_gift"]
      },
      {
        "condition": {
          "type": "cel",
          "expression": "output.score >= 50"
        },
        "next_nodes": ["standard_sales"]
      },
      {
        "condition": {
          "type": "cel",
          "expression": "output.score < 50"
        },
        "next_nodes": ["nurture_campaign"]
      }
    ],
    "default": ["manual_review"]
  }
}
```

**SDK handles branching:**

```go
func (sdk *SDK) handleBranch(ctx *NodeContext, output interface{}) error {
    var nextNodes []string
    context := sdk.LoadContext(ctx.RunID)

    // Evaluate rules in order
    for _, rule := range ctx.Node.Branch.Rules {
        conditionMet, _ := sdk.conditionEvaluator.Evaluate(rule.Condition, output, context)
        if conditionMet {
            nextNodes = rule.NextNodes
            break
        }
    }

    // No rule matched, use default
    if len(nextNodes) == 0 {
        nextNodes = ctx.Node.Branch.Default
    }

    // Emit to selected paths
    for _, node := range nextNodes {
        sdk.Emit(node, output)
    }

    return nil
}
```

### 4. Agent Node with Branch (LLM-Driven Routing)

**Purpose**: Context-aware, LLM-based routing decisions using branch config.

**Config:**

```json
{
  "node_id": "qualify_lead_agent",
  "type": "agent",
  "config_ref": {
    "type_url": "svc.agent/Config@1",
    "cas_ref": "sha256:cfg004"
  },
  "dependencies": ["enrich_data"],
  "dependents": [],

  "branch": {
    "enabled": true,
    "type": "agent_driven",  // ← LLM decides
    "available_next_nodes": [
      "enterprise_sales",
      "standard_sales",
      "nurture_campaign",
      "disqualify"
    ],
    "default": ["manual_review"]
  },

  "agent_config": {
    "model": "gpt-4",
    "temperature": 0.3,
    "available_tools": ["search_database", "call_api"]
  }
}
```

**Agent-driven branching:**

```go
func (w *AgentWorker) handleBranch(ctx *NodeContext, output interface{}) error {
    // Call LLM with available options
    prompt := fmt.Sprintf(`
You are a lead qualification agent.

Customer data: %v
Execution history: %v
Available next actions: %v

Decide which path(s) to route this lead to and why.
`, output, ctx.GetContext(), ctx.Node.Branch.AvailableNextNodes)

    decision := w.llmClient.Call(prompt, ctx.Node.AgentConfig.Model)
    // → {next_nodes: ["enterprise_sales", "send_gift"], reasoning: "..."}

    // Validate LLM picked from allowed list
    for _, node := range decision.NextNodes {
        if !contains(ctx.Node.Branch.AvailableNextNodes, node) {
            return fmt.Errorf("agent tried to route to unauthorized node: %s", node)
        }
    }

    // Store decision in context
    ctx.SetContext("qualify_lead_agent:decision", decision)

    // Emit to LLM-selected nodes
    for _, node := range decision.NextNodes {
        ctx.Emit(node, output)
    }

    return nil
}
```

### 5. Human Node (HITL)

**Purpose**: Human approval, pause/resume workflow.

**Config:**

```json
{
  "node_id": "manager_approval",
  "type": "human",
  "config_ref": {
    "type_url": "svc.human/Config@1",
    "cas_ref": "sha256:cfg004"
  },
  "dependencies": ["validate_high_value_deal"],
  "dependents": ["setup_enterprise_account"],
  "human_config": {
    "timeout_hours": 48,
    "approvers": ["sales-manager@company.com"],
    "escalation_policy": "auto_approve_after_timeout",
    "approval_ui_url": "https://approvals.company.com"
  }
}
```

**Execution:**

```go
func (w *HumanWorker) Execute(ctx *NodeContext) error {
    config := ctx.LoadConfig()
    payload := ctx.LoadPayload()

    // 1. Create approval record
    approvalID := uuid.New()

    // 2. Track in Redis (pending approvals)
    ctx.Redis.SAdd(ctx.Context,
        "pending_approvals:" + ctx.RunID,
        approvalID.String())

    // 3. Store approval details
    ctx.Redis.HSet(ctx.Context, "approval:" + approvalID.String(),
        "run_id", ctx.RunID,
        "node_id", ctx.NodeID,
        "status", "pending",
        "timeout_at", time.Now().Add(config.TimeoutHours * time.Hour),
        "approver", config.Approver,
        "payload_cas_ref", ctx.Token.Payload.CasRef,
    )

    // 4. Send to external HITL service
    err := w.hitlClient.CreateApprovalTask(ctx.Context, &HITLTask{
        ApprovalID: approvalID,
        RunID:      ctx.RunID,
        Title:      "Approve enterprise deal",
        Approver:   config.Approver,
        Deadline:   time.Now().Add(config.TimeoutHours * time.Hour),
        CallbackURL: fmt.Sprintf("/api/v1/approvals/%s/decide", approvalID),
    })

    if err != nil {
        return err
    }

    // 5. NO EMIT! Wait for callback from HITL service
    log.Info("approval created, workflow paused",
        "approval_id", approvalID,
        "run_id", ctx.RunID)

    return nil
}
```

**Resume on approval:**

```go
// API endpoint: POST /api/v1/approvals/:id/decide
func (h *ApprovalHandler) Decide(c echo.Context) error {
    approvalID := c.Param("id")

    var req DecisionRequest
    c.Bind(&req)  // {decision: "approve", comments: "..."}

    // Load approval
    approval := h.redis.HGetAll(ctx, "approval:" + approvalID)
    runID := approval["run_id"]
    nodeID := approval["node_id"]

    // Store decision
    decisionData := map[string]interface{}{
        "decision": req.Decision,
        "comments": req.Comments,
        "decided_by": req.DecidedBy,
        "decided_at": time.Now(),
    }
    decisionRef := h.cas.Store(decisionData)

    // Update approval
    h.redis.HSet(ctx, "approval:" + approvalID,
        "status", "decided",
        "decision", req.Decision,
        "decision_cas_ref", decisionRef)

    // Remove from pending
    h.redis.SRem(ctx, "pending_approvals:" + runID, approvalID)

    // Load IR to get next nodes
    ir := h.loadIR(runID)
    node := ir.Nodes[nodeID]

    var nextNodes []string
    if req.Decision == "approve" {
        nextNodes = node.Dependents
    } else {
        nextNodes = node.RejectionPath  // Alternative path
    }

    // RESUME: Emit tokens
    originalPayload := h.cas.Get(approval["payload_cas_ref"])
    enrichedPayload := merge(originalPayload, decisionData)
    newPayloadRef := h.cas.Store(enrichedPayload)

    for _, nextNode := range nextNodes {
        token := createToken(runID, nodeID, nextNode, newPayloadRef)
        h.publishToken(token)
    }

    // Apply counter (+N)
    h.applyDelta(runID, fmt.Sprintf("emit:approval_%s", approvalID), len(nextNodes))

    return c.JSON(200, map[string]interface{}{
        "approval_id": approvalID,
        "status": "resumed",
        "next_nodes": nextNodes,
    })
}
```

---

## Condition Evaluation Engine

### Overview

The Condition Evaluation Engine powers **loop and branch logic** by providing flexible, safe, and expressive condition evaluation strategies.

**Design Principles:**
1. ✅ **Multiple strategies** - Choose the right tool for each use case
2. ✅ **Context access** - Reference previous node outputs
3. ✅ **Schema validation** - Ensure output structure correctness
4. ✅ **Safety** - No arbitrary code execution (except sandboxed)
5. ✅ **Performance** - Fast evaluation with caching

### Supported Condition Types

| Type | Use Case | Example |
|------|----------|---------|
| **CEL** | Simple logic, comparisons | `output.score >= 80` |
| **JSONPath** | Extract nested values | `$.data.customers[?(@.tier == 'enterprise')]` |
| **JSON Schema** | Validate structure | Match against expected schema |
| **Cross-Node** | Compare between nodes | `current.revenue > ctx.forecast.revenue` |
| **Composite** | AND/OR/NOT logic | Combine multiple conditions |
| **Python Sandbox** | Complex business logic | Custom Python function (sandboxed) |

---

### 1. CEL (Common Expression Language)

**Purpose**: Fast, safe expressions for simple logic.

**Capabilities:**
- Comparisons: `==`, `!=`, `>`, `<`, `>=`, `<=`
- Logic: `&&`, `||`, `!`
- Arithmetic: `+`, `-`, `*`, `/`, `%`
- String functions: `contains()`, `startsWith()`, `endsWith()`
- List operations: `in`, `all()`, `exists()`
- Ternary: `x > 10 ? "high" : "low"`

**Example:**

```json
{
  "condition": {
    "type": "cel",
    "expression": "output.status != 200 && output.retry_count < 5"
  }
}
```

**With context access:**

```json
{
  "condition": {
    "type": "cel",
    "expression": "output.score >= 80 && ctx.enrich_data.output.revenue > 100000"
  }
}
```

---

### 2. JSON Schema Validation

**Purpose**: Validate output structure matches expected schema.

**Example: Loop until valid schema**

```json
{
  "loop": {
    "enabled": true,
    "condition": {
      "type": "schema_validation",
      "schema": {
        "type": "object",
        "required": ["status", "data"],
        "properties": {
          "status": {"const": "success"},
          "data": {"type": "array", "minItems": 1}
        }
      },
      "invert": true  // Loop while schema does NOT match
    },
    "max_iterations": 3
  }
}
```

**Schema from CAS:**

```json
{
  "condition": {
    "type": "schema_validation",
    "schema_ref": "cas://sha256:schema123"
  }
}
```

---

### 3. JSONPath Queries

**Purpose**: Extract and compare values from nested structures.

**Example:**

```json
{
  "condition": {
    "type": "jsonpath",
    "query": "$.data.customers[?(@.tier == 'enterprise')].length()",
    "operator": ">",
    "value": 0
  }
}
```

**Complex query:**

```json
{
  "condition": {
    "type": "jsonpath",
    "query": "$.transactions[*].amount",
    "operator": "sum_gt",
    "value": 10000
  }
}
```

---

### 4. Cross-Node Comparisons

**Purpose**: Compare current output with previous node outputs.

**Example: Compare with previous node**

```json
{
  "condition": {
    "type": "cross_node_comparison",
    "left": {
      "source": "current",
      "path": "output.final_score"
    },
    "operator": ">",
    "right": {
      "source": "context",
      "node": "initial_score",
      "path": "output.score"
    }
  }
}
```

**Complex expression:**

```json
{
  "condition": {
    "type": "cross_node_comparison",
    "expression": "(current.output.revenue - ctx.forecast.output.predicted_revenue) / ctx.forecast.output.predicted_revenue > 0.1"
  }
}
```

---

### 5. Composite Conditions (AND/OR/NOT)

**Purpose**: Combine multiple conditions.

**Example: AND logic**

```json
{
  "condition": {
    "type": "composite",
    "operator": "AND",
    "conditions": [
      {
        "type": "cel",
        "expression": "output.status != 'success'"
      },
      {
        "type": "jsonpath",
        "query": "$.errors.length()",
        "operator": "<",
        "value": 3
      },
      {
        "type": "schema_validation",
        "schema_ref": "cas://sha256:retry_schema"
      }
    ]
  }
}
```

**OR example:**

```json
{
  "condition": {
    "type": "composite",
    "operator": "OR",
    "conditions": [
      {"type": "cel", "expression": "output.score >= 80"},
      {
        "type": "cross_node_comparison",
        "left": {"source": "context", "node": "referral_check", "path": "output.is_referral"},
        "operator": "==",
        "right": {"value": true}
      }
    ]
  }
}
```

---

### 6. Python Sandbox (Advanced)

**Purpose**: Ultra-complex logic that can't be expressed with other methods.

**Safety:**
- Runs in RestrictedPython sandbox
- No file I/O, network access, or imports
- CPU/memory limits enforced
- Timeout after 1 second

**Example:**

```json
{
  "condition": {
    "type": "python_sandbox",
    "code_ref": "cas://sha256:python_evaluator",
    "timeout_ms": 1000,
    "max_memory_mb": 50
  }
}
```

**Python code (stored in CAS):**

```python
def evaluate(output, context):
    """
    output: current node output
    context: dict of all previous node outputs
    """
    # Complex business logic
    revenue = output.get('revenue', 0)
    previous_revenue = context.get('last_month', {}).get('output', {}).get('revenue', 0)

    growth_rate = (revenue - previous_revenue) / previous_revenue if previous_revenue > 0 else 0

    # Multi-factor decision
    if growth_rate > 0.2 and output.get('churn_rate', 1) < 0.05:
        return True

    return False
```

---

### SDK Implementation

**Core evaluator:**

```go
package workflow

import (
    "github.com/google/cel-go/cel"
    "github.com/xeipuuv/gojsonschema"
    "github.com/ohler55/ojg/jp"
)

type ConditionEvaluator struct {
    celEnv *cel.Env
    pythonSandbox *PythonSandbox
}

func (e *ConditionEvaluator) Evaluate(condition Condition, output interface{}, context map[string]interface{}) (bool, error) {
    switch condition.Type {
    case "cel":
        return e.evaluateCEL(condition.Expression, output, context)
    case "schema_validation":
        return e.evaluateSchema(condition.Schema, output, condition.Invert)
    case "jsonpath":
        return e.evaluateJSONPath(condition.Query, condition.Operator, condition.Value, output)
    case "cross_node_comparison":
        return e.evaluateCrossNode(condition, output, context)
    case "composite":
        return e.evaluateComposite(condition, output, context)
    case "python_sandbox":
        return e.evaluatePython(condition.CodeRef, output, context)
    default:
        return false, fmt.Errorf("unknown condition type: %s", condition.Type)
    }
}

func (e *ConditionEvaluator) evaluateCEL(expr string, output, context interface{}) (bool, error) {
    // Build CEL environment
    env, _ := cel.NewEnv(
        cel.Variable("output", cel.DynType),
        cel.Variable("ctx", cel.DynType),
    )

    // Compile
    ast, issues := env.Compile(expr)
    if issues != nil && issues.Err() != nil {
        return false, issues.Err()
    }

    // Create program
    prg, err := env.Program(ast)
    if err != nil {
        return false, err
    }

    // Evaluate
    out, _, err := prg.Eval(map[string]interface{}{
        "output": output,
        "ctx":    context,
    })

    if err != nil {
        return false, err
    }

    return out.Value().(bool), nil
}
```

---

### Context Management

**SDK loads context from Redis:**

```go
func (sdk *SDK) LoadContext(runID string) map[string]interface{} {
    contextKey := fmt.Sprintf("context:%s", runID)

    // Redis hash stores all node outputs
    allOutputs := sdk.redis.HGetAll(contextKey).Val()

    context := make(map[string]interface{})

    for nodeID, outputRefJSON := range allOutputs {
        var outputRef OutputRef
        json.Unmarshal([]byte(outputRefJSON), &outputRef)

        // Load actual output from CAS
        output := sdk.cas.Get(outputRef.CASRef)

        context[nodeID] = map[string]interface{}{
            "output": output,
            "metadata": outputRef.Metadata,
        }
    }

    return context
}
```

**Redis structure:**

```redis
HSET context:run_123
  "fetch_data" '{"output_ref": "cas://sha256:abc", "completed_at": "2025-10-13T10:00:00Z"}'
  "validate_data" '{"output_ref": "cas://sha256:def", "completed_at": "2025-10-13T10:00:05Z"}'
  "score_customer" '{"output_ref": "cas://sha256:ghi", "completed_at": "2025-10-13T10:00:10Z"}'
```

---

### Real-World Example: Complex Payment Retry

```json
{
  "node_id": "call_payment_api",
  "type": "task",
  "config_ref": "cas://sha256:payment_config",

  "loop": {
    "enabled": true,
    "condition": {
      "type": "composite",
      "operator": "AND",
      "conditions": [
        {
          "type": "cel",
          "expression": "output.status != 'success'"
        },
        {
          "type": "jsonpath",
          "query": "$.error_code",
          "operator": "in",
          "value": ["TIMEOUT", "RATE_LIMIT", "TEMP_ERROR"]
        },
        {
          "type": "cross_node_comparison",
          "left": {"source": "current", "path": "output.amount"},
          "operator": "==",
          "right": {"source": "context", "node": "calculate_total", "path": "output.final_amount"}
        }
      ]
    },
    "max_iterations": 5,
    "loop_back_to": "call_payment_api",
    "break_path": ["confirm_payment"],
    "timeout_path": ["refund_customer"]
  },

  "branch": {
    "enabled": true,
    "type": "conditional",
    "rules": [
      {
        "condition": {
          "type": "cel",
          "expression": "output.status == 'success' && output.amount >= 1000"
        },
        "next_nodes": ["send_premium_confirmation", "update_crm"]
      },
      {
        "condition": {
          "type": "cel",
          "expression": "output.status == 'success'"
        },
        "next_nodes": ["send_standard_confirmation"]
      }
    ],
    "default": ["manual_review"]
  }
}
```

---

### Performance Considerations

**CEL compilation caching:**

```go
type ConditionCache struct {
    cache map[string]*cel.Program
    mu    sync.RWMutex
}

func (c *ConditionCache) GetOrCompile(expr string) (*cel.Program, error) {
    c.mu.RLock()
    if prg, exists := c.cache[expr]; exists {
        c.mu.RUnlock()
        return prg, nil
    }
    c.mu.RUnlock()

    // Compile and cache
    c.mu.Lock()
    defer c.mu.Unlock()

    // Double-check
    if prg, exists := c.cache[expr]; exists {
        return prg, nil
    }

    // Compile
    env, _ := cel.NewEnv(...)
    ast, _ := env.Compile(expr)
    prg, err := env.Program(ast)

    if err == nil {
        c.cache[expr] = prg
    }

    return prg, err
}
```

**Benchmarks:**
- CEL evaluation: ~0.1ms (cached)
- JSON Schema validation: ~0.5ms
- JSONPath query: ~1ms
- Cross-node comparison: ~0.2ms
- Python sandbox: ~50-100ms

---

## Agentic Service Implementation

### Overview

The **Agentic Service** is a Python-based Redis worker that processes LLM-powered tasks during workflow execution. It provides the non-deterministic execution capability through two execution lanes:

1. **Fast Lane**: Ephemeral data pipelines (execute_pipeline)
2. **Patch Lane**: Persistent workflow modifications (patch_workflow)

**Key Principle**: Agent nodes delegate complex decision-making to LLMs while maintaining the choreography pattern - they consume tokens, make decisions, and emit to next nodes just like any other worker.

### Architecture Integration

```
┌────────────────────────────────────────────────────────────┐
│                   Workflow Execution                        │
└────────────────────────────────────────────────────────────┘
                          ↓
              Token reaches "agent" node
                          ↓
           ┌────────────────────────────────┐
           │   Go Runner (Task Worker)      │
           │   Publishes job to Redis       │
           └────────────┬───────────────────┘
                        │
                        ↓ RPUSH agent:jobs
           ┌────────────────────────────────┐
           │     Redis Queue (agent:jobs)   │
           └────────────┬───────────────────┘
                        │
                        ↓ BLPOP
           ┌────────────────────────────────────────┐
           │   Agent Service (Python Worker)        │
           │   - BLPOP from agent:jobs              │
           │   - Call OpenAI with 5 tools           │
           │   - Execute tool (pipeline/patch)      │
           │   - Store result in Postgres/CAS       │
           │   - RPUSH to agent:results:{job_id}    │
           └────────────┬───────────────────────────┘
                        │
                        ↓
           ┌────────────────────────────────┐
           │  agent:results:{job_id}        │
           │  Redis Queue (per-job)         │
           └────────────┬───────────────────┘
                        │
                        ↓ BLPOP
           ┌────────────────────────────────┐
           │  Go Runner picks up result     │
           │  Emits tokens to next nodes    │
           │  (counter +N)                  │
           └────────────────────────────────┘
```

### Message Protocol

**Input (agent:jobs):**

```json
{
  "version": "1.0",
  "job_id": "550e8400-...",
  "run_id": "run-2024-001",
  "node_id": "agent_analyze_data",
  "workflow_tag": "main",
  "user_id": "sdutt",
  "prompt": "fetch flight prices NYC to LAX, sort by price, show top 3",
  "context": {
    "previous_results": [
      {
        "node_id": "fetch_data",
        "result_ref": "cas://sha256:abc123",
        "preview": {"count": 150}
      }
    ],
    "session_id": "sess-xyz789"
  },
  "timeout_sec": 300,
  "retry_count": 0
}
```

**Output (agent:results:{job_id}):**

```json
{
  "version": "1.0",
  "job_id": "550e8400-...",
  "status": "completed",
  "result_ref": "artifact://uuid-result-id",
  "result_preview": {
    "type": "dataset",
    "row_count": 3,
    "sample": [{"airline": "Delta", "price": 299}]
  },
  "metadata": {
    "tool_calls": [{
      "tool": "execute_pipeline",
      "args": {"pipeline": [...]}
    }],
    "tokens_used": 1523,
    "cache_hit": true,
    "llm_model": "gpt-4o"
  }
}
```

### Five Tools for Agents

#### 1. execute_pipeline (Fast Lane)

**Purpose**: Execute ephemeral data transformations without modifying the workflow.

**Primitives** (10 composable operations):
- `http_request`: GET/POST to APIs
- `table_sort`: Sort by field
- `table_filter`: Filter rows by condition
- `table_select`: Project columns
- `top_k`: Take first K rows
- `groupby`: Aggregate data
- `join`: Join two datasets
- `jq_transform`: JSON transformation
- `regex_extract`: Pattern extraction
- `parse_date`: Date parsing

**Example:**

```json
{
  "tool": "execute_pipeline",
  "args": {
    "session_id": "sess-123",
    "pipeline": [
      {"step": "http_request", "url": "api.flights.com/search", "params": {"origin": "NYC", "dest": "LAX"}},
      {"step": "table_sort", "field": "price", "order": "asc"},
      {"step": "top_k", "k": 3}
    ]
  }
}
```

**Result**: Stored in CAS, returns `cas://sha256:...`

#### 2. patch_workflow (Patch Lane)

**Purpose**: Permanently modify workflow structure (add nodes/edges).

**Example:**

```json
{
  "tool": "patch_workflow",
  "args": {
    "workflow_tag": "main",
    "patch_spec": {
      "operations": [
        {
          "op": "add",
          "path": "/nodes/-",
          "value": {
            "id": "send_email",
            "type": "task",
            "config": {"action": "email", "to": "ops@example.com"}
          }
        },
        {
          "op": "add",
          "path": "/edges/-",
          "value": {"from": "process_data", "to": "send_email"}
        }
      ],
      "description": "Add email notification"
    }
  }
}
```

**Result**: Forwards to orchestrator API, creates new artifact version.

#### 3. search_tools

**Purpose**: Discover existing composite tools/workflows.

```json
{
  "tool": "search_tools",
  "args": {
    "query": "flight price comparison",
    "limit": 10
  }
}
```

#### 4. openapi_action (Meta-Tool)

**Purpose**: Call any OpenAPI-compliant API dynamically.

```json
{
  "tool": "openapi_action",
  "args": {
    "openapi_url": "https://api.weather.com/openapi.json",
    "operation_id": "getCurrentWeather",
    "params": {"city": "New York"},
    "auth_profile": "weatherapi"
  }
}
```

#### 5. delegate_to_agent (K8s Scaffold)

**Purpose**: Delegate to external agents (future: K8s jobs).

```json
{
  "tool": "delegate_to_agent",
  "args": {
    "agent_type": "code_interpreter",
    "inputs": {"code": "import pandas as pd\n..."}
  }
}
```

### Database Schema for Agent Results

```sql
-- Store agent execution results
CREATE TABLE agent_results (
    result_id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    job_id UUID NOT NULL UNIQUE,
    run_id TEXT NOT NULL,
    node_id TEXT NOT NULL,

    -- Result storage
    result_data JSONB,              -- For small results (<10MB)
    cas_id TEXT,                    -- Reference to CAS blob
    s3_ref TEXT,                    -- Future: S3 reference

    -- Metadata
    status TEXT NOT NULL,           -- 'completed', 'failed'
    error JSONB,
    tool_calls JSONB,
    tokens_used INTEGER,
    cache_hit BOOLEAN DEFAULT false,
    execution_time_ms INTEGER,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,

    INDEX idx_agent_results_job_id (job_id),
    INDEX idx_agent_results_run_id (run_id)
);
```

### Token Optimization (OpenAI Prompt Caching)

**System prompt structure** for 50-80% cost reduction:

```python
SYSTEM_PREFIX = """
You are an orchestration agent for workflow execution.

[TOOL SCHEMAS - 2000 tokens]
- execute_pipeline
- patch_workflow
- search_tools
- openapi_action
- delegate_to_agent

[FEW-SHOT EXAMPLES - 500 tokens]
- Example 1: data pipeline
- Example 2: workflow patch

[POLICY RULES - 300 tokens]
- Fast lane vs patch lane
- When to use each tool
"""  # Total: ~3000 tokens → CACHED by OpenAI

messages = [
    {"role": "system", "content": SYSTEM_PREFIX},  # Cached!
    {"role": "user", "content": job['prompt']}     # Dynamic
]
```

**Benefits**:
- 80% latency reduction for cached prefix
- 50% cost reduction on input tokens
- Automatic (no code changes)

### Flow Example: Agent Node Execution

```
1. Workflow execution reaches agent node
   Counter: 5 → consume(-1) → 4

2. Go worker publishes to Redis:
   RPUSH agent:jobs {job_data}

3. Python agent worker picks up:
   BLPOP agent:jobs → gets job

4. Agent calls OpenAI with 5 tools + context:
   - System prompt (cached)
   - User prompt: "analyze customer, decide next action"
   - Context: previous node outputs from Redis

5. LLM responds with tool call:
   execute_pipeline([
     {step: "table_filter", condition: {...}},
     {step: "groupby", by: "category"}
   ])

6. Agent executes pipeline:
   - Runs filter + groupby
   - Stores result in CAS: cas://sha256:xyz

7. Agent publishes result:
   RPUSH agent:results:{job_id} {
     status: "completed",
     result_ref: "cas://sha256:xyz"
   }

8. Go worker picks up result:
   BLPOP agent:results:{job_id}

9. Go worker emits to next nodes:
   emit(["high_value_sales", "send_gift"])
   Counter: 4 → emit(+2) → 6

10. Workflow continues with token choreography
```

### Auto-Promotion Pattern

When a pipeline is used ≥2 times, promote to reusable composite tool:

```python
def track_usage(pipeline_hash):
    count = redis.HINCRBY("pipeline_usage", pipeline_hash, 1)

    if count >= 2:
        # Promote to composite tool
        tool_name = generate_name(pipeline)
        register_tool(tool_name, pipeline)

        # Update LLM tool schema dynamically
        add_tool_to_schema(tool_name)
```

### Integration with Token Counter

**Agent node behaves exactly like any other node:**

1. **Consume**: Apply -1 to counter (idempotent)
2. **Execute**: Call LLM, run tools (may take seconds)
3. **Emit**: Apply +N to counter based on LLM decision
4. **Context**: Store agent decision in Redis context store

**Key difference**: Agent decides next_nodes at runtime instead of compile-time.

---

## HITL (Human-in-the-Loop)

### Counter Behavior with HITL

```
Step  Event                           Counter  Pending Approvals
────────────────────────────────────────────────────────────────────
 1    Token arrives at HITL node         1     []
 2    HITL consumes (-1)                 0     []
 3    Create approval (no emit yet)      0     [approval_456]
      ← Counter = 0 BUT not complete!

... 5 minutes pass (human reviewing) ...

 4    Human approves                     0     [approval_456]
 5    Emit to next nodes (+2)            2     []
 6    Remove from pending                2     []  ← Workflow resumes
```

### Completion Check with HITL

```go
func (s *RunSupervisor) CheckCompletion(runID string) {
    counter := s.redis.Get("counter:" + runID).Int()

    if counter == 0 {
        // Check 1: Pending messages in Redis
        pendingMsgs := s.redis.XPending("wf.data", runID).Val()

        // Check 2: Pending approvals (CRITICAL!)
        pendingApprovals := s.redis.SCard("pending_approvals:" + runID).Val()

        // Check 3: Active executions
        activeExecs := s.getActiveExecutions(runID)

        if pendingMsgs == 0 &&
           pendingApprovals == 0 &&  // Must be zero!
           len(activeExecs) == 0 {

            // Truly complete
            s.markCompleted(runID)
            s.flushToDB(runID)
        } else {
            log.Info("counter=0 but work pending",
                "pending_approvals", pendingApprovals)
        }
    }
}
```

### Timeout Handling

**Option 1: Redis Expiry + Keyspace Notifications**

```go
// When creating approval, set expiry
redis.Set("approval:" + approvalID, data, 48*time.Hour)

// Watch for expirations
pubsub := redis.Subscribe("__keyevent@0__:expired")
for msg := range pubsub.Channel() {
    if strings.HasPrefix(msg.Payload, "approval:") {
        approvalID := strings.TrimPrefix(msg.Payload, "approval:")
        handleTimeout(approvalID)
    }
}

func handleTimeout(approvalID string) {
    // Load from backup or Postgres
    approval := loadApproval(approvalID)

    // Auto-reject or escalate based on policy
    if approval.EscalationPolicy == "auto_approve" {
        decide(approvalID, DecisionRequest{Decision: "approve", DecidedBy: "system"})
    } else {
        decide(approvalID, DecisionRequest{Decision: "timeout", DecidedBy: "system"})
    }
}
```

**Option 2: Scheduled Checker**

```go
func checkTimeouts() {
    ticker := time.NewTicker(30 * time.Second)

    for range ticker.C {
        // Query all pending approvals
        approvals := db.Query(`
            SELECT approval_id, timeout_at
            FROM approvals
            WHERE status = 'pending'
        `)

        for _, approval := range approvals {
            if time.Now().After(approval.TimeoutAt) {
                handleTimeout(approval.ApprovalID)
            }
        }
    }
}
```

---

## Event Sourcing & Replay

### Event Log Structure

Every state change is recorded as an event:

```json
{
  "event_id": "018d-event-001",
  "run_id": "018d-run-123",
  "event_type": "node.consumed",
  "sequence_num": 1,
  "timestamp": "2025-10-13T10:00:01.123Z",
  "event_data": {
    "node_id": "validate_customer",
    "token_id": "018d-token-002",
    "counter_before": 1,
    "counter_after": 0,
    "hop": 1,
    "attempt": 1
  }
}
```

### Replay Algorithm

```go
func ReplayRun(runID string) (*RunState, error) {
    // 1. Load all events in order
    events := db.Query(`
        SELECT event_type, event_data
        FROM event_log
        WHERE run_id = ?
        ORDER BY sequence_num
    `, runID)

    // 2. Initialize empty state
    state := &RunState{
        RunID:   runID,
        Counter: 0,
        Applied: make(map[string]bool),
        Context: make(map[string]interface{}),
    }

    // 3. Replay each event
    for _, event := range events {
        switch event.EventType {
        case "node.consumed":
            state.Counter += -1
            state.Applied["consume:"+event.Data.TokenID] = true

        case "node.emitted":
            state.Counter += len(event.Data.ToNodes)
            state.Applied["emit:"+event.Data.TokenID] = true
            state.Context[event.Data.NodeID+":output"] = event.Data.OutputRef

        case "approval.created":
            state.PendingApprovals[event.Data.ApprovalID] = true

        case "approval.decided":
            delete(state.PendingApprovals, event.Data.ApprovalID)
        }
    }

    return state, nil
}
```

### Benefits

1. **Full audit trail** - every state change recorded
2. **Time travel debugging** - replay to any point
3. **Conflict resolution** - compare event logs
4. **Compliance** - immutable history

---

## Execution Flow Examples

### Example 1: Simple Sequential Flow

**Workflow**: `A → B → C`

```
Counter evolution:
1. Initialize: counter = 1
2. A consumes: counter = 0
3. A emits to B: counter = 1
4. B consumes: counter = 0
5. B emits to C: counter = 1
6. C consumes: counter = 0
7. C completes (no emit): counter = 0 → DONE
```

### Example 2: Parallel Fan-Out

**Workflow**: `A → (B, C) → D`

```
Counter evolution:
1. Initialize: counter = 1
2. A consumes: counter = 0
3. A emits to B,C: counter = 2
4. B consumes: counter = 1
5. C consumes: counter = 0 (or 1 if B consumed first)
6. B emits to D: counter = 1
7. C emits to D: counter = 2
8. D consumes first token: counter = 1
9. D waits (wait_for_all)
10. D consumes second token: counter = 0
11. D completes: counter = 0 → DONE
```

### Example 3: HITL with Approval

**Workflow**: `A → Approval (HITL) → B`

```
Counter evolution:
1. Initialize: counter = 1
2. A consumes: counter = 0
3. A emits to Approval: counter = 1
4. Approval consumes: counter = 0
5. Create approval record (no emit): counter = 0, pending = [approval_456]
   ← Supervisor sees: counter=0 but pending approvals → NOT DONE

... 5 minutes pass ...

6. Human approves via API
7. Emit to B: counter = 1, pending = []
8. B consumes: counter = 0
9. B completes: counter = 0, pending = [] → DONE
```

### Example 4: Agent Dynamic Routing

**Workflow**: `A → Agent → ???` (runtime decision)

```
Counter evolution:
1. Initialize: counter = 1
2. A consumes: counter = 0
3. A emits to Agent: counter = 1
4. Agent consumes: counter = 0
5. Agent calls LLM: decides ["high_value_sales", "send_gift"]
6. Agent emits to 2 nodes: counter = 2
7. high_value_sales consumes: counter = 1
8. send_gift consumes: counter = 0
9. Both complete: counter = 0 → DONE
```

---

## Scalability & Performance

### Redis Hot Path Performance

```
Operation            Throughput        Latency (p50)
────────────────────────────────────────────────────
Counter increment    100,000 ops/s     < 1ms
Applied key check    150,000 ops/s     < 1ms
Lua script (APPLY)   80,000 ops/s      < 2ms
Stream publish       50,000 msgs/s     < 5ms
```

### Postgres Write Strategy

**Option 1: Flush on Completion (Fastest)**

```
Redis: All hot-path writes
Postgres: Final state only

Throughput: ~100K runs/minute
Postgres load: Minimal (one INSERT per run)
```

**Option 2: Periodic Snapshots (Safer)**

```
Redis: Hot path
Postgres: Snapshot every 10s

Throughput: ~80K runs/minute
Postgres load: Medium (batch writes)
Recovery: Max 10s data loss
```

**Option 3: Event Sourcing (Most Durable)**

```
Redis: Hot path
Postgres: Event log append (async)

Throughput: ~60K runs/minute
Postgres load: High (many inserts)
Recovery: Full replay possible
```

### Horizontal Scaling

```
Workers scale independently:

Task workers:  10 instances → 1M tasks/hour
Agent workers: 5 instances → 100K decisions/hour
Human workers: 2 instances → handles all HITL

Redis cluster: 3 masters + 3 replicas
Postgres: Read replicas for event log queries
```

---

## Implementation Phases

### Phase 1: Core Execution (Must Have - Days 1-3)

**Goal**: Deterministic workflow execution with counter system.

- [ ] Database schema (runs, event_log, applied_keys, snapshots)
- [ ] Redis Lua script (atomic APPLY)
- [ ] IR compiler (DSL → IR with dependencies/dependents)
- [ ] Run Supervisor (completion detection, periodic flush)
- [ ] Node SDK (Consume, Emit, LoadConfig, LoadPayload)
- [ ] Task worker (simple execution)
- [ ] Message bus (Redis Streams setup)
- [ ] REST API (POST /runs, GET /runs/:id)

**Test scenarios:**
- Sequential flow (A → B → C)
- Parallel fan-out (A → B,C → D)
- Counter correctness
- Idempotency (duplicate delivery)

### Phase 2: Advanced Node Types (Must Have - Days 4-5)

**Goal**: Decision, Agent, Human nodes.

- [ ] Decision worker (if/else routing)
- [ ] Agent worker (LLM integration, context loading)
- [ ] Human worker (HITL with approval creation)
- [ ] Approval API (POST /approvals/:id/decide)
- [ ] Timeout handling (Redis expiry or scheduler)
- [ ] Context store (Redis hash for execution history)

**Test scenarios:**
- Decision routing
- Agent dynamic emit
- HITL pause/resume
- Timeout auto-reject

### Phase 3: Event Sourcing & Observability (Nice to Have - Days 6-7)

**Goal**: Full replay, audit trail, monitoring.

- [ ] Event log writer (async append)
- [ ] Replay function (rebuild state from events)
- [ ] Node type registry (extensibility)
- [ ] GET /node-types (for UI)
- [ ] Metrics (Prometheus exports)
- [ ] Tracing (OpenTelemetry)

**Test scenarios:**
- Replay from event log
- Time travel debugging

### Phase 4: Production Hardening (Later - Week 2)

**Goal**: Resilience, security, ops.

- [ ] Retry + DLQ logic
- [ ] Hop limits + deadline enforcement
- [ ] Redis AOF persistence
- [ ] Postgres read replicas
- [ ] Authentication + authorization
- [ ] Rate limiting
- [ ] Graceful shutdown
- [ ] Health checks

---

## Production Hardening & Gaps

This section outlines critical production concerns that must be addressed before deploying to production. These are **not** part of the MVP but are essential for a robust, production-grade system.

### 1. Atomic Emit & Publish

**Problem**: If a worker crashes after updating the counter but before publishing messages to Redis, the counter increases but tokens are never emitted, causing workflow stalls.

**Solution**: Transactional Outbox Pattern

```go
// BAD: Non-atomic
func (sdk *SDK) Emit(nextNodes []string, payload interface{}) {
    // 1. Apply counter +N
    sdk.applyDelta(runID, "emit:"+tokenID, len(nextNodes))

    // 2. Publish messages (CRASH HERE = counter wrong!)
    for _, node := range nextNodes {
        sdk.redis.Publish("wf.data", createToken(...))
    }
}

// GOOD: Transactional outbox
func (sdk *SDK) Emit(nextNodes []string, payload interface{}) {
    tx := sdk.db.Begin()

    // 1. Write to outbox table (atomic with counter update)
    for _, node := range nextNodes {
        tx.Exec(`
            INSERT INTO outbox (run_id, to_node, payload_ref, status)
            VALUES (?, ?, ?, 'pending')
        `, runID, node, payloadRef)
    }

    // 2. Apply counter +N (same transaction)
    tx.Exec(`
        INSERT INTO applied_keys (run_id, op_key, delta)
        VALUES (?, ?, ?)
        ON CONFLICT DO NOTHING
    `, runID, "emit:"+tokenID, len(nextNodes))

    tx.Commit()

    // 3. Background relay publishes from outbox → Redis
    // (separate goroutine polls outbox and publishes)
}
```

**Alternative**: Use Redis Transactions (MULTI/EXEC) for smaller systems.

---

### 2. Routing Policy & Capability Guardrails

**Problem**: Agent nodes can emit to any node, potentially creating security/safety issues.

**Solution**: Capability Registry + Allowlist

```sql
-- Capability registry
CREATE TABLE node_capabilities (
    node_id TEXT PRIMARY KEY,
    can_emit_to TEXT[],                 -- Allowed downstream nodes
    max_fanout INT DEFAULT 10,
    requires_approval BOOLEAN DEFAULT false,
    allowed_tools TEXT[]                -- For agent nodes
);

-- Runtime policy check
CREATE TABLE run_policies (
    run_id UUID PRIMARY KEY,
    allowed_transitions JSONB,          -- {"node_a": ["node_b", "node_c"]}
    max_hops INT DEFAULT 100,
    deadline_at TIMESTAMPTZ
);
```

**OPA Integration** (Optional):

```rego
# policy.rego
package workflow

allow_transition {
    input.from_node == "agent_qualify"
    input.to_node in ["high_value_sales", "standard_sales"]
    input.user.role == "agent"
}

deny_transition {
    input.to_node == "delete_customer"
    input.from_node != "admin_console"
}
```

---

### 3. Quotas & Backpressure

**Per-run caps**:

```go
type RunLimits struct {
    MaxFanout      int           // Max emits from single node
    MaxHops        int           // Max token traversals
    MaxWallTime    time.Duration // Total run deadline
    MaxBytesScanned int64        // Data processing limit
}

func (sdk *SDK) Emit(nextNodes []string) error {
    limits := sdk.getRunLimits(runID)

    // Check fanout
    if len(nextNodes) > limits.MaxFanout {
        return sdk.terminate(runID, "exceeded_max_fanout")
    }

    // Check hops
    if token.Headers.Hop >= limits.MaxHops {
        return sdk.terminate(runID, "exceeded_max_hops")
    }

    // Check wall time
    if time.Now().After(limits.Deadline) {
        return sdk.terminate(runID, "deadline_exceeded")
    }

    // Proceed with emit
}
```

**Backpressure handling**:
- **Throttle**: Slow down token emission (sleep between emits)
- **Summarize**: Agent condenses large datasets before emitting
- **Terminate**: Fail-fast on quota breach

---

### 4. Multi-Tenancy & Security

**Redis namespace isolation**:

```go
// Tenant-specific keys
func (sdk *SDK) getCounterKey(tenantID, runID string) string {
    return fmt.Sprintf("tenant:%s:counter:%s", tenantID, runID)
}

// Stream per tenant
func (sdk *SDK) getStreamName(tenantID string) string {
    return fmt.Sprintf("tenant:%s:wf.data", tenantID)
}
```

**CAS IAM**:

```go
// S3 bucket per tenant with IAM policies
func (cas *CAS) Store(tenantID, content []byte) (string, error) {
    bucket := fmt.Sprintf("workflow-cas-%s", tenantID)
    key := sha256(content)

    return s3.PutObject(bucket, key, content, &s3.PutObjectInput{
        ServerSideEncryption: "AES256",
        Metadata: map[string]string{
            "tenant_id": tenantID,
        },
    })
}
```

**Envelope signing** (cross-service):

```go
type SignedToken struct {
    Token     Token
    Signature string  // HMAC-SHA256(token, secret)
    SignedBy  string
}

func (sdk *SDK) PublishToken(token Token) error {
    signature := hmac.Sign(token, sdk.secret)

    signedToken := SignedToken{
        Token:     token,
        Signature: signature,
        SignedBy:  sdk.serviceID,
    }

    sdk.redis.Publish("wf.data", signedToken)
}
```

---

### 5. Retry & DLQ Semantics

**Standard backoff**:

```go
type RetryConfig struct {
    MaxAttempts int
    BackoffMs   int
    Multiplier  float64  // Exponential backoff
}

func (sdk *SDK) executeWithRetry(token Token) error {
    config := RetryConfig{MaxAttempts: 3, BackoffMs: 1000, Multiplier: 2.0}

    for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
        err := sdk.execute(token)

        if err == nil {
            return nil
        }

        if !isRetryable(err) {
            return sdk.sendToDLQ(token, err)
        }

        backoff := config.BackoffMs * int(math.Pow(config.Multiplier, float64(attempt-1)))
        time.Sleep(time.Duration(backoff) * time.Millisecond)
    }

    return sdk.sendToDLQ(token, "max_retries_exceeded")
}
```

**DLQ format**:

```sql
CREATE TABLE dlq (
    dlq_id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    run_id UUID NOT NULL,
    token_id UUID NOT NULL,
    node_id TEXT NOT NULL,

    failure_reason TEXT,
    error_details JSONB,
    attempts INT,

    token_data JSONB,           -- Original token for replay
    payload_cas_ref TEXT,

    created_at TIMESTAMPTZ DEFAULT now(),
    reprocessed_at TIMESTAMPTZ,
    status TEXT DEFAULT 'pending'  -- pending|reprocessed|discarded
);
```

**Requeue tooling**:

```bash
# CLI tool to reprocess DLQ items
$ workflow-cli dlq reprocess --run-id run_123 --node-id validate_customer
```

---

### 6. Snapshots & Recovery SLOs

**Counter snapshot frequency**:

```go
func periodicSnapshot() {
    ticker := time.NewTicker(10 * time.Second)

    for range ticker.C {
        runs := redis.Keys("counter:*")

        for _, key := range runs {
            runID := extractRunID(key)
            counter := redis.Get(key).Int()

            db.Exec(`
                INSERT INTO run_counter_snapshots (run_id, value, snapshot_at)
                VALUES (?, ?, now())
            `, runID, counter)
        }
    }
}
```

**Redis persistence mode**:

```conf
# redis.conf
appendonly yes
appendfsync everysec    # Fsync every second (balance durability/performance)
auto-aof-rewrite-percentage 100
auto-aof-rewrite-min-size 64mb
```

**Recovery SLOs**:
- **RPO** (Recovery Point Objective): Max 10 seconds of data loss (snapshot interval)
- **RTO** (Recovery Time Objective): Max 5 minutes to restore service

**Recovery procedure**:

```go
func recoverFromRedisFailure() {
    // 1. Get last known counter values from Postgres
    snapshots := db.Query(`
        SELECT run_id, value
        FROM run_counter_snapshots
        WHERE snapshot_at = (
            SELECT MAX(snapshot_at) FROM run_counter_snapshots
        )
    `)

    // 2. Replay applied_keys since last snapshot
    for _, snap := range snapshots {
        appliedSince := db.Query(`
            SELECT delta FROM applied_keys
            WHERE run_id = ? AND applied_at > ?
        `, snap.RunID, snap.SnapshotAt)

        counter := snap.Value
        for _, op := range appliedSince {
            counter += op.Delta
        }

        // 3. Restore to Redis
        redis.Set("counter:"+snap.RunID, counter)
    }
}
```

---

### 7. Cancellation & Timeouts

**Graceful cancellation**:

```go
func (s *Supervisor) CancelRun(runID string) error {
    // 1. Mark run as cancelled
    db.Exec("UPDATE runs SET status = 'CANCELLED' WHERE run_id = ?", runID)

    // 2. Publish cancellation event
    redis.Publish("cancel:"+runID, "cancel_requested")

    // 3. Workers check cancellation before processing
    // (best-effort drain - tokens in flight may complete)

    // 4. After timeout, force cleanup
    time.AfterFunc(30*time.Second, func() {
        s.forceCleanup(runID)
    })
}

// Worker checks cancellation
func (w *Worker) Execute(token Token) error {
    if isCancelled(token.RunID) {
        return ErrRunCancelled
    }

    // Proceed with execution
}
```

**Global run deadline**:

```go
// Set at run creation
deadline := time.Now().Add(workflow.MaxDuration)

db.Exec(`
    INSERT INTO runs (run_id, deadline_at, ...)
    VALUES (?, ?, ...)
`, runID, deadline)

// Background checker
func checkDeadlines() {
    expiredRuns := db.Query(`
        SELECT run_id FROM runs
        WHERE status = 'RUNNING'
          AND deadline_at < now()
    `)

    for _, run := range expiredRuns {
        CancelRun(run.RunID)
    }
}
```

---

### 8. Compensation & Rollback Hooks

**Per-node compensators**:

```json
{
  "node_id": "charge_customer",
  "type": "task",
  "config": {
    "action": "stripe.charge"
  },
  "compensator": {
    "action": "stripe.refund",
    "timeout_sec": 60,
    "retry": 3
  }
}
```

**Invocation modes**:
- **Manual**: User-triggered via API
- **Automatic**: On workflow failure or cancellation

```go
func (s *Supervisor) Rollback(runID string) error {
    // 1. Get completed nodes in reverse order
    completed := db.Query(`
        SELECT node_id, output_cas_ref
        FROM node_executions
        WHERE run_id = ? AND status = 'SUCCESS'
        ORDER BY ended_at DESC
    `, runID)

    // 2. Execute compensators
    for _, node := range completed {
        compensator := loadCompensator(node.NodeID)

        if compensator != nil {
            err := executeCompensator(compensator, node.OutputRef)
            if err != nil {
                log.Error("compensation failed", "node", node.NodeID, "error", err)
            }
        }
    }
}
```

---

### 9. Observability & SLOs

**Prometheus metrics**:

```go
var (
    tokensConsumed = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "workflow_tokens_consumed_total"},
        []string{"run_id", "node_id"},
    )

    tokensEmitted = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "workflow_tokens_emitted_total"},
        []string{"run_id", "node_id", "fan_out"},
    )

    retries = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "workflow_retries_total"},
        []string{"node_id", "error_type"},
    )

    dlqSize = prometheus.NewGauge(
        prometheus.GaugeOpts{Name: "workflow_dlq_size"},
    )

    hopLatency = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "workflow_hop_latency_ms",
            Buckets: []float64{10, 50, 100, 500, 1000, 5000},
        },
        []string{"from_node", "to_node"},
    )

    counterDivergence = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{Name: "workflow_counter_divergence"},
        []string{"run_id"},
    )
)
```

**Tracing integration**:

```go
import "go.opentelemetry.io/otel"

func (w *Worker) Execute(token Token) error {
    ctx, span := otel.Tracer("workflow").Start(
        context.Background(),
        "node.execute",
        trace.WithAttributes(
            attribute.String("run_id", token.RunID),
            attribute.String("node_id", token.ToNode),
            attribute.Int("hop", token.Headers.Hop),
        ),
    )
    defer span.End()

    // Propagate trace_id in token
    token.Headers.TraceID = span.SpanContext().TraceID().String()

    // Execute node
    result, err := w.executeNode(ctx, token)

    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
    }

    return err
}
```

**Alerts**:

```yaml
# Prometheus alerts
groups:
  - name: workflow_alerts
    rules:
      - alert: WorkflowStalled
        expr: workflow_counter_value > 0 AND rate(workflow_tokens_consumed_total[5m]) == 0
        for: 10m
        annotations:
          summary: "Workflow {{ $labels.run_id }} appears stalled"

      - alert: DLQSurge
        expr: increase(workflow_dlq_size[5m]) > 100
        for: 2m
        annotations:
          summary: "DLQ size increased by >100 in 5 minutes"

      - alert: CounterDivergence
        expr: abs(workflow_counter_divergence) > 5
        for: 1m
        annotations:
          summary: "Counter divergence detected for run {{ $labels.run_id }}"
```

---

### 10. Data Lifecycle & Retention

**CAS garbage collection**:

```sql
-- Mark CAS objects for retention
CREATE TABLE cas_retention (
    cas_id TEXT PRIMARY KEY,
    referenced_by TEXT[],           -- ["run_123", "artifact_456"]
    created_at TIMESTAMPTZ DEFAULT now(),
    retain_until TIMESTAMPTZ,
    status TEXT DEFAULT 'active'    -- active|grace_period|eligible_for_gc
);

-- GC policy: Retain until run final + 30 days grace
UPDATE cas_retention
SET status = 'eligible_for_gc'
WHERE retain_until < now()
  AND status = 'grace_period';
```

**Event log archival**:

```sql
-- Partition event_log by month
CREATE TABLE event_log_2025_01 PARTITION OF event_log
FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');

-- Archive old partitions to cold storage (S3)
-- Detach partition
ALTER TABLE event_log DETACH PARTITION event_log_2024_10;

-- Export to S3
COPY event_log_2024_10 TO PROGRAM 'aws s3 cp - s3://workflow-archive/events/2024-10.csv';

-- Drop partition
DROP TABLE event_log_2024_10;
```

**PII handling**:

```go
// Encrypt PII fields before storing in CAS
func (cas *CAS) StoreSensitive(data map[string]interface{}) (string, error) {
    sensitiveFields := []string{"email", "ssn", "phone"}

    for _, field := range sensitiveFields {
        if val, exists := data[field]; exists {
            encrypted := encrypt(val, cas.encryptionKey)
            data[field] = encrypted
        }
    }

    return cas.Store(data)
}
```

---

### 11. Versioning Policy

**Explicit rules**:

1. **Tokens pin wf_version**: Once a run starts with v1.0, all tokens carry v1.0 (no mid-flight version changes)

```json
{
  "run_id": "run_123",
  "wf_version": "1.0",  // Frozen for this run
  "token_id": "...",
  ...
}
```

2. **Patches bump version**: Each patch creates a new version

```
main@v1.0 → patch → main@v1.1 → patch → main@v1.2
```

3. **Handover node for live migration** (optional):

```json
{
  "node_id": "version_handover",
  "type": "migration",
  "config": {
    "from_version": "1.0",
    "to_version": "2.0",
    "transform": "cas://sha256:transform_script"
  }
}
```

4. **Config schema compatibility**: Breaking changes require new type_url

```
// Compatible: Add optional field
svc.validate/Config@v1 → svc.validate/Config@v1  ✅

// Breaking: Remove required field
svc.validate/Config@v1 → svc.validate/Config@v2  ⚠️
```

---

### 12. Testing & Operations

**Deterministic fixtures for counter math**:

```go
func TestCounterInvariant(t *testing.T) {
    tests := []struct {
        name     string
        ops      []Operation
        expected int
    }{
        {
            name: "simple_sequence",
            ops: []Operation{
                {Type: "init", Delta: 1},
                {Type: "consume", Delta: -1},
                {Type: "emit", Delta: 2},
                {Type: "consume", Delta: -1},
                {Type: "consume", Delta: -1},
            },
            expected: 0,
        },
        {
            name: "parallel_fanout",
            ops: []Operation{
                {Type: "init", Delta: 1},
                {Type: "consume", Delta: -1},
                {Type: "emit", Delta: 3},  // Fan-out to 3 nodes
                {Type: "consume", Delta: -1},
                {Type: "consume", Delta: -1},
                {Type: "consume", Delta: -1},
            },
            expected: 0,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            counter := 0
            for _, op := range tt.ops {
                counter += op.Delta
            }
            assert.Equal(t, tt.expected, counter)
        })
    }
}
```

**Chaos tests for duplicate delivery**:

```go
func TestDuplicateDelivery(t *testing.T) {
    sdk := setupSDK()
    token := createTestToken()

    // Simulate duplicate delivery
    go sdk.Consume(token)
    go sdk.Consume(token)  // Same token delivered twice

    time.Sleep(100 * time.Millisecond)

    // Counter should only decrement once
    counter := sdk.getCounter(token.RunID)
    assert.Equal(t, -1, counter)

    // Applied keys should have exactly one entry
    applied := sdk.getAppliedKeys(token.RunID)
    assert.Len(t, applied, 1)
}
```

**Load tests per shard**:

```bash
# Vegeta load test
echo "POST http://localhost:8080/api/v1/runs
Content-Type: application/json
@request.json" | vegeta attack -rate=1000 -duration=60s | vegeta report
```

**Runbook: Counter stuck at non-zero**:

```markdown
## Symptom
Counter ≠ 0 but no tokens are moving (workflow appears stalled)

## Diagnosis
1. Check pending messages:
   ```
   redis-cli XPENDING wf.data workflow-group
   ```

2. Check applied_keys vs counter:
   ```sql
   SELECT
     SUM(delta) as expected_counter,
     (SELECT value FROM run_counters WHERE run_id = 'run_123') as actual_counter
   FROM applied_keys
   WHERE run_id = 'run_123';
   ```

3. Check DLQ:
   ```sql
   SELECT * FROM dlq WHERE run_id = 'run_123';
   ```

## Resolution
- If counter diverged: Recompute from applied_keys and update
- If tokens stuck in Redis: Manually ACK or requeue
- If node failed: Check DLQ and reprocess
```

---

## Open Questions

1. **Redis Persistence**: AOF vs RDB vs Cluster? What's acceptable data loss?

2. **Event Log Volume**: How long to retain events? Archive strategy?

3. **Agent Tool Registration**: How do agents discover available tools? Registry?

4. **HITL UI**: Build in-house or integrate with existing approval system?

5. **Schema Validation**: Enforce config schemas at compile time or runtime?

6. **Multi-Tenancy**: How to isolate runs between tenants? Separate Redis namespaces?

7. **Backpressure**: What happens if workers can't keep up? Queue limits?

8. **Cost Allocation**: How to attribute Redis/Postgres costs per workflow?

9. **Testing Strategy**: Integration tests with real Redis? Docker Compose?

10. **Migration Path**: How to roll out changes to IR format without breaking in-flight runs?

---

## Summary

This design provides:

✅ **Deterministic execution** via token choreography
✅ **Agentic routing** with LLM-based decisions
✅ **Event-driven architecture** (Redis Streams + event log)
✅ **Persistent, replayable state** (event sourcing)
✅ **Human-in-the-loop** (approval system)
✅ **Visual orchestration** (DSL → IR, patches for edits)
✅ **Horizontal scalability** (Redis hot path, worker pools)
✅ **Resilience** (idempotent tokens, immutable CAS)
✅ **Extensibility** (node type registry, plug-in workers)

**Next Steps**: Review this design, answer open questions, then proceed with Phase 1 implementation.

---

**End of Design Document**