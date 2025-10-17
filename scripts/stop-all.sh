#!/bin/bash
# Stop all orchestrator services

set -e

echo "=========================================="
echo "Stopping Orchestrator Services"
echo "=========================================="

# Stop movers first (if running)
echo "Stopping movers..."
./scripts/stop-mover.sh all 2>/dev/null || true

# Stop services
for service in orchestrator workflow-runner http-worker hitl-worker; do
    pid_file="/tmp/${service}.pid"
    if [ -f "$pid_file" ]; then
        pid=$(cat "$pid_file")
        echo "Stopping ${service} (PID: ${pid})..."
        kill "$pid" 2>/dev/null || echo "  Already stopped"
        rm -f "$pid_file"
    fi
done

# Clean up log files (optional)
# rm -f /tmp/*.log

echo ""
echo "✅ All services stopped"
