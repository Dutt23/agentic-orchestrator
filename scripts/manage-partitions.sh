#!/usr/bin/env bash
set -euo pipefail

# ========================================
# Partition Management Script
# ========================================
# Creates and manages partitions for the run table
# Can be run manually or via cron

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

# ========================================
# Configuration
# ========================================

POSTGRES_HOST="${POSTGRES_HOST:-localhost}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"
POSTGRES_DB="${POSTGRES_DB:-orchestrator}"
POSTGRES_USER="${POSTGRES_USER:-postgres}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-}"

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

# Run psql with optional password and connection method
psql_cmd() {
    local use_socket="${USE_UNIX_SOCKET:-auto}"

    if [ "$use_socket" = "true" ]; then
        if [ -n "${POSTGRES_PASSWORD}" ]; then
            PGPASSWORD="${POSTGRES_PASSWORD}" psql -U "${POSTGRES_USER}" "$@"
        else
            psql -U "${POSTGRES_USER}" "$@"
        fi
    elif [ "$use_socket" = "false" ]; then
        if [ -n "${POSTGRES_PASSWORD}" ]; then
            PGPASSWORD="${POSTGRES_PASSWORD}" psql -h "${POSTGRES_HOST}" -p "${POSTGRES_PORT}" -U "${POSTGRES_USER}" "$@"
        else
            psql -h "${POSTGRES_HOST}" -p "${POSTGRES_PORT}" -U "${POSTGRES_USER}" "$@"
        fi
    else
        # Auto mode
        if [ -n "${POSTGRES_PASSWORD}" ]; then
            PGPASSWORD="${POSTGRES_PASSWORD}" psql "$@"
        else
            psql "$@"
        fi
    fi
}

# ========================================
# Check Connection
# ========================================

check_connection() {
    log_step "Checking database connection..."

    # Try Unix socket first for localhost
    if [ "${POSTGRES_HOST}" = "localhost" ] || [ "${POSTGRES_HOST}" = "127.0.0.1" ]; then
        if psql_cmd -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -c "SELECT 1;" > /dev/null 2>&1; then
            export USE_UNIX_SOCKET=true
            log_info "Connected via Unix socket"
            return 0
        fi
    fi

    # Try TCP
    if psql_cmd -h "${POSTGRES_HOST}" -p "${POSTGRES_PORT}" \
        -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -c "SELECT 1;" > /dev/null 2>&1; then
        export USE_UNIX_SOCKET=false
        log_info "Connected via TCP"
        return 0
    fi

    log_error "Cannot connect to database"
    exit 1
}

# ========================================
# List Existing Partitions
# ========================================

list_partitions() {
    log_step "Listing existing partitions..."
    echo ""

    PARTITIONS=$(psql_cmd -d "${POSTGRES_DB}" -t -c "
        SELECT
            c.relname AS partition_name,
            pg_get_expr(c.relpartbound, c.oid) AS partition_bounds
        FROM pg_class c
        JOIN pg_inherits i ON c.oid = i.inhrelid
        JOIN pg_class p ON p.oid = i.inhparent
        WHERE p.relname = 'run'
        ORDER BY c.relname;
    ")

    if [ -z "$PARTITIONS" ]; then
        log_warn "No partitions found for 'run' table"
    else
        echo "Existing partitions:"
        echo "$PARTITIONS" | while IFS='|' read -r name bounds; do
            name=$(echo "$name" | xargs)
            bounds=$(echo "$bounds" | xargs)
            printf "  %-15s %s\n" "$name" "$bounds"
        done
    fi
    echo ""
}

# ========================================
# Create Partition for Year
# ========================================

create_partition() {
    local year=$1
    local partition_name="run_${year}"
    local start_date="${year}-01-01"
    local end_year=$((year + 1))
    local end_date="${end_year}-01-01"

    log_step "Creating partition for year ${year}..."

    # Check if partition already exists
    PARTITION_EXISTS=$(psql_cmd -d "${POSTGRES_DB}" \
        -tc "SELECT 1 FROM pg_tables WHERE tablename='${partition_name}';" | xargs)

    if [ "$PARTITION_EXISTS" = "1" ]; then
        log_warn "Partition '${partition_name}' already exists, skipping"
        return 0
    fi

    # Create the partition
    psql_cmd -d "${POSTGRES_DB}" <<SQL
CREATE TABLE ${partition_name} PARTITION OF run
    FOR VALUES FROM ('${start_date}') TO ('${end_date}');
SQL

    if [ $? -eq 0 ]; then
        log_info "✓ Created partition: ${partition_name} (${start_date} to ${end_date})"
    else
        log_error "✗ Failed to create partition: ${partition_name}"
        exit 1
    fi
}

# ========================================
# Auto-create Upcoming Partitions
# ========================================

auto_create_partitions() {
    local years_ahead="${1:-2}"  # Default: create 2 years ahead
    local current_year=$(date +%Y)

    log_step "Auto-creating partitions for next ${years_ahead} year(s)..."
    echo ""

    for i in $(seq 0 $years_ahead); do
        local year=$((current_year + i))
        create_partition "$year"
    done

    echo ""
    log_info "Partition auto-creation complete"
}

# ========================================
# Drop Old Partitions (with confirmation)
# ========================================

drop_partition() {
    local year=$1
    local partition_name="run_${year}"

    log_step "Dropping partition for year ${year}..."

    # Check if partition exists
    PARTITION_EXISTS=$(psql_cmd -d "${POSTGRES_DB}" \
        -tc "SELECT 1 FROM pg_tables WHERE tablename='${partition_name}';" | xargs)

    if [ "$PARTITION_EXISTS" != "1" ]; then
        log_warn "Partition '${partition_name}' does not exist, skipping"
        return 0
    fi

    # Count rows in partition
    ROW_COUNT=$(psql_cmd -d "${POSTGRES_DB}" \
        -tc "SELECT COUNT(*) FROM ${partition_name};" | xargs)

    log_warn "Partition '${partition_name}' contains ${ROW_COUNT} rows"
    read -p "Are you sure you want to drop this partition? (yes/NO) " -r
    echo

    if [[ $REPLY == "yes" ]]; then
        psql_cmd -d "${POSTGRES_DB}" -c "DROP TABLE ${partition_name};"
        if [ $? -eq 0 ]; then
            log_info "✓ Dropped partition: ${partition_name}"
        else
            log_error "✗ Failed to drop partition: ${partition_name}"
            exit 1
        fi
    else
        log_info "Aborted"
    fi
}

# ========================================
# Show Partition Statistics
# ========================================

show_stats() {
    log_step "Partition statistics..."
    echo ""

    psql_cmd -d "${POSTGRES_DB}" <<SQL
SELECT
    c.relname AS partition_name,
    pg_size_pretty(pg_total_relation_size(c.oid)) AS total_size,
    (SELECT COUNT(*) FROM ONLY c.oid) AS row_count,
    pg_get_expr(c.relpartbound, c.oid) AS partition_bounds
FROM pg_class c
JOIN pg_inherits i ON c.oid = i.inhrelid
JOIN pg_class p ON p.oid = i.inhparent
WHERE p.relname = 'run'
ORDER BY c.relname;
SQL

    echo ""
}

# ========================================
# Usage
# ========================================

usage() {
    cat <<EOF
Usage: $(basename "$0") [COMMAND] [OPTIONS]

Commands:
    list                List all existing partitions
    create YEAR         Create partition for specific year
    auto [YEARS]        Auto-create partitions (default: current + 2 years ahead)
    drop YEAR           Drop partition for specific year (with confirmation)
    stats               Show partition statistics (size, row counts)

Examples:
    $(basename "$0") list
    $(basename "$0") create 2026
    $(basename "$0") auto 3
    $(basename "$0") drop 2020
    $(basename "$0") stats

Environment Variables:
    POSTGRES_HOST       Database host (default: localhost)
    POSTGRES_PORT       Database port (default: 5432)
    POSTGRES_DB         Database name (default: orchestrator)
    POSTGRES_USER       Database user (default: postgres)
    POSTGRES_PASSWORD   Database password (default: empty)

Cron Example (run monthly to create partitions 2 years ahead):
    0 0 1 * * cd /path/to/orchestrator && ./scripts/manage-partitions.sh auto 2

EOF
}

# ========================================
# Main
# ========================================

main() {
    if [ $# -eq 0 ]; then
        usage
        exit 0
    fi

    local command=$1
    shift

    case "$command" in
        list)
            check_connection
            list_partitions
            ;;
        create)
            if [ $# -eq 0 ]; then
                log_error "Missing YEAR argument"
                echo "Usage: $(basename "$0") create YEAR"
                exit 1
            fi
            check_connection
            create_partition "$1"
            ;;
        auto)
            local years_ahead="${1:-2}"
            check_connection
            auto_create_partitions "$years_ahead"
            ;;
        drop)
            if [ $# -eq 0 ]; then
                log_error "Missing YEAR argument"
                echo "Usage: $(basename "$0") drop YEAR"
                exit 1
            fi
            check_connection
            drop_partition "$1"
            ;;
        stats)
            check_connection
            show_stats
            ;;
        help|--help|-h)
            usage
            ;;
        *)
            log_error "Unknown command: $command"
            echo ""
            usage
            exit 1
            ;;
    esac
}

# Run main function
main "$@"
