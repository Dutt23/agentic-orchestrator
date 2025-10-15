#!/bin/bash

# Fanout Service Startup Script

set -e

# Change to script directory
cd "$(dirname "$0")"

# Configuration
export REDIS_HOST="${REDIS_HOST:-localhost}"
export REDIS_PORT="${REDIS_PORT:-6379}"
export PORT="${PORT:-8084}"

echo "Starting Fanout Service..."
echo "Redis: $REDIS_HOST:$REDIS_PORT"
echo "Port: $PORT"

# Build the service every time
echo "Building fanout service..."
go build -o fanout .

# Run the service
exec ./fanout
