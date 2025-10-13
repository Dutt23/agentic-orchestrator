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

# Build the service before running
echo "[${SERVICE_NAME}] Building..."
cd "${PROJECT_ROOT}"
mkdir -p bin
go build -o "bin/${SERVICE_NAME}" "./cmd/${SERVICE_NAME}" || {
    echo "[${SERVICE_NAME}] Build failed!"
    exit 1
}
echo "[${SERVICE_NAME}] Build complete"

# Run the service
exec "./bin/${SERVICE_NAME}" "$@"
