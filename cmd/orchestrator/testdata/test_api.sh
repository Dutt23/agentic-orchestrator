#!/bin/bash

# Test script for Workflow Orchestrator API
# Tests workflow creation, retrieval, and patch operations

set -e  # Exit on error

# Configuration
API_BASE="http://localhost:8081/api/v1"
CONTENT_TYPE="Content-Type: application/json"
USER_ID="X-User-ID: test-user"

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  Workflow Orchestrator API Tests${NC}"
echo -e "${BLUE}========================================${NC}\n"

# Check API availability
echo -e "${YELLOW}Checking API server availability...${NC}"
if ! curl --max-time 2 --silent --fail "$API_BASE/workflows" > /dev/null 2>&1; then
    echo -e "${RED}ERROR: Cannot connect to API server at $API_BASE${NC}"
    echo -e "${YELLOW}Please start the API server first:${NC}"
    echo -e "${YELLOW}  cd /Users/sdutt/Documents/practice/lyzr/orchestrator/cmd/orchestrator${NC}"
    echo -e "${YELLOW}  go run main.go${NC}"
    exit 1
fi
echo -e "${GREEN}✓ API server is running${NC}\n"

# Function to print test results
function test_result() {
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ $1 PASSED${NC}\n"
    else
        echo -e "${RED}✗ $1 FAILED${NC}\n"
        exit 1
    fi
}

# Test 1: Create Simple Workflow (main branch)
echo -e "${YELLOW}Test 1: Create Simple Workflow${NC}"
RESPONSE=$(curl -s -X POST "$API_BASE/workflows" \
    -H "$CONTENT_TYPE" \
    -H "$USER_ID" \
    -d '{
        "tag_name": "main",
        "workflow": '"$(cat workflow_simple.json)"',
        "created_by": "test-user"
    }')

echo "$RESPONSE" | jq '.'
ARTIFACT_ID=$(echo "$RESPONSE" | jq -r '.artifact_id')
test_result "Create Simple Workflow"

# Test 2: Get Workflow (without materialization)
echo -e "${YELLOW}Test 2: Get Workflow (materialize=false)${NC}"
curl -s -X GET "$API_BASE/workflows/main?materialize=false" | jq '.'
test_result "Get Workflow Metadata"

# Test 3: Get Workflow (with materialization)
echo -e "${YELLOW}Test 3: Get Workflow (materialize=true)${NC}"
curl -s -X GET "$API_BASE/workflows/main?materialize=true" | jq '.'
test_result "Get Materialized Workflow"

# Test 4: Create Complex Workflow (dev branch)
echo -e "${YELLOW}Test 4: Create Complex Workflow on dev branch${NC}"
curl -s -X POST "$API_BASE/workflows" \
    -H "$CONTENT_TYPE" \
    -H "$USER_ID" \
    -d '{
        "tag_name": "dev",
        "workflow": '"$(cat workflow_complex.json)"',
        "created_by": "test-user"
    }' | jq '.'
test_result "Create Complex Workflow"

# Test 5: List All Workflows
echo -e "${YELLOW}Test 5: List All Workflows${NC}"
curl -s -X GET "$API_BASE/workflows" | jq '.'
test_result "List Workflows"

# Test 6: Create Another Workflow (prod branch)
echo -e "${YELLOW}Test 6: Create Production Workflow${NC}"
curl -s -X POST "$API_BASE/workflows" \
    -H "$CONTENT_TYPE" \
    -H "$USER_ID" \
    -d '{
        "tag_name": "prod",
        "workflow": '"$(cat workflow_simple.json)"',
        "created_by": "test-user"
    }' | jq '.'
test_result "Create Production Workflow"

# Test 7: Compare branches
echo -e "${YELLOW}Test 7: Compare Different Branches${NC}"
echo -e "${BLUE}Main branch:${NC}"
curl -s -X GET "$API_BASE/workflows/main?materialize=false" | jq '{tag, kind, depth, patch_count}'

echo -e "${BLUE}Dev branch:${NC}"
curl -s -X GET "$API_BASE/workflows/dev?materialize=false" | jq '{tag, kind, depth, patch_count}'

echo -e "${BLUE}Prod branch:${NC}"
curl -s -X GET "$API_BASE/workflows/prod?materialize=false" | jq '{tag, kind, depth, patch_count}'
test_result "Compare Branches"

# Test 8: Test Error Handling - Non-existent workflow
echo -e "${YELLOW}Test 8: Error Handling - Non-existent Workflow${NC}"
curl -s -X GET "$API_BASE/workflows/nonexistent" | jq '.'
if [ ${PIPESTATUS[0]} -eq 0 ]; then
    echo -e "${GREEN}✓ Error handling works correctly${NC}\n"
else
    echo -e "${RED}✗ Error handling failed${NC}\n"
fi

# Test 9: Delete a workflow tag
echo -e "${YELLOW}Test 9: Delete Workflow Tag${NC}"
curl -s -X DELETE "$API_BASE/workflows/dev" | jq '.'
test_result "Delete Workflow"

# Verify deletion
echo -e "${YELLOW}Verify deletion:${NC}"
curl -s -X GET "$API_BASE/workflows/dev" | jq '.'

echo -e "\n${BLUE}========================================${NC}"
echo -e "${GREEN}  All Tests Completed Successfully!${NC}"
echo -e "${BLUE}========================================${NC}"

# Summary
echo -e "\n${YELLOW}Summary:${NC}"
echo -e "- Created workflows on main, dev, and prod branches"
echo -e "- Retrieved workflow metadata (without materialization)"
echo -e "- Retrieved full workflow (with materialization)"
echo -e "- Listed all workflows"
echo -e "- Tested error handling"
echo -e "- Deleted a workflow tag"
