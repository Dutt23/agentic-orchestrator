#!/bin/bash

# Stop Redis server

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
REDIS_DIR="$PROJECT_ROOT/.redis"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo "Stopping Redis..."
echo ""

# Try redis-cli shutdown (works for both global and local)
if command -v redis-cli &> /dev/null; then
    if redis-cli ping 2>/dev/null | grep -q PONG; then
        redis-cli shutdown
        sleep 1
        echo -e "${GREEN}✓ Redis stopped${NC}"
    else
        echo -e "${YELLOW}⚠ Redis is not running${NC}"
    fi
    exit 0
fi

# Try local Redis CLI
REDIS_CLI="$REDIS_DIR/redis-7.2.4/src/redis-cli"
if [ -f "$REDIS_CLI" ]; then
    if "$REDIS_CLI" ping 2>/dev/null | grep -q PONG; then
        "$REDIS_CLI" shutdown
        sleep 1
        echo -e "${GREEN}✓ Redis stopped${NC}"
    else
        echo -e "${YELLOW}⚠ Redis is not running${NC}"
    fi
    exit 0
fi

echo -e "${YELLOW}⚠ Redis CLI not found${NC}"
echo "Redis may not be installed or is already stopped"
