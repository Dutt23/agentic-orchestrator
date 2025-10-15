#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="runner"
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# Load common environment
if [ -f "${PROJECT_ROOT}/.env" ]; then
    set -a
    source "${PROJECT_ROOT}/.env"
    set +a
fi

# Service-specific configuration
export SERVICE_NAME="${SERVICE_NAME}"
export PORT="${RUNNER_PORT:-8082}"
export LOG_LEVEL="${LOG_LEVEL:-info}"
export LOG_FORMAT="${LOG_FORMAT:-json}"

# Runner-specific settings
export RUNNER_WORKERS="${RUNNER_WORKERS:-$GOMAXPROCS}"
export RUNNER_QUEUE_SIZE="${RUNNER_QUEUE_SIZE:-1000}"
export CAS_DIR="${PROJECT_ROOT}/data/cas"

# Performance tuning
export GOMAXPROCS="${GOMAXPROCS:-8}"
export GOGC="${GOGC:-100}"
export GOMEMLIMIT="${GOMEMLIMIT:-2GiB}"

# Ensure CAS directory exists
mkdir -p "${CAS_DIR}"

echo "[${SERVICE_NAME}] Starting on port ${PORT}..."
echo "[${SERVICE_NAME}] Workers: ${RUNNER_WORKERS}"
echo "[${SERVICE_NAME}] CAS directory: ${CAS_DIR}"

# Build if needed
if [ ! -f "${PROJECT_ROOT}/bin/${SERVICE_NAME}" ]; then
    echo "[${SERVICE_NAME}] Building..."
    cd "${PROJECT_ROOT}"
    go build -o "bin/${SERVICE_NAME}" "./cmd/${SERVICE_NAME}"
fi

# Run the service
cd "${PROJECT_ROOT}"
exec "./bin/${SERVICE_NAME}" "$@"
