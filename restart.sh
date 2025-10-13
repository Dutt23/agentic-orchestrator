#!/bin/bash
# Orchestrator Platform - Restart Script

echo "🔄 Restarting Orchestrator Platform..."
echo ""

# Change to script directory
cd "$(dirname "$0")"

# Stop services
./stop.sh

# Wait a moment
sleep 2

# Start services
./start.sh
