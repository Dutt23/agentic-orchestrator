#!/bin/bash
# Orchestrator Platform - Status Script

# Change to script directory
cd "$(dirname "$0")"

echo "📊 Orchestrator Platform Status"
echo "================================"
echo ""

# Check if supervisor is running
if [ ! -f pids/supervisor.sock ]; then
    echo "❌ Services are not running"
    echo ""
    echo "Start with: ./start.sh"
    exit 1
fi

if ! supervisorctl -c supervisord.conf status &> /dev/null; then
    echo "❌ Service manager not responding"
    exit 1
fi

# Show status
supervisorctl -c supervisord.conf status

echo ""
echo "🌐 Service URLs:"
echo "  • Orchestrator API:  http://localhost:8081"
echo "  • Fanout WebSocket:  ws://localhost:8084"
echo "  • Frontend:          http://localhost:5173"
echo ""
echo "📝 View logs:"
echo "  • All:            tail -f logs/*.log"
echo "  • Orchestrator:   tail -f logs/orchestrator.log"
echo "  • Workflow:       tail -f logs/workflow-runner.log"
echo "  • Fanout:         tail -f logs/fanout.log"
echo "  • Frontend:       tail -f logs/frontend.log"
