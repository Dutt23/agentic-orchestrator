#!/bin/bash

# Setup and run Redis server locally
# This script downloads, compiles, and runs Redis in the local project

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
REDIS_DIR="$PROJECT_ROOT/.redis"
REDIS_VERSION="7.2.4"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo "=========================================="
echo "Redis Setup Script"
echo "=========================================="
echo ""

# Check if Redis is already running
check_redis_running() {
    if redis-cli ping 2>/dev/null | grep -q PONG; then
        echo -e "${GREEN}✓ Redis is already running${NC}"
        echo ""
        redis-cli INFO server | grep redis_version
        echo ""
        echo "Redis is ready to use!"
        echo ""
        echo "To stop: redis-cli shutdown"
        exit 0
    fi
}

# Check if Redis is installed globally
check_global_redis() {
    if command -v redis-server &> /dev/null; then
        echo -e "${GREEN}✓ Redis is installed globally${NC}"
        echo ""
        echo "Starting Redis server..."
        redis-server --daemonize yes
        sleep 1

        if redis-cli ping 2>/dev/null | grep -q PONG; then
            echo -e "${GREEN}✓ Redis started successfully${NC}"
            redis-cli INFO server | grep redis_version
            echo ""
            echo "To stop: redis-cli shutdown"
            exit 0
        fi
    fi
}

# Download and build Redis locally
install_redis_local() {
    echo "Redis not found. Installing locally..."
    echo ""

    # Create directory
    mkdir -p "$REDIS_DIR"
    cd "$REDIS_DIR"

    # Check if already downloaded
    if [ -d "redis-$REDIS_VERSION" ]; then
        echo "Redis $REDIS_VERSION already downloaded"
    else
        echo "Downloading Redis $REDIS_VERSION..."
        curl -L "https://download.redis.io/releases/redis-$REDIS_VERSION.tar.gz" -o redis.tar.gz

        echo "Extracting..."
        tar xzf redis.tar.gz
        rm redis.tar.gz
    fi

    cd "redis-$REDIS_VERSION"

    # Check if already compiled
    if [ -f "src/redis-server" ]; then
        echo -e "${GREEN}✓ Redis already compiled${NC}"
    else
        echo "Compiling Redis (this may take a minute)..."
        make -j$(sysctl -n hw.ncpu 2>/dev/null || nproc 2>/dev/null || echo 2)
        echo -e "${GREEN}✓ Redis compiled successfully${NC}"
    fi

    echo ""
}

# Start Redis server
start_redis() {
    REDIS_SERVER="$REDIS_DIR/redis-$REDIS_VERSION/src/redis-server"
    REDIS_CLI="$REDIS_DIR/redis-$REDIS_VERSION/src/redis-cli"

    if [ ! -f "$REDIS_SERVER" ]; then
        echo -e "${RED}✗ Redis server binary not found${NC}"
        exit 1
    fi

    # Create config file
    REDIS_CONF="$REDIS_DIR/redis.conf"
    cat > "$REDIS_CONF" <<EOF
# Redis configuration for local development
port 6379
bind 127.0.0.1
daemonize yes
pidfile $REDIS_DIR/redis.pid
logfile $REDIS_DIR/redis.log
dir $REDIS_DIR
dbfilename dump.rdb
appendonly yes
appendfilename "appendonly.aof"
EOF

    echo "Starting Redis server..."
    "$REDIS_SERVER" "$REDIS_CONF"

    sleep 1

    # Verify Redis started
    if "$REDIS_CLI" ping 2>/dev/null | grep -q PONG; then
        echo -e "${GREEN}✓ Redis started successfully${NC}"
        echo ""
        "$REDIS_CLI" INFO server | grep redis_version
        echo ""
        echo "Redis is ready to use!"
        echo ""
        echo "Connection info:"
        echo "  Host: localhost"
        echo "  Port: 6379"
        echo ""
        echo "Useful commands:"
        echo "  Check status: $REDIS_CLI ping"
        echo "  Stop Redis:   $REDIS_CLI shutdown"
        echo "  View logs:    tail -f $REDIS_DIR/redis.log"
        echo "  CLI:          $REDIS_CLI"
        echo ""
    else
        echo -e "${RED}✗ Failed to start Redis${NC}"
        echo "Check logs: $REDIS_DIR/redis.log"
        exit 1
    fi
}

# Main execution
echo "Checking Redis installation..."
echo ""

# Step 1: Check if already running
check_redis_running

# Step 2: Check if installed globally
check_global_redis

# Step 3: Install locally
install_redis_local

# Step 4: Start Redis
start_redis

echo "Setup complete!"
echo ""
