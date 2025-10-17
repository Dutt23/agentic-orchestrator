#!/bin/bash
# Start all orchestrator services (bare metal)
# Optionally starts movers if USE_MOVER=true

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "=========================================="
echo "Starting Orchestrator Services"
echo "=========================================="

# Load environment
if [ -f "${PROJECT_ROOT}/.env" ]; then
    set -a
    source "${PROJECT_ROOT}/.env"
    set +a
fi

echo "Configuration:"
echo "  USE_MOVER: ${USE_MOVER:-false}"
echo "  LOG_LEVEL: ${LOG_LEVEL:-info}"
echo ""

# Start infrastructure (must be running)
echo "Checking infrastructure..."
if ! pg_isready -h localhost -U orchestrator > /dev/null 2>&1; then
    echo "⚠️  PostgreSQL not running. Start it first:"
    echo "   brew services start postgresql"
    exit 1
fi

if ! redis-cli ping > /dev/null 2>&1; then
    echo "⚠️  Redis not running. Start it first:"
    echo "   brew services start redis"
    exit 1
fi

echo "✅ Infrastructure ready"
echo ""

# Start services with movers
echo "Starting services..."
echo ""

# Orchestrator
echo "[1/4] Starting orchestrator..."
"${PROJECT_ROOT}/cmd/orchestrator/start.sh" > /tmp/orchestrator.log 2>&1 &
echo $! > /tmp/orchestrator.pid
sleep 2

# Workflow Runner
echo "[2/4] Starting workflow-runner..."
"${PROJECT_ROOT}/cmd/workflow-runner/start.sh" > /tmp/workflow-runner.log 2>&1 &
echo $! > /tmp/workflow-runner.pid
sleep 2

# HTTP Worker
echo "[3/4] Starting http-worker..."
if [ -f "${PROJECT_ROOT}/cmd/http-worker/start.sh" ]; then
    "${PROJECT_ROOT}/cmd/http-worker/start.sh" > /tmp/http-worker.log 2>&1 &
    echo $! > /tmp/http-worker.pid
else
    echo "  (Skipped - no start.sh)"
fi
sleep 1

# HITL Worker
echo "[4/4] Starting hitl-worker..."
if [ -f "${PROJECT_ROOT}/cmd/hitl-worker/start.sh" ]; then
    "${PROJECT_ROOT}/cmd/hitl-worker/start.sh" > /tmp/hitl-worker.log 2>&1 &
    echo $! > /tmp/hitl-worker.pid
else
    echo "  (Skipped - no start.sh)"
fi
sleep 1

echo ""
echo "=========================================="
echo "✅ All Services Started"
echo "=========================================="
echo ""
echo "Process IDs:"
[ -f /tmp/orchestrator.pid ] && echo "  orchestrator:    $(cat /tmp/orchestrator.pid)"
[ -f /tmp/workflow-runner.pid ] && echo "  workflow-runner: $(cat /tmp/workflow-runner.pid)"
[ -f /tmp/http-worker.pid ] && echo "  http-worker:     $(cat /tmp/http-worker.pid)"
[ -f /tmp/hitl-worker.pid ] && echo "  hitl-worker:     $(cat /tmp/hitl-worker.pid)"
echo ""

if [ "${USE_MOVER:-false}" = "true" ]; then
    echo "Movers:"
    [ -f /tmp/mover-orchestrator.pid ] && echo "  mover-orchestrator:    $(cat /tmp/mover-orchestrator.pid)"
    [ -f /tmp/mover-workflow-runner.pid ] && echo "  mover-workflow-runner: $(cat /tmp/mover-workflow-runner.pid)"
    echo ""
fi

echo "Logs:"
echo "  tail -f /tmp/orchestrator.log"
echo "  tail -f /tmp/workflow-runner.log"
if [ "${USE_MOVER:-false}" = "true" ]; then
    echo "  tail -f /tmp/mover-orchestrator.log"
fi
echo ""

echo "To stop:"
echo "  ./scripts/stop-all.sh"
echo ""
