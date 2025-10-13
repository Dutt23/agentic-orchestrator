# Agentic Service Implementation Guide

**Version**: 1.0
**Author**: Orchestrator Team
**Date**: 2025-10-12
**Status**: Implementation Ready

---

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [Message Protocol](#2-message-protocol)
3. [Database Schema](#3-database-schema)
4. [Service Design](#4-service-design)
5. [Tool Specifications](#5-tool-specifications)
6. [Flow Diagrams](#6-flow-diagrams)
7. [Token Caching Strategy](#7-token-caching-strategy)
8. [Implementation Phases](#8-implementation-phases)
9. [Code Structure](#9-code-structure)
10. [Testing Strategy](#10-testing-strategy)

---

## 1. Architecture Overview

### 1.1 System Context

The Agentic Service is a **Redis worker** that processes LLM-powered tasks during workflow execution. It is NOT a REST API - jobs are dispatched via Redis queues.

```
┌─────────────────────────────────────────────────────────────┐
│                     Workflow Execution                       │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
                    ┌─────────────────┐
                    │ Regular Nodes   │
                    │ (Go Runner)     │
                    └────────┬────────┘
                             │
                    Encounters "agent" node
                             │
                             ▼
                    ┌─────────────────┐
                    │  Publish Job    │
                    │  Redis Queue    │───→ agent:jobs
                    └─────────────────┘
                             │
                             ▼
           ┌──────────────────────────────────────┐
           │   Agent Service (Python Worker)      │
           │   - Pick job from Redis              │
           │   - Call LLM (OpenAI/Anthropic)      │
           │   - Execute tools                    │
           │   - Store result in DB               │
           │   - Publish result to Redis          │
           └──────────────┬───────────────────────┘
                          │
                          ▼
                 ┌─────────────────┐
                 │  agent:results  │
                 │  Redis Queue    │
                 └────────┬────────┘
                          │
                          ▼
                 ┌─────────────────┐
                 │  Runner picks   │
                 │  up result &    │
                 │  continues      │
                 └─────────────────┘
```

### 1.2 Design Principles

1. **Two-Lane Execution**:
   - **Fast Lane**: Ephemeral data pipelines (`execute_pipeline`)
   - **Patch Lane**: Persistent workflow modifications (`patch_workflow`)

2. **Minimal Tool Set**: 5 composable tools (not infinite function allowlist)

3. **Result Persistence**: Store in PostgreSQL (MVP) → migrate to S3/EBS later

4. **Backward Compatibility**: Versioned message protocol

5. **Token Optimization**: OpenAI prompt caching for cost/latency savings

### 1.3 Hybrid Architecture

**Primary Mode**: Redis Worker
**Secondary Mode**: HTTP endpoints for:
- Health checks (`GET /health`)
- Metrics (`GET /metrics`)
- Manual testing (`POST /test/chat`)

### 1.4 Storage Strategy

**Phase 1 (MVP)**: PostgreSQL
```sql
-- Store results in artifact table or dedicated agent_results table
-- Results <10MB: Store JSON directly
-- Results >10MB: Store in CAS, reference by cas_id
```

**Phase 2 (Later)**: S3/EBS
```
-- Large results (>10MB) go to S3
-- DB stores only metadata + S3 reference
-- Backward compatible: check cas_id first, fallback to S3 ref
```

---

## 2. Message Protocol

### 2.1 Input Message (Redis Queue: `agent:jobs`)

```json
{
  "version": "1.0",
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "run_id": "run-2024-001",
  "node_id": "agent_analyze_data",
  "workflow_tag": "main",
  "workflow_owner": "sdutt",
  "user_id": "sdutt",
  "prompt": "fetch flight prices from NYC to LAX, sort by price, show cheapest 3",
  "context": {
    "previous_results": [
      {
        "node_id": "fetch_data",
        "result_ref": "cas://sha256:abc123...",
        "preview": {"count": 150}
      }
    ],
    "workflow_state": {
      "current_step": 5,
      "total_steps": 10
    },
    "session_id": "sess-xyz789"
  },
  "timeout_sec": 300,
  "retry_count": 0,
  "created_at": "2024-10-12T10:30:00Z"
}
```

**Fields**:
- `version`: Protocol version for backward compatibility
- `job_id`: Unique job identifier (UUID)
- `run_id`: Workflow run identifier
- `node_id`: Which workflow node triggered this job
- `prompt`: Natural language instruction
- `context`: Previous results, state, session
- `timeout_sec`: Max execution time
- `retry_count`: Retry attempt number

### 2.2 Output Message (Redis Queue: `agent:results`)

**Success**:
```json
{
  "version": "1.0",
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "completed",
  "result_ref": "artifact://uuid-result-id",
  "result_preview": {
    "type": "dataset",
    "row_count": 3,
    "sample": [
      {"airline": "Delta", "price": 299}
    ]
  },
  "metadata": {
    "tool_calls": [
      {
        "tool": "execute_pipeline",
        "args": {
          "pipeline": [
            {"step": "http_request", "url": "api.flights.com"},
            {"step": "table_sort", "field": "price"},
            {"step": "top_k", "k": 3}
          ]
        }
      }
    ],
    "tokens_used": 1523,
    "cache_hit": true,
    "execution_time_ms": 1247,
    "llm_model": "gpt-4o"
  },
  "completed_at": "2024-10-12T10:30:15Z"
}
```

**Failure**:
```json
{
  "version": "1.0",
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "failed",
  "error": {
    "type": "execution_error",
    "message": "Failed to fetch from api.flights.com: connection timeout",
    "retryable": true,
    "tool": "execute_pipeline",
    "step": "http_request"
  },
  "completed_at": "2024-10-12T10:30:15Z"
}
```

### 2.3 Queue Names

```
Input:  agent:jobs
Output: agent:results:{job_id}  (per-job result channel)
```

### 2.4 Version Compatibility

Future versions maintain backward compatibility:
- v1.x: Add optional fields only
- v2.x: Can change required fields (agent checks version first)

---

## 3. Database Schema

### 3.1 Agent Results Table

```sql
CREATE TABLE IF NOT EXISTS agent_results (
    result_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL UNIQUE,
    run_id TEXT NOT NULL,
    node_id TEXT NOT NULL,

    -- Result storage
    result_data JSONB,              -- For small results (<10MB)
    cas_id TEXT,                    -- Reference to CAS blob
    s3_ref TEXT,                    -- Future: S3 reference

    -- Metadata
    status TEXT NOT NULL,           -- 'completed', 'failed'
    error JSONB,                    -- Error details if failed
    tool_calls JSONB,               -- Array of tool invocations
    tokens_used INTEGER,
    cache_hit BOOLEAN DEFAULT false,
    execution_time_ms INTEGER,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,

    -- Indexes
    INDEX idx_agent_results_job_id (job_id),
    INDEX idx_agent_results_run_id (run_id),
    INDEX idx_agent_results_created_at (created_at DESC)
);
```

### 3.2 Storage Strategy

```python
def store_result(job_id, result_data):
    # Estimate size
    size_bytes = len(json.dumps(result_data).encode('utf-8'))

    if size_bytes < 10 * 1024 * 1024:  # <10MB
        # Store directly in JSONB
        db.execute("""
            INSERT INTO agent_results (job_id, result_data, ...)
            VALUES (%s, %s, ...)
        """, (job_id, result_data))
        return f"artifact://{result_id}"

    else:  # >=10MB
        # Store in CAS
        cas_id = cas.store(result_data)
        db.execute("""
            INSERT INTO agent_results (job_id, cas_id, ...)
            VALUES (%s, %s, ...)
        """, (job_id, cas_id))
        return f"cas://{cas_id}"
```

### 3.3 Retrieval

```python
def get_result(result_ref):
    if result_ref.startswith("artifact://"):
        result_id = result_ref.split("://")[1]
        row = db.query("SELECT result_data, cas_id, s3_ref FROM agent_results WHERE result_id = %s", (result_id,))

        if row.result_data:
            return row.result_data
        elif row.cas_id:
            return cas.fetch(row.cas_id)
        elif row.s3_ref:
            return s3.fetch(row.s3_ref)

    elif result_ref.startswith("cas://"):
        return cas.fetch(result_ref.split("://")[1])
```

---

## 4. Service Design

### 4.1 Service Architecture

```python
# main.py - Hybrid architecture

class AgentService:
    def __init__(self):
        self.redis = redis.Redis(...)
        self.db = psycopg2.connect(...)
        self.llm = OpenAI(api_key=...)
        self.worker_pool = ThreadPoolExecutor(max_workers=4)
        self.running = True

    def start(self):
        # Start HTTP server in background thread
        http_thread = Thread(target=self.run_http_server)
        http_thread.daemon = True
        http_thread.start()

        # Start Redis worker pool
        futures = []
        for i in range(4):
            future = self.worker_pool.submit(self.worker_loop, worker_id=i)
            futures.append(future)

        # Wait for all workers
        for future in futures:
            future.result()

    def worker_loop(self, worker_id):
        while self.running:
            try:
                # Blocking pop from Redis (5 second timeout)
                job_data = self.redis.blpop('agent:jobs', timeout=5)

                if job_data:
                    queue, payload = job_data
                    job = json.loads(payload)
                    self.process_job(job)

            except Exception as e:
                logger.error(f"Worker {worker_id} error: {e}")
                time.sleep(1)

    def process_job(self, job):
        job_id = job['job_id']

        try:
            # Call LLM with tools
            result = self.execute_with_llm(job)

            # Store result in DB
            result_ref = self.store_result(job_id, result)

            # Publish success to Redis
            self.redis.rpush(
                f"agent:results:{job_id}",
                json.dumps({
                    "version": "1.0",
                    "job_id": job_id,
                    "status": "completed",
                    "result_ref": result_ref,
                    "metadata": {...}
                })
            )

        except Exception as e:
            # Publish failure to Redis
            self.redis.rpush(
                f"agent:results:{job_id}",
                json.dumps({
                    "version": "1.0",
                    "job_id": job_id,
                    "status": "failed",
                    "error": {
                        "type": type(e).__name__,
                        "message": str(e),
                        "retryable": is_retryable(e)
                    }
                })
            )

    def run_http_server(self):
        app = FastAPI()

        @app.get("/health")
        def health():
            return {"status": "ok", "workers": 4}

        @app.post("/test/chat")
        def test_chat(prompt: str):
            # Manual testing endpoint
            job = create_test_job(prompt)
            result = self.execute_with_llm(job)
            return result

        uvicorn.run(app, host="0.0.0.0", port=8082)
```

### 4.2 Graceful Shutdown

```python
def shutdown(self):
    logger.info("Shutting down agent service...")
    self.running = False

    # Wait for current jobs to finish (max 30 seconds)
    self.worker_pool.shutdown(wait=True, timeout=30)

    # Close connections
    self.redis.close()
    self.db.close()

    logger.info("Shutdown complete")

# Signal handlers
signal.signal(signal.SIGTERM, lambda s, f: service.shutdown())
signal.signal(signal.SIGINT, lambda s, f: service.shutdown())
```

---

## 5. Tool Specifications

### 5.1 Tool 1: execute_pipeline (Fast Lane)

**Purpose**: Execute ephemeral data pipelines without modifying workflow.

**Schema**:
```json
{
  "type": "function",
  "function": {
    "name": "execute_pipeline",
    "strict": true,
    "description": "Execute ephemeral data pipeline using composable primitives",
    "parameters": {
      "type": "object",
      "required": ["session_id", "pipeline"],
      "additionalProperties": false,
      "properties": {
        "session_id": {
          "type": "string",
          "description": "Session ID for context tracking"
        },
        "pipeline": {
          "type": "array",
          "description": "Array of pipeline steps to execute",
          "items": {
            "type": "object",
            "required": ["step"],
            "properties": {
              "step": {
                "type": "string",
                "enum": [
                  "http_request",
                  "table_sort",
                  "table_filter",
                  "table_select",
                  "top_k",
                  "groupby",
                  "join",
                  "jq_transform",
                  "regex_extract",
                  "parse_date"
                ]
              },
              "url": {"type": "string"},
              "method": {"type": "string", "enum": ["GET", "POST"]},
              "params": {"type": "object"},
              "field": {"type": "string"},
              "order": {"type": "string", "enum": ["asc", "desc"]},
              "condition": {"type": "object"},
              "fields": {"type": "array", "items": {"type": "string"}},
              "k": {"type": "integer", "minimum": 1},
              "by": {"type": "array", "items": {"type": "string"}},
              "agg": {"type": "string", "enum": ["count", "sum", "avg", "min", "max"]},
              "left": {"type": "string"},
              "right": {"type": "string"},
              "on": {"type": "string"},
              "query": {"type": "string"},
              "pattern": {"type": "string"},
              "format": {"type": "string"}
            }
          }
        },
        "input_ref": {
          "type": "string",
          "description": "Optional CAS reference to input data"
        }
      }
    }
  }
}
```

**Primitives**:

1. **http_request**: Fetch data from APIs
   ```python
   {"step": "http_request", "url": "https://api.example.com/data", "method": "GET", "params": {"limit": 100}}
   ```

2. **table_sort**: Sort records
   ```python
   {"step": "table_sort", "field": "price", "order": "asc"}
   ```

3. **table_filter**: Filter records
   ```python
   {"step": "table_filter", "condition": {"field": "price", "op": "<", "value": 500}}
   ```

4. **table_select**: Select specific fields
   ```python
   {"step": "table_select", "fields": ["id", "name", "price"]}
   ```

5. **top_k**: Take first K records
   ```python
   {"step": "top_k", "k": 10}
   ```

6. **groupby**: Aggregate data
   ```python
   {"step": "groupby", "by": ["category"], "agg": "sum", "field": "revenue"}
   ```

7. **join**: Join two datasets
   ```python
   {"step": "join", "left": "cas://sha256:...", "right": "cas://sha256:...", "on": "id"}
   ```

8. **jq_transform**: JSON transformation
   ```python
   {"step": "jq_transform", "query": ".items[] | select(.price < 100)"}
   ```

9. **regex_extract**: Extract patterns
   ```python
   {"step": "regex_extract", "pattern": r"(\d{3})-(\d{3})-(\d{4})", "field": "phone"}
   ```

10. **parse_date**: Parse date strings
    ```python
    {"step": "parse_date", "field": "created_at", "format": "%Y-%m-%d"}
    ```

### 5.2 Tool 2: patch_workflow (Patch Lane)

**Purpose**: Create persistent workflow modifications.

**Schema**:
```json
{
  "type": "function",
  "function": {
    "name": "patch_workflow",
    "strict": true,
    "description": "Create persistent workflow changes (always/whenever/schedule)",
    "parameters": {
      "type": "object",
      "required": ["workflow_tag", "patch_spec"],
      "additionalProperties": false,
      "properties": {
        "workflow_tag": {
          "type": "string",
          "description": "Tag of workflow to patch"
        },
        "workflow_owner": {
          "type": "string",
          "description": "Owner of the workflow"
        },
        "patch_spec": {
          "type": "object",
          "description": "JSON Patch operations",
          "properties": {
            "operations": {
              "type": "array",
              "items": {
                "type": "object",
                "required": ["op", "path"],
                "properties": {
                  "op": {"type": "string", "enum": ["add", "remove", "replace"]},
                  "path": {"type": "string"},
                  "value": {}
                }
              }
            },
            "description": {"type": "string"}
          }
        }
      }
    }
  }
}
```

**Example**:
```python
{
  "workflow_tag": "main",
  "workflow_owner": "sdutt",
  "patch_spec": {
    "operations": [
      {
        "op": "add",
        "path": "/nodes/-",
        "value": {
          "id": "send_email",
          "type": "task",
          "name": "Send Notification",
          "config": {"action": "email", "to": "ops@example.com"}
        }
      },
      {
        "op": "add",
        "path": "/edges/-",
        "value": {"from": "process_data", "to": "send_email"}
      }
    ],
    "description": "Add email notification after data processing"
  }
}
```

### 5.3 Tool 3: search_tools

**Purpose**: Discover existing composite tools/workflows.

**Schema**:
```json
{
  "type": "function",
  "function": {
    "name": "search_tools",
    "strict": true,
    "description": "Search for existing composite tools and workflows",
    "parameters": {
      "type": "object",
      "required": ["query"],
      "additionalProperties": false,
      "properties": {
        "query": {
          "type": "string",
          "description": "Search query (keywords or description)"
        },
        "limit": {
          "type": "integer",
          "description": "Max results to return",
          "default": 10
        }
      }
    }
  }
}
```

### 5.4 Tool 4: openapi_action (Meta-Tool)

**Purpose**: Call any OpenAPI-compliant API without prebuild.

**Schema**:
```json
{
  "type": "function",
  "function": {
    "name": "openapi_action",
    "strict": true,
    "description": "Call any OpenAPI-compliant API dynamically",
    "parameters": {
      "type": "object",
      "required": ["openapi_url", "operation_id"],
      "additionalProperties": false,
      "properties": {
        "openapi_url": {
          "type": "string",
          "description": "URL to OpenAPI spec (JSON/YAML)"
        },
        "operation_id": {
          "type": "string",
          "description": "Operation ID from spec"
        },
        "params": {
          "type": "object",
          "description": "Parameters for the operation"
        },
        "auth_profile": {
          "type": "string",
          "description": "Named auth configuration to use"
        }
      }
    }
  }
}
```

### 5.5 Tool 5: delegate_to_agent (K8s Scaffold)

**Purpose**: Delegate complex tasks to external agents (future K8s jobs).

**Schema**:
```json
{
  "type": "function",
  "function": {
    "name": "delegate_to_agent",
    "strict": true,
    "description": "Delegate task to external agent (K8s job)",
    "parameters": {
      "type": "object",
      "required": ["agent_type", "inputs"],
      "additionalProperties": false,
      "properties": {
        "agent_type": {
          "type": "string",
          "enum": ["code_interpreter", "custom_container", "langchain_agent"],
          "description": "Type of agent to use"
        },
        "container_image": {
          "type": "string",
          "description": "Docker image for custom_container type"
        },
        "inputs": {
          "type": "object",
          "description": "Input data for the agent"
        },
        "config": {
          "type": "object",
          "description": "Agent-specific configuration"
        }
      }
    }
  }
}
```

---

## 6. Flow Diagrams

### 6.1 Fast Lane Flow

```
User: "fetch flights NYC→LAX, sort by price, show top 3"
         │
         ▼
    ┌─────────┐
    │ LLM     │
    │ Analyzes│
    └────┬────┘
         │
         ▼
    execute_pipeline([
        {step: "http_request", url: "api.flights.com"},
        {step: "table_sort", field: "price", order: "asc"},
        {step: "top_k", k: 3}
    ])
         │
         ▼
    ┌────────────────┐
    │ Execute Each   │
    │ Step           │
    │ 1. HTTP GET    │
    │ 2. Sort in mem │
    │ 3. Take top 3  │
    └────────┬───────┘
             │
             ▼
    ┌────────────────┐
    │ Store to CAS   │
    │ Return ref     │
    └────────┬───────┘
             │
             ▼
    ┌────────────────┐
    │ Return to user │
    │ [3 flights]    │
    └────────────────┘

No workflow modification!
```

### 6.2 Patch Lane Flow

```
User: "add email notification when price < $500"
         │
         ▼
    ┌─────────┐
    │ LLM     │
    │ Analyzes│
    └────┬────┘
         │
         ▼
    patch_workflow({
        operations: [
            {op: "add", path: "/nodes/-", value: {email_node}},
            {op: "add", path: "/edges/-", value: {condition_edge}}
        ]
    })
         │
         ▼
    ┌──────────────────┐
    │ Forward to       │
    │ Orchestrator API │
    │ POST /workflows/ │
    │      main:patch  │
    └────────┬─────────┘
             │
             ▼
    ┌──────────────────┐
    │ Orchestrator     │
    │ applies patch    │
    │ creates new      │
    │ artifact version │
    └────────┬─────────┘
             │
             ▼
    ┌──────────────────┐
    │ Return patch_id  │
    │ to user          │
    └──────────────────┘

Workflow permanently modified!
```

### 6.3 Complete Job Flow

```
Workflow Execution
       │
       ▼
┌──────────────┐
│ Agent Node   │
│ Encountered  │
└──────┬───────┘
       │
       ▼
┌──────────────────────────────┐
│ Runner publishes to Redis    │
│ RPUSH agent:jobs {job_data}  │
└──────────┬───────────────────┘
           │
           ▼
┌───────────────────────────────┐
│ Worker picks up job           │
│ BLPOP agent:jobs              │
└──────────┬────────────────────┘
           │
           ▼
┌───────────────────────────────┐
│ Call OpenAI with tools        │
│ - System prompt (cached)      │
│ - User prompt                 │
│ - 5 tools                     │
└──────────┬────────────────────┘
           │
           ▼
┌───────────────────────────────┐
│ LLM returns tool calls        │
│ Example:                      │
│ - execute_pipeline(...)       │
└──────────┬────────────────────┘
           │
           ▼
┌───────────────────────────────┐
│ Execute tool                  │
│ - Run pipeline steps          │
│ - Store result to DB/CAS      │
└──────────┬────────────────────┘
           │
           ▼
┌───────────────────────────────┐
│ Publish result to Redis       │
│ RPUSH agent:results:{job_id}  │
│ {status, result_ref}          │
└──────────┬────────────────────┘
           │
           ▼
┌───────────────────────────────┐
│ Runner picks up result        │
│ BLPOP agent:results:{job_id}  │
└──────────┬────────────────────┘
           │
           ▼
┌───────────────────────────────┐
│ Continue workflow execution   │
│ with agent result             │
└───────────────────────────────┘
```

---

## 7. Token Caching Strategy

### 7.1 OpenAI Prompt Caching

OpenAI caches the **prefix** of your messages when it's:
- ≥1024 tokens
- Identical across requests

**Message Structure**:
```python
SYSTEM_PREFIX = """
You are an orchestration agent...

[TOOL SCHEMAS - 2000 tokens]

[FEW-SHOT EXAMPLES - 500 tokens]

[POLICY RULES - 300 tokens]
"""  # Total: ~3000 tokens → CACHED

messages = [
    {"role": "system", "content": SYSTEM_PREFIX},  # Cached!
    {"role": "user", "content": job['prompt']}     # Dynamic
]
```

**Benefits**:
- ~80% latency reduction for cached prefix
- ~50% cost reduction on input tokens
- Automatic (no code changes needed)

### 7.2 Session Context

```python
session_store = {
    "sess-123": {
        "conversation_history": [
            {"role": "user", "content": "fetch flights"},
            {"role": "assistant", "content": "..."}
        ],
        "last_result_ref": "cas://sha256:...",
        "pipeline_usage": {
            "pipeline_abc123": 3  # Used 3 times
        }
    }
}

# Include session context in prompt
def build_prompt(job):
    session = session_store.get(job['context']['session_id'], {})

    context_prompt = ""
    if session.get('last_result_ref'):
        context_prompt = f"\n\nPrevious result available at: {session['last_result_ref']}"

    return job['prompt'] + context_prompt
```

### 7.3 Result Caching

```python
# Cache pipeline results by hash
pipeline_hash = sha256(json.dumps(pipeline, sort_keys=True))

result = cache.get(f"pipeline:{pipeline_hash}")
if not result:
    result = execute_pipeline(pipeline)
    cache.set(f"pipeline:{pipeline_hash}", result, ttl=3600)
```

---

## 8. Implementation Phases

### Phase 1: Core MVP (3-4 hours)

**Goal**: Basic agent working end-to-end

**Tasks**:
1. Setup Python project structure
2. Redis worker loop
3. OpenAI integration with 2 tools:
   - `execute_pipeline` (5 primitives)
   - `patch_workflow`
4. Database schema + result storage
5. Basic HTTP server (health check)

**Test**: "fetch flights, sort, show top 3" works

### Phase 2: Tool Discovery (2 hours)

**Goal**: Agent can discover existing tools

**Tasks**:
1. Add `search_tools` function
2. Composite registry (in-memory or DB table)
3. Search by name/description
4. LLM prompt update

**Test**: Agent checks for existing tools before creating new pipeline

### Phase 3: Meta-Tool for APIs (3 hours)

**Goal**: Call any API without prebuild

**Tasks**:
1. Add `openapi_action` tool
2. OpenAPI spec parser
3. Domain allowlist config
4. Auth profile management

**Test**: "fetch weather from weatherapi.com" works without prebuild

### Phase 4: Auto-Promotion (2 hours)

**Goal**: Repeated pipelines become composite tools

**Tasks**:
1. Usage tracker (count pipeline executions)
2. Promotion policy (≥2 uses → composite)
3. Dynamic tool registration
4. Update LLM tool schema

**Test**: Pipeline used 3 times automatically becomes reusable tool

### Phase 5: Agent Delegation (2 hours - Scaffold)

**Goal**: Infrastructure for K8s job delegation

**Tasks**:
1. Add `delegate_to_agent` tool
2. K8s Job YAML generator
3. Status polling endpoint
4. Results retrieval scaffold

**Test**: Can generate job spec (don't execute yet)

### Phase 6: Production Ready (3 hours)

**Goal**: Deployable service

**Tasks**:
1. Error handling & retries
2. Logging & metrics
3. Docker containerization
4. Integration tests
5. Documentation

**Test**: Full integration test with orchestrator

---

## 9. Code Structure

```
cmd/agent-runner-py/
├── main.py                     # Entry point, worker loop, HTTP server
├── requirements.txt            # Dependencies
├── Dockerfile                  # Container image
├── config.yaml                 # Configuration
│
├── agent/
│   ├── __init__.py
│   ├── llm_client.py          # OpenAI SDK wrapper, prompt caching
│   ├── tools.py               # Tool schemas (5 tools)
│   ├── system_prompt.py       # System prompt (cached prefix)
│   └── session.py             # Session management
│
├── pipeline/
│   ├── __init__.py
│   ├── executor.py            # Pipeline execution engine
│   ├── primitives/
│   │   ├── __init__.py
│   │   ├── http_request.py   # GET/POST requests
│   │   ├── table_ops.py      # sort, filter, select, top_k, groupby, join
│   │   └── transforms.py     # jq, regex, parse_date
│   └── usage_tracker.py       # Track pipeline usage for auto-promotion
│
├── workflow/
│   ├── __init__.py
│   ├── patch_client.py        # Forward patches to orchestrator API
│   └── validator.py           # Validate patch specs
│
├── registry/
│   ├── __init__.py
│   ├── composite_store.py     # Store/retrieve composite tools
│   └── search.py              # Search tools by query
│
├── meta/
│   ├── __init__.py
│   ├── openapi_client.py      # Fetch/parse OpenAPI specs
│   └── auth.py                # Auth profile management
│
├── delegation/
│   ├── __init__.py
│   ├── job_builder.py         # Build K8s Job specs
│   ├── status.py              # Poll job status
│   └── templates/
│       └── k8s_job.yaml       # K8s Job template
│
├── storage/
│   ├── __init__.py
│   ├── db.py                  # PostgreSQL client
│   ├── cas.py                 # CAS integration
│   └── redis_client.py        # Redis client
│
└── tests/
    ├── __init__.py
    ├── test_primitives.py     # Unit tests for primitives
    ├── test_tools.py          # Tool execution tests
    ├── test_integration.py    # End-to-end tests
    └── examples.py            # Example prompts with expected outputs
```

---

## 10. Testing Strategy

### 10.1 Example Prompts

**Test 1: Fast Lane - Simple Query**
```
Prompt: "fetch flights from NYC to LAX, sort by price, show top 3"

Expected Tool Call:
{
  "tool": "execute_pipeline",
  "args": {
    "session_id": "test-123",
    "pipeline": [
      {"step": "http_request", "url": "api.flights.com/search", "params": {"origin": "NYC", "dest": "LAX"}},
      {"step": "table_sort", "field": "price", "order": "asc"},
      {"step": "top_k", "k": 3}
    ]
  }
}

Expected Result: Array of 3 flight objects
```

**Test 2: Patch Lane - Persistent Change**
```
Prompt: "add email notification whenever price drops below $500"

Expected Tool Call:
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
            "id": "price_check",
            "type": "condition",
            "expr": "price < 500"
          }
        },
        {
          "op": "add",
          "path": "/nodes/-",
          "value": {
            "id": "send_email",
            "type": "task",
            "config": {"action": "email", "subject": "Price drop!"}
          }
        },
        {
          "op": "add",
          "path": "/edges/-",
          "value": {"from": "fetch_flights", "to": "price_check"}
        },
        {
          "op": "add",
          "path": "/edges/-",
          "value": {"from": "price_check", "to": "send_email", "condition": "true"}
        }
      ],
      "description": "Add price drop email notification"
    }
  }
}

Expected Result: patch_id returned
```

**Test 3: Multi-Turn with Context**
```
Turn 1: "fetch flights NYC to LAX"
  → Result: 50 flights stored in session

Turn 2: "now filter those under $500"
  → LLM uses session context, filters previous result

Expected Tool Call:
{
  "tool": "execute_pipeline",
  "args": {
    "session_id": "test-123",
    "input_ref": "cas://sha256:...",  # From turn 1
    "pipeline": [
      {"step": "table_filter", "condition": {"field": "price", "op": "<", "value": 500}}
    ]
  }
}
```

### 10.2 Integration Test

```python
# tests/test_integration.py

def test_full_workflow():
    # 1. Setup Redis with mock job
    job = {
        "version": "1.0",
        "job_id": "test-job-123",
        "prompt": "fetch flights, sort by price, top 3",
        "context": {"session_id": "test-sess"}
    }
    redis.rpush("agent:jobs", json.dumps(job))

    # 2. Worker picks up job (run in background)
    # Worker processes job...

    # 3. Check result in Redis
    result = redis.blpop(f"agent:results:{job['job_id']}", timeout=30)
    assert result is not None

    result_data = json.loads(result[1])
    assert result_data['status'] == 'completed'
    assert 'result_ref' in result_data

    # 4. Verify result in DB
    db_result = db.query("SELECT * FROM agent_results WHERE job_id = %s", (job['job_id'],))
    assert db_result is not None
    assert db_result.result_data is not None or db_result.cas_id is not None
```

### 10.3 Mock OpenAI for Testing

```python
# tests/mock_openai.py

class MockOpenAI:
    def chat_completions_create(self, messages, tools):
        prompt = messages[-1]['content']

        if "fetch" in prompt and "sort" in prompt:
            return MockResponse(
                tool_calls=[{
                    "function": {
                        "name": "execute_pipeline",
                        "arguments": json.dumps({
                            "session_id": "test",
                            "pipeline": [
                                {"step": "http_request", "url": "api.flights.com"},
                                {"step": "table_sort", "field": "price"},
                                {"step": "top_k", "k": 3}
                            ]
                        })
                    }
                }]
            )
```

---

## Appendix A: Dependencies

```txt
# requirements.txt

# Core
fastapi==0.104.1
uvicorn[standard]==0.24.0
redis==5.0.1
psycopg2-binary==2.9.9

# LLM
openai==1.3.0

# Data processing
jq==1.6.0
pyjq==2.6.0

# OpenAPI
openapi-core==0.18.2
requests==2.31.0

# Utilities
pydantic==2.5.0
python-dotenv==1.0.0
```

---

## Appendix B: Configuration

```yaml
# config.yaml

service:
  name: agent-runner
  port: 8082
  workers: 4

redis:
  host: localhost
  port: 6379
  db: 0
  job_queue: agent:jobs
  result_queue_prefix: agent:results

database:
  host: localhost
  port: 5432
  dbname: orchestrator
  user: sdutt
  password: ${DB_PASSWORD}

llm:
  provider: openai
  model: gpt-4o
  temperature: 0.1
  max_tokens: 4000
  timeout_sec: 30

orchestrator:
  api_url: http://localhost:8081/api/v1

storage:
  max_inline_bytes: 10485760  # 10MB
  cas_enabled: true
  s3_enabled: false

auth_profiles:
  weatherapi:
    type: api_key
    header: X-API-Key
    value: ${WEATHER_API_KEY}

  internal:
    type: bearer
    token: ${INTERNAL_TOKEN}
```

---

## Appendix C: Deployment

```dockerfile
# Dockerfile

FROM python:3.11-slim

WORKDIR /app

# Install dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy application
COPY . .

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:8082/health || exit 1

# Run service
CMD ["python", "main.py"]
```

```yaml
# docker-compose.yml

version: '3.8'

services:
  agent-runner:
    build: ./cmd/agent-runner-py
    ports:
      - "8082:8082"
    environment:
      - REDIS_HOST=redis
      - DB_HOST=postgres
      - OPENAI_API_KEY=${OPENAI_API_KEY}
    depends_on:
      - redis
      - postgres
    restart: unless-stopped

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"

  postgres:
    image: postgres:15-alpine
    environment:
      - POSTGRES_DB=orchestrator
      - POSTGRES_USER=sdutt
      - POSTGRES_PASSWORD=${DB_PASSWORD}
    ports:
      - "5432:5432"
```

---

**END OF DOCUMENT**
