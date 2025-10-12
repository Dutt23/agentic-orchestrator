#!/usr/bin/env bash

# Master test runner - runs all test scripts
# Usage: ./run_all_tests.sh [test_name]
#   If test_name is provided, runs only that test
#   Otherwise, runs all tests

set -e

# Source common utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/test_common.sh"

# ============================================================================
# Test Configuration
# ============================================================================

# Override defaults if needed
export API_BASE="${API_BASE:-http://localhost:8081/api/v1}"
export DB_NAME="${DB_NAME:-orchestrator}"
export DB_USER="${DB_USER:-sdutt}"

# ============================================================================
# Test Suite Definition
# ============================================================================

# Test definitions: "key:script:description"
TESTS=(
    "api:test_api.sh:Basic API Workflow Tests"
    "patches:test_patches.sh:Patch Chain Tests"
    "compaction:test_compaction.sh:Compaction Logic Tests"
)

# ============================================================================
# Functions
# ============================================================================

function get_test_info() {
    local search_key="$1"
    for test_def in "${TESTS[@]}"; do
        IFS=':' read -r key script description <<< "$test_def"
        if [ "$key" = "$search_key" ]; then
            echo "$script:$description"
            return 0
        fi
    done
    return 1
}

function run_test() {
    local test_key="$1"
    local test_info=$(get_test_info "$test_key")

    if [ -z "$test_info" ]; then
        print_error "Unknown test: $test_key"
        return 1
    fi

    local test_script="${test_info%%:*}"
    local test_description="${test_info##*:}"

    print_header "$test_description"

    if [ ! -f "$SCRIPT_DIR/$test_script" ]; then
        print_error "Test script not found: $test_script"
        return 1
    fi

    # Run the test script
    if bash "$SCRIPT_DIR/$test_script"; then
        print_success "$test_description completed"
        echo ""
        return 0
    else
        print_error "$test_description failed"
        return 1
    fi
}

function list_tests() {
    echo -e "${CYAN}Available tests:${NC}"
    echo ""
    for test_def in "${TESTS[@]}"; do
        IFS=':' read -r key script description <<< "$test_def"
        echo -e "  ${BLUE}$key${NC} - $description"
    done
    echo ""
}

function run_all_tests() {
    local failed_tests=()
    local passed_tests=()

    print_header "Running All Tests"

    for test_def in "${TESTS[@]}"; do
        IFS=':' read -r test_key script description <<< "$test_def"
        if run_test "$test_key"; then
            passed_tests+=("$test_key")
        else
            failed_tests+=("$test_key")
        fi
    done

    # Print summary
    print_header "Test Summary"

    if [ ${#passed_tests[@]} -gt 0 ]; then
        echo -e "${GREEN}Passed (${#passed_tests[@]}):${NC}"
        for test in "${passed_tests[@]}"; do
            echo -e "  ${GREEN}✓${NC} $test"
        done
        echo ""
    fi

    if [ ${#failed_tests[@]} -gt 0 ]; then
        echo -e "${RED}Failed (${#failed_tests[@]}):${NC}"
        for test in "${failed_tests[@]}"; do
            echo -e "  ${RED}✗${NC} $test"
        done
        echo ""
        return 1
    fi

    print_success "All tests passed!"
    return 0
}

# ============================================================================
# Main
# ============================================================================

print_header "Orchestrator Test Suite"

# Check prerequisites
check_api "$DEFAULT_USER" || exit 1
check_db || exit 1

# Parse arguments
if [ $# -eq 0 ]; then
    # No arguments - run all tests
    run_all_tests
    exit $?
elif [ "$1" == "list" ] || [ "$1" == "--list" ] || [ "$1" == "-l" ]; then
    # List available tests
    list_tests
    exit 0
elif [ "$1" == "help" ] || [ "$1" == "--help" ] || [ "$1" == "-h" ]; then
    # Show help
    echo "Usage: $0 [test_name|list|help]"
    echo ""
    echo "Options:"
    echo "  test_name    Run a specific test (api, patches, compaction)"
    echo "  list, -l     List available tests"
    echo "  help, -h     Show this help message"
    echo "  (no args)    Run all tests"
    echo ""
    list_tests
    exit 0
else
    # Run specific test
    test_name="$1"
    if run_test "$test_name"; then
        exit 0
    else
        exit 1
    fi
fi
