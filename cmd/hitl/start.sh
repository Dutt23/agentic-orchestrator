#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="hitl"
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# Load common environment
if [ -f "${PROJECT_ROOT}/.env" ]; then
    set -a
    source "${PROJECT_ROOT}/.env"
    set +a
fi

# Service-specific configuration
export SERVICE_NAME="${SERVICE_NAME}"
export PORT="${HITL_PORT:-8084}"
export LOG_LEVEL="${LOG_LEVEL:-info}"

# HITL-specific settings
export APPROVAL_TIMEOUT="${APPROVAL_TIMEOUT:-3600}"  # 1 hour default
export ENABLE_NOTIFICATIONS="${ENABLE_NOTIFICATIONS:-false}"

echo "[${SERVICE_NAME}] Starting on port ${PORT}..."
echo "[${SERVICE_NAME}] Approval timeout: ${APPROVAL_TIMEOUT}s"

# Build if needed
if [ ! -f "${PROJECT_ROOT}/bin/${SERVICE_NAME}" ]; then
    echo "[${SERVICE_NAME}] Building..."
    cd "${PROJECT_ROOT}"
    go build -o "bin/${SERVICE_NAME}" "./cmd/${SERVICE_NAME}"
fi

# Run the service
cd "${PROJECT_ROOT}"
exec "./bin/${SERVICE_NAME}" "$@"
