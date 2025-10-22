# Docker Network Modes Explained

## Bridge Mode (Default)

**How it works:**
```
┌─────────────────────────────────────────────┐
│  Your Computer (Host)                       │
│                                             │
│  ┌───────────────────────────────────────┐ │
│  │ Docker Bridge Network (docker0)       │ │
│  │                                       │ │
│  │  Container A (172.18.0.2)            │ │
│  │         ↕ veth pair                   │ │
│  │  Container B (172.18.0.3)            │ │
│  │         ↕ veth pair                   │ │
│  │  iptables NAT rules                   │ │
│  └───────────────┬───────────────────────┘ │
│                  │                          │
│         Host Network (en0/wifi)             │
└─────────────────────────────────────────────┘
```

**What happens:**
1. Each container gets private IP (172.18.x.x)
2. Virtual ethernet pairs (veth) connect containers to bridge
3. iptables NAT translates private → host IP
4. Port mappings: `8081:8081` forwards host:8081 → container:8081

**Overhead:**
- ✅ **Isolation** - Containers can't access host directly
- ✅ **Portable** - Works everywhere
- ❌ **Latency** - +1-5ms per network call
  - veth pair traversal
  - iptables rules processing
  - NAT translation
- ❌ **Throughput** - ~10-20% reduction
- ❌ **CPU** - Extra processing for bridge/NAT

**When to use:**
- Development (safe, isolated)
- Multiple projects (no port conflicts)
- Security (containers can't access host)

---

## Host Mode

**How it works:**
```
┌─────────────────────────────────────────────┐
│  Your Computer (Host)                       │
│                                             │
│  Container A ──────┐                        │
│                    │                        │
│  Container B ──────┼─→ Directly uses       │
│                    │   host network stack   │
│  Container C ──────┘   (no bridge!)        │
│                                             │
│         Host Network (en0/wifi)             │
└─────────────────────────────────────────────┘
```

**What happens:**
1. Container uses host's network namespace **directly**
2. No bridge, no veth, no NAT, no iptables
3. Container's `localhost` = host's `localhost`
4. Services bind to host ports directly

**Performance:**
- ✅ **Zero overhead** - Direct network stack access
- ✅ **Throughput** - Same as bare metal
- ✅ **Latency** - No bridge delay
- ❌ **No isolation** - Container can access everything on host
- ❌ **Port conflicts** - Can't run two services on same port

**When to use:**
- Performance testing (measure true speed)
- Production (maximize throughput)
- When network overhead matters

---

## Example: HTTP Request Between Containers

### Bridge Mode
```
Container A (orchestrator)
    ↓ Send to Container B
veth0 (Container A)
    ↓
docker0 bridge
    ↓ iptables DNAT
veth1 (Container B)
    ↓
Container B (workflow-runner)

Latency: ~2ms
```

### Host Mode
```
Container A (orchestrator)
    ↓ Direct via lo (loopback)
Container B (workflow-runner)

Latency: ~0.1ms (20x faster!)
```

---

## Mover Performance Impact

### Bridge Mode (macOS Docker)
```
Without mover:
  Request: 10ms total
  - Postgres query: 5ms
  - Docker bridge: 2ms
  - App overhead: 3ms

With mover:
  Request: 7ms total (30% faster)
  - Postgres query (io_uring): 2ms ✅
  - Docker bridge: 2ms (unchanged)
  - App overhead: 3ms

Bridge overhead masks mover's benefits!
```

### Host Mode (Linux)
```
Without mover:
  Request: 8ms total
  - Postgres query: 5ms
  - Network: 0ms (host)
  - App: 3ms

With mover:
  Request: 5ms total (40% faster)
  - Postgres query (io_uring): 2ms ✅
  - Network: 0ms (host)
  - App: 3ms

Full mover benefits visible!
```

---

## Configuration

### Bridge Mode (Default)
```bash
docker-compose up
```

### Host Mode (Linux only)
```bash
docker-compose -f docker-compose.yml -f docker-compose.host-network.yml up
```

---

## macOS Docker Desktop Limitation

**Important:** macOS Docker uses a VM:

```
macOS
  ↓
Docker Desktop VM (Linux)
  ↓
Containers

network_mode: "host" → Host of the VM, not your Mac!
```

**Result:** Host mode on macOS doesn't help (still goes through VM)

**Only on native Linux:**
- Container → Direct host network → True zero overhead

---

## Summary

**Bridge mode:**
- Safe, isolated, portable
- Adds 1-5ms latency
- Good for development

**Host mode:**
- Zero overhead, maximum speed
- No isolation
- Linux only (for real benefits)

For mover testing: Use bridge on macOS (functional), use host on Linux (full performance).
