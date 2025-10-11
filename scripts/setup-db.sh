#!/usr/bin/env bash
set -euo pipefail

# ========================================
# Database Setup Script
# ========================================
# Sets up PostgreSQL database and runs migrations
# for the Orchestrator Platform

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

# ========================================
# Configuration (from environment or defaults)
# ========================================

POSTGRES_HOST="${POSTGRES_HOST:-localhost}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"
POSTGRES_DB="${POSTGRES_DB:-orchestrator}"
POSTGRES_USER="${POSTGRES_USER:-postgres}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-}"  # Empty by default (no password)

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

log_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Run psql with optional password and connection method
psql_cmd() {
    local use_socket="${USE_UNIX_SOCKET:-auto}"

    # If USE_UNIX_SOCKET is set, use that preference
    if [ "$use_socket" = "true" ]; then
        # Unix socket - don't use -h or -p flags
        if [ -n "${POSTGRES_PASSWORD}" ]; then
            PGPASSWORD="${POSTGRES_PASSWORD}" psql -U "${POSTGRES_USER}" "$@"
        else
            psql -U "${POSTGRES_USER}" "$@"
        fi
    elif [ "$use_socket" = "false" ]; then
        # TCP connection - use -h and -p flags
        if [ -n "${POSTGRES_PASSWORD}" ]; then
            PGPASSWORD="${POSTGRES_PASSWORD}" psql -h "${POSTGRES_HOST}" -p "${POSTGRES_PORT}" -U "${POSTGRES_USER}" "$@"
        else
            psql -h "${POSTGRES_HOST}" -p "${POSTGRES_PORT}" -U "${POSTGRES_USER}" "$@"
        fi
    else
        # Auto mode - let psql decide (used during connection check)
        if [ -n "${POSTGRES_PASSWORD}" ]; then
            PGPASSWORD="${POSTGRES_PASSWORD}" psql "$@"
        else
            psql "$@"
        fi
    fi
}

# ========================================
# Check Prerequisites
# ========================================

check_prerequisites() {
    log_step "Checking prerequisites..."

    if ! command_exists "psql"; then
        log_error "PostgreSQL client (psql) not found!"
        log_error "Install with:"
        if [[ "$OSTYPE" == "darwin"* ]]; then
            echo "  brew install postgresql@15"
        else
            echo "  sudo apt-get install postgresql-client"
        fi
        exit 1
    fi

    log_info "PostgreSQL client found: $(psql --version)"
}

# ========================================
# Check PostgreSQL Server
# ========================================

check_postgres_server() {
    log_step "Checking PostgreSQL server..."

    # Try Unix domain socket first (common on macOS/Linux for local connections)
    if [ "${POSTGRES_HOST}" = "localhost" ] || [ "${POSTGRES_HOST}" = "127.0.0.1" ]; then
        log_info "Trying Unix domain socket connection first..."
        ERROR_OUTPUT=$(psql_cmd -U "${POSTGRES_USER}" -d postgres -c "SELECT version();" 2>&1)

        if [ $? -eq 0 ]; then
            log_info "PostgreSQL server is running (via Unix socket)"
            echo "$ERROR_OUTPUT" | head -1 | xargs
            # Use Unix socket for subsequent connections
            export USE_UNIX_SOCKET=true
            return 0
        else
            log_warn "Unix socket connection failed, trying TCP..."
        fi
    fi

    # Try TCP connection
    ERROR_OUTPUT=$(psql_cmd -h "${POSTGRES_HOST}" -p "${POSTGRES_PORT}" \
        -U "${POSTGRES_USER}" -d postgres -c "SELECT version();" 2>&1)

    if [ $? -eq 0 ]; then
        log_info "PostgreSQL server is running (via TCP)"
        echo "$ERROR_OUTPUT" | head -1 | xargs
        export USE_UNIX_SOCKET=false
        return 0
    else
        log_error "Cannot connect to PostgreSQL server!"
        log_error "Connection details:"
        echo "  Host: ${POSTGRES_HOST}"
        echo "  Port: ${POSTGRES_PORT}"
        echo "  User: ${POSTGRES_USER}"
        echo ""
        log_error "Error message:"
        echo "$ERROR_OUTPUT" | head -5  # Show first 5 lines of error
        echo ""
        log_error "Please ensure PostgreSQL is running:"
        if [[ "$OSTYPE" == "darwin"* ]]; then
            echo "  # Using Homebrew:"
            echo "  brew services start postgresql@15"
            echo ""
            echo "  # Or using Docker:"
        else
            echo "  # On Ubuntu/Debian:"
            echo "  sudo systemctl start postgresql"
            echo ""
            echo "  # Or using Docker:"
        fi
        echo "  docker run -d --name postgres -p 5432:5432 \\"
        echo "    -e POSTGRES_PASSWORD=postgres postgres:15"
        echo ""
        exit 1
    fi
}

# ========================================
# Create Database
# ========================================

create_database() {
    log_step "Creating database '${POSTGRES_DB}'..."

    # Check if database already exists
    DB_EXISTS=$(psql_cmd -d postgres -tc "SELECT 1 FROM pg_database WHERE datname='${POSTGRES_DB}';" | xargs)

    if [ "$DB_EXISTS" = "1" ]; then
        log_warn "Database '${POSTGRES_DB}' already exists"

        read -p "Drop and recreate? (y/N) " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            log_info "Dropping database..."
            psql_cmd -d postgres -c "DROP DATABASE ${POSTGRES_DB};"

            log_info "Creating database..."
            psql_cmd -d postgres -c "CREATE DATABASE ${POSTGRES_DB};"

            log_info "Database '${POSTGRES_DB}' recreated"
        else
            log_info "Keeping existing database"
        fi
    else
        log_info "Creating database..."
        psql_cmd -d postgres -c "CREATE DATABASE ${POSTGRES_DB};"
        log_info "Database '${POSTGRES_DB}' created successfully"
    fi
}

# ========================================
# Run Migrations
# ========================================

run_migrations() {
    log_step "Running database migrations..."

    # Find migration files
    MIGRATION_DIR="${PROJECT_ROOT}/migrations"

    if [ ! -d "$MIGRATION_DIR" ]; then
        log_error "Migrations directory not found: $MIGRATION_DIR"
        exit 1
    fi

    # Find all .sql files in migrations directory
    MIGRATION_FILES=($(find "$MIGRATION_DIR" -name "*.sql" -type f | sort))

    if [ ${#MIGRATION_FILES[@]} -eq 0 ]; then
        log_warn "No migration files found in $MIGRATION_DIR"
        return 0
    fi

    log_info "Found ${#MIGRATION_FILES[@]} migration file(s)"

    for migration_file in "${MIGRATION_FILES[@]}"; do
        local filename=$(basename "$migration_file")
        log_info "Applying migration: $filename"
        echo ""

        # Run migration with ON_ERROR_STOP and show output
        if psql_cmd -d "${POSTGRES_DB}" \
            -v ON_ERROR_STOP=1 \
            --echo-errors \
            -f "$migration_file" 2>&1; then
            echo ""
            log_info "✓ Migration applied: $filename"
        else
            echo ""
            log_error "✗ Migration failed: $filename"
            log_error "See error output above for details"
            exit 1
        fi
    done

    log_info "All migrations applied successfully"
}

# ========================================
# Verify Schema
# ========================================

verify_schema() {
    log_step "Verifying schema..."

    # Check if key tables exist
    TABLES=(
        "cas_blob"
        "artifact"
        "tag"
        "tag_move"
        "patch_chain_member"
        "run"
        "run_snapshot_index"
    )

    local all_exist=true
    for table in "${TABLES[@]}"; do
        TABLE_EXISTS=$(psql_cmd -d "${POSTGRES_DB}" \
            -tc "SELECT 1 FROM information_schema.tables WHERE table_name='${table}';" | xargs)

        if [ "$TABLE_EXISTS" = "1" ]; then
            log_info "✓ Table exists: $table"
        else
            log_error "✗ Table missing: $table"
            all_exist=false
        fi
    done

    # Check for run table partitions (partitioned tables need at least one partition)
    if [ "$all_exist" = true ]; then
        CURRENT_YEAR=$(date +%Y)
        PARTITION_NAME="run_${CURRENT_YEAR}"

        PARTITION_EXISTS=$(psql_cmd -d "${POSTGRES_DB}" \
            -tc "SELECT 1 FROM pg_tables WHERE tablename='${PARTITION_NAME}';" | xargs)

        if [ "$PARTITION_EXISTS" = "1" ]; then
            log_info "✓ Partition exists: $PARTITION_NAME"
        else
            log_warn "⚠ Current year partition missing: $PARTITION_NAME"
            log_warn "  The 'run' table may need partition maintenance"
            log_warn "  Existing partitions:"
            psql_cmd -d "${POSTGRES_DB}" \
                -tc "SELECT tablename FROM pg_tables WHERE tablename LIKE 'run_%' ORDER BY tablename;" | \
                sed 's/^/    /'
        fi
    fi

    if [ "$all_exist" = true ]; then
        log_info "Schema verification successful"
    else
        log_error "Schema verification failed - some tables are missing"
        exit 1
    fi

    # Show table counts
    echo ""
    log_info "Table statistics:"
    for table in "${TABLES[@]}"; do
        COUNT=$(psql_cmd -d "${POSTGRES_DB}" \
            -tc "SELECT COUNT(*) FROM ${table};" 2>/dev/null | xargs || echo "N/A")
        printf "  %-25s %s rows\n" "$table:" "$COUNT"
    done
}

# ========================================
# Show Connection Info
# ========================================

show_connection_info() {
    echo ""
    echo "========================================="
    echo "  Database Setup Complete"
    echo "========================================="
    echo ""
    echo "Connection details:"
    echo "  Host:     ${POSTGRES_HOST}"
    echo "  Port:     ${POSTGRES_PORT}"
    echo "  Database: ${POSTGRES_DB}"
    echo "  User:     ${POSTGRES_USER}"
    echo ""
    echo "Connection string:"
    if [ -n "${POSTGRES_PASSWORD}" ]; then
        echo "  postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DB}"
    else
        echo "  postgres://${POSTGRES_USER}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DB}"
    fi
    echo ""
    echo "Connect using psql:"
    if [ "${USE_UNIX_SOCKET}" = "true" ]; then
        echo "  psql -U ${POSTGRES_USER} -d ${POSTGRES_DB}  # (via Unix socket)"
    elif [ -n "${POSTGRES_PASSWORD}" ]; then
        echo "  PGPASSWORD=${POSTGRES_PASSWORD} psql -h ${POSTGRES_HOST} -p ${POSTGRES_PORT} -U ${POSTGRES_USER} -d ${POSTGRES_DB}"
    else
        echo "  psql -h ${POSTGRES_HOST} -p ${POSTGRES_PORT} -U ${POSTGRES_USER} -d ${POSTGRES_DB}"
    fi
    echo ""
    log_info "You can now start the orchestrator service with: ./start.sh start"
    echo ""
}

# ========================================
# Main
# ========================================

main() {
    echo "========================================="
    echo "  Orchestrator Database Setup"
    echo "========================================="
    echo ""

    check_prerequisites
    check_postgres_server
    create_database
    run_migrations
    verify_schema
    show_connection_info
}

# Run main function
main