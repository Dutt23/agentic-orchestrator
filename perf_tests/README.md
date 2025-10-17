# Performance Tests

This directory contains performance benchmarks and tests for the orchestrator platform.

## Test Categories

### 1. Mover Performance (`mover/`)
- Bare metal vs Docker comparison
- With/without io_uring optimization
- UDS vs TCP performance
- Zero-copy vs standard I/O

### 2. Format Benchmarks (`formats/`)
- JSON vs MessagePack vs CBOR
- Serialization/deserialization speed
- Payload size comparison
- Cross-language compatibility

### 3. Workflow Execution (`workflows/`)
- Throughput tests (workflows/sec)
- Latency tests (p50, p95, p99)
- Patch application performance
- Materialization speed with N patches

### 4. Network Tuning (`network/`)
- TCP tuning impact (BBR, tcp_tw_reuse, etc.)
- io_uring network performance
- gRPC vs REST comparison

## Running Tests

### Quick Test
```bash
# Compare with/without mover
./scripts/test-bare-metal.sh
```

### Full Benchmark Suite
```bash
cd perf_tests
go test -bench=. -benchmem ./...
```

### Specific Category
```bash
cd perf_tests/mover
go test -bench=. -benchtime=10s
```

## Test Requirements

- Postgres running on localhost:5432
- Redis running on localhost:6379
- Mover binary built: `cd common/mover && cargo build --release`
- Go services built: `go build -o bin/* ./cmd/*`

## Metrics Collected

**Latency:**
- p50, p95, p99, p99.9
- Min, max, average

**Throughput:**
- Operations per second
- Bytes per second

**Resources:**
- CPU usage
- Memory allocations
- System calls count

**I/O:**
- Disk reads/writes
- Network send/receive
- Cache hits/misses

## Baseline Results

_To be filled after running tests_

| Test | Baseline | Optimized | Improvement |
|------|----------|-----------|-------------|
| CAS Read (1KB) | TBD | TBD | TBD |
| CAS Read (1MB) | TBD | TBD | TBD |
| Apply 100 patches | TBD | TBD | TBD |
| WebSocket fanout | TBD | TBD | TBD |

## Adding New Tests

1. Create directory: `perf_tests/<category>/`
2. Add `*_test.go` files
3. Use `testing.B` for benchmarks
4. Document in this README

## See Also

- [SCALABILITY.md](../submission_doc/operations/SCALABILITY.md) - Performance tuning guide
- [Mover README](../common/mover/README.md) - Mover service documentation
