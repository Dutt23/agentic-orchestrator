#!/bin/bash
# Run performance tests comparing with/without mover

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Configuration
NUM_CALLS=${PERF_NUM_CALLS:-100000}
CONCURRENCY=${PERF_CONCURRENCY:-10}
ORCHESTRATOR_URL=${ORCHESTRATOR_URL:-http://localhost:8081}

echo "=========================================="
echo "Performance Test Runner"
echo "=========================================="
echo "Configuration:"
echo "  Calls:       $NUM_CALLS"
echo "  Concurrency: $CONCURRENCY"
echo "  URL:         $ORCHESTRATOR_URL"
echo ""

# Check if orchestrator is running
if ! curl -s "$ORCHESTRATOR_URL/health" > /dev/null; then
    echo "❌ Orchestrator not running at $ORCHESTRATOR_URL"
    echo "   Start it first with: ./scripts/start-all.sh"
    exit 1
fi

echo "✅ Orchestrator is running"
echo ""

# Test 1: Without Mover (Baseline)
echo "=========================================="
echo "Test 1: WITHOUT Mover (Baseline)"
echo "=========================================="

cd "$PROJECT_ROOT/perf_tests/workflows"

export USE_MOVER=false
export PERF_NUM_CALLS=$NUM_CALLS
export PERF_CONCURRENCY=$CONCURRENCY

go test -run=TestFetchWorkflowsConcurrent -v > /tmp/perf-without-mover.log 2>&1
cat /tmp/perf-without-mover.log

echo ""
echo "Results saved to: /tmp/perf-without-mover.log"
echo ""

# Test 2: With Mover
echo "=========================================="
echo "Test 2: WITH Mover (Optimized)"
echo "=========================================="

# Check if mover is running
if [ ! -S "/tmp/mover-orchestrator.sock" ]; then
    echo "⚠️  Mover socket not found. Start mover first:"
    echo "   ./scripts/start-mover.sh orchestrator"
    echo ""
    echo "Skipping mover test..."
    exit 0
fi

export USE_MOVER=true

go test -run=TestFetchWorkflowsConcurrent -v > /tmp/perf-with-mover.log 2>&1
cat /tmp/perf-with-mover.log

echo ""
echo "Results saved to: /tmp/perf-with-mover.log"
echo ""

# Compare results
echo "=========================================="
echo "Comparison"
echo "=========================================="
echo ""
echo "Without mover:"
grep "ops/sec\|Latency:" /tmp/perf-without-mover.log | head -5
echo ""
echo "With mover:"
grep "ops/sec\|Latency:" /tmp/perf-with-mover.log | head -5
echo ""
