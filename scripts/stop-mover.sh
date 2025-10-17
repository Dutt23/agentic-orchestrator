#!/bin/bash
# Stop mover service
# Usage: ./stop-mover.sh orchestrator
# Usage: ./stop-mover.sh all

set -e

if [ -z "$1" ]; then
    echo "Usage: $0 <service-name|all>"
    echo "Example: $0 orchestrator"
    echo "Example: $0 all"
    exit 1
fi

stop_mover() {
    local service=$1
    local pid_file="/tmp/mover-${service}.pid"

    if [ -f "$pid_file" ]; then
        local pid=$(cat "$pid_file")
        echo "Stopping mover for ${service} (PID: ${pid})..."
        kill "$pid" 2>/dev/null || echo "  Already stopped"
        rm -f "$pid_file"
        rm -f "/tmp/mover-${service}.sock"
        echo "  Mover stopped"
    else
        echo "Mover for ${service} not running (no PID file)"
    fi
}

if [ "$1" = "all" ]; then
    echo "Stopping all movers..."
    for pid_file in /tmp/mover-*.pid; do
        if [ -f "$pid_file" ]; then
            service=$(basename "$pid_file" .pid | sed 's/mover-//')
            stop_mover "$service"
        fi
    done
else
    stop_mover "$1"
fi
