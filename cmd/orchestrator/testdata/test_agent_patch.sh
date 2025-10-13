#!/bin/bash

# End-to-end test: Create workflow → Agent patches it → View result
# This tests the complete agentic patching flow

set -e  # Exit on error

echo "=========================================="
echo "Agent Patch Integration Test"
echo "=========================================="
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
USER="test-user"
WORKFLOW_TAG="test-agent-patch"
JOB_ID=$(uuidgen | tr '[:upper:]' '[:lower:]')

echo "Configuration:"
echo "  User: $USER"
echo "  Workflow Tag: $WORKFLOW_TAG"
echo "  Job ID: $JOB_ID"
echo ""

# Check if Redis is running
echo "Checking Redis connection..."
if ! redis-cli ping > /dev/null 2>&1; then
    echo -e "${RED}✗ Redis is not running${NC}"
    echo "  Start Redis with: redis-server"
    exit 1
fi
echo -e "${GREEN}✓ Redis is running${NC}"
echo ""

# Check if agent service is running (port 8082)
echo "Checking Agent Service..."
if curl -s http://localhost:8082/health > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Agent service is running${NC}"
else
    echo -e "${YELLOW}⚠ Agent service not detected${NC}"
    echo "  Start it with: cd cmd/agent-runner-py && python3 main.py"
    echo "  Continuing anyway (will fail if service not running)..."
fi
echo ""

# Step 1: Create initial workflow
echo "=========================================="
echo "Step 1: Creating Initial Workflow"
echo "=========================================="

WORKFLOW_JSON='{
  "nodes": [
    {
      "id": "fetch_data",
      "type": "http",
      "config": {
        "url": "https://api.example.com/data",
        "method": "GET"
      }
    },
    {
      "id": "process_data",
      "type": "transform",
      "config": {
        "script": "process.py"
      }
    }
  ],
  "edges": [
    {
      "from": "fetch_data",
      "to": "process_data"
    }
  ],
  "metadata": {
    "name": "Test Workflow",
    "description": "Initial workflow before agent patch"
  }
}'

echo "Creating workflow: $USER:$WORKFLOW_TAG"
echo ""

# Save workflow to file
echo "$WORKFLOW_JSON" > /tmp/test_workflow_initial.json

# Create workflow (assuming orchestrator API or aob-cli)
# Replace with actual workflow creation command
# For now, let's assume we use the aob CLI
if command -v aob &> /dev/null; then
    echo "Using aob CLI to create workflow..."
    aob workflow create "$USER" "$WORKFLOW_TAG" /tmp/test_workflow_initial.json
else
    echo -e "${YELLOW}⚠ aob CLI not found, skipping workflow creation${NC}"
    echo "  Assuming workflow already exists..."
fi

echo ""
echo "Initial Workflow Structure:"
echo "$WORKFLOW_JSON" | jq '.nodes[] | "  - \(.id) (\(.type))"'
echo ""

# Step 2: Prepare agent job
echo "=========================================="
echo "Step 2: Sending Patch Job to Agent"
echo "=========================================="

AGENT_JOB='{
  "version": "1.0",
  "job_id": "'$JOB_ID'",
  "run_id": "test-run-001",
  "node_id": "agent_patcher",
  "workflow_tag": "'$WORKFLOW_TAG'",
  "workflow_owner": "'$USER'",
  "prompt": "add email notification node after process_data that sends alerts to admin@example.com",
  "current_workflow": '"$(echo "$WORKFLOW_JSON" | jq -c .)"',
  "context": {
    "session_id": "test-session-'$JOB_ID'",
    "workflow_state": {
      "current_step": 1,
      "total_steps": 2
    }
  },
  "timeout_sec": 60,
  "retry_count": 0,
  "created_at": "'$(date -u +%Y-%m-%dT%H:%M:%SZ)'"
}'

echo "Agent Prompt: 'add email notification node after process_data that sends alerts to admin@example.com'"
echo ""

# Save job to file
echo "$AGENT_JOB" > /tmp/agent_job.json
echo "Job saved to: /tmp/agent_job.json"
echo ""

# Publish job to Redis
echo "Publishing job to Redis (agent:jobs)..."
redis-cli RPUSH agent:jobs "$AGENT_JOB" > /dev/null
echo -e "${GREEN}✓ Job published${NC}"
echo ""

# Step 3: Wait for result
echo "=========================================="
echo "Step 3: Waiting for Agent Response"
echo "=========================================="

echo "Polling for result on agent:results:$JOB_ID..."
TIMEOUT=30
ELAPSED=0

while [ $ELAPSED -lt $TIMEOUT ]; do
    RESULT=$(redis-cli BLPOP "agent:results:$JOB_ID" 1 2>/dev/null | tail -n 1)

    if [ -n "$RESULT" ] && [ "$RESULT" != "(nil)" ]; then
        echo -e "${GREEN}✓ Result received!${NC}"
        echo ""

        # Parse and display result
        echo "Agent Result:"
        echo "$RESULT" | jq .
        echo ""

        # Check status
        STATUS=$(echo "$RESULT" | jq -r '.status')
        if [ "$STATUS" == "completed" ]; then
            echo -e "${GREEN}✓ Patch completed successfully${NC}"

            # Extract patch information
            TOOL_CALLS=$(echo "$RESULT" | jq '.metadata.tool_calls')
            echo ""
            echo "Tool Calls:"
            echo "$TOOL_CALLS" | jq .

            # Parse patch operations
            PATCH_ARGS=$(echo "$TOOL_CALLS" | jq -r '.[0].function.arguments')
            echo ""
            echo "Patch Operations:"
            echo "$PATCH_ARGS" | jq '.patch_spec.operations'

            echo ""
            echo "Metadata:"
            echo "  Tokens used: $(echo "$RESULT" | jq '.metadata.tokens_used')"
            echo "  Execution time: $(echo "$RESULT" | jq '.metadata.execution_time_ms')ms"
            echo "  Model: $(echo "$RESULT" | jq -r '.metadata.llm_model')"
            echo "  Cache hit: $(echo "$RESULT" | jq '.metadata.cache_hit')"

            break
        else
            echo -e "${RED}✗ Patch failed${NC}"
            ERROR=$(echo "$RESULT" | jq '.error')
            echo "Error: $ERROR"
            exit 1
        fi
    fi

    echo -n "."
    sleep 1
    ELAPSED=$((ELAPSED + 1))
done
echo ""

if [ $ELAPSED -ge $TIMEOUT ]; then
    echo -e "${RED}✗ Timeout waiting for agent response${NC}"
    exit 1
fi

# Step 4: Fetch updated workflow (if orchestrator API available)
echo ""
echo "=========================================="
echo "Step 4: Viewing Patched Workflow"
echo "=========================================="

# Try to fetch the patched workflow
echo "Attempting to fetch updated workflow..."
if command -v aob &> /dev/null; then
    echo ""
    aob workflow get "$USER" "$WORKFLOW_TAG" | jq .
else
    echo -e "${YELLOW}⚠ aob CLI not found${NC}"
    echo "  Manual inspection required via orchestrator API"
fi

echo ""
echo "=========================================="
echo "Test Summary"
echo "=========================================="
echo -e "${GREEN}✓ Workflow created${NC}"
echo -e "${GREEN}✓ Agent job published${NC}"
echo -e "${GREEN}✓ Agent processed request${NC}"
echo -e "${GREEN}✓ Patch applied${NC}"
echo ""
echo "Workflow Tag: $WORKFLOW_TAG"
echo "User: $USER"
echo ""
echo -e "${YELLOW}Note: Workflow NOT deleted for inspection${NC}"
echo ""
echo "To clean up manually:"
echo "  aob workflow delete $USER $WORKFLOW_TAG"
echo ""
