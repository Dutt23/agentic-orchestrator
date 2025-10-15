#!/bin/bash

# Database Migration Runner
# Runs all migrations in order from the migrations/ directory

set -e

# Configuration
DB_NAME="${DB_NAME:-orchestrator}"
DB_USER="${DB_USER:-sdutt}"
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_PASSWORD="${DB_PASSWORD:-}"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MIGRATIONS_DIR="$SCRIPT_DIR/migrations"

# ============================================================================
# Helper Functions
# ============================================================================

function print_header() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}========================================${NC}\n"
}

function print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

function print_error() {
    echo -e "${RED}✗ $1${NC}"
}

function print_info() {
    echo -e "${YELLOW}→ $1${NC}"
}

# Build connection string
function get_db_url() {
    if [ -n "$DB_PASSWORD" ]; then
        echo "postgresql://$DB_USER:$DB_PASSWORD@$DB_HOST:$DB_PORT/$DB_NAME"
    else
        echo "postgresql://$DB_USER:@$DB_HOST:$DB_PORT/$DB_NAME"
    fi
}

# Check database connectivity
function check_db() {
    print_info "Checking database connectivity..."

    if ! psql "$(get_db_url)" -c "SELECT 1" > /dev/null 2>&1; then
        print_error "Cannot connect to database"
        echo -e "${YELLOW}Connection: $(get_db_url)${NC}"
        return 1
    fi

    print_success "Database connection OK"
    return 0
}

# Create migrations tracking table
function create_migrations_table() {
    print_info "Creating migrations tracking table..."

    psql "$(get_db_url)" <<'SQL' 2>&1 | grep -v "already exists" || true
CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    description TEXT
);
SQL

    print_success "Migrations table ready"
}

# Check if migration has been applied
function is_migration_applied() {
    local version="$1"
    local result=$(psql "$(get_db_url)" -t -c "SELECT COUNT(*) FROM schema_migrations WHERE version = '$version'")
    [ "$result" -gt 0 ]
}

# Record migration as applied
function record_migration() {
    local version="$1"
    local description="$2"

    psql "$(get_db_url)" -c "INSERT INTO schema_migrations (version, description) VALUES ('$version', '$description') ON CONFLICT (version) DO NOTHING" > /dev/null
}

# Run a single migration file
function run_migration() {
    local migration_file="$1"
    local filename=$(basename "$migration_file")
    local version="${filename%%_*}"  # Extract version number (e.g., "001" from "001_final_schema.sql")
    local description="${filename#*_}"  # Extract description
    description="${description%.sql}"   # Remove .sql extension

    # Check if already applied
    if is_migration_applied "$version"; then
        print_info "Skipping $filename (already applied)"
        return 0
    fi

    print_info "Running migration: $filename"

    # Run the migration
    if psql "$(get_db_url)" -f "$migration_file" > /dev/null 2>&1; then
        record_migration "$version" "$description"
        print_success "Migration $filename completed"
        return 0
    else
        print_error "Migration $filename failed"
        # Show error details
        psql "$(get_db_url)" -f "$migration_file"
        return 1
    fi
}

# Get list of migrations in order
function get_migrations() {
    find "$MIGRATIONS_DIR" -name "*.sql" -type f | sort
}

# Show migration status
function show_status() {
    print_header "Migration Status"

    echo -e "${BLUE}Applied migrations:${NC}"
    psql "$(get_db_url)" -c "SELECT version, description, applied_at FROM schema_migrations ORDER BY version"

    echo ""
    echo -e "${BLUE}Pending migrations:${NC}"

    local has_pending=false
    for migration_file in $(get_migrations); do
        local filename=$(basename "$migration_file")
        local version="${filename%%_*}"

        if ! is_migration_applied "$version"; then
            echo "  - $filename"
            has_pending=true
        fi
    done

    if [ "$has_pending" = false ]; then
        echo "  (none)"
    fi
}

# Run all pending migrations
function run_all_migrations() {
    print_header "Running Database Migrations"

    local migration_count=0
    local failed=false

    for migration_file in $(get_migrations); do
        if ! run_migration "$migration_file"; then
            failed=true
            break
        fi
        migration_count=$((migration_count + 1))
    done

    echo ""

    if [ "$failed" = true ]; then
        print_error "Migration failed!"
        return 1
    fi

    if [ $migration_count -eq 0 ]; then
        print_success "All migrations already applied"
    else
        print_success "Successfully applied $migration_count migration(s)"
    fi

    return 0
}

# ============================================================================
# Main
# ============================================================================

print_header "Database Migration Tool"

# Show configuration
echo -e "${YELLOW}Configuration:${NC}"
echo "  Database: $DB_NAME"
echo "  User: $DB_USER"
echo "  Host: $DB_HOST:$DB_PORT"
echo "  Migrations: $MIGRATIONS_DIR"
echo ""

# Check prerequisites
if [ ! -d "$MIGRATIONS_DIR" ]; then
    print_error "Migrations directory not found: $MIGRATIONS_DIR"
    exit 1
fi

if ! command -v psql &> /dev/null; then
    print_error "psql command not found. Please install PostgreSQL client."
    exit 1
fi

# Check database connection
if ! check_db; then
    exit 1
fi

echo ""

# Create migrations table
create_migrations_table

echo ""

# Parse command
case "${1:-migrate}" in
    migrate)
        run_all_migrations
        ;;
    status)
        show_status
        ;;
    help|--help|-h)
        echo "Usage: $0 [command]"
        echo ""
        echo "Commands:"
        echo "  migrate    Run all pending migrations (default)"
        echo "  status     Show migration status"
        echo "  help       Show this help message"
        echo ""
        echo "Environment variables:"
        echo "  DB_NAME      Database name (default: orchestrator)"
        echo "  DB_USER      Database user (default: sdutt)"
        echo "  DB_HOST      Database host (default: localhost)"
        echo "  DB_PORT      Database port (default: 5432)"
        echo "  DB_PASSWORD  Database password (default: empty)"
        ;;
    *)
        print_error "Unknown command: $1"
        echo "Run '$0 help' for usage information"
        exit 1
        ;;
esac
