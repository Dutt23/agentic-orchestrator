#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="orchestrator"
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
export PORT="${ORCHESTRATOR_PORT:-8080}"
export LOG_LEVEL="${LOG_LEVEL:-info}"
export LOG_FORMAT="${LOG_FORMAT:-json}"

# Performance tuning
export GOMAXPROCS="${GOMAXPROCS:-8}"
export GOGC="${GOGC:-100}"
export GOMEMLIMIT="${GOMEMLIMIT:-2GiB}"

echo "[${SERVICE_NAME}] Starting on port ${PORT}..."
echo "[${SERVICE_NAME}] Environment: ${ENVIRONMENT:-development}"
echo "[${SERVICE_NAME}] GOMAXPROCS: ${GOMAXPROCS}"

# Build if needed
if [ ! -f "${PROJECT_ROOT}/bin/${SERVICE_NAME}" ]; then
    echo "[${SERVICE_NAME}] Building..."
    cd "${PROJECT_ROOT}"
    go build -o "bin/${SERVICE_NAME}" "./cmd/${SERVICE_NAME}"
fi

# Run the service
cd "${PROJECT_ROOT}"
exec "./bin/${SERVICE_NAME}" "$@"
