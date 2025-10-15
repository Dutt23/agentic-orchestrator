#!/bin/bash
# Orchestrator Platform - Master Stop Script

set -e

echo "🛑 Stopping Orchestrator Platform..."
echo ""

# Change to script directory
cd "$(dirname "$0")"

# Check if supervisor is running
if [ ! -f pids/supervisor.sock ]; then
    echo "⚠️  Services are not running (no supervisor socket found)"
    exit 0
fi

if ! supervisorctl -c supervisord.conf status &> /dev/null; then
    echo "⚠️  Services are not running (supervisor not responding)"
    rm -f pids/supervisor.sock
    exit 0
fi

# Stop all services
echo "📋 Current status:"
supervisorctl -c supervisord.conf status
echo ""

echo "🔧 Stopping all services..."
supervisorctl -c supervisord.conf stop all

# Shutdown supervisor
echo "🔧 Shutting down service manager..."
supervisorctl -c supervisord.conf shutdown

# Clean up
rm -f pids/supervisor.sock pids/supervisord.pid

echo ""
echo "✅ All services stopped!"
