# Building Mover Service

## Platform Requirements

**Mover requires Linux** - io_uring is Linux kernel 5.1+ only.

---

## On macOS (Your Current Machine)

**You CANNOT build or run mover natively.**

**Options:**

### Option 1: Test Without Mover (Baseline)
```bash
USE_MOVER=false ./scripts/start-all.sh
# Run performance tests to establish baseline
```

### Option 2: Build in Docker (Linux Container)
```bash
cd docker
docker-compose --profile mover build mover-orchestrator
# Builds in Linux container
```

**Note:** Even if Docker build succeeds, mover won't provide benefits on macOS due to:
- Docker Desktop runs in VM
- No access to host's (macOS's) network stack
- Bridge network overhead remains

---

## On Linux Server (For Real Testing)

### Native Build
```bash
cd common/mover
cargo build --release
# Binary: target/release/mover
```

### Docker Build
```bash
cd docker
docker-compose --profile mover build
```

### With Host Networking (Essential!)
```bash
docker-compose -f docker-compose.yml -f docker-compose.host-network.yml --profile mover up
```

---

## Summary

**macOS:**
- Establish baseline without mover
- Document architecture
- Code/test other components

**Linux:**
- Build mover
- Use host networking
- Run real performance tests

The mover foundation is complete; actual testing needs Linux! 🚀
