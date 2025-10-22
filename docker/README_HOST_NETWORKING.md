# Host Networking for Performance Testing

## Why Host Networking?

Docker bridge network adds overhead:
- **Latency:** +1-5ms per call (veth pairs, iptables, NAT)
- **Throughput:** ~10-20% reduction
- **CPU:** Extra processing for bridge/NAT

**Problem:** This masks mover's microsecond-level improvements!

With host networking:
- ✅ Services use host's network stack directly
- ✅ No bridge, no veth, no iptables
- ✅ Same performance as bare metal

## Usage

```bash
# Regular mode (development)
docker-compose up

# Host networking (performance testing)
docker-compose -f docker-compose.yml -f docker-compose.host-network.yml up
```

## What Changes

**Without override (bridge mode):**
```
postgres:5432 (in bridge network)
redis:6379 (in bridge network)
Services communicate via Docker DNS
```

**With override (host mode):**
```
localhost:5432 (host's postgres)
localhost:6379 (host's redis)
Services communicate via localhost
```

## Important Notes

### 1. Linux Only

**Host networking only works on Linux**

macOS Docker Desktop:
- ❌ Host mode not supported (runs in VM)
- Uses bridge mode even with host specified

For macOS:
- Develop/test without mover
- Deploy to Linux for real testing

### 2. Port Conflicts

**All services bind to host ports:**
- Postgres: 5432
- Redis: 6379
- Orchestrator: 8081
- Fanout: 8085
- Frontend: 3000

**Check for conflicts:**
```bash
lsof -i :5432
lsof -i :6379
# etc.
```

### 3. No Network Isolation

**Security implications:**
- Services exposed on host network
- No Docker network isolation
- **Only for testing, not production!**

## Performance Comparison

**Bridge mode (default):**
```
Request: 10ms total
  - Bridge overhead: 1-2ms
  - Actual work: 8-9ms
```

**Host mode:**
```
Request: 8ms total
  - Bridge overhead: 0ms
  - Actual work: 8ms
```

**With mover (host mode):**
```
Request: 0.5ms total
  - Bridge overhead: 0ms
  - Actual work (via io_uring): 0.5ms
```

**Can't measure mover's true benefit without host mode!**

## Testing Strategy

### Development (macOS)
```bash
# Baseline test
USE_MOVER=false docker-compose up
```

### Linux Server (Real Testing)
```bash
# Baseline with host networking
docker-compose -f docker-compose.yml -f docker-compose.host-network.yml up

# With mover (build inside Docker)
USE_MOVER=true docker-compose -f docker-compose.yml -f docker-compose.host-network.yml up
```

## See Also

- [README_PLATFORM.md](../../common/mover/README_PLATFORM.md) - Platform requirements
- [SCALABILITY.md](../../submission_doc/operations/SCALABILITY.md) - Performance tuning
