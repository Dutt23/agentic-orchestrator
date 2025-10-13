# Architecture Integration Guide

**Date**: 2025-10-13
**Purpose**: Document how workflow-runner integrates with existing components

---

## System Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                     Lyzr Orchestration Platform                  │
├─────────────────────────────────────────────────────────────────┤
│                                                                   │
│  ┌─────────────────┐       ┌─────────────────┐                 │
│  │  Orchestrator   │       │ Agent-Runner    │                 │
│  │  (Go)           │       │ (Python)        │                 │
│  │                 │       │                 │                 │
│  │  - Workflow     │       │  - LLM calls    │                 │
│  │    CRUD         │       │  - execute_     │                 │
│  │  - Artifact     │       │    pipeline     │                 │
│  │    management   │       │  - patch_       │                 │
│  │  - Tag system   │       │    workflow     │                 │
│  └────────┬────────┘       └────────┬────────┘                 │
│           │                         │                           │
│           │   ┌─────────────────────┘                           │
│           │   │                                                 │
│           ↓   ↓                                                 │
│  ┌─────────────────┐       ┌─────────────────┐                 │
│  │ Workflow-Runner │       │     Redis       │                 │
│  │ (Go)            │←─────→│                 │                 │
│  │                 │       │  - Streams      │                 │
│  │  - Token        │       │  - Counters     │                 │
│  │    choreography │       │  - Queues       │                 │
│  │  - Execution    │       │  - Context      │                 │
│  │  - Completion   │       └─────────────────┘                 │
│  └────────┬────────┘              ↑                             │
│           │                       │                             │
│           ↓                       │                             │
│  ┌─────────────────────────────────┘                           │
│  │                                                               │
│  │  ┌────────────────┐     ┌────────────────┐                 │
│  │  │   Postgres     │     │      CAS       │                 │
│  │  │                │     │   (S3/MinIO)   │                 │
│  │  │  - Runs        │     │                │                 │
│  │  │  - Events      │     │  - Payloads    │                 │
│  │  │  - Artifacts   │     │  - Configs     │                 │
│  │  └────────────────┘     └────────────────┘                 │
│  │                                                               │
└───────────────────────────────────────────────────────────────────┘
```

---

## Existing Components to Reuse

### 1. Workflow Schema (`common/schema/workflow.schema.json`)

**Current schema supports:**
- Nodes with types: `function`, `http`, `conditional`, `loop`, `parallel`, `transform`, `aggregate`, `filter`
- Edges with optional conditions
- Retry policies
- Timeout configuration

**Our adaptation:**
- Use this schema as the **input DSL**
- Extend with our types: `task`, `agent`, `human`
- Map existing types to our types:
  - `function` → `task`
  - `conditional` → `task` with branch config
  - `loop` → `task` with loop config
  - `parallel` → multiple nodes with same dependencies

### 2. Agent-Runner-Py (`cmd/agent-runner-py/`)

**Already implemented:**
- ✅ Redis queue consumer (`BLPOP` from `agent:jobs`)
- ✅ Result publisher (`RPUSH` to `agent:results:{job_id}`)
- ✅ LLM client with tool calling
- ✅ Two tools: `execute_pipeline`, `patch_workflow`
- ✅ Worker pool (4 workers by default)
- ✅ Health check HTTP server
- ✅ Intent classifier

**Perfect match for our design!** This is the agent worker we need.

---

## Integration Points

### 1. Workflow Submission Flow

```
User → Orchestrator API: POST /workflows
  ├─> Validate against workflow.schema.json
  ├─> Store in artifact table
  └─> Tag as "main"

User → Orchestrator API: POST /runs {tag: "main"}
  ├─> Resolve tag → artifact_id
  ├─> Load workflow DSL from CAS
  ├─> Call workflow-runner: POST /compile
  │   └─> Compile DSL → IR (with terminal flags)
  ├─> Store IR in CAS
  ├─> Create run in Postgres
  ├─> Initialize counter in Redis
  └─> Publish seed tokens to Redis Streams
```

### 2. Token Routing by Node Type

**Workflow-Runner publishes to different streams:**

```go
func (sdk *SDK) publishToken(ctx context.Context, token *Token) error {
    var streamName string

    switch token.NodeType {
    case "task":
        streamName = "wf.tasks"      // Task worker consumes
    case "agent":
        streamName = "wf.agents"     // NOT USED - use job queue instead
    case "human":
        streamName = "wf.humans"     // HITL worker consumes (future)
    default:
        streamName = "wf.tasks"
    }

    // Special case for agent nodes:
    // Instead of publishing to stream, publish to agent:jobs queue
    if token.NodeType == "agent" {
        return sdk.publishAgentJob(ctx, token)
    }

    // Normal token publish to stream
    return sdk.redis.XAdd(...)
}
```

### 3. Agent Node Integration

**When workflow-runner reaches an agent node:**

```go
func (sdk *SDK) publishAgentJob(ctx context.Context, token *Token) error {
    // Load IR to get node details
    node := sdk.loadNode(token.RunID, token.ToNode)

    // Load current workflow (for patch operations)
    ir := sdk.loadIR(token.RunID)

    // Load execution context
    context := sdk.LoadContext(ctx, token.RunID)

    // Create job message (matches agent-runner-py format)
    job := map[string]interface{}{
        "version":     "1.0",
        "job_id":      uuid.New().String(),
        "run_id":      token.RunID,
        "node_id":     token.ToNode,
        "prompt":      sdk.loadPrompt(token),  // From node config or payload
        "context": map[string]interface{}{
            "previous_results": context,
            "session_id":       token.RunID,
        },
        "current_workflow": ir,  // For patch operations
        "timeout_sec":      300,
        "retry_count":      0,
    }

    // Publish to agent:jobs queue (RPUSH)
    jobJSON, _ := json.Marshal(job)
    return sdk.redis.RPush(ctx, "agent:jobs", jobJSON).Err()
}
```

**Agent-runner-py processes job:**
1. `BLPOP agent:jobs` → gets job
2. Calls LLM with tools (execute_pipeline or patch_workflow)
3. Executes tool
4. Stores result in memory/DB
5. `RPUSH agent:results:{job_id}` → publishes result

**Workflow-runner picks up result:**

```go
func (w *AgentWorker) waitForResult(ctx context.Context, jobID string) (*AgentResult, error) {
    resultQueue := fmt.Sprintf("agent:results:%s", jobID)

    // Blocking pop with timeout
    result, err := w.redis.BLPop(ctx, 5*time.Minute, resultQueue).Result()
    if err != nil {
        return nil, fmt.Errorf("timeout waiting for agent result")
    }

    var agentResult AgentResult
    json.Unmarshal([]byte(result[1]), &agentResult)

    if agentResult.Status == "failed" {
        return nil, fmt.Errorf("agent failed: %v", agentResult.Error)
    }

    return &agentResult, nil
}
```

---

## Workflow Schema Mapping

### Input DSL (workflow.schema.json) → IR

```json
// Input: workflow.schema.json format
{
  "nodes": [
    {"id": "fetch", "type": "function", "config": {...}},
    {"id": "check", "type": "conditional", "config": {"condition": "..."}},
    {"id": "process", "type": "loop", "config": {"max_iterations": 5}}
  ],
  "edges": [
    {"from": "fetch", "to": "check"},
    {"from": "check", "to": "process", "condition": "score > 80"}
  ]
}

// Output: IR format
{
  "version": "1.0",
  "nodes": {
    "fetch": {
      "id": "fetch",
      "type": "task",                    // Mapped from "function"
      "config_ref": "cas://sha256:...",
      "dependencies": [],
      "dependents": ["check"],
      "is_terminal": false
    },
    "check": {
      "id": "check",
      "type": "task",                    // Mapped from "conditional"
      "config_ref": "cas://sha256:...",
      "dependencies": ["fetch"],
      "dependents": [],                  // Will be determined by branch
      "is_terminal": false,
      "branch": {                        // Created from "conditional" type
        "enabled": true,
        "type": "conditional",
        "rules": [
          {
            "condition": {
              "type": "cel",
              "expression": "output.score > 80"
            },
            "next_nodes": ["process"]
          }
        ],
        "default": []
      }
    },
    "process": {
      "id": "process",
      "type": "task",                    // Mapped from "loop"
      "config_ref": "cas://sha256:...",
      "dependencies": ["check"],
      "dependents": [],
      "is_terminal": true,
      "loop": {                          // Created from "loop" type
        "enabled": true,
        "max_iterations": 5,
        "loop_back_to": "process",
        "break_path": [],
        "timeout_path": []
      }
    }
  }
}
```

### Type Mapping Rules

| Input Type | IR Type | Additional Config |
|------------|---------|-------------------|
| `function` | `task` | None |
| `http` | `task` | None |
| `transform` | `task` | None |
| `aggregate` | `task` | None |
| `filter` | `task` | None |
| `conditional` | `task` | + `branch` config |
| `loop` | `task` | + `loop` config |
| `parallel` | Multiple `task` nodes | Same dependencies |

---

## Redis Key Conventions

**Shared between all services:**

```redis
# Token counter (workflow-runner)
counter:run_{uuid}

# Applied keys (workflow-runner)
applied:run_{uuid}

# Context (workflow-runner)
context:run_{uuid}

# Pending tokens for join (workflow-runner)
pending_tokens:run_{uuid}:node_{id}

# Loop state (workflow-runner)
loop:run_{uuid}:node_{id}

# Agent jobs queue (agent-runner-py reads)
agent:jobs

# Agent results (agent-runner-py writes, workflow-runner reads)
agent:results:{job_id}

# Completion events channel (workflow-runner publishes)
completion_events

# Streams (workflow-runner publishes/consumes)
wf.tasks
wf.agents  # Not used - agent:jobs queue instead
wf.humans  # Future HITL
```

---

## Configuration

### Agent-Runner-Py Config (`cmd/agent-runner-py/config.yaml`)

```yaml
redis:
  host: localhost
  port: 6379
  db: 0
  job_queue: "agent:jobs"               # Reads from here
  result_queue_prefix: "agent:results"  # Writes to here
  timeout: 5

orchestrator:
  api_url: "http://localhost:8080"      # For patch_workflow forwarding

llm:
  provider: "openai"
  model: "gpt-4o"
  api_key: "${OPENAI_API_KEY}"

service:
  port: 8082
  workers: 4
```

### Workflow-Runner Config (to be created)

```yaml
service:
  name: "workflow-runner"
  port: 8081
  workers: 10

redis:
  host: localhost
  port: 6379
  db: 0

database:
  host: localhost
  port: 5432
  database: orchestrator
  user: postgres
  password: postgres

cas:
  backend: "s3"
  bucket: "workflow-cas"
  endpoint: "http://localhost:9000"  # MinIO

orchestrator:
  api_url: "http://localhost:8080"
```

---

## Deployment Architecture

```
┌──────────────────┐
│   Orchestrator   │ :8080
│   (Go)           │
└────────┬─────────┘
         │
         ↓
┌──────────────────┐
│ Workflow-Runner  │ :8081
│   (Go)           │
│                  │
│  - Task workers  │ ←───── Redis Streams: wf.tasks
│  - Agent workers │ ←───── Redis Queue: agent:jobs (publishes)
│  - Supervisor    │ ←───── Redis Channel: completion_events
└────────┬─────────┘
         │
         ↓
┌──────────────────┐
│ Agent-Runner-Py  │ :8082
│   (Python)       │
│                  │
│  - LLM client    │ ←───── Redis Queue: agent:jobs (consumes)
│  - Tool executor │ ────→  Redis Queue: agent:results:* (publishes)
└──────────────────┘
```

---

## Next Steps for Integration

1. **Update IR Compiler** (`cmd/workflow-runner/compiler/ir.go`):
   - Import workflow.schema.json
   - Add type mapping logic
   - Convert conditional → branch config
   - Convert loop → loop config

2. **Add Agent Worker** (`cmd/workflow-runner/worker/agent_worker.go`):
   - Publish to `agent:jobs` queue (reuse agent-runner-py)
   - Wait for result from `agent:results:{job_id}`
   - Continue execution with result

3. **Test End-to-End**:
   - Submit workflow via orchestrator API
   - Verify token choreography
   - Verify agent integration
   - Verify completion detection

---

**Ready to implement the integration!**
