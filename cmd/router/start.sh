#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="router"
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# Load common environment
if [ -f "${PROJECT_ROOT}/.env" ]; then
    set -a
    source "${PROJECT_ROOT}/.env"
    set +a
fi

# Service-specific configuration
export SERVICE_NAME="${SERVICE_NAME}"
export PORT="${ROUTER_PORT:-8081}"
export LOG_LEVEL="${LOG_LEVEL:-info}"

# Router-specific settings
export ORCHESTRATOR_URL="${ORCHESTRATOR_URL:-http://localhost:8080}"
export HITL_URL="${HITL_URL:-http://localhost:8084}"
export PARSER_URL="${PARSER_URL:-http://localhost:8085}"
export JWT_SECRET="${JWT_SECRET:-change-me-in-production}"

# Performance tuning
export GOMAXPROCS="${GOMAXPROCS:-8}"

echo "[${SERVICE_NAME}] Starting on port ${PORT}..."
echo "[${SERVICE_NAME}] Orchestrator: ${ORCHESTRATOR_URL}"
echo "[${SERVICE_NAME}] HITL: ${HITL_URL}"
echo "[${SERVICE_NAME}] Parser: ${PARSER_URL}"

# Build if needed
if [ ! -f "${PROJECT_ROOT}/bin/${SERVICE_NAME}" ]; then
    echo "[${SERVICE_NAME}] Building..."
    cd "${PROJECT_ROOT}"
    go build -o "bin/${SERVICE_NAME}" "./cmd/${SERVICE_NAME}"
fi

# Run the service
cd "${PROJECT_ROOT}"
exec "./bin/${SERVICE_NAME}" "$@"
