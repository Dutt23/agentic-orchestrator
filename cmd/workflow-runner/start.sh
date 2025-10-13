#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="workflow-runner"
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SERVICE_DIR="${PROJECT_ROOT}/cmd/${SERVICE_NAME}"

# Load common environment
if [ -f "${PROJECT_ROOT}/.env" ]; then
    set -a
    source "${PROJECT_ROOT}/.env"
    set +a
fi

# Service-specific configuration
export SERVICE_NAME="${SERVICE_NAME}"
export ORCHESTRATOR_URL="${ORCHESTRATOR_URL:-http://localhost:8081}"
export LOG_LEVEL="${LOG_LEVEL:-info}"
export LOG_FORMAT="${LOG_FORMAT:-json}"

# Performance tuning
export GOMAXPROCS="${GOMAXPROCS:-8}"

echo "[${SERVICE_NAME}] Starting..."
echo "[${SERVICE_NAME}] Environment: ${ENVIRONMENT:-development}"
echo "[${SERVICE_NAME}] Orchestrator URL: ${ORCHESTRATOR_URL}"

# Run the service
cd "${PROJECT_ROOT}"
exec "./bin/${SERVICE_NAME}" "$@"
