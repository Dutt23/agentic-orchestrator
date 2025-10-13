#!/bin/bash

# Simplified test: Just test agent patching without full orchestrator
# Tests: Intent classification → LLM → Patch generation

set -e

echo "=========================================="
echo "Agent Patch Flow Test (Standalone)"
echo "=========================================="
echo ""

# Check dependencies
echo "Checking dependencies..."
python3 -c "import openai, redis, yaml" 2>/dev/null || {
    echo "Missing dependencies. Installing..."
    pip3 install openai redis pyyaml python-dotenv
}
echo "✓ Dependencies OK"
echo ""

# Check OpenAI API key
if [ ! -f .env ]; then
    echo "✗ .env file not found"
    echo "  Create .env with: OPENAI_API_KEY=sk-..."
    exit 1
fi

# Check Redis
if ! redis-cli ping > /dev/null 2>&1; then
    echo "✗ Redis not running"
    echo "  Start with: redis-server"
    exit 1
fi
echo "✓ Redis running"
echo ""

# Generate job ID
JOB_ID=$(python3 -c "import uuid; print(uuid.uuid4())")

# Test workflow
WORKFLOW='{
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
  "edges": [
    {"from": "fetch_flights", "to": "process_results"}
  ]
}'

echo "Initial Workflow:"
echo "$WORKFLOW" | jq -C .
echo ""

# Create agent job
echo "Creating patch job..."
echo "Prompt: 'add email notification after process_results when price < 500'"
echo ""

JOB=$(cat <<EOF
{
  "version": "1.0",
  "job_id": "$JOB_ID",
  "run_id": "test-run-001",
  "node_id": "agent_test",
  "workflow_tag": "test-workflow",
  "workflow_owner": "test-user",
  "prompt": "add email notification node after process_results that sends alert when price drops below 500",
  "current_workflow": $WORKFLOW,
  "context": {
    "session_id": "test-session-$JOB_ID"
  },
  "timeout_sec": 60,
  "retry_count": 0,
  "created_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF
)

# Start agent service if not running
if ! curl -s http://localhost:8082/health > /dev/null 2>&1; then
    echo "Starting agent service..."
    python3 main.py &
    AGENT_PID=$!
    sleep 3
    echo "✓ Agent service started (PID: $AGENT_PID)"
else
    echo "✓ Agent service already running"
    AGENT_PID=""
fi
echo ""

# Publish job
echo "Publishing job to Redis..."
echo "$JOB" | redis-cli -x RPUSH agent:jobs > /dev/null
echo "✓ Job published (ID: $JOB_ID)"
echo ""

# Wait for result
echo "Waiting for agent response (max 30s)..."
TIMEOUT=30
ELAPSED=0

while [ $ELAPSED -lt $TIMEOUT ]; do
    RESULT=$(redis-cli BLPOP "agent:results:$JOB_ID" 1 2>/dev/null | tail -n 1)

    if [ -n "$RESULT" ] && [ "$RESULT" != "(nil)" ]; then
        echo ""
        echo "=========================================="
        echo "Agent Response Received!"
        echo "=========================================="
        echo ""

        # Pretty print result
        echo "$RESULT" | jq -C .
        echo ""

        STATUS=$(echo "$RESULT" | jq -r '.status')
        if [ "$STATUS" == "completed" ]; then
            echo "✓ Status: COMPLETED"
            echo ""

            # Extract patch operations
            echo "Patch Operations Generated:"
            echo "$RESULT" | jq -C '.metadata.tool_calls[0].function.arguments' | jq -r . | jq -C '.patch_spec.operations'
            echo ""

            echo "Metadata:"
            echo "  Tokens: $(echo "$RESULT" | jq '.metadata.tokens_used')"
            echo "  Time: $(echo "$RESULT" | jq '.metadata.execution_time_ms')ms"
            echo "  Model: $(echo "$RESULT" | jq -r '.metadata.llm_model')"
            echo ""

            echo "=========================================="
            echo "✓ Test PASSED"
            echo "=========================================="

            # Cleanup
            if [ -n "$AGENT_PID" ]; then
                echo ""
                echo "Stopping agent service..."
                kill $AGENT_PID 2>/dev/null || true
            fi

            exit 0
        else
            echo "✗ Status: FAILED"
            echo "$RESULT" | jq '.error'
            exit 1
        fi
    fi

    echo -n "."
    sleep 1
    ELAPSED=$((ELAPSED + 1))
done

echo ""
echo "✗ Timeout waiting for response"

# Cleanup
if [ -n "$AGENT_PID" ]; then
    kill $AGENT_PID 2>/dev/null || true
fi

exit 1
