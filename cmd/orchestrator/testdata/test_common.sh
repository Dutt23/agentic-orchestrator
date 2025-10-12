#!/bin/bash

# Common test utilities and configuration
# Source this file in test scripts: source "$(dirname "$0")/test_common.sh"

# ============================================================================
# Configuration (can be overridden by environment variables)
# ============================================================================

# API Configuration
export API_BASE="${API_BASE:-http://localhost:8081/api/v1}"
export API_HOST="${API_HOST:-localhost:8081}"
export CONTENT_TYPE="Content-Type: application/json"

# Database Configuration
export DB_NAME="${DB_NAME:-orchestrator}"
export DB_USER="${DB_USER:-sdutt}"
export DB_HOST="${DB_HOST:-localhost}"
export DB_PORT="${DB_PORT:-5432}"

# Test Configuration
export DEFAULT_USER="${DEFAULT_USER:-test-user}"
export DEPTH_THRESHOLD="${DEPTH_THRESHOLD:-10}"

# ============================================================================
# Colors for output
# ============================================================================

export GREEN='\033[0;32m'
export BLUE='\033[0;34m'
export RED='\033[0;31m'
export YELLOW='\033[1;33m'
export CYAN='\033[0;36m'
export NC='\033[0m' # No Color

# ============================================================================
# Helper Functions
# ============================================================================

# Print a section header
function print_header() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}========================================${NC}\n"
}

# Print a test step
function print_step() {
    echo -e "${YELLOW}$1${NC}"
}

# Print a substep (blue text)
function print_substep() {
    echo -e "${BLUE}$1${NC}"
}

# Print success message
function print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

# Print error message
function print_error() {
    echo -e "${RED}✗ $1${NC}"
}

# Print info message
function print_info() {
    echo -e "${CYAN}→ $1${NC}"
}

# Check if API server is available
function check_api() {
    local user="${1:-$DEFAULT_USER}"
    print_step "Checking API server availability..."

    if ! curl --max-time 2 --silent --fail \
        -H "X-User-ID: $user" \
        "$API_BASE/workflows" > /dev/null 2>&1; then
        print_error "Cannot connect to API server at $API_BASE"
        echo -e "${YELLOW}Please start the API server first:${NC}"
        echo -e "${YELLOW}  ./cmd/orchestrator/start.sh${NC}"
        echo -e "${YELLOW}Or:${NC}"
        echo -e "${YELLOW}  cd cmd/orchestrator && go run main.go${NC}"
        return 1
    fi

    print_success "API server is running at $API_BASE"
    echo ""
    return 0
}

# Check if database is available
function check_db() {
    print_step "Checking database availability..."

    if ! psql -U "$DB_USER" -h "$DB_HOST" -p "$DB_PORT" -d "$DB_NAME" -c "SELECT 1" > /dev/null 2>&1; then
        print_error "Cannot connect to database $DB_NAME"
        echo -e "${YELLOW}Connection details: postgresql://$DB_USER@$DB_HOST:$DB_PORT/$DB_NAME${NC}"
        return 1
    fi

    print_success "Database connection OK"
    echo ""
    return 0
}

# Test result checker
function test_result() {
    local test_name="$1"
    local exit_code="${2:-$?}"

    if [ $exit_code -eq 0 ]; then
        print_success "$test_name PASSED"
        echo ""
        return 0
    else
        print_error "$test_name FAILED (exit code: $exit_code)"
        echo ""
        exit 1
    fi
}

# Make API request with common headers
function api_request() {
    local method="$1"
    local endpoint="$2"
    local user="${3:-$DEFAULT_USER}"
    local data="$4"

    local url="$API_BASE$endpoint"
    local args=(
        -s
        -X "$method"
        -H "$CONTENT_TYPE"
        -H "X-User-ID: $user"
    )

    if [ -n "$data" ]; then
        args+=(-d "$data")
    fi

    curl "${args[@]}" "$url"
}

# Execute SQL with common connection parameters
function exec_sql() {
    local sql="$1"
    local quiet="${2:-false}"

    local args=(
        -U "$DB_USER"
        -h "$DB_HOST"
        -p "$DB_PORT"
        -d "$DB_NAME"
    )

    if [ "$quiet" = "true" ]; then
        args+=(-q -t)
    fi

    if [ -f "$sql" ]; then
        # SQL is a file
        psql "${args[@]}" -f "$sql"
    else
        # SQL is a string
        psql "${args[@]}" -c "$sql"
    fi
}

# Execute SQL from heredoc
function exec_sql_heredoc() {
    psql -U "$DB_USER" -h "$DB_HOST" -p "$DB_PORT" -d "$DB_NAME"
}

# Clean up test data by username
function cleanup_test_data() {
    local username="$1"

    if [ -z "$username" ]; then
        print_error "cleanup_test_data requires username parameter"
        return 1
    fi

    print_step "Cleaning up test data for user: $username"

    exec_sql "DELETE FROM tag WHERE username = '$username'" true

    print_success "Test data cleaned up"
    echo ""
}

# Clean up test data by tag name pattern
function cleanup_test_tag() {
    local username="$1"
    local tag_name="$2"

    if [ -z "$username" ] || [ -z "$tag_name" ]; then
        print_error "cleanup_test_tag requires username and tag_name parameters"
        return 1
    fi

    print_step "Cleaning up tag: $username/$tag_name"

    exec_sql "DELETE FROM tag WHERE username = '$username' AND tag_name = '$tag_name'" true

    print_success "Tag cleaned up"
    echo ""
}

# Wait for a condition with timeout
function wait_for() {
    local description="$1"
    local command="$2"
    local timeout="${3:-30}"
    local interval="${4:-1}"

    print_step "Waiting for: $description (timeout: ${timeout}s)"

    local elapsed=0
    while [ $elapsed -lt $timeout ]; do
        if eval "$command" > /dev/null 2>&1; then
            print_success "$description ready after ${elapsed}s"
            return 0
        fi
        sleep "$interval"
        elapsed=$((elapsed + interval))
    done

    print_error "Timeout waiting for: $description"
    return 1
}

# Pretty print JSON
function print_json() {
    echo "$1" | jq '.' 2>/dev/null || echo "$1"
}

# Extract field from JSON response
function json_field() {
    local json="$1"
    local field="$2"
    echo "$json" | jq -r ".$field" 2>/dev/null
}

# ============================================================================
# Initialization
# ============================================================================

# Get script directory (works when sourced)
if [ -n "${BASH_SOURCE[0]}" ]; then
    export TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
else
    export TEST_DIR="$(pwd)"
fi

# Ensure we're in the test data directory
cd "$TEST_DIR"

# Check for required tools
for tool in curl jq psql; do
    if ! command -v $tool &> /dev/null; then
        print_error "Required tool '$tool' is not installed"
        exit 1
    fi
done

# Export all functions for subshells
export -f print_header print_step print_substep print_success print_error print_info
export -f check_api check_db test_result
export -f api_request exec_sql exec_sql_heredoc
export -f cleanup_test_data cleanup_test_tag
export -f wait_for print_json json_field
