#!/bin/bash

# Integration Test Runner for Workflow-Runner
# Runs all integration tests with proper Redis setup verification

set -e

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  Workflow-Runner Integration Tests${NC}"
echo -e "${BLUE}========================================${NC}\n"

# Check Redis
echo -e "${YELLOW}Checking Redis availability...${NC}"
if ! redis-cli -h localhost -p 6379 ping > /dev/null 2>&1; then
    echo -e "${RED}ERROR: Redis not running on localhost:6379${NC}"
    echo -e "${YELLOW}Please start Redis:${NC}"
    echo -e "  ${BLUE}docker run -d -p 6379:6379 redis:7-alpine${NC}"
    echo -e "  ${BLUE}# OR${NC}"
    echo -e "  ${BLUE}redis-server${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Redis is running${NC}\n"

# Clean test database
echo -e "${YELLOW}Cleaning test database (DB 15)...${NC}"
redis-cli -n 15 FLUSHDB > /dev/null
echo -e "${GREEN}✓ Test database cleaned${NC}\n"

# Run tests
echo -e "${YELLOW}Running integration tests...${NC}\n"

if [ "$1" == "verbose" ]; then
    go test -v -timeout 60s
elif [ "$1" == "patch" ]; then
    echo -e "${BLUE}Running patch tests only (most complex)...${NC}\n"
    go test -v -run "Patch"
elif [ "$1" == "quick" ]; then
    go test -timeout 30s
else
    go test -v -timeout 60s
fi

# Check result
if [ $? -eq 0 ]; then
    echo -e "\n${GREEN}========================================${NC}"
    echo -e "${GREEN}  All Tests PASSED! ✓${NC}"
    echo -e "${GREEN}========================================${NC}\n"
else
    echo -e "\n${RED}========================================${NC}"
    echo -e "${RED}  Tests FAILED ✗${NC}"
    echo -e "${RED}========================================${NC}\n"
    exit 1
fi

# Show coverage
echo -e "${YELLOW}Running with coverage...${NC}"
go test -cover -coverprofile=coverage.out -timeout 60s > /dev/null 2>&1
if [ -f coverage.out ]; then
    COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}')
    echo -e "${BLUE}Coverage: ${GREEN}${COVERAGE}${NC}\n"

    # Generate HTML report
    go tool cover -html=coverage.out -o coverage.html
    echo -e "${BLUE}Coverage report: ${GREEN}coverage.html${NC}\n"
fi

echo -e "${YELLOW}Test Summary:${NC}"
echo -e "- Sequential flow execution"
echo -e "- Parallel fan-out"
echo -e "- Branch with CEL conditions"
echo -e "- Loop with CEL conditions"
echo -e "- ${GREEN}Runtime patching (MVP key feature)${NC}"
echo -e "- Agent mock flow"
echo -e "- Complex multi-node patches"
echo -e "- Conditional patches"
