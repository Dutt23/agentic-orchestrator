#!/bin/bash

# Fanout Service Startup Script

set -e

# Configuration
export REDIS_HOST="${REDIS_HOST:-localhost}"
export REDIS_PORT="${REDIS_PORT:-6379}"
export PORT="${PORT:-8084}"

echo "Starting Fanout Service..."
echo "Redis: $REDIS_HOST:$REDIS_PORT"
echo "Port: $PORT"

# Build if binary doesn't exist
if [ ! -f "fanout" ]; then
    echo "Building fanout service..."
    go build -o fanout .
fi

# Run the service
exec ./fanout
