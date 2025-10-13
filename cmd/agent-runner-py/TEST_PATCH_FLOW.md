# Test Agent Patch Flow

## Two Test Scripts Available:

### 1. Standalone Test (Simpler)
**Location:** `cmd/agent-runner-py/test_patch_flow.sh`

Tests just the agent service in isolation.

**Prerequisites:**
- Redis running (`redis-server`)
- OpenAI API key in `.env`
- Python dependencies installed

**What it does:**
1. Creates a test workflow with 2 nodes
2. Sends patch job: "add email notification after process_results when price < 500"
3. Waits for agent to process
4. Shows the generated patch operations

**Run:**
```bash
cd /Users/sdutt/Documents/practice/lyzr/orchestrator/cmd/agent-runner-py
./test_patch_flow.sh
```

---

### 2. Full Integration Test
**Location:** `cmd/orchestrator/testdata/test_agent_patch.sh`

Tests the complete flow with orchestrator.

**Prerequisites:**
- Redis running
- Orchestrator API running
- Agent service running
- `aob` CLI installed

**What it does:**
1. Creates a real workflow via orchestrator/aob
2. Sends patch job to agent
3. Agent patches the workflow
4. Fetches and displays the patched workflow
5. Leaves workflow in place for inspection

**Run:**
```bash
cd /Users/sdutt/Documents/practice/lyzr/orchestrator
./cmd/orchestrator/testdata/test_agent_patch.sh
```

---

## Expected Output

### Standalone Test Output:

```
==========================================
Agent Patch Flow Test (Standalone)
==========================================

✓ Dependencies OK
✓ Redis running

Initial Workflow:
{
  "nodes": [
    {
      "id": "fetch_flights",
      "type": "http",
      "config": {"url": "https://api.flights.com/search"}
    },
    {
      "id": "process_results",
      "type": "transform",
      "config": {"script": "process.py"}
    }
  ],
  "edges": [...]
}

Creating patch job...
Prompt: 'add email notification after process_results when price < 500'

✓ Agent service already running

Publishing job to Redis...
✓ Job published (ID: abc-123-...)

Waiting for agent response (max 30s)...
....................

==========================================
Agent Response Received!
==========================================

{
  "version": "1.0",
  "job_id": "abc-123-...",
  "status": "completed",
  "result_ref": "artifact://...",
  "metadata": {
    "tool_calls": [
      {
        "function": {
          "name": "patch_workflow",
          "arguments": {
            "workflow_tag": "test-workflow",
            "patch_spec": {
              "operations": [
                {
                  "op": "add",
                  "path": "/nodes/-",
                  "value": {
                    "id": "price_check",
                    "type": "conditional",
                    "config": {"condition": "price < 500"}
                  }
                },
                {
                  "op": "add",
                  "path": "/nodes/-",
                  "value": {
                    "id": "email_notification",
                    "type": "function",
                    "config": {
                      "handler": "send_email",
                      "to": "admin@example.com"
                    }
                  }
                },
                {
                  "op": "add",
                  "path": "/edges/-",
                  "value": {"from": "process_results", "to": "price_check"}
                },
                {
                  "op": "add",
                  "path": "/edges/-",
                  "value": {"from": "price_check", "to": "email_notification", "condition": "true"}
                }
              ],
              "description": "Add email notification when price < 500"
            }
          }
        }
      }
    ],
    "tokens_used": 653,
    "execution_time_ms": 1456,
    "llm_model": "gpt-4o-mini"
  }
}

✓ Status: COMPLETED

Patch Operations Generated:
[
  {
    "op": "add",
    "path": "/nodes/-",
    "value": {
      "id": "price_check",
      "type": "conditional",
      ...
    }
  },
  ...
]

Metadata:
  Tokens: 653
  Time: 1456ms
  Model: gpt-4o-mini

==========================================
✓ Test PASSED
==========================================
```

---

## Manual Test (Without Script)

You can also test manually:

### 1. Start Services
```bash
# Terminal 1: Redis
redis-server

# Terminal 2: Agent Service
cd cmd/agent-runner-py
python3 main.py
```

### 2. Send Test Job
```bash
# Create test job
cat > /tmp/test_job.json <<'EOF'
{
  "version": "1.0",
  "job_id": "test-123",
  "run_id": "run-001",
  "node_id": "agent",
  "workflow_tag": "my-workflow",
  "workflow_owner": "test-user",
  "prompt": "add email notification node after processing",
  "current_workflow": {
    "nodes": [
      {"id": "fetch", "type": "http", "config": {}}
    ],
    "edges": []
  },
  "context": {"session_id": "test"}
}
EOF

# Publish to Redis
redis-cli RPUSH agent:jobs "$(cat /tmp/test_job.json)"

# Wait for result
redis-cli BLPOP agent:results:test-123 30
```

---

## Troubleshooting

### API Quota Exceeded
```
Error code: 429 - insufficient_quota
```
**Solution:** Add credits to OpenAI account or use different API key

### Redis Not Running
```
✗ Redis not running
```
**Solution:** `redis-server`

### Agent Service Not Responding
```
✗ Timeout waiting for response
```
**Solutions:**
- Check agent service is running: `curl http://localhost:8082/health`
- Check logs: `tail -f agent.log`
- Verify Redis connection
- Check OpenAI API key is valid

---

## Clean Up

The standalone test auto-cleans up. For the full integration test:

```bash
# Clean up test workflow
aob workflow delete test-user test-agent-patch

# Or manually via API
curl -X DELETE http://localhost:8081/api/v1/workflows/test-user/test-agent-patch
```
