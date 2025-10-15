#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="api"
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# Load common environment
if [ -f "${PROJECT_ROOT}/.env" ]; then
    set -a
    source "${PROJECT_ROOT}/.env"
    set +a
fi

# Service-specific configuration
export SERVICE_NAME="${SERVICE_NAME}"
export PORT="${API_PORT:-8081}"
export LOG_LEVEL="${LOG_LEVEL:-info}"

# API gateway settings
export ROUTER_URL="${ROUTER_URL:-http://localhost:8081}"
export ORCHESTRATOR_URL="${ORCHESTRATOR_URL:-http://localhost:8080}"
export ENABLE_CORS="${ENABLE_CORS:-true}"
export RATE_LIMIT="${RATE_LIMIT:-1000}"  # requests per minute

echo "[${SERVICE_NAME}] Starting on port ${PORT}..."
echo "[${SERVICE_NAME}] Rate limit: ${RATE_LIMIT} req/min"

# Build if needed
if [ ! -f "${PROJECT_ROOT}/bin/${SERVICE_NAME}" ]; then
    echo "[${SERVICE_NAME}] Building..."
    cd "${PROJECT_ROOT}"
    go build -o "bin/${SERVICE_NAME}" "./cmd/${SERVICE_NAME}"
fi

# Run the service
cd "${PROJECT_ROOT}"
exec "./bin/${SERVICE_NAME}" "$@"
