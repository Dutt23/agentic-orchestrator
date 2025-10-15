#!/bin/bash
# Rate Limiting Test Script
# Tests workflow-aware rate limiting by submitting multiple workflow runs

set -e

ORCHESTRATOR_URL="${ORCHESTRATOR_URL:-http://localhost:8081}"
USERNAME="${USERNAME:-test-user}"

echo "========================================"
echo "Rate Limiting Test"
echo "========================================"
echo "Orchestrator: $ORCHESTRATOR_URL"
echo "Username: $USERNAME"
echo ""

# Color codes
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test 1: Simple workflow (no agents) - should allow 100/min
echo "Test 1: Simple Workflow (no agents)"
echo "Expected: 100 runs/minute allowed"
echo "----------------------------------------"

SIMPLE_WORKFLOW='{
  "nodes": [
    {"id": "http_1", "type": "http", "config": {"url": "https://api.example.com", "method": "GET"}}
  ],
  "edges": []
}'

# Create simple workflow
echo "Creating simple workflow..."
SIMPLE_TAG="test-simple-$(date +%s)"
curl -s -X POST "$ORCHESTRATOR_URL/api/v1/workflows" \
  -H "Content-Type: application/json" \
  -H "X-User-ID: $USERNAME" \
  -d "{\"tag_name\": \"$SIMPLE_TAG\", \"workflow\": $SIMPLE_WORKFLOW}" > /dev/null

echo -e "${GREEN}✓${NC} Simple workflow created: $SIMPLE_TAG"
echo ""

# Submit 5 runs rapidly
echo "Submitting 5 runs rapidly..."
for i in {1..5}; do
  RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$ORCHESTRATOR_URL/api/v1/workflows/$SIMPLE_TAG/execute" \
    -H "Content-Type: application/json" \
    -H "X-User-ID: $USERNAME" \
    -d '{"inputs": {}}')

  HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
  BODY=$(echo "$RESPONSE" | sed '$d')

  if [ "$HTTP_CODE" == "201" ]; then
    RUN_ID=$(echo "$BODY" | jq -r '.run_id')
    echo -e "${GREEN}✓${NC} Run $i: SUCCESS (HTTP $HTTP_CODE) - Run ID: ${RUN_ID:0:8}..."
  else
    echo -e "${RED}✗${NC} Run $i: RATE LIMITED (HTTP $HTTP_CODE)"
    echo "  Response: $BODY"
  fi

  sleep 0.1
done

echo ""
echo "========================================"
echo ""

# Test 2: Agent workflow - should allow only 20/min
echo "Test 2: Standard Workflow (1-2 agents)"
echo "Expected: 20 runs/minute allowed"
echo "----------------------------------------"

AGENT_WORKFLOW='{
  "nodes": [
    {"id": "agent_1", "type": "agent", "config": {"task": "analyze data"}},
    {"id": "http_1", "type": "http", "config": {"url": "https://api.example.com", "method": "POST"}}
  ],
  "edges": [
    {"from": "agent_1", "to": "http_1"}
  ]
}'

# Create agent workflow
echo "Creating agent workflow..."
AGENT_TAG="test-agent-$(date +%s)"
curl -s -X POST "$ORCHESTRATOR_URL/api/v1/workflows" \
  -H "Content-Type: application/json" \
  -H "X-User-ID: $USERNAME" \
  -d "{\"tag_name\": \"$AGENT_TAG\", \"workflow\": $AGENT_WORKFLOW}" > /dev/null

echo -e "${GREEN}✓${NC} Agent workflow created: $AGENT_TAG"
echo ""

# Submit 25 runs to hit the limit (20/min for standard tier)
echo "Submitting 25 runs to hit rate limit (limit: 20/min)..."
SUCCESS_COUNT=0
RATE_LIMITED_COUNT=0

for i in {1..25}; do
  RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$ORCHESTRATOR_URL/api/v1/workflows/$AGENT_TAG/execute" \
    -H "Content-Type: application/json" \
    -H "X-User-ID: $USERNAME" \
    -d '{"inputs": {}}')

  HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
  BODY=$(echo "$RESPONSE" | sed '$d')

  if [ "$HTTP_CODE" == "201" ]; then
    SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
    RUN_ID=$(echo "$BODY" | jq -r '.run_id')
    echo -e "${GREEN}✓${NC} Run $i: SUCCESS (HTTP $HTTP_CODE) - Run ID: ${RUN_ID:0:8}..."
  elif [ "$HTTP_CODE" == "429" ]; then
    RATE_LIMITED_COUNT=$((RATE_LIMITED_COUNT + 1))
    TIER=$(echo "$BODY" | jq -r '.details.tier')
    LIMIT=$(echo "$BODY" | jq -r '.details.limit')
    RETRY_AFTER=$(echo "$BODY" | jq -r '.details.retry_after_seconds')

    if [ "$RATE_LIMITED_COUNT" == "1" ]; then
      # Show detailed info on first rate limit
      echo -e "${YELLOW}⚠${NC} Run $i: RATE LIMITED (HTTP $HTTP_CODE)"
      echo "  Tier: $TIER"
      echo "  Limit: $LIMIT requests/minute"
      echo "  Retry after: $RETRY_AFTER seconds"
      echo "  Message: $(echo "$BODY" | jq -r '.message')"
    else
      echo -e "${YELLOW}⚠${NC} Run $i: RATE LIMITED (HTTP $HTTP_CODE)"
    fi
  else
    echo -e "${RED}✗${NC} Run $i: ERROR (HTTP $HTTP_CODE)"
    echo "  Response: $BODY"
  fi

  sleep 0.05
done

echo ""
echo "========================================"
echo "Summary"
echo "========================================"
echo -e "Successful runs: ${GREEN}$SUCCESS_COUNT${NC}"
echo -e "Rate limited:    ${YELLOW}$RATE_LIMITED_COUNT${NC}"
echo ""

if [ "$SUCCESS_COUNT" -eq 20 ] && [ "$RATE_LIMITED_COUNT" -eq 5 ]; then
  echo -e "${GREEN}✓ Rate limiting working correctly!${NC}"
  echo "  - Allowed 20 runs (standard tier limit)"
  echo "  - Blocked 5 runs (exceeded limit)"
else
  echo -e "${YELLOW}⚠ Unexpected results${NC}"
  echo "  Expected: 20 success, 5 rate limited"
  echo "  Got: $SUCCESS_COUNT success, $RATE_LIMITED_COUNT rate limited"
fi

echo ""
echo "Test complete!"
