#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="api-rust"
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

# Backend service URLs
export ORCHESTRATOR_URL="${ORCHESTRATOR_URL:-http://localhost:8080}"
export RUNNER_URL="${RUNNER_URL:-http://localhost:8082}"
export HITL_URL="${HITL_URL:-http://localhost:8083}"

# Performance settings
export RATE_LIMIT="${RATE_LIMIT:-1000}"  # requests per minute

echo "[${SERVICE_NAME}] Starting API Gateway on port ${PORT}..."
echo "[${SERVICE_NAME}] Orchestrator: ${ORCHESTRATOR_URL}"
echo "[${SERVICE_NAME}] Runner: ${RUNNER_URL}"
echo "[${SERVICE_NAME}] Rate limit: ${RATE_LIMIT} req/min"

# Build if needed (release mode for performance)
if [ ! -f "${PROJECT_ROOT}/bin/api-gateway" ] || [ "${REBUILD:-0}" = "1" ]; then
    echo "[${SERVICE_NAME}] Building (release mode)..."
    cd "${PROJECT_ROOT}/cmd/api-rust"
    cargo build --release
    mkdir -p "${PROJECT_ROOT}/bin"
    cp "${PROJECT_ROOT}/target/release/api-gateway" "${PROJECT_ROOT}/bin/"
fi

# Run the service
cd "${PROJECT_ROOT}"
exec "${PROJECT_ROOT}/bin/api-gateway" "$@"
