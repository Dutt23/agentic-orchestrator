# Vision Architecture (Production Target)

> **Event-driven orchestration with agentic overlays and full observability**

## 📖 Document Overview

**Purpose:** Complete production architecture with Kafka, WASM, and advanced scaling

**In this document:**
- [System Architecture](#system-architecture) - Complete production diagram
- [Core Principles](#core-architectural-principles) - Event sourcing, overlays, scalability
- [Key Differences](#key-differences-from-phase-1-mvp) - MVP vs. Production comparison
- [Advanced Components](#advanced-components-not-in-phase-1) - WASM optimizer, Kafka, etc.
- [Agent Execution Environments](#agent-execution-environments) - K8s, Lambda, customer infra
- [Data Flow](#data-flow-end-to-end) - Complete request lifecycle
- [Performance Targets](#performance-targets) - 10K workflows/sec goal
- [Core Innovation Explained](#core-innovation-how-agent-resolution-works) - Deep dive on patching

---

## Overview

This document describes the complete production architecture. While Phase 1 (MVP) proves the core concepts with Redis, this vision represents a horizontally scalable, enterprise-grade platform capable of 10K+ workflows/sec with Kafka and WASM optimization.

**Source:** [../references/arch.txt](../references/arch.txt) - Complete vision specification

---

## System Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                  Agentic Orchestration Platform (Production)     │
├─────────────────────────────────────────────────────────────────┤
│                                                                   │
│  ┌─────────────────── CONTROL PLANE ────────────────────┐       │
│  │                                                        │       │
│  │  Edge Proxy (HAProxy/Envoy)                          │       │
│  │  ├─ TLS termination                                  │       │
│  │  ├─ Rate limiting (workflow-aware)                   │       │
│  │  ├─ SSE/WS upgrade                                   │       │
│  │  └─ Sticky routing by run_id                        │       │
│  │          ↓                                            │       │
│  │  API Service                                          │       │
│  │  ├─ Start/Cancel/Replay runs                        │       │
│  │  ├─ Approve/Reject HITL                             │       │
│  │  └─ ApplyPatch, GetRun/Timeline                     │       │
│  │          ↓                                            │       │
│  │  Orchestrator                                         │       │
│  │  ├─ Workflow resolution (base + patches)            │       │
│  │  ├─ Materialization (overlays)                      │       │
│  │  ├─ Durable timers                                   │       │
│  │  └─ Outbox → Kafka                                   │       │
│  │          ↓                                            │       │
│  │  Validator                                            │       │
│  │  ├─ Validates agent patches                         │       │
│  │  ├─ Schema validation                               │       │
│  │  ├─ Agent spawn limit checks                        │       │
│  │  └─ SSRF protection                                  │       │
│  │                                                        │       │
│  └────────────────────────────────────────────────────────┘       │
│                                                                   │
│  ┌─────────────────── DATA PLANE ──────────────────────┐        │
│  │                                                       │        │
│  │  Runner (Go)                                         │        │
│  │  ├─ Execute function/map/join/composite             │        │
│  │  ├─ Idempotency keys                                │        │
│  │  └─ CAS I/O                                         │        │
│  │                                                       │        │
│  │  Agent Runner (Any Execution Env)                   │        │
│  │  ├─ K8s Jobs, Lambda, or customer env              │        │
│  │  ├─ LLM/tool execution                              │        │
│  │  ├─ Structured outputs                              │        │
│  │  └─ AIR patch proposals                             │        │
│  │                                                       │        │
│  │  Fanout                                              │        │
│  │  ├─ Timeline/logs/events streaming                  │        │
│  │  ├─ SSE/WS for real-time UI                        │        │
│  │  ├─ Bounded buffers + heartbeats                   │        │
│  │  └─ Multi-user support                              │        │
│  │                                                       │        │
│  │  HITL Service                                        │        │
│  │  ├─ Human approval gates                            │        │
│  │  ├─ Timeout + escalation                            │        │
│  │  └─ Branching (approve/reject paths)               │        │
│  │                                                       │        │
│  │  Optimizer (WASM)                                    │        │
│  │  ├─ Graph rewrite passes                           │        │
│  │  ├─ HTTP fusion, pruning, caching                  │        │
│  │  └─ Emits OptimizedPatch overlays                  │        │
│  │                                                       │        │
│  └─────────────────────────────────────────────────────┘        │
│                                                                   │
│  ┌─────────────────── STORAGE LAYER ──────────────────┐         │
│  │                                                      │         │
│  │  Kafka/Redpanda (Event Bus + Queues)               │         │
│  │  ├─ workflow.events (domain events)                │         │
│  │  ├─ node.jobs.{high|medium|low} (work by priority) │         │
│  │  └─ node.results (executor outputs)                │         │
│  │                                                      │         │
│  │  Postgres (Metadata + Event Log)                   │         │
│  │  ├─ runs (workflow instances)                      │         │
│  │  ├─ patches (agent modifications)                  │         │
│  │  ├─ artifacts (workflow versions)                  │         │
│  │  └─ event_log (append-only audit)                  │         │
│  │                                                      │         │
│  │  S3/MinIO (CAS - Content-Addressed Storage)        │         │
│  │  ├─ All data content-addressed (sha256)            │         │
│  │  ├─ Workflow definitions                            │         │
│  │  ├─ Node outputs                                    │         │
│  │  └─ Immutable, deduplicated                        │         │
│  │                                                      │         │
│  │  Dragonfly/Redis (Cache)                           │         │
│  │  ├─ Memoized node results                          │         │
│  │  ├─ Session state                                   │         │
│  │  └─ Metadata + CAS pointers                        │         │
│  │                                                      │         │
│  └──────────────────────────────────────────────────────┘        │
└─────────────────────────────────────────────────────────────────┘
```

---

## Core Architectural Principles

### 1. Deterministic Event Sourcing

**Principle:** Everything is append-only, no mutation

```
Write: Orchestrator → Postgres (event log) → Outbox → Kafka
Execute: Workers consume from Kafka → Store outputs in CAS → Publish results
```

**Benefits:**
- Full audit trail
- Replay from any state
- Conflict resolution via event comparison
- Deterministic reproduction

### 2. Agentic Overlays (Not Mutations!)

**Principle:** Agents propose patches, never mutate base graph

```
Base Workflow (v1.0)
  + Agent Patch 1 (adds nodes)
  + Agent Patch 2 (adds edges)
  + Optimizer Patch (fuses nodes)
  = Materialized Workflow (v1.3)
```

**This is the core innovation!**

**Benefits:**
- Reproducibility (base + patches = deterministic)
- Rollback (remove patch)
- Audit trail (who changed what)
- A/B testing (apply different patch sets)

### 3. Horizontal Scalability via Partitioning

**Partition by run_id:**
```
Kafka partitions (64-256)
  ├─ run_001 → partition 0
  ├─ run_002 → partition 1
  └─ run_003 → partition 0

Workers consume from partitions
→ Multiple workers per partition (consumer groups)
→ Linear scaling
```

### 4. OS-Level Efficiency

**CPU Pinning:**
```
Control Plane: Cores 0-7  (Orchestrator, API, Validator)
Runner Plane: Cores 8-15  (Runners, Agent Runners)
Fanout Plane: Cores 16-19 (Real-time streaming)
```

**Network Stack:**
```
SO_REUSEPORT: Distribute accepts across threads
HTTP/2: Multiplexing for SSE/WS
GRO/GSO: Packet batching (20-30% CPU reduction)
```

**See:** [../operations/SCALABILITY.md](../operations/SCALABILITY.md) and [../../scripts/systemd/README.md](../../scripts/systemd/README.md)

---

## Key Differences from Phase 1 (MVP)

| Aspect | Phase 1 (MVP) | Vision (Production) |
|--------|--------------|---------------------|
| **Event Bus** | Redis Streams | Kafka/Redpanda (64-256 partitions) |
| **Queuing** | RPUSH/BLPOP | Kafka topics (priority-based) |
| **State** | Redis hot path | Event sourcing + Postgres |
| **Communication** | HTTP REST | gRPC (low latency) |
| **Serialization** | JSON | Protobuf (typed, efficient) |
| **Caching** | Basic Redis | Dragonfly (25x throughput) |
| **Agent Execution** | Local processes | K8s Jobs, Lambda, or customer env |
| **Optimizer** | Rust stubs | WASM (compiled, dynamic) |
| **Streaming** | WebSocket | WebTransport (QUIC-based) |
| **Observability** | Logs + metrics | eBPF uprobes + distributed tracing |
| **CLI** | Basic | Full aob CLI (stream, approve, replay) |
| **Throughput** | ~1K workflows/sec | ~10K workflows/sec |
| **Latency** | 5-10ms/hop | <2ms/hop |

---

## Advanced Components (Not in Phase 1)

### 1. WASM Optimizer

**Purpose:** Runtime graph optimization without code deployment

**Optimization Passes:**

1. **HTTP Coalescing** - Merge sequential HTTP nodes into batch call
2. **Predicate Pushdown** - Move filters closer to data source
3. **Common Subexpression Elimination** - Deduplicate identical subgraphs
4. **Loop-Invariant Code Motion** - Hoist computations out of loops
5. **Dead Path Pruning** - Remove unreachable nodes
6. **Parallel Executor** - Convert sequential to parallel where safe
7. **Semantic Cache** - Identify cacheable subgraphs

**Example:**
```
Before:
  fetch_A → parse_A → fetch_B → parse_B → merge

After (optimized):
  batch_fetch(A, B) → parallel_parse(A, B) → merge
```

**Implementation:**
```rust
// crates/dag-optimizer/src/optimizers/http_coalescer.rs
pub fn coalesce_http_nodes(ir: &mut IR) -> Vec<OptimizedPatch> {
    // Find sequential HTTP nodes with same host
    // Merge into composite HTTP batch node
    // Return patch operations as JSON Patch
}
```

**Benefits:**
- Dynamic optimization (no redeploy)
- Safe (validated patches via same validation as agent patches)
- Observable (optimizer emits audit log)

**Code:** [../../crates/dag-optimizer/](../../crates/dag-optimizer/)

---

### 2. Agent Execution Environments

**Purpose:** Isolated, scalable agent execution - customer brings their own environment

**Supported Environments:**

1. **Kubernetes Jobs** (default)
   ```yaml
   apiVersion: batch/v1
   kind: Job
   metadata:
     name: agent-job-{{ .JobID }}
   spec:
     template:
       spec:
         containers:
         - name: agent
           image: acme/agent-runner:v1.2
           resources:
             limits: {memory: "2Gi", cpu: "1000m"}
   ```

2. **AWS Lambda**
   ```json
   {
     "FunctionName": "agent-runner",
     "Runtime": "python3.11",
     "Handler": "main.handler",
     "Environment": {
       "JOB_ID": "{{.JobID}}",
       "RUN_ID": "{{.RunID}}"
     }
   }
   ```

3. **Customer-Provided Environment**
   ```
   Customer deploys agent runner in their infra:
   - On-prem K8s cluster
   - Cloud Run
   - ECS/Fargate
   - Custom Docker setup

   Requirements:
   - Can consume from Kafka topic: agent.jobs
   - Can publish to Kafka topic: agent.results
   - Has network access to OpenAI/Anthropic APIs
   ```

**Benefits:**
- **Flexibility**: Customer chooses infrastructure
- **Isolation**: Agents run in sandboxed environments
- **Resource limits**: CPU/memory enforcement per environment
- **Scalability**: Environment-specific autoscaling
- **Cost control**: Customer pays for their agent compute

---

### 3. Kafka Event Bus

**Purpose:** Replace Redis Streams with production-grade message bus

**Topics:**

```
workflow.events        → Domain events (run lifecycle)
node.jobs.high        → High-priority work
node.jobs.medium      → Medium-priority work
node.jobs.low         → Low-priority work
node.results          → Execution outputs
```

**Configuration:**

```properties
# Producers
enable.idempotence=true
acks=all
linger.ms=5
compression.type=snappy

# Partitions
num.partitions=64  # Scale linearly

# Consumer groups
group.id=workers
enable.auto.commit=false
```

**Benefits:**
- **10x throughput** vs. Redis Streams
- Durable, replicated
- Priority-based queuing
- Exactly-once semantics

---

### 4. aob CLI Tool

**Purpose:** Developer-friendly command-line interface for workflow management

**Key Commands:**

```bash
# Start workflow
aob run start workflow.json --inputs input.json

# Stream logs in real-time
aob logs stream run_7f3e4a

# Filter logs by node
aob logs stream run_7f3e4a --node enrich

# HITL approvals
aob approve ticket_456 approve --reason "LGTM"
aob approve ticket_789 reject --reason "Need more data"

# Replay from checkpoint
aob replay run_7f3e4a --from parse --mode freeze

# Patch management
aob patch list run_7f3e4a
aob patch show patch_abc123
aob patch approve patch_abc123

# Workflow validation
aob workflow validate workflow.json
aob workflow list
```

**Features:**
- SSE-based log streaming (real-time)
- JSON output for scripting
- Progress indicators and spinners
- Fast (Rust-based)
- Low latency (<10ms startup)

**Documentation:** [../cli/README.md](../cli/README.md), [../cli/COMMANDS.md](../cli/COMMANDS.md)

---

## Data Flow (End-to-End)

### 1. Submit Run

```
User/CLI → API: POST /runs {tag: "main", inputs: {...}}
```

### 2. Resolve & Materialize

```
API → Load base workflow from tag
API → Fetch existing patches for workflow
API → Materialize: base + patches → executable workflow
API → Compile to IR (Intermediate Representation)
API → Store IR in cache (Redis/Dragonfly)
```

**This is where agent resolution happens!**

### 3. Initialize Run

```
API → Postgres: INSERT INTO runs (...)
API → Redis/Kafka: Publish first tokens to entry nodes
API → Response: {run_id, status: "RUNNING"}
```

### 4. Execution (Choreography)

```
Worker → Consume from Kafka (or Redis Stream)
Worker → Load config from CAS
Worker → Execute business logic
Worker → Store output in CAS
Worker → Publish completion signal
Coordinator → Route to next nodes based on DAG
```

### 5. Agent Patch Application

```
Agent Worker → Calls LLM → Generates patch
Agent Worker → POST /runs/{run_id}/patch
Orchestrator → Validates patch
Orchestrator → Applies patch (creates new artifact version)
Orchestrator → Recompiles IR
Orchestrator → Updates cached IR
Coordinator → Loads NEW IR on next completion
Coordinator → Routes to NEW nodes from patched IR
```

**Workflow continues with modified topology!**

### 6. Completion Detection

```
Coordinator → Check counter == 0
Coordinator → Check no pending HITL approvals
Coordinator → Mark run as COMPLETED
Coordinator → Cleanup ephemeral state
```

---

## Performance Targets

### Throughput

| Metric | Target | Notes |
|--------|--------|-------|
| Workflow starts | 10,000/sec | Kafka partition parallelism |
| Node executions | 100,000/sec | Horizontal runner scaling |
| Agent decisions | 1,000/sec | LLM API limits + caching |
| HITL approvals | 500/sec | Human bottleneck |

### Latency

| Operation | P50 | P95 | P99 |
|-----------|-----|-----|-----|
| Kafka publish | <1ms | 2ms | 5ms |
| Node execution (simple) | 5ms | 20ms | 50ms |
| Agent decision (cached LLM) | 200ms | 1s | 5s |
| Coordinator routing | <1ms | 2ms | 5ms |

### Scalability

| Component | Scaling Strategy |
|-----------|------------------|
| API Service | Horizontal (stateless) |
| Orchestrator | Horizontal (partition by run_id) |
| Runner | Horizontal (consumer groups) |
| Agent Runner | Environment-specific autoscaling |
| Fanout | Horizontal (sticky sessions) |
| Kafka | Add brokers + partitions |
| Postgres | Read replicas + sharding |

---

## Deployment Modes

### Fast Path Mode

**Goal:** Maximum throughput, minimal tracing

**Optimizations:**
- Minimal event logging (only critical events)
- No overlay diffs stored (just final state)
- Aggressive caching
- Batch Kafka publishes
- Async metadata updates

**Use Case:** High-volume deterministic workflows

---

### Full Fidelity Mode

**Goal:** Complete observability, audit trail

**Features:**
- Every state change logged
- Overlay diffs preserved (base + each patch)
- Real-time UI updates
- Distributed tracing (span per node)
- Full replay capability

**Use Case:** Regulated workloads, demos, debugging

---

## Core Innovation: How Agent Resolution Works

### Problem Statement

Traditional systems:
- **Static workflows**: Can't modify during execution
- **Dynamic agents**: No deterministic backbone

**Our approach:** Merge both!

### Solution: Materialization + Recompilation

**Step-by-step:**

1. **Base Workflow** (stored in database)
   ```json
   {
     "nodes": [
       {"id": "fetch", "type": "http"},
       {"id": "process", "type": "function"}
     ],
     "edges": [{"from": "fetch", "to": "process"}]
   }
   ```

2. **Agent Decides to Patch** (mid-execution)
   ```python
   llm_response = llm.call("Add email notification")
   patch = {
     "operations": [
       {"op": "add", "path": "/nodes/-", "value": {"id": "email", "type": "task"}},
       {"op": "add", "path": "/edges/-", "value": {"from": "process", "to": "email"}}
     ]
   }
   ```

3. **Validation** (3 layers)
   - Python: Syntax + agent spawn limit
   - Go: Schema validation
   - Coordinator: Security check

4. **Materialization** (base + patches)
   ```json
   {
     "nodes": [
       {"id": "fetch", "type": "http"},
       {"id": "process", "type": "function"},
       {"id": "email", "type": "task"}  ← NEW!
     ],
     "edges": [
       {"from": "fetch", "to": "process"},
       {"from": "process", "to": "email"}  ← NEW!
     ]
   }
   ```

5. **Recompilation to IR**
   ```
   Materialized workflow → IR Compiler → Intermediate Representation
   → Cache in Redis: ir:{run_id}
   ```

6. **Coordinator Picks Up** (on next completion signal)
   ```go
   ir := loadIR(runID)  // Gets NEW version!
   nextNodes := determineNextNodes(ir, completedNode)
   // Routes to "email" node from patched IR
   ```

**This is how we resolve agent intentions into execution!**

---

## Migration Path from Phase 1

**See:** [MIGRATION_PATH.md](./MIGRATION_PATH.md) for detailed evolution strategy.

**Summary:**
1. **Phase 2:** Add Kafka alongside Redis (dual-write)
2. **Phase 3:** Move workers to Kafka (dual-read), verify consistency
3. **Phase 4:** Remove Redis writes (Kafka primary)
4. **Phase 5:** Compile WASM optimizer
5. **Phase 6:** Add gRPC alongside HTTP
6. **Phase 7:** Deploy agent runners in customer environments

**Timeline:** 6-12 months for full migration (incremental, no downtime)

---

## References

**Vision Source:**
- [../references/arch.txt](../references/arch.txt) - Complete vision document
- [../references/readme.MD](../references/readme.MD) - Project README

**Implementation Guides:**
- [../../docs/CHOREOGRAPHY_EXECUTION_DESIGN.md](../../docs/CHOREOGRAPHY_EXECUTION_DESIGN.md) - Execution model
- [../../docs/AGENT_SERVICE.md](../../docs/AGENT_SERVICE.md) - Agent implementation
- [../references/performance-tuning.MD](../references/performance-tuning.MD) - Performance guide

**Operations:**
- [../../scripts/systemd/README.md](../../scripts/systemd/README.md) - OS-level tuning
- [../operations/SCALABILITY.md](../operations/SCALABILITY.md) - Scaling strategies

---

**Status:** Fully designed, architecture validated through Phase 1 implementation, ready for incremental evolution.
