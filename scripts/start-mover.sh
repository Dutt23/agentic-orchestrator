#!/bin/bash
# Start mover service for a specific Go service
# Usage: ./start-mover.sh orchestrator
# Usage: ./start-mover.sh workflow-runner

set -e

if [ -z "$1" ]; then
    echo "Usage: $0 <service-name>"
    echo "Example: $0 orchestrator"
    exit 1
fi

SERVICE_NAME=$1
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Configuration
export MOVER_SOCKET="/tmp/mover-${SERVICE_NAME}.sock"
export SERVICE_NAME="${SERVICE_NAME}"
export DATABASE_URL="${DATABASE_URL:-postgres://orchestrator:orchestrator@localhost:5432/orchestrator}"
export IOURING_ENTRIES="${IOURING_ENTRIES:-4096}"
export IOURING_FLAGS="${IOURING_FLAGS:-clamp}"

echo "Starting mover for ${SERVICE_NAME}..."
echo "  Socket: ${MOVER_SOCKET}"
echo "  Database: ${DATABASE_URL}"

# Build if needed
if [ ! -f "${PROJECT_ROOT}/common/mover/target/release/mover" ]; then
    echo "Building mover..."
    cd "${PROJECT_ROOT}/common/mover"
    cargo build --release
fi

# Remove old socket
rm -f "${MOVER_SOCKET}"

# Start mover in background
cd "${PROJECT_ROOT}/common/mover"
./target/release/mover > "/tmp/mover-${SERVICE_NAME}.log" 2>&1 &

# Save PID
echo $! > "/tmp/mover-${SERVICE_NAME}.pid"

echo "Mover started (PID: $(cat /tmp/mover-${SERVICE_NAME}.pid))"
echo "  Logs: /tmp/mover-${SERVICE_NAME}.log"
echo "  Socket: ${MOVER_SOCKET}"
