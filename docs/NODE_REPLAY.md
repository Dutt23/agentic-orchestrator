# Node-Level Replay Feature

## Introduction

**Node-level replay** allows you to retry or re-execute specific nodes in a workflow without rerunning the entire workflow from the beginning. This is particularly useful for:

- **Recovering from transient failures** (network timeouts, temporary service unavailability)
- **Fixing configuration errors** (incorrect prompts, wrong API endpoints)
- **Development & debugging** (testing specific nodes in isolation)
- **Cost optimization** (avoiding reprocessing expensive upstream operations)

### Replay vs. Full Rerun

| Scenario | Node Replay | Full Rerun |
|----------|-------------|------------|
| Single node failed | ✅ Fast, preserves upstream work | ❌ Wastes compute on successful nodes |
| Config needs fixing | ✅ Override config on-the-fly | ❌ Must update workflow definition |
| Testing changes | ✅ Iterate quickly on one node | ❌ Wait for entire workflow |
| Cost | 💰 Pay only for replayed node | 💰💰💰 Pay for entire workflow |

---

## Architecture Overview

### Token Injection Flow

```
┌─────────────┐
│   Client    │
│  (API Call) │
└──────┬──────┘
       │ POST /api/v1/runs/{run_id}/nodes/{node_id}/replay
       ↓
┌─────────────────┐
│ ReplayService   │
│ - Validate      │
│ - Load IR       │
│ - Load context  │
└──────┬──────────┘
       │ Inject token
       ↓
┌─────────────────┐
│  Redis Stream   │
│ wf.tasks.{type} │
└──────┬──────────┘
       │ XREADGROUP
       ↓
┌─────────────────┐
│  Worker         │
│ (HTTP/Agent/    │
│  HITL)          │
└──────┬──────────┘
       │ Execute
       ↓
┌─────────────────┐
│  Coordinator    │
│ (Normal flow)   │
└─────────────────┘
```

### Components Involved

- **ReplayService**: Orchestrates replay logic, validates requests
- **Redis**: Stores IR (workflow structure) and context (node outputs)
- **Streams**: `wf.tasks.http`, `wf.tasks.agent`, `wf.tasks.hitl`, `wf.tasks.function`
- **Workers**: Pick up injected tokens and execute nodes
- **Coordinator**: Handles completion signals normally

---

## API Documentation

### Endpoint

```
POST /api/v1/runs/:run_id/nodes/:node_id/replay
```

### Headers

```
X-User-ID: <username>       # Required for authentication
Content-Type: application/json
```

### Request Body

```json
{
  "config": {                  // Optional: Override node configuration
    "task": "Updated prompt",
    "temperature": 0.7
  },
  "reset_counter": false,      // Optional: Recalculate workflow counter
  "force_replay": false,       // Optional: Allow replay on non-failed runs
  "reason": "Why replaying"    // Optional: Audit trail explanation
}
```

### Response (200 OK)

```json
{
  "replayed": true,
  "token_id": "abc-fetch-456",
  "stream": "wf.tasks.http",
  "message": "Token injected, node will execute shortly"
}
```

### Error Responses

#### 404 - Node Not Found
```json
{
  "error": "node not found in workflow IR",
  "run_id": "abc-123",
  "node_id": "invalid_node"
}
```

#### 400 - Invalid Run State
```json
{
  "error": "run must be in FAILED state or use force_replay=true",
  "run_id": "abc-123",
  "current_status": "COMPLETED"
}
```

#### 404 - IR Expired
```json
{
  "error": "workflow IR not found (may have expired after 24h TTL)",
  "run_id": "abc-123"
}
```

---

## Practical Examples

### Example 1: Retry a Failed HTTP Node

**Scenario**: An HTTP node failed due to a temporary network timeout. The service is now available.

**Workflow Structure**:
```
fetch_user_data (HTTP) -> process_data (function) -> store_results (HTTP)
      ↓ FAILED
```

**Solution**: Replay the `fetch_user_data` node without rerunning anything.

```bash
curl -X POST http://localhost:8081/api/v1/runs/550e8400-e29b-41d4-a716-446655440000/nodes/fetch_user_data/replay \
  -H "X-User-ID: alice" \
  -H "Content-Type: application/json" \
  -d '{
    "reason": "Retrying after network issue resolved"
  }'
```

**Response**:
```json
{
  "replayed": true,
  "token_id": "550e8400-fetch-1234567890",
  "stream": "wf.tasks.http",
  "message": "Token injected, node will execute shortly"
}
```

**What Happens**:
1. Token injected to `wf.tasks.http` stream
2. HTTP worker picks up token
3. Makes API call to fetch user data
4. On success, signals coordinator
5. Coordinator routes to next node (`process_data`)
6. Workflow continues normally

---

### Example 2: Fix Agent Prompt and Replay

**Scenario**: An agent node failed because the prompt was too vague, resulting in poor output.

**Original Config**:
```json
{
  "task": "Analyze data",
  "model": "claude-3-5-sonnet-20241022"
}
```

**Problem**: Prompt was too vague, agent produced unhelpful output.

**Solution**: Replay with a more specific prompt.

```bash
curl -X POST http://localhost:8081/api/v1/runs/550e8400-e29b-41d4-a716-446655440000/nodes/analyze_sales/replay \
  -H "X-User-ID: alice" \
  -H "Content-Type: application/json" \
  -d '{
    "config": {
      "task": "Analyze the Q4 sales data and identify the top 3 performing products by revenue. Provide specific numbers and percentage growth.",
      "model": "claude-3-5-sonnet-20241022",
      "temperature": 0.7
    },
    "reason": "Made prompt more specific with clear success criteria"
  }'
```

**What Happens**:
1. New config with improved prompt is used
2. Original config in IR is NOT modified (this run only)
3. Agent executes with new instructions
4. Better output is produced
5. Workflow continues

---

### Example 3: Replay Middle Node in Linear Workflow

**Scenario**: A 5-step linear workflow where step 3 failed. Steps 1-2 were expensive data processing operations.

**Workflow**:
```
download_data (HTTP) -> transform (function) -> validate (function) -> upload (HTTP) -> notify (HTTP)
                                                      ↓ FAILED
```

**Why Step 3 Failed**: Data validation found unexpected null values (bug in transform logic, now fixed).

**Solution**: Replay `validate` node, reusing output from `transform`.

```bash
curl -X POST http://localhost:8081/api/v1/runs/550e8400-e29b-41d4-a716-446655440000/nodes/validate/replay \
  -H "X-User-ID: alice" \
  -H "Content-Type: application/json" \
  -d '{
    "reason": "Replaying after fixing transform function bug"
  }'
```

**What Happens**:
1. ReplayService loads context from `transform:output`
2. Creates token with `transform` node's output as `payload_ref`
3. `validate` node receives same input as original execution
4. Executes validation (now passes due to fixed transform)
5. Continues to `upload` and `notify`

**Cost Savings**: Avoided re-downloading and re-transforming large dataset.

---

### Example 4: Force Replay on Running Workflow

**Scenario**: Workflow appears stuck (HITL node waiting for approval that will never come, or worker crashed).

**Current State**: Run status is `RUNNING`, but coordinator shows no activity for 30 minutes.

**Solution**: Manually push the stuck node forward or retry it.

```bash
curl -X POST http://localhost:8081/api/v1/runs/550e8400-e29b-41d4-a716-446655440000/nodes/manual_review/replay \
  -H "X-User-ID: alice" \
  -H "Content-Type: application/json" \
  -d '{
    "force_replay": true,
    "reason": "Manual intervention - HITL approval stuck, bypassing"
  }'
```

**⚠️ Warning**: This can cause race conditions if the node is actually still processing. Use with caution.

---

### Example 5: Testing Node Changes in Development

**Scenario**: Developing a new agent node, want to iterate quickly on the prompt without rerunning entire workflow.

**Workflow** (dev environment):
```
load_test_data -> run_agent -> verify_output
                     ↑
                  (iterating)
```

**Iteration Loop**:
```bash
# First run - agent output wasn't good
curl -X POST http://localhost:8081/api/v1/runs/dev-run-123/nodes/run_agent/replay \
  -H "X-User-ID: dev-alice" \
  -d '{
    "config": { "task": "Try prompt version 2..." },
    "force_replay": true,
    "reason": "Testing prompt variation #2"
  }'

# Second run - getting better
curl -X POST http://localhost:8081/api/v1/runs/dev-run-123/nodes/run_agent/replay \
  -H "X-User-ID: dev-alice" \
  -d '{
    "config": { "task": "Try prompt version 3..." },
    "force_replay": true,
    "reason": "Testing prompt variation #3"
  }'

# Final run - perfect!
```

**Development Velocity**: Instead of waiting 5 minutes for full workflow × 5 iterations = 25 minutes, each replay takes 10 seconds = 50 seconds total.

---

## Use Cases

### 1. Transient Failures

**Common Scenarios**:
- API rate limiting (429 errors)
- Network timeouts
- Temporary service unavailability
- Database connection pool exhaustion

**Solution**: Wait for the issue to resolve, then replay the failed node.

**Example**:
```bash
# External API returned 429 (rate limit exceeded)
# Wait 60 seconds, then replay
curl -X POST .../nodes/call_external_api/replay \
  -d '{"reason": "Retrying after rate limit reset"}'
```

---

### 2. Configuration Fixes

**Common Scenarios**:
- Typo in API endpoint URL
- Wrong model name for agent
- Incorrect temperature setting
- Malformed request body

**Solution**: Replay with corrected configuration.

**Example**:
```bash
# Agent used wrong model
curl -X POST .../nodes/summarize/replay \
  -d '{
    "config": {
      "model": "claude-3-5-sonnet-20241022",  # Fixed!
      "task": "Summarize the article"
    },
    "reason": "Fixed model name typo"
  }'
```

---

### 3. Development & Testing

**Common Scenarios**:
- Testing prompt variations
- Debugging node behavior
- Performance profiling
- Integration testing

**Solution**: Use `force_replay=true` to iterate rapidly.

**Example**:
```bash
# Test different temperature settings
for temp in 0.3 0.5 0.7 0.9; do
  curl -X POST .../nodes/creative_writing/replay \
    -d "{
      \"config\": {\"temperature\": $temp},
      \"force_replay\": true,
      \"reason\": \"Testing temperature=$temp\"
    }"
done
```

---

### 4. Partial Reruns

**Common Scenarios**:
- Expensive ETL pipeline where final step failed
- Multi-stage ML workflow where training succeeded but evaluation failed
- Long-running data processing where downstream validation failed

**Solution**: Replay from failure point, reusing upstream results.

**Example**:
```bash
# 3-hour data processing succeeded
# 10-minute validation failed due to schema mismatch (now fixed)
# Replay validation instead of rerunning 3-hour processing
curl -X POST .../nodes/validate_schema/replay \
  -d '{"reason": "Schema updated, revalidating"}'
```

---

## How It Works (Technical Details)

### 1. Token Injection Mechanics

When you call the replay endpoint, here's what happens under the hood:

```go
// 1. Load IR from Redis
ir := redis.Get("ir:{run_id}")

// 2. Find the target node
node := ir.Nodes[nodeID]

// 3. Load upstream output (if any)
payloadRef := ""
if len(node.Dependencies) > 0 {
    predID := node.Dependencies[0]
    payloadRef = redis.HGet("context:{run_id}", predID+":output")
}

// 4. Create token
token := {
    "id": uuid.New(),
    "run_id": runID,
    "to_node": nodeID,
    "payload_ref": payloadRef,  // CAS reference to upstream output
    "config": config,            // Original or overridden
    "metadata": {...}
}

// 5. Inject to stream
stream := getStreamForNodeType(node.Type)
redis.XAdd(stream, "token", json.Marshal(token))

// 6. Update counter
sdk.Emit(ctx, runID, "", []string{nodeID}, payloadRef)
```

### 2. Counter Management

The **workflow counter** tracks active nodes:
- Starts at N (number of entry nodes)
- Each completion: -1
- Each new node scheduled: +1
- Reaches 0: workflow completes

**Replay Impact**:

**Default (`reset_counter=false`)**:
```
Counter before replay: 0 (workflow considered complete)
Action: Increment by 1
Counter after replay: 1 (workflow active again)
```

**With `reset_counter=true`**:
```
Action: Recalculate based on active nodes in IR
Counter = (total nodes - completed nodes)
```

**⚠️ Warning**: Misusing `reset_counter` can cause:
- Premature completion (counter hits 0 too early)
- Stuck workflows (counter never reaches 0)

### 3. Context Preservation

All node outputs are stored in Redis:
```
Key: context:{run_id}
Hash Fields:
  node_A:output -> cas:sha256:abc123...
  node_B:output -> cas:sha256:def456...
  node_C:failure -> {"error": "timeout", "retryable": true}
```

**Replay Process**:
1. Loads `node_A:output` from context
2. Passes CAS reference to replayed node
3. Node loads actual data from CAS: `cas:sha256:abc123`
4. Executes with loaded input

**TTL**: Context expires after 24 hours (same as IR).

---

## Configuration Options

### Request Body Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `config` | object | No | Node's original config | Override node configuration for this execution |
| `reset_counter` | boolean | No | `false` | Recalculate counter instead of incrementing |
| `force_replay` | boolean | No | `false` | Allow replay on RUNNING/COMPLETED runs |
| `reason` | string | No | `""` | Human-readable explanation for audit logs |

### Config Override Examples

#### HTTP Node
```json
{
  "config": {
    "url": "https://api.example.com/v2/users",  // Changed endpoint
    "method": "GET",
    "headers": {
      "Authorization": "Bearer new-token"       // Rotated token
    }
  }
}
```

#### Agent Node
```json
{
  "config": {
    "task": "Updated prompt with more specific instructions",
    "model": "claude-3-5-sonnet-20241022",
    "temperature": 0.5,  // More deterministic
    "max_tokens": 2000
  }
}
```

#### HITL Node
```json
{
  "config": {
    "message": "Updated approval message",
    "timeout": 3600,     // 1 hour instead of default
    "required": false    // Make approval optional
  }
}
```

---

## Error Scenarios & Troubleshooting

### Error 1: Node Not Found

**Response**:
```json
{
  "error": "node not found in workflow IR",
  "run_id": "550e8400-e29b-41d4-a716-446655440000",
  "node_id": "invalid_node"
}
```

**Causes**:
- Typo in node ID
- Node was removed by a patch
- Wrong run ID

**Solution**:
```bash
# List nodes in the run's IR
redis-cli GET "ir:550e8400-e29b-41d4-a716-446655440000" | jq '.nodes | keys'
```

---

### Error 2: Invalid Run State

**Response**:
```json
{
  "error": "run must be in FAILED state or use force_replay=true",
  "run_id": "550e8400-e29b-41d4-a716-446655440000",
  "current_status": "COMPLETED"
}
```

**Causes**:
- Run already completed successfully
- Trying to replay without `force_replay` flag

**Solution**:
```bash
# Option 1: Add force_replay flag
curl -X POST .../replay -d '{"force_replay": true, "reason": "..."}'

# Option 2: Check run status first
curl http://localhost:8081/api/v1/runs/550e8400-e29b-41d4-a716-446655440000
```

---

### Error 3: IR Not Found (Expired)

**Response**:
```json
{
  "error": "workflow IR not found (may have expired after 24h TTL)",
  "run_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

**Causes**:
- Run is older than 24 hours
- Redis was flushed
- IR key was manually deleted

**Solution**: Cannot replay. Must create a new run.

---

### Error 4: Upstream Context Missing

**Response**:
```json
{
  "error": "upstream node output not found in context",
  "run_id": "550e8400-e29b-41d4-a716-446655440000",
  "node_id": "process_data",
  "missing_dependency": "fetch_data"
}
```

**Causes**:
- Upstream node never completed
- Context expired (24h TTL)
- CAS blob was deleted

**Solution**: Replay upstream node first, or start fresh run.

---

## Limitations & Considerations

### Current Limitations

1. **24-Hour TTL**: IR and context expire after 24 hours. Cannot replay old runs.
2. **No Automatic Retry**: Must manually trigger replays via API.
3. **Counter Complexity**: Misusing `reset_counter` can break workflow completion.
4. **No Distributed Lock**: Race conditions possible if node is already executing.
5. **Single Node Only**: Cannot replay multiple nodes atomically.

### Best Practices

#### ✅ DO

- **Always provide a `reason`** for audit trail
- **Test in development** before production replays
- **Check run status** before replaying
- **Verify upstream outputs** are available
- **Use `force_replay` sparingly** (only when necessary)

#### ❌ DON'T

- **Don't replay entry nodes** without understanding counter implications
- **Don't use `reset_counter=true`** unless you understand the counter state
- **Don't replay already-executing nodes** (check logs first)
- **Don't rely on replays** as primary error handling (fix root causes)
- **Don't replay very old runs** (context may be expired)

### Security Considerations

- **Authentication**: Requires valid `X-User-ID` header
- **Authorization**: User must own the workflow (not yet enforced, TODO)
- **Audit Logging**: All replays are logged with username and reason
- **Rate Limiting**: No rate limits yet (TODO)

---

## Future Enhancements

### Planned Features

1. **Automatic Retry Logic**
   - Exponential backoff for transient failures
   - Max retry count per node
   - Retry policies (immediate, delayed, on-schedule)

2. **Dead Letter Queue (DLQ)**
   - Persistent storage for failed nodes
   - Manual review and replay from UI
   - Batch replay operations

3. **Checkpoint-Based Replay**
   - Save workflow state at specific nodes
   - Replay from checkpoint instead of individual nodes
   - Branching from checkpoints

4. **Multi-Node Replay**
   - Replay subgraph atomically
   - Parallel node replay
   - Conditional replay (if node X failed, replay X and Y)

5. **UI for Replay Operations**
   - Visual workflow graph with replay buttons
   - Replay history timeline
   - Config diff viewer (original vs. override)

6. **Analytics & Monitoring**
   - Replay success rate
   - Most-replayed nodes (indicates reliability issues)
   - Cost analysis (savings from replay vs. full rerun)

---

## Related Documentation

- [Workflow Execution Architecture](./WORKFLOW_EXECUTION.md) - How workflows execute
- [Redis Streams & Consumer Groups](./REDIS_ARCHITECTURE.md) - Stream-based messaging
- [Failure Handling & Recovery](./FAILURE_HANDLING.md) - Comprehensive error handling
- [API Reference](./API_REFERENCE.md) - Complete API documentation

---

## Quick Reference

### Basic Retry (Failed Node)
```bash
POST /api/v1/runs/{run_id}/nodes/{node_id}/replay
Content-Type: application/json
X-User-ID: alice

{
  "reason": "Retrying after transient failure"
}
```

### Config Override
```bash
{
  "config": {
    "task": "Updated prompt",
    "temperature": 0.7
  },
  "reason": "Fixed configuration"
}
```

### Force Replay (Running/Completed)
```bash
{
  "force_replay": true,
  "reason": "Manual intervention required"
}
```

### Check Run Status
```bash
GET /api/v1/runs/{run_id}
```

### List Nodes in IR
```bash
# Via Redis CLI
redis-cli GET "ir:{run_id}" | jq '.nodes | keys'
```

---

## Support

For questions or issues:
- File a GitHub issue: `https://github.com/lyzr/orchestrator/issues`
- Check logs: `kubectl logs -f workflow-runner-pod`
- Redis inspection: `redis-cli MONITOR`
