# Workflow Fetching Fix

## Problem

The agent-runner-py was not receiving the current workflow structure, which prevented:
1. LLM from seeing the full workflow context
2. LLM from knowing which node ID to use when creating patch edges
3. Proper workflow owner propagation for API calls

## Root Cause

In `storage/redis_client.py`, the job was constructed without:
- `current_workflow`: Expected by `main.py` at line 167
- `current_node_id`: Expected by `main.py` at line 172
- Proper `workflow_owner`: Was hardcoded as 'test-user'

## Solution

### Changes to `cmd/agent-runner-py/storage/redis_client.py`

**Before:**
```python
job = {
    'job_id': token.get('id'),
    'run_id': token.get('run_id'),
    'node_id': token.get('to_node'),
    'task': metadata.get('task', ''),
    'context': metadata.get('context', {}),
    'workflow_owner': 'test-user',  # ❌ Hardcoded
    'workflow_tag': metadata.get('workflow_tag', ''),
    'token': token,
    'message_id': message_id
}
```

**After:**
```python
# Fetch current workflow IR from Redis
run_id = token.get('run_id')
current_workflow = None
if run_id:
    ir_key = f"ir:{run_id}"
    try:
        ir_data = self.client.get(ir_key)
        if ir_data:
            current_workflow = json.loads(ir_data)
            logger.info(f"Fetched workflow IR from Redis: {ir_key}")
    except Exception as e:
        logger.error(f"Failed to fetch workflow IR from Redis: {e}")

job = {
    'job_id': token.get('id'),
    'run_id': run_id,
    'node_id': token.get('to_node'),
    'task': metadata.get('task', ''),
    'context': metadata.get('context', {}),
    'workflow_owner': token.get('workflow_owner', 'test-user'),  # ✓ From coordinator
    'workflow_tag': metadata.get('workflow_tag', ''),
    'current_workflow': current_workflow,  # ✓ Fetched from Redis
    'current_node_id': token.get('to_node'),  # ✓ Node that will execute
    'token': token,
    'message_id': message_id
}
```

## Data Flow

### 1. Coordinator Stores IR
```go
// cmd/workflow-runner/coordinator/coordinator.go
key := fmt.Sprintf("ir:%s", runID)
data, _ := json.Marshal(ir)
c.redis.Set(ctx, key, data, 0)
```

### 2. Coordinator Publishes Token
```go
// cmd/workflow-runner/coordinator/coordinator.go (line 714)
token["workflow_owner"] = username  // From IR metadata
token["to_node"] = nextNodeID       // The agent node that will execute
```

### 3. Redis Client Fetches Workflow
```python
# cmd/agent-runner-py/storage/redis_client.py
ir_key = f"ir:{run_id}"
current_workflow = json.loads(self.client.get(ir_key))
```

### 4. Agent Receives Complete Context
```python
# cmd/agent-runner-py/main.py (line 167-174)
if job.get('current_workflow'):
    enhanced_context['current_workflow'] = job['current_workflow']

if node_id:
    enhanced_context['current_node_id'] = node_id
```

### 5. LLM Creates Patch with Correct Edges
```json
{
  "name": "patch_workflow",
  "arguments": {
    "patch_spec": {
      "operations": [
        {
          "op": "add",
          "path": "/nodes/-",
          "value": {"id": "new_node", "type": "http", "config": {...}}
        },
        {
          "op": "add",
          "path": "/edges/-",
          "value": {
            "from": "agent_1",  // ✓ Uses current_node_id from context
            "to": "new_node"
          }
        }
      ]
    }
  }
}
```

## Impact

### Before Fix
- ❌ LLM couldn't see workflow structure
- ❌ LLM didn't know current node ID
- ❌ Patches created nodes without edges (orphaned)
- ❌ workflow_owner was always 'test-user'

### After Fix
- ✅ LLM sees full workflow structure
- ✅ LLM knows current_node_id for edge creation
- ✅ Patches include proper edge connections
- ✅ workflow_owner propagates correctly from IR metadata

## Testing

To verify the fix:

1. **Check IR is fetched:**
```bash
# Look for log line:
# "Fetched workflow IR from Redis: ir:{run_id}, nodes=..."
```

2. **Check workflow_owner propagation:**
```bash
# Log should show:
# "workflow_owner='<actual-username>'"  # Not 'test-user'
```

3. **Check current_node_id:**
```bash
# Log should show:
# "current_node_id='agent_1'"  # Or whatever the agent node ID is
```

4. **Verify patch includes edges:**
```bash
# Check patch operations include:
# {"op": "add", "path": "/edges/-", "value": {"from": "agent_1", "to": "..."}}
```

## Related Files

- `cmd/agent-runner-py/storage/redis_client.py` - Fetches workflow from Redis
- `cmd/agent-runner-py/main.py` - Uses workflow and node_id in context
- `cmd/workflow-runner/coordinator/coordinator.go` - Stores IR and publishes tokens
- `cmd/agent-runner-py/agent/tools.py` - Tool descriptions emphasizing edges
- `cmd/agent-runner-py/PATCH_WORKFLOW_GUIDE.md` - Documentation on patching

## Next Steps

The coordinator will now:
1. Detect agent completion
2. Fetch all run patches from orchestrator
3. Materialize workflow (base + patches)
4. Update IR in Redis
5. Route to newly added nodes

This completes the end-to-end self-modifying workflow feature.
