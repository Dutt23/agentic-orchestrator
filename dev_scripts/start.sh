#!/bin/bash
# Orchestrator Platform - Master Start Script

set -e

echo "🚀 Starting Orchestrator Platform..."
echo ""

# Change to script directory
cd "$(dirname "$0")"

# Load environment variables if .env exists
if [ -f .env ]; then
    echo "📝 Loading environment from .env..."
    export $(cat .env | grep -v '^#' | xargs)
else
    echo "⚠️  No .env file found. Using defaults..."
    echo "   Create .env from .env.example if needed."
fi

# Create necessary directories
mkdir -p logs pids

# Check if supervisord is installed
if ! command -v supervisord &> /dev/null; then
    echo "❌ supervisord not found!"
    echo ""
    echo "Install with:"
    echo "  macOS:   brew install supervisor"
    echo "  Ubuntu:  sudo apt-get install supervisor"
    echo "  Python:  pip install supervisor"
    echo ""
    exit 1
fi

# Check if already running
if [ -f pids/supervisor.sock ]; then
    if supervisorctl -c supervisord.conf status &> /dev/null; then
        echo "⚠️  Services already running!"
        echo ""
        echo "Current status:"
        supervisorctl -c supervisord.conf status
        echo ""
        echo "Use ./stop.sh to stop or ./restart.sh to restart"
        exit 0
    else
        # Stale socket file
        rm -f pids/supervisor.sock
    fi
fi

# Start supervisord
echo "🔧 Starting service manager..."
supervisord -c supervisord.conf

# Wait a moment for services to start
sleep 2

# Show status
echo ""
echo "✅ Services started!"
echo ""
supervisorctl -c supervisord.conf status

echo ""
echo "📊 Service URLs:"
echo "  • Orchestrator:  http://localhost:8081"
echo "  • Fanout (WS):   ws://localhost:8084"
echo "  • Frontend:      http://localhost:5173"
echo ""
echo "💡 Commands:"
echo "  • View status:   ./status.sh"
echo "  • View logs:     tail -f logs/<service>.log"
echo "  • Stop all:      ./stop.sh"
echo "  • Restart all:   ./restart.sh"
echo ""
