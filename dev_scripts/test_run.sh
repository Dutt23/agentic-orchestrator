#!/bin/bash
# Test script to trigger workflow run via API

# First, create a simple HTTP workflow and store it
WORKFLOW_DATA='{
  "workflow_tag": "test",
  "workflow": {
    "nodes": [
      {
        "id": "http_test",
        "type": "http",
        "config": {
          "url": "https://httpbin.org/get",
          "method": "GET"
        }
      }
    ],
    "edges": []
  },
  "inputs": {
    "test_param": "hello"
  }
}'

echo "Testing POST /api/v1/runs endpoint..."
echo ""

# Send POST request to execute run
curl -X POST \
  http://localhost:8081/api/v1/runs \
  -H "Content-Type: application/json" \
  -H "X-User-ID: test-user" \
  -d "$WORKFLOW_DATA" \
  | jq .

echo ""
echo "Check Redis streams for emitted token:"
echo "redis-cli XREAD COUNT 10 STREAMS wf.tasks.http 0"
