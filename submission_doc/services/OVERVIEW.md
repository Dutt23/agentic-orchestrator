# Services Overview

> **Comprehensive catalog of all microservices with links to detailed documentation**

## 📖 Document Overview

**Purpose:** Complete catalog of 7 services with responsibilities and documentation links

**In this document:**
- [Service Architecture](#service-architecture) - System diagram
- [1. Orchestrator](#1-orchestrator-service) - API + workflow management
- [2. Workflow-Runner](#2-workflow-runner-coordinator) - Stateless coordinator
- [3. Agent-Runner-Py](#3-agent-runner-py) - LLM integration
- [4. HTTP-Worker](#4-http-worker) - HTTP execution with SSRF protection
- [5. HITL-Worker](#5-hitl-worker) - Human approvals
- [6. Fanout](#6-fanout-service) - WebSocket streaming
- [7. aob CLI](#7-aob-cli-tool) - Developer tool
- [Communication Patterns](#service-communication-patterns) - How services interact
- [Deployment](#deployment) - Running services

---

## Service Architecture

The platform consists of 6 core microservices, each with a single, well-defined responsibility. All services communicate via Redis Streams (MVP) with interfaces designed for Kafka migration.

```
Browser/CLI
    ↓
┌─────────────────────────────────────────────────┐
│            Orchestrator (API + State)           │
│  • Workflow CRUD                                │
│  • Patch application                            │
│  • Rate limiting                                │
│  [cmd/orchestrator/]                            │
└─────────────┬───────────────────────────────────┘
              ↓ Redis Streams
┌─────────────────────────────────────────────────┐
│       Workflow-Runner (Coordinator)             │
│  • Stateless choreography                      │
│  • Node routing                                 │
│  • Completion detection                         │
│  [cmd/workflow-runner/]                         │
└─────────────┬───────────────────────────────────┘
              ↓ wf.tasks.{type} streams
┌──────────────────────────────────────────────────────────┐
│                   Worker Tier                            │
│  ┌─────────────┐  ┌─────────────┐  ┌────────────────┐  │
│  │ HTTP Worker │  │ HITL Worker │  │ Agent Runner   │  │
│  │ [cmd/http-  │  │ [cmd/hitl-  │  │ [cmd/agent-    │  │
│  │  worker/]   │  │  worker/]   │  │  runner-py/]   │  │
│  └─────────────┘  └─────────────┘  └────────────────┘  │
└───────────────────────┬──────────────────────────────────┘
                        ↓ Completion signals
               Back to Coordinator
                        ↓
┌─────────────────────────────────────────────────┐
│              Fanout (WebSocket)                 │
│  • Real-time UI updates                         │
│  • SSE/WebSocket streaming                      │
│  [cmd/fanout/]                                  │
└─────────────────────────────────────────────────┘
```

---

## 1. Orchestrator Service

**Path:** `cmd/orchestrator/`

**Role:** API gateway + workflow metadata management + materialization

**Technology:** Go, Echo framework, Postgres, Redis

### Responsibilities

- **Workflow CRUD**: Create, read, update, delete workflows
- **Run submission**: Accept workflow execution requests
- **Patch application**: Apply agent-generated patches to workflows (JSON Patch operations)
- **Materialization**: Merge base workflow + patches → executable workflow
- **Rate limiting**: Workflow-aware tiered rate limiting (Simple/Standard/Heavy)
- **Tag management**: Git-like workflow versioning

### Key Features

1. **Layered Architecture**
   - **Handlers** (`handlers/`): HTTP request/response
   - **Services** (`service/`): Business logic
   - **Repository** (`repository/`): Data access

2. **Materialization System**
   ```
   Base Workflow (from DB)
     + Patch 1 (agent adds nodes)
     + Patch 2 (agent modifies edges)
     = Materialized Workflow
     → Compile to IR
     → Store in Redis
   ```

3. **Workflow-Aware Rate Limiting**
   ```go
   // Inspects workflow complexity
   func InspectWorkflow(w *Workflow) RateLimitTier {
       agentCount := countNodesByType(w, "agent")
       if agentCount == 0 { return SimpleTier }  // 100/min
       if agentCount <= 2 { return StandardTier } // 20/min
       return HeavyTier  // 5/min
   }
   ```

### API Endpoints

```
POST   /api/v1/runs                    # Submit workflow execution
GET    /api/v1/runs/:id                # Get run status
POST   /api/v1/runs/:id/patch          # Apply patch mid-execution
GET    /api/v1/workflows/:tag          # Get workflow by tag
POST   /api/v1/workflows               # Create workflow
GET    /api/v1/tags                    # List all tags
POST   /api/v1/tags/:name/move         # Move tag (version control)
```

### Documentation

- **README:** [../../cmd/orchestrator/README.md](../../cmd/orchestrator/README.md)
- **Architecture:** [../../cmd/orchestrator/ARCHITECTURE.md](../../cmd/orchestrator/ARCHITECTURE.md)
- **Schema:** [../../docs/schema/](../../docs/schema/)

### Performance

- **Throughput:** ~5,000 req/sec
- **Latency (P50):** 20ms
- **Bottleneck:** Postgres writes

---

## 2. Workflow-Runner (Coordinator)

**Path:** `cmd/workflow-runner/`

**Role:** Stateless choreography coordinator

**Technology:** Go, Redis Streams

### Responsibilities

- **Completion signal processing**: BLPOP from `completion_signals` queue
- **IR loading**: Load latest compiled workflow from Redis (may be patched!)
- **Node routing**: Determine next nodes from DAG topology
- **Stream routing**: Route tokens to appropriate worker streams by node type
- **Completion detection**: Check counter == 0 for workflow completion
- **Security**: Enforce agent spawn limits (max 5 per workflow)

### Key Features

1. **Stateless Design**
   - Zero persistent state in coordinator
   - All state in Redis: `ir:{run_id}`, `context:{run_id}`, `counter:{run_id}`
   - Can crash and resume without data loss
   - Horizontal scaling via consumer groups

2. **Dynamic Patch Reloading**
   ```go
   func (c *Coordinator) handleCompletion(signal CompletionSignal) {
       // ALWAYS load latest IR (may be patched!)
       ir := c.loadIR(signal.RunID)

       // Determine next nodes from (possibly new) IR
       nextNodes := c.determineNextNodes(ir, signal)

       // Route to appropriate streams
       for _, node := range nextNodes {
           stream := c.getStreamForNodeType(node.Type)
           c.publishToken(stream, ...)
       }
   }
   ```

3. **Node Router**
   - Routes by node type: `agent` → `wf.tasks.agent`, `http` → `wf.tasks.http`
   - Handles skipped nodes (unknown types) → auto-complete with warning
   - Handles branches, loops, joins

### Data Flow

```
1. Worker completes node
2. Worker: RPUSH completion_signals {run_id, node_id, result_ref}
3. Coordinator: BLPOP completion_signals
4. Coordinator: Load IR from Redis (ir:{run_id})
5. Coordinator: Determine next nodes from DAG
6. Coordinator: Route to wf.tasks.{type} streams
7. Workers: XREAD from their type-specific streams
```

### Key Files

- `coordinator/completion_handler.go` - Main loop (BLPOP)
- `coordinator/patch_loader.go` - Reloads IR after agent patches
- `coordinator/node_router.go` - Routing logic + skipped node handling
- `coordinator/completion.go` - Counter == 0 detection

### Documentation

- **Design:** [../../docs/CHOREOGRAPHY_EXECUTION_DESIGN.md](../../docs/CHOREOGRAPHY_EXECUTION_DESIGN.md) (98KB)
- **Run Lifecycle:** [../../docs/RUN_LIFECYCLE_ARCHITECTURE.md](../../docs/RUN_LIFECYCLE_ARCHITECTURE.md)

### Performance

- **Throughput:** ~1,000 workflows/sec (single instance)
- **Latency (P50):** 5ms per routing decision
- **Scalability:** Horizontal via consumer groups

---

## 3. Agent-Runner-Py

**Path:** `cmd/agent-runner-py/`

**Role:** LLM-powered agent execution (can run in K8s, Lambda, or customer environment)

**Technology:** Python 3.11, OpenAI SDK, httpx

### Responsibilities

- **LLM integration**: Call OpenAI with function calling (5 tools)
- **Data pipelines**: Execute ephemeral pipelines (`execute_pipeline` tool)
- **Workflow patching**: Generate workflow patches (`patch_workflow` tool)
- **Self-correction**: Retry on validation errors with examples
- **Result storage**: Store in CAS or database

### Key Features

1. **Two-Lane Architecture**
   - **Fast Lane** (`execute_pipeline`): Ephemeral data operations (no workflow modification)
   - **Patch Lane** (`patch_workflow`): Permanent workflow changes (add/modify nodes)

2. **5 Tools for LLM**
   - `execute_pipeline` - Composable data transformations (10 primitives)
   - `patch_workflow` - JSON Patch operations on workflow
   - `search_tools` - Discover existing composite tools
   - `openapi_action` - Call any OpenAPI-compliant API
   - `delegate_to_agent` - Delegate to external agents (K8s jobs - future)

3. **Pipeline Primitives** (10 composable operations)
   - `http_request` - GET/POST to APIs
   - `table_sort`, `table_filter`, `table_select` - Table operations
   - `top_k` - Take first K records
   - `groupby`, `join` - Aggregations
   - `jq_transform`, `regex_extract`, `parse_date` - Transformations

4. **LLM Optimizations**
   - **Prompt caching**: System prompt (3K tokens) cached by OpenAI → 50% cost savings, 80% latency reduction
   - **HTTP connection pooling**: Reuses TCP connections → 200-300ms latency savings
   - **Validation loop**: Sends validation errors back to LLM with examples for retry

### Example Flow

```
1. Agent node triggered
2. Coordinator → XADD wf.tasks.agent {job_data}
3. Agent worker → XREAD wf.tasks.agent
4. Agent → OpenAI API call with 5 tools
5. LLM responds: execute_pipeline([...])
6. Agent executes pipeline
7. Agent stores result in CAS
8. Agent → RPUSH completion_signals {result_ref}
9. Coordinator processes completion
```

### Documentation

- **README:** [../../cmd/agent-runner-py/README.md](../../cmd/agent-runner-py/README.md)
- **Agent Service:** [../../cmd/agent-runner-py/docs/AGENT_SERVICE.md](../../cmd/agent-runner-py/docs/AGENT_SERVICE.md)
- **LLM Optimizations:** [../../cmd/agent-runner-py/docs/LLM_OPTIMIZATIONS.md](../../cmd/agent-runner-py/docs/LLM_OPTIMIZATIONS.md)
- **Rate Limiting:** [../../cmd/agent-runner-py/docs/RATE_LIMITING_PLAN.md](../../cmd/agent-runner-py/docs/RATE_LIMITING_PLAN.md)
- **Patch Flow:** [../../cmd/agent-runner-py/docs/TEST_PATCH_FLOW.md](../../cmd/agent-runner-py/docs/TEST_PATCH_FLOW.md)

### Execution Environments

Agents can run in:
- **Local processes** (MVP) - Development and testing
- **Kubernetes Jobs** - Production with isolation
- **AWS Lambda** - Serverless, auto-scaling
- **Customer infrastructure** - On-prem, custom Docker, etc.

**Requirements:**
- Consume from work queue (Redis Stream or Kafka)
- Publish results back
- Network access to LLM APIs

### Performance

- **Throughput:** ~100 decisions/sec
- **Latency (P50):** 500ms (with cache), 2000ms (no cache)
- **Bottleneck:** OpenAI API rate limits

---

## 4. HTTP-Worker

**Path:** `cmd/http-worker/`

**Role:** Execute HTTP requests with SSRF protection

**Technology:** Go, net/http

### Responsibilities

- **HTTP execution**: Execute GET/POST requests
- **SSRF protection**: 4-layer validation (protocol, IP, host, path)
- **Metrics tracking**: Request duration, success rate
- **Error handling**: Retries, timeouts

### Key Features

1. **4-Layer SSRF Protection**
   ```
   Layer 1: Protocol validation (only HTTP/HTTPS)
   Layer 2: IP validation (block private ranges)
   Layer 3: Host validation (block metadata endpoints)
   Layer 4: Path validation (block sensitive paths)
   ```

2. **Blocked Targets**
   - Private IPs: 10.0.0.0/8, 192.168.0.0/16, 172.16.0.0/12, 127.0.0.1
   - Cloud metadata: 169.254.169.254, metadata.google.internal
   - Localhost aliases: localhost, 0.0.0.0, ::1
   - Sensitive paths: /admin, /.env, /api/keys

3. **Security Validators**
   ```go
   // cmd/http-worker/security/
   ├── protocol_validator.go  // HTTP/HTTPS only
   ├── ip_validator.go        // Block private ranges
   ├── host_validator.go      // Block metadata endpoints
   └── path_validator.go      // Block sensitive paths
   ```

### Documentation

- **Security:** [../../cmd/http-worker/security/SECURITY.md](../../cmd/http-worker/security/SECURITY.md)
- **Validators:** [../../cmd/http-worker/security/](../../cmd/http-worker/security/)

### Performance

- **Throughput:** ~10,000 req/sec
- **Latency (P50):** 50ms (network-dependent)
- **Bottleneck:** Network I/O

---

## 5. HITL-Worker

**Path:** `cmd/hitl-worker/`

**Role:** Human-in-the-loop approval workflow

**Technology:** Go, Redis

### Responsibilities

- **Approval creation**: Create approval records in Redis
- **Pause workflow**: Set counter to 0 with pending approval flag
- **Resume on decision**: Send completion signal after human approval
- **Timeout handling**: Auto-approve/reject on timeout
- **Branching**: Support approve path vs. reject path

### Key Features

1. **Workflow Pause Mechanism**
   ```
   1. HITL node triggered
   2. Worker consumes token (counter - 1)
   3. Create approval: SADD pending_approvals:{run_id} {approval_id}
   4. Counter = 0 BUT pending_approvals != 0
   5. Supervisor: Don't mark as complete (workflow paused)
   6. Human approves → SREM pending_approvals:{run_id}
   7. Worker: RPUSH completion_signals → emits to next nodes
   8. Counter increments, workflow resumes
   ```

2. **Approval State**
   ```redis
   # Approval details
   HSET approval:{approval_id}
     run_id "run_123"
     node_id "manager_approval"
     status "pending"
     created_at "2025-10-15T10:00:00Z"
     timeout_at "2025-10-15T12:00:00Z"

   # Pending tracking
   SADD pending_approvals:run_123 {approval_id}
   ```

3. **Branching Support**
   ```
   Approve → Next nodes from config.dependents
   Reject → Next nodes from config.rejection_path
   ```

### API Endpoints

```
POST /approvals/:id/decide
Body: {decision: "approve|reject", comments: "..."}
```

### Performance

- **Throughput:** ~500 approvals/sec (human bottleneck)
- **Latency:** Depends on human response time (minutes to hours)

---

## 6. Fanout Service

**Path:** `cmd/fanout/`

**Role:** WebSocket pub/sub for real-time UI updates

**Technology:** Go, WebSocket, Redis Pub/Sub

### Responsibilities

- **WebSocket management**: Maintain connections per run
- **Event broadcasting**: Subscribe to Redis pub/sub, forward to WebSocket clients
- **Multi-user support**: Multiple clients can watch same run
- **Connection lifecycle**: Handle connect, disconnect, reconnect
- **CORS support**: Allow frontend connections

### Key Features

1. **Event Types**
   - `node_started`
   - `node_completed`
   - `node_failed`
   - `workflow_completed`
   - `workflow_failed`
   - `approval_required`

2. **Connection Management**
   ```go
   // Track active connections
   connections := map[string][]*websocket.Conn{
       "run_123": [conn1, conn2],
       "run_456": [conn3],
   }
   ```

3. **Redis Pub/Sub Integration**
   ```go
   // Subscribe to run-specific channel
   pubsub := redis.Subscribe("run:" + runID)

   // Forward to WebSocket clients
   for msg := range pubsub.Channel() {
       for _, conn := range connections[runID] {
           conn.WriteJSON(msg.Payload)
       }
   }
   ```

### Documentation

- **Architecture:** [../../cmd/fanout/docs/NCHAN_ARCHITECTURE.md](../../cmd/fanout/docs/NCHAN_ARCHITECTURE.md)
- **README:** [../../cmd/fanout/docs/README.md](../../cmd/fanout/docs/README.md)

### Performance

- **Concurrent connections:** ~10,000 (per instance)
- **Latency:** <10ms (Redis pub/sub + WebSocket)
- **Throughput:** ~50,000 events/sec

---

## 7. aob CLI Tool

**Path:** `cmd/aob-cli/`

**Role:** Developer-friendly command-line interface

**Technology:** Rust, tokio (async), reqwest, clap

### Responsibilities

- **Run management**: Start, status, list, cancel workflows
- **Log streaming**: Real-time SSE-based log streaming
- **HITL approvals**: Approve/reject from command line
- **Patch management**: List, show, approve/reject agent patches
- **Replay**: Replay runs from checkpoints
- **Workflow validation**: Validate workflow files before submission

### Key Features

1. **Fast & Lightweight**
   - Small binary size
   - Fast startup
   - Low memory footprint

2. **Real-Time Streaming**
   ```bash
   # Stream logs with SSE
   aob logs stream run_7f3e4a

   # Filter by node
   aob logs stream run_7f3e4a --node enrich

   # Show only errors
   aob logs stream run_7f3e4a --filter errors
   ```

3. **HITL Workflow**
   ```bash
   # Approve from CLI (no UI needed)
   aob approve ticket_456 approve --reason "LGTM"

   # Reject with reason
   aob approve ticket_789 reject --reason "Need validation"
   ```

4. **Developer Experience**
   ```bash
   # Start and follow logs
   aob run start workflow.json -f

   # JSON output for scripting
   RUN_ID=$(aob run start workflow.json --output json | jq -r '.run_id')
   aob run status "$RUN_ID"

   # Validate before running
   aob workflow validate workflow.json && aob run start workflow.json
   ```

5. **Patch Management**
   ```bash
   # List all patches for a run
   aob patch list run_7f3e4a

   # Show patch diff
   aob patch show patch_abc123

   # Approve patch
   aob patch approve patch_abc123 --reason "Safe"
   ```

### Commands

| Command | Description |
|---------|-------------|
| `aob run start` | Start workflow execution |
| `aob run status` | Get run status |
| `aob run list` | List recent runs |
| `aob run cancel` | Cancel running workflow |
| `aob logs stream` | Stream logs in real-time (SSE) |
| `aob approve` | Approve/reject HITL requests |
| `aob patch list/show/approve/reject` | Manage agent patches |
| `aob replay` | Replay from checkpoint |
| `aob workflow validate/list/show` | Workflow management |

### Documentation

- **README:** [../../cmd/aob-cli/README.md](../../cmd/aob-cli/README.md)
- **Commands:** [../../cmd/aob-cli/COMMANDS.md](../../cmd/aob-cli/COMMANDS.md)
- **Also available in:** [../cli/README.md](../cli/README.md), [../cli/COMMANDS.md](../cli/COMMANDS.md)

### Performance

- **Binary:** Optimized with LTO
- **Startup:** <10ms
- **Memory:** ~5MB
- **Streaming latency:** <50ms (SSE)

---

## Service Communication Patterns

### 1. Command Pattern (Request → Response)

```
User → Orchestrator: POST /runs
Orchestrator → Postgres: INSERT run
Orchestrator → Redis: Store IR
Orchestrator → User: {run_id}
```

### 2. Event Pattern (Fire & Forget)

```
Worker → Coordinator: RPUSH completion_signals
(async, no response expected)
```

### 3. Stream Pattern (Real-Time)

```
Worker → Redis Pub/Sub: PUBLISH run:123 {event}
Fanout → WebSocket: Forward to clients
```

---

## Shared Libraries

**Path:** `common/`

### 1. SDK (`common/sdk/`)
- Workflow operations
- IR compilation
- CAS integration

### 2. Metrics (`common/metrics/`)
- Prometheus metrics
- Request duration
- Error rates

### 3. Rate Limiting (`common/ratelimit/`)
- Workflow inspection
- Tiered rate limits
- Lua scripts (atomic Redis operations)

### 4. Worker (`common/worker/`)
- Completion signal helpers
- Stream consumption utilities

### 5. Validation (`common/validation/`)
- Patch validators
- Schema validation

---

## Service Dependencies

```
Orchestrator
  ├── Postgres (run metadata)
  ├── Redis (IR storage)
  └── (no dependencies on other services)

Workflow-Runner (Coordinator)
  ├── Redis (IR, context, completion signals)
  └── (no dependencies on other services)

Workers (HTTP, HITL, Agent)
  ├── Redis (task streams)
  └── (no dependencies on other services)

Fanout
  ├── Redis (pub/sub)
  └── (no dependencies on other services)

→ All services are independent (loosely coupled)
```

---

## Deployment

### Local Development

```bash
# Terminal 1: Orchestrator
make start-orchestrator

# Terminal 2: Coordinator
make start-workflow-runner

# Terminal 3: HTTP Worker
make start-http-worker

# Terminal 4: HITL Worker
make start-hitl-worker

# Terminal 5: Agent Runner
cd cmd/agent-runner-py && python main.py

# Terminal 6: Fanout
make start-fanout
```

### Production (Systemd)

See [../../scripts/systemd/README.md](../../scripts/systemd/README.md) for:
- CPU affinity (control plane vs. runner plane)
- Resource limits
- Service unit files
- Health checks

---

## Service Scaling Strategy

| Service | Scaling | Bottleneck |
|---------|---------|------------|
| Orchestrator | Horizontal (stateless) | Postgres writes |
| Coordinator | Horizontal (consumer groups) | Single instance throughput |
| HTTP Worker | Horizontal (consumer groups) | Network I/O |
| Agent Runner | Horizontal (consumer groups) | OpenAI API limits |
| HITL Worker | Horizontal (consumer groups) | Human response time |
| Fanout | Horizontal (sticky sessions) | WebSocket connections |

---

## Monitoring

All services expose:
- **Metrics**: Prometheus `/metrics` endpoint
- **Health**: `/health` endpoint
- **Logs**: Structured JSON logs with trace IDs

**Key Metrics:**
- `workflow_runs_total` - Total runs started
- `node_executions_total` - Total node executions
- `agent_decisions_total` - Total LLM calls
- `http_requests_total` - Total HTTP requests
- `hitl_approvals_total` - Total approvals
- `completion_signals_total` - Total signals processed
- `websocket_connections` - Active WebSocket connections

---

## Testing Services

### Unit Tests
```bash
# Go services
cd cmd/orchestrator && go test ./...

# Python service
cd cmd/agent-runner-py && pytest tests/
```

### Integration Tests
```bash
# End-to-end workflow
./test_run.sh

# Rate limiting
./test_rate_limit.sh
```

### Manual Testing
1. Start all services
2. Create workflow in UI
3. Submit run
4. Watch real-time execution in UI
5. Test HITL approval
6. Test agent patching

---

## Service Health Checks

All services implement:

```http
GET /health

Response:
{
  "status": "ok",
  "service": "orchestrator",
  "version": "1.0.0",
  "uptime": "2h34m",
  "dependencies": {
    "postgres": "ok",
    "redis": "ok"
  }
}
```

---

## Summary

| Service | Role | Language | Lines of Code |
|---------|------|----------|---------------|
| Orchestrator | API + metadata | Go | ~15,000 |
| Workflow-Runner | Choreography | Go | ~8,000 |
| Agent-Runner-Py | LLM integration | Python | ~5,000 |
| HTTP-Worker | HTTP execution | Go | ~3,000 |
| HITL-Worker | Approvals | Go | ~2,000 |
| Fanout | WebSocket | Go | ~4,000 |
| aob CLI | Developer tool | Rust | ~3,000 |
| **Total** | | | ~40,000 |

**Common libraries:** ~8,000 lines (shared across services)

---

**All services designed for horizontal scaling, observability, and migration to production infrastructure (Kafka, gRPC, customer execution environments).**
