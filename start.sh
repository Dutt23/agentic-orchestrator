#!/usr/bin/env bash
set -euo pipefail

# ========================================
# Orchestrator Platform - Unified Start Script
# ========================================
# Starts all services with performance tuning applied
# For MVP: uses local processes instead of K8s/Docker

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOGS_DIR="${PROJECT_ROOT}/logs"
PIDS_DIR="${PROJECT_ROOT}/.pids"
DATA_DIR="${PROJECT_ROOT}/data"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ========================================
# Configuration
# ========================================

# Postgres
export POSTGRES_HOST="${POSTGRES_HOST:-localhost}"
export POSTGRES_PORT="${POSTGRES_PORT:-5432}"
export POSTGRES_DB="${POSTGRES_DB:-orchestrator}"
export POSTGRES_USER="${POSTGRES_USER:-postgres}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-postgres}"
export POSTGRES_MAX_CONNS="${POSTGRES_MAX_CONNS:-50}"
export POSTGRES_MIN_CONNS="${POSTGRES_MIN_CONNS:-10}"

# Go Performance Tuning (from performance-tuning.MD)
export GOMAXPROCS="${GOMAXPROCS:-8}"
export GOGC="${GOGC:-100}"
export GOMEMLIMIT="${GOMEMLIMIT:-2GiB}"

# Service Ports
export ORCHESTRATOR_PORT="${ORCHESTRATOR_PORT:-8080}"
export API_PORT="${API_PORT:-8081}"
export RUNNER_PORT="${RUNNER_PORT:-8082}"
export FANOUT_PORT="${FANOUT_PORT:-8090}"
export UI_PORT="${UI_PORT:-3000}"

# Cache (in-memory for MVP)
export CACHE_SIZE_MB="${CACHE_SIZE_MB:-512}"

# Logging
export LOG_LEVEL="${LOG_LEVEL:-info}"
export LOG_FORMAT="${LOG_FORMAT:-json}"

# ========================================
# Helper Functions
# ========================================

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_service() {
    local service=$1
    local message=$2
    echo -e "${BLUE}[$service]${NC} $message"
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Check if port is in use
port_in_use() {
    lsof -i :"$1" >/dev/null 2>&1
}

# Apply system tuning (requires sudo)
apply_system_tuning() {
    log_info "Applying system performance tuning..."

    # Check if we can apply sysctl settings
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        if [ "$EUID" -eq 0 ]; then
            log_info "Applying sysctl settings..."
            sysctl -w net.core.somaxconn=4096
            sysctl -w net.core.netdev_max_backlog=32768
            sysctl -w net.ipv4.tcp_max_syn_backlog=8192
            sysctl -w net.ipv4.tcp_fin_timeout=30
            sysctl -w net.ipv4.tcp_tw_reuse=1
            log_info "System tuning applied"
        else
            log_warn "Not running as root. Skipping sysctl tuning (optional for MVP)"
        fi
    elif [[ "$OSTYPE" == "darwin"* ]]; then
        log_info "macOS detected. Some tuning options are limited."
        # On macOS, we can only adjust ulimits
        ulimit -n 10240 2>/dev/null || log_warn "Could not increase file descriptors limit"
    fi
}

# Setup directories
setup_directories() {
    log_info "Setting up directories..."
    mkdir -p "${LOGS_DIR}" "${PIDS_DIR}" "${DATA_DIR}/cas" "${DATA_DIR}/postgres"
    log_info "Directories created"
}

# Check dependencies
check_dependencies() {
    log_info "Checking dependencies..."

    local missing_deps=()

    if ! command_exists "go"; then
        missing_deps+=("go")
    fi

    if ! command_exists "psql"; then
        missing_deps+=("postgresql-client")
    fi

    if ! command_exists "node"; then
        missing_deps+=("node")
    fi

    if [ ${#missing_deps[@]} -ne 0 ]; then
        log_error "Missing dependencies: ${missing_deps[*]}"
        log_error "Please install them before continuing"
        exit 1
    fi

    log_info "All dependencies satisfied"
}

# Start Postgres (if not running)
start_postgres() {
    log_service "postgres" "Checking PostgreSQL..."

    if port_in_use "${POSTGRES_PORT}"; then
        log_service "postgres" "Already running on port ${POSTGRES_PORT}"
        return 0
    fi

    if command_exists "pg_ctl"; then
        log_service "postgres" "Starting local PostgreSQL..."

        # Initialize if needed
        if [ ! -d "${DATA_DIR}/postgres/data" ]; then
            initdb -D "${DATA_DIR}/postgres/data"
        fi

        pg_ctl -D "${DATA_DIR}/postgres/data" -l "${LOGS_DIR}/postgres.log" start
        sleep 2

        # Create database
        createdb "${POSTGRES_DB}" 2>/dev/null || true

        log_service "postgres" "Started successfully"
    else
        log_warn "PostgreSQL not installed locally. Please start it manually."
        log_warn "Or use Docker: docker run -d -p 5432:5432 -e POSTGRES_PASSWORD=postgres postgres:15"
        exit 1
    fi
}

# Initialize database schema
init_database() {
    log_service "postgres" "Initializing database schema..."

    # Find all migration files in migrations directory
    if [ -d "${PROJECT_ROOT}/migrations" ]; then
        for migration_file in "${PROJECT_ROOT}/migrations"/*.sql; do
            if [ -f "$migration_file" ]; then
                local filename=$(basename "$migration_file")
                log_service "postgres" "Applying migration: $filename"

                PGPASSWORD="${POSTGRES_PASSWORD}" psql -h "${POSTGRES_HOST}" -p "${POSTGRES_PORT}" \
                    -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" \
                    -f "$migration_file" \
                    >> "${LOGS_DIR}/migrations.log" 2>&1 || log_warn "Schema already applied or error occurred"
            fi
        done
        log_service "postgres" "Schema initialized"
    else
        log_warn "Migrations directory not found. Skipping schema initialization."
    fi
}

# Build Go services
build_services() {
    log_info "Building Go services..."

    cd "${PROJECT_ROOT}"

    if [ -d "cmd/orchestrator" ]; then
        log_service "build" "Building orchestrator..."
        go build -o bin/orchestrator ./cmd/orchestrator
    fi

    if [ -d "cmd/api" ]; then
        log_service "build" "Building api..."
        go build -o bin/api ./cmd/api
    fi

    if [ -d "cmd/runner" ]; then
        log_service "build" "Building runner..."
        go build -o bin/runner ./cmd/runner
    fi

    if [ -d "cmd/fanout" ]; then
        log_service "build" "Building fanout..."
        go build -o bin/fanout ./cmd/fanout
    fi

    log_info "Build complete"
}

# Start Orchestrator
start_orchestrator() {
    log_service "orchestrator" "Starting orchestrator service..."

    if [ ! -f "${PROJECT_ROOT}/bin/orchestrator" ]; then
        log_warn "Orchestrator binary not found. Skipping."
        return 0
    fi

    "${PROJECT_ROOT}/bin/orchestrator" \
        --port "${ORCHESTRATOR_PORT}" \
        --postgres-url "postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DB}" \
        --log-level "${LOG_LEVEL}" \
        >> "${LOGS_DIR}/orchestrator.log" 2>&1 &

    echo $! > "${PIDS_DIR}/orchestrator.pid"
    log_service "orchestrator" "Started with PID $(cat ${PIDS_DIR}/orchestrator.pid)"
}

# Start API Service
start_api() {
    log_service "api" "Starting API service..."

    if [ ! -f "${PROJECT_ROOT}/bin/api" ]; then
        log_warn "API binary not found. Skipping."
        return 0
    fi

    "${PROJECT_ROOT}/bin/api" \
        --port "${API_PORT}" \
        --orchestrator-url "http://localhost:${ORCHESTRATOR_PORT}" \
        --log-level "${LOG_LEVEL}" \
        >> "${LOGS_DIR}/api.log" 2>&1 &

    echo $! > "${PIDS_DIR}/api.pid"
    log_service "api" "Started with PID $(cat ${PIDS_DIR}/api.pid)"
}

# Start Runner
start_runner() {
    log_service "runner" "Starting runner service..."

    if [ ! -f "${PROJECT_ROOT}/bin/runner" ]; then
        log_warn "Runner binary not found. Skipping."
        return 0
    fi

    "${PROJECT_ROOT}/bin/runner" \
        --port "${RUNNER_PORT}" \
        --workers "${GOMAXPROCS}" \
        --orchestrator-url "http://localhost:${ORCHESTRATOR_PORT}" \
        --cas-dir "${DATA_DIR}/cas" \
        --log-level "${LOG_LEVEL}" \
        >> "${LOGS_DIR}/runner.log" 2>&1 &

    echo $! > "${PIDS_DIR}/runner.pid"
    log_service "runner" "Started with PID $(cat ${PIDS_DIR}/runner.pid)"
}

# Start Fanout (SSE/WS)
start_fanout() {
    log_service "fanout" "Starting fanout service..."

    if [ ! -f "${PROJECT_ROOT}/bin/fanout" ]; then
        log_warn "Fanout binary not found. Skipping."
        return 0
    fi

    "${PROJECT_ROOT}/bin/fanout" \
        --port "${FANOUT_PORT}" \
        --orchestrator-url "http://localhost:${ORCHESTRATOR_PORT}" \
        --log-level "${LOG_LEVEL}" \
        >> "${LOGS_DIR}/fanout.log" 2>&1 &

    echo $! > "${PIDS_DIR}/fanout.pid"
    log_service "fanout" "Started with PID $(cat ${PIDS_DIR}/fanout.pid)"
}

# Start UI
start_ui() {
    log_service "ui" "Starting UI..."

    if [ ! -d "${PROJECT_ROOT}/ui" ]; then
        log_warn "UI directory not found. Skipping."
        return 0
    fi

    cd "${PROJECT_ROOT}/ui"

    if [ ! -d "node_modules" ]; then
        log_service "ui" "Installing dependencies..."
        npm install
    fi

    REACT_APP_API_URL="http://localhost:${API_PORT}" \
    REACT_APP_FANOUT_URL="http://localhost:${FANOUT_PORT}" \
    PORT="${UI_PORT}" \
    npm start >> "${LOGS_DIR}/ui.log" 2>&1 &

    echo $! > "${PIDS_DIR}/ui.pid"
    log_service "ui" "Started with PID $(cat ${PIDS_DIR}/ui.pid)"
}

# Wait for service to be healthy
wait_for_service() {
    local service=$1
    local url=$2
    local max_attempts=30
    local attempt=0

    log_service "$service" "Waiting for health check..."

    while [ $attempt -lt $max_attempts ]; do
        if curl -sf "$url/health" >/dev/null 2>&1; then
            log_service "$service" "Healthy!"
            return 0
        fi

        attempt=$((attempt + 1))
        sleep 1
    done

    log_warn "$service failed health check after $max_attempts attempts"
    return 1
}

# Show status
show_status() {
    echo ""
    echo "========================================="
    echo "  Orchestrator Platform Status"
    echo "========================================="
    echo ""

    if [ -f "${PIDS_DIR}/orchestrator.pid" ] && kill -0 "$(cat ${PIDS_DIR}/orchestrator.pid)" 2>/dev/null; then
        echo -e "${GREEN}✓${NC} Orchestrator:  http://localhost:${ORCHESTRATOR_PORT}"
    else
        echo -e "${RED}✗${NC} Orchestrator:  not running"
    fi

    if [ -f "${PIDS_DIR}/api.pid" ] && kill -0 "$(cat ${PIDS_DIR}/api.pid)" 2>/dev/null; then
        echo -e "${GREEN}✓${NC} API:           http://localhost:${API_PORT}"
    else
        echo -e "${RED}✗${NC} API:           not running"
    fi

    if [ -f "${PIDS_DIR}/runner.pid" ] && kill -0 "$(cat ${PIDS_DIR}/runner.pid)" 2>/dev/null; then
        echo -e "${GREEN}✓${NC} Runner:        http://localhost:${RUNNER_PORT}"
    else
        echo -e "${RED}✗${NC} Runner:        not running"
    fi

    if [ -f "${PIDS_DIR}/fanout.pid" ] && kill -0 "$(cat ${PIDS_DIR}/fanout.pid)" 2>/dev/null; then
        echo -e "${GREEN}✓${NC} Fanout:        http://localhost:${FANOUT_PORT}"
    else
        echo -e "${RED}✗${NC} Fanout:        not running"
    fi

    if [ -f "${PIDS_DIR}/ui.pid" ] && kill -0 "$(cat ${PIDS_DIR}/ui.pid)" 2>/dev/null; then
        echo -e "${GREEN}✓${NC} UI:            http://localhost:${UI_PORT}"
    else
        echo -e "${RED}✗${NC} UI:            not running"
    fi

    echo ""
    echo "Logs:     ${LOGS_DIR}"
    echo "Data:     ${DATA_DIR}"
    echo ""
}

# Stop all services
stop_all() {
    log_info "Stopping all services..."

    for pidfile in "${PIDS_DIR}"/*.pid; do
        if [ -f "$pidfile" ]; then
            local pid=$(cat "$pidfile")
            local service=$(basename "$pidfile" .pid)

            if kill -0 "$pid" 2>/dev/null; then
                log_service "$service" "Stopping PID $pid..."
                kill "$pid" 2>/dev/null || true

                # Wait for graceful shutdown
                local attempt=0
                while kill -0 "$pid" 2>/dev/null && [ $attempt -lt 10 ]; do
                    sleep 1
                    attempt=$((attempt + 1))
                done

                # Force kill if still running
                if kill -0 "$pid" 2>/dev/null; then
                    log_service "$service" "Force killing..."
                    kill -9 "$pid" 2>/dev/null || true
                fi
            fi

            rm -f "$pidfile"
        fi
    done

    log_info "All services stopped"
}

# Tail logs
tail_logs() {
    if [ -d "${LOGS_DIR}" ]; then
        tail -f "${LOGS_DIR}"/*.log
    else
        log_error "Logs directory not found"
    fi
}

# ========================================
# Main
# ========================================

case "${1:-start}" in
    start)
        log_info "Starting Orchestrator Platform..."
        apply_system_tuning
        setup_directories
        check_dependencies
        start_postgres
        init_database
        build_services

        sleep 2

        start_orchestrator
        sleep 1
        start_api
        sleep 1
        start_runner
        sleep 1
        start_fanout
        sleep 1
        start_ui

        sleep 3
        show_status

        log_info "Platform started successfully!"
        log_info "Run './start.sh logs' to follow logs"
        ;;

    stop)
        stop_all
        ;;

    restart)
        stop_all
        sleep 2
        exec "$0" start
        ;;

    status)
        show_status
        ;;

    logs)
        tail_logs
        ;;

    *)
        echo "Usage: $0 {start|stop|restart|status|logs}"
        exit 1
        ;;
esac