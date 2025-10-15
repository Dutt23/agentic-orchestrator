# Run Patches Architecture

## Overview

Run patches enable workflows to modify themselves during execution. This allows agent nodes to dynamically add nodes and edges based on runtime decisions, creating truly self-modifying workflows.

## Key Concepts

### 1. Run-Specific Patches

- **Scope**: Patches are tied to a specific workflow run (`run_id`), not to the workflow definition
- **Isolation**: Each run has its own independent patch chain
- **Sequence**: Patches are numbered sequentially (1, 2, 3...) per run
- **Storage**: Stored in `run_patches` table with references to CAS blobs

### 2. Patch Operations

Patches use JSON Patch (RFC 6902) format with three operations:
- `add`: Add new nodes or edges
- `remove`: Remove existing nodes or edges
- `replace`: Update node properties

### 3. Edge Connectivity

**CRITICAL**: When adding nodes, you MUST also add edges connecting them to the current node. New nodes without edges are orphaned and will never execute.

## Architecture Components

### 1. Agent Runner (Python)

**Location**: `cmd/agent-runner-py/`

**Responsibilities**:
- Receives agent tasks from Redis streams
- Calls LLM with tools (`execute_pipeline`, `patch_workflow`)
- When LLM calls `patch_workflow`, forwards to orchestrator API
- Completes and signals coordinator

**Key Files**:
- `main.py`: Job processing and tool execution
- `agent/tools.py`: Tool definitions with edge connectivity guidance
- `workflow/patch_client.py`: HTTP client for patch creation

### 2. Orchestrator (Go)

**Location**: `cmd/orchestrator/`

**Responsibilities**:
- Stores patches in database and CAS
- Assigns sequence numbers
- Provides APIs for patch creation and retrieval

**Key Files**:
- `service/run_patch.go`: Business logic for run patches
- `repository/run_patch.go`: Database operations
- `handlers/run_patch.go`: HTTP handlers
- `migrations/006_add_run_patches.sql`: Database schema

**API Endpoints**:
- `POST /api/v1/runs/:run_id/patches` - Create patch
- `GET /api/v1/runs/:run_id/patches` - List patches for run
- `GET /api/v1/runs/:run_id/patches/:cas_id/operations` - Get patch operations

### 3. Workflow Runner - Coordinator (Go)

**Location**: `cmd/workflow-runner/coordinator/`

**Responsibilities**:
- Detects when agent nodes complete
- Fetches run patches from orchestrator
- Materializes patched workflow (base + patches)
- Updates IR in Redis
- Routes to new nodes added by patches

**Key Files**:
- `coordinator.go`: Main coordination logic
- `orchestrator_client.go`: HTTP client for fetching patches

**Key Methods**:
- `handleCompletion()`: Processes node completion signals
- `reloadIRIfPatched()`: Checks for patches and updates IR
- `irToWorkflow()`: Converts IR back to workflow format for patching

## Execution Flow

### 1. Workflow Starts

```
User -> Orchestrator -> Workflow Runner
                     -> Compile to IR
                     -> Store in Redis (ir:{run_id})
                     -> Start execution
```

### 2. Agent Node Executes

```
Coordinator -> Publishes token to wf.tasks.agent stream
Agent Worker -> Pops job from Redis
             -> Calls LLM with context (including current_workflow, current_node_id)
             -> LLM returns tool call (patch_workflow)
             -> Creates patch via orchestrator API
```

### 3. Patch Created

```
Agent Runner -> POST /api/v1/runs/{run_id}/patches
Orchestrator -> Store operations in CAS
             -> Create artifact
             -> Insert into run_patches table (seq=1, 2, 3...)
             -> Return patch_id to agent
```

### 4. Agent Completes

```
Agent Runner -> Signals completion to coordinator
Coordinator  -> Detects agent node type
             -> Calls reloadIRIfPatched()
             -> Fetches all patches for run_id from orchestrator
             -> Converts current IR to workflow format
             -> Applies patches sequentially (patch ops on workflow JSON)
             -> Stores patched workflow
             -> Next loadIR() will use patched version
```

### 5. Continue Execution

```
Coordinator -> Reloads IR (now includes patched nodes)
            -> Finds edges from completed agent node
            -> Routes to next nodes (including newly added ones)
            -> New nodes execute normally
```

## Database Schema

### run_patches Table

```sql
CREATE TABLE run_patches (
    id UUID PRIMARY KEY,
    run_id VARCHAR(255) NOT NULL,
    artifact_id UUID NOT NULL REFERENCES artifact(artifact_id),
    seq INTEGER NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_by VARCHAR(255),
    UNIQUE(run_id, seq)
);
```

### Key Points:
- `seq`: Order of patch application (1, 2, 3...)
- `artifact_id`: Reference to patch artifact in CAS
- `UNIQUE(run_id, seq)`: Ensures strict ordering per run

## Example: Agent Self-Patching

### User Request
"Add a branch that checks if the value is > 5, then make an HTTP call to /api/notify"

### LLM Context
```json
{
  "current_workflow": {
    "nodes": [...],
    "edges": [...]
  },
  "current_node_id": "agent_1"
}
```

### LLM Tool Call
```json
{
  "name": "patch_workflow",
  "arguments": {
    "patch_spec": {
      "operations": [
        {
          "op": "add",
          "path": "/nodes/-",
          "value": {
            "id": "branch_1",
            "type": "branch",
            "config": {"rules": [...]}
          }
        },
        {
          "op": "add",
          "path": "/nodes/-",
          "value": {
            "id": "http_1",
            "type": "http",
            "config": {"url": "https://api.example.com/notify"}
          }
        },
        {
          "op": "add",
          "path": "/edges/-",
          "value": {"from": "agent_1", "to": "branch_1"}
        },
        {
          "op": "add",
          "path": "/edges/-",
          "value": {"from": "branch_1", "to": "http_1"}
        }
      ]
    }
  }
}
```

### Result
- Patch stored with `seq=1`
- Agent completes
- Coordinator detects completion
- IR updated with new nodes and edges
- Execution continues to `branch_1` -> `http_1`

## Key Design Decisions

### 1. Why Run-Specific?

Run patches are separate from the workflow's main patch chain because:
- Different runs may take different execution paths
- Patches should not pollute the base workflow definition
- Each run needs independent modification capability
- Users can view patch history per run for debugging

### 2. Why Sequential Application?

Patches are applied in strict sequence order because:
- Later patches may depend on nodes added by earlier patches
- Deterministic execution order is critical
- Easy to reason about and debug

### 3. Why Materialize Each Time?

Instead of storing the patched IR directly:
- Base workflow + patches is the source of truth
- Easier to debug (can view each patch)
- Can reconstruct workflow at any point
- Supports future features like patch rollback

### 4. Why Edge Enforcement?

The LLM is explicitly instructed to add edges because:
- Orphaned nodes will never execute
- Common mistake in self-patching systems
- Tool description includes prominent warnings
- Examples show correct edge creation

## Future Enhancements

- [x] Auto-reload and apply patches to IR in coordinator
- [ ] Validation: Check all nodes are reachable before accepting patch
- [ ] UI: Visual diff showing what was patched
- [ ] Rollback: Ability to undo specific patches
- [ ] Optimize: Cache materialized workflows to avoid recomputation
- [ ] Conflict Detection: Warn if patches modify same nodes
- [ ] Patch Templates: Pre-defined patterns for common modifications

## Testing

### Unit Tests
- Patch creation and storage
- Sequence number generation
- Operation validation

### Integration Tests
- End-to-end agent self-patching
- Multiple patches in sequence
- Edge connectivity validation
- IR reload and materialization

### Test Files
- `test_patch_flow.sh`: End-to-end patch workflow test
- Integration tests in `cmd/orchestrator/handlers/run_patch_test.go`

## Debugging

### View Patches for a Run

```bash
GET /api/v1/runs/{run_id}/patches
```

### View Patch Operations

```bash
GET /api/v1/runs/{run_id}/patches/{cas_id}/operations
```

### Check IR in Redis

```bash
redis-cli GET ir:{run_id}
```

### Common Issues

**New nodes don't execute:**
- Check if edges were added connecting them
- Verify patch was stored (check logs)
- Check coordinator reloaded IR (look for "patched workflow stored" log)

**Patches not applied:**
- Check agent node completed successfully
- Verify coordinator detected agent type
- Check orchestrator API is reachable from coordinator

**Sequence numbers wrong:**
- Check database `UNIQUE(run_id, seq)` constraint
- Verify concurrent patch creation is handled correctly

## Configuration

### Agent Runner

```yaml
orchestrator:
  api_url: "http://localhost:8081"  # Orchestrator base URL
```

### Workflow Runner

```bash
export ORCHESTRATOR_URL="http://localhost:8081"
```

## References

- [Patch Workflow Guide](../cmd/agent-runner-py/PATCH_WORKFLOW_GUIDE.md)
- [JSON Patch RFC 6902](https://tools.ietf.org/html/rfc6902)
- [Workflow Schema](../common/schema/workflow.schema.json)
