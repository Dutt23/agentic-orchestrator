#!/bin/bash
# Test orchestrator performance - bare metal vs Docker
# Compares with and without mover

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "=========================================="
echo "Bare Metal Performance Test"
echo "=========================================="
echo ""

# Ensure services are built
echo "Building services..."
cd "$PROJECT_ROOT"
go build -o bin/orchestrator ./cmd/orchestrator
cargo build --release --manifest-path common/mover/Cargo.toml

# Test 1: Without Mover
echo ""
echo "Test 1: WITHOUT Mover (baseline)"
echo "----------------------------------------"
export USE_MOVER=false

./scripts/start-mover.sh orchestrator
sleep 2

# TODO: Add actual benchmark
echo "  Running benchmark..."
# curl http://localhost:8081/api/v1/workflows...
echo "  Result: TBD (add benchmark)"

./scripts/stop-mover.sh orchestrator

# Test 2: With Mover
echo ""
echo "Test 2: WITH Mover (optimized)"
echo "----------------------------------------"
export USE_MOVER=true

./scripts/start-mover.sh orchestrator
sleep 2

# TODO: Add actual benchmark
echo "  Running benchmark..."
echo "  Result: TBD (add benchmark)"

./scripts/stop-mover.sh orchestrator

echo ""
echo "=========================================="
echo "Test Complete"
echo "=========================================="
echo ""
echo "Comparison:"
echo "  Baseline (no mover): TBD"
echo "  Optimized (mover):   TBD"
echo "  Improvement:         TBD"
