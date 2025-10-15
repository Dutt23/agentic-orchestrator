#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="parser"
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# Load common environment
if [ -f "${PROJECT_ROOT}/.env" ]; then
    set -a
    source "${PROJECT_ROOT}/.env"
    set +a
fi

# Service-specific configuration
export SERVICE_NAME="${SERVICE_NAME}"
export PORT="${PARSER_PORT:-8085}"
export LOG_LEVEL="${LOG_LEVEL:-info}"

# Parser-specific settings
export SCHEMA_DIR="${PROJECT_ROOT}/examples/schemas"
export ENABLE_STRICT_VALIDATION="${ENABLE_STRICT_VALIDATION:-true}"

echo "[${SERVICE_NAME}] Starting on port ${PORT}..."
echo "[${SERVICE_NAME}] Schema directory: ${SCHEMA_DIR}"

# Build if needed
if [ ! -f "${PROJECT_ROOT}/bin/${SERVICE_NAME}" ]; then
    echo "[${SERVICE_NAME}] Building..."
    cd "${PROJECT_ROOT}"
    go build -o "bin/${SERVICE_NAME}" "./cmd/${SERVICE_NAME}"
fi

# Run the service
cd "${PROJECT_ROOT}"
exec "./bin/${SERVICE_NAME}" "$@"
