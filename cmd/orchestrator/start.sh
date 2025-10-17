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

# Start mover if USE_MOVER is enabled
if [ "${USE_MOVER:-false}" = "true" ]; then
    echo "[${SERVICE_NAME}] Starting mover..."
    "${PROJECT_ROOT}/scripts/start-mover.sh" "${SERVICE_NAME}"
    sleep 1  # Give mover time to initialize
fi

# Service-specific configuration
export SERVICE_NAME="${SERVICE_NAME}"
export PORT="${ORCHESTRATOR_PORT:-8081}"
export LOG_LEVEL="${LOG_LEVEL:-info}"
export LOG_FORMAT="${LOG_FORMAT:-text}"

# Performance tuning
export GOMAXPROCS="${GOMAXPROCS:-8}"
export GOGC="${GOGC:-100}"
export GOMEMLIMIT="${GOMEMLIMIT:-2GiB}"

echo "[${SERVICE_NAME}] Starting on port ${PORT}..."
echo "[${SERVICE_NAME}] Environment: ${ENVIRONMENT:-development}"
echo "[${SERVICE_NAME}] GOMAXPROCS: ${GOMAXPROCS}"

# Always rebuild to ensure latest changes
echo "[${SERVICE_NAME}] Building..."
cd "${PROJECT_ROOT}"
go build -o "bin/${SERVICE_NAME}" "./cmd/${SERVICE_NAME}"

# Run the service
cd "${PROJECT_ROOT}"
exec "./bin/${SERVICE_NAME}" "$@"
