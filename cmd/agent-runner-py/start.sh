#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="agent-runner-py"
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SERVICE_DIR="${PROJECT_ROOT}/cmd/agent-runner-py"

# Load common environment
if [ -f "${PROJECT_ROOT}/.env" ]; then
    set -a
    source "${PROJECT_ROOT}/.env"
    set +a
fi

# Load service-specific .env
if [ -f "${SERVICE_DIR}/.env" ]; then
    set -a
    source "${SERVICE_DIR}/.env"
    set +a
fi

# Service-specific configuration
export SERVICE_NAME="${SERVICE_NAME}"
export PORT="${AGENT_PORT:-8082}"
export LOG_LEVEL="${LOG_LEVEL:-info}"

# Redis configuration
export REDIS_HOST="${REDIS_HOST:-localhost}"
export REDIS_PORT="${REDIS_PORT:-6379}"
export REDIS_DB="${REDIS_DB:-0}"

# LLM configuration
export OPENAI_MODEL="${OPENAI_MODEL:-gpt-4o}"
export OPENAI_TEMPERATURE="${OPENAI_TEMPERATURE:-0.1}"
export OPENAI_MAX_TOKENS="${OPENAI_MAX_TOKENS:-4000}"

# Orchestrator URL for patch forwarding
export ORCHESTRATOR_URL="${ORCHESTRATOR_URL:-http://localhost:8081/api/v1}"

# Worker configuration
export WORKER_COUNT="${WORKER_COUNT:-4}"

echo "[${SERVICE_NAME}] Starting Agent Runner Service on port ${PORT}..."
echo "[${SERVICE_NAME}] Redis: ${REDIS_HOST}:${REDIS_PORT}"
echo "[${SERVICE_NAME}] Orchestrator: ${ORCHESTRATOR_URL}"
echo "[${SERVICE_NAME}] Workers: ${WORKER_COUNT}"
echo "[${SERVICE_NAME}] LLM Model: ${OPENAI_MODEL}"

# Check Redis connection
if ! redis-cli -h "${REDIS_HOST}" -p "${REDIS_PORT}" ping > /dev/null 2>&1; then
    echo "[${SERVICE_NAME}] ERROR: Redis is not running at ${REDIS_HOST}:${REDIS_PORT}"
    echo "[${SERVICE_NAME}] Start Redis with: redis-server"
    echo "[${SERVICE_NAME}] Or run: ${PROJECT_ROOT}/scripts/setup_redis.sh"
    exit 1
fi
echo "[${SERVICE_NAME}] ✓ Redis connection OK"

# Check OpenAI API key
if [ -z "${OPENAI_API_KEY:-}" ]; then
    echo "[${SERVICE_NAME}] ERROR: OPENAI_API_KEY not set"
    echo "[${SERVICE_NAME}] Set it in ${SERVICE_DIR}/.env"
    exit 1
fi
echo "[${SERVICE_NAME}] ✓ OpenAI API key configured"

# Check Python dependencies
if ! python3 -c "import openai, redis, yaml, fastapi" 2>/dev/null; then
    echo "[${SERVICE_NAME}] WARNING: Some Python dependencies missing"
    echo "[${SERVICE_NAME}] Installing dependencies..."
    cd "${SERVICE_DIR}"
    pip3 install -r requirements.txt --quiet --user
    echo "[${SERVICE_NAME}] ✓ Dependencies installed"
fi

# Run the service
cd "${SERVICE_DIR}"
echo "[${SERVICE_NAME}] Starting Python service..."
echo ""
exec python3 main.py "$@"
