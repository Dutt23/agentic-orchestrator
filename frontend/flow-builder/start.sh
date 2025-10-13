#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="flow-builder"
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SERVICE_DIR="${PROJECT_ROOT}/frontend/flow-builder"

# Load common environment if exists
if [ -f "${PROJECT_ROOT}/.env" ]; then
    set -a
    source "${PROJECT_ROOT}/.env"
    set +a
fi

# Load service-specific .env if exists
if [ -f "${SERVICE_DIR}/.env" ]; then
    set -a
    source "${SERVICE_DIR}/.env"
    set +a
fi

# Service-specific configuration
export SERVICE_NAME="${SERVICE_NAME}"
export PORT="${FLOW_BUILDER_PORT:-5173}"

echo "[${SERVICE_NAME}] Starting Flow Builder Frontend on port ${PORT}..."

# Check if node/npm is installed
if ! command -v node &> /dev/null; then
    echo "[${SERVICE_NAME}] ERROR: Node.js is not installed"
    echo "[${SERVICE_NAME}] Install Node.js from: https://nodejs.org/"
    exit 1
fi

if ! command -v npm &> /dev/null; then
    echo "[${SERVICE_NAME}] ERROR: npm is not installed"
    echo "[${SERVICE_NAME}] Install npm with Node.js from: https://nodejs.org/"
    exit 1
fi

echo "[${SERVICE_NAME}] ✓ Node.js $(node --version)"
echo "[${SERVICE_NAME}] ✓ npm $(npm --version)"

# Check if node_modules exists
if [ ! -d "${SERVICE_DIR}/node_modules" ]; then
    echo "[${SERVICE_NAME}] Installing dependencies..."
    cd "${SERVICE_DIR}"
    npm install
    echo "[${SERVICE_NAME}] ✓ Dependencies installed"
else
    echo "[${SERVICE_NAME}] ✓ Dependencies already installed"
fi

# Run the service
cd "${SERVICE_DIR}"
echo "[${SERVICE_NAME}] Starting Vite dev server..."
echo "[${SERVICE_NAME}] Frontend will be available at: http://localhost:${PORT}"
echo ""
exec npm run dev
