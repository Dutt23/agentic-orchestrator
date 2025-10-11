# Systemd Slice-Based Deployment

## Overview

This directory contains systemd slice and service files for deploying Orchestrator services with proper CPU affinity, NUMA awareness, and resource isolation.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Orchestrator Platform                     │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  orchestrator-control.slice (Cores 0-7, NUMA node 0)        │
│  ├─ orchestrator.service    (Event sourcing, state machine) │
│  ├─ router.service          (Intent validation)             │
│  ├─ parser.service          (AIR patch validation)          │
│  ├─ hitl.service            (Human approvals)               │
│  └─ api.service             (HTTP gateway)                  │
│                                                               │
│  orchestrator-runner.slice (Cores 8-15, NUMA node 1)        │
│  ├─ runner.service          (Function/map/join execution)   │
│  └─ agent-runner.service    (LLM + AIR proposals)           │
│                                                               │
│  orchestrator-fanout.slice (Cores 16-19, NUMA node 0)       │
│  └─ fanout.service          (SSE/WebSocket streaming)       │
│                                                               │
└─────────────────────────────────────────────────────────────┘
```

## CPU Affinity Strategy

### Control Plane (orchestrator-control.slice)
- **Cores:** 0-7 (8 cores)
- **NUMA Node:** 0
- **Services:** orchestrator, router, parser, hitl, api
- **Rationale:** Control plane needs low latency, frequent communication, shared L3 cache
- **Memory:** 4-12GB (guaranteed)

### Runner Plane (orchestrator-runner.slice)
- **Cores:** 8-15 (8 cores)
- **NUMA Node:** 1 (if available)
- **Services:** runner, agent-runner
- **Rationale:** Execution isolated from control plane, can scale independently
- **Memory:** 8-24GB (higher for workloads)

### Fanout Plane (orchestrator-fanout.slice)
- **Cores:** 16-19 (4 cores)
- **NUMA Node:** 0 (prefer same as control for low latency)
- **Services:** fanout
- **Rationale:** Real-time streaming needs consistent latency, many connections
- **Memory:** 2-8GB (connection buffers)

## Installation

### 1. Install Slices and Services

```bash
# Run installation script
sudo ./scripts/install-systemd.sh

# Or manually
sudo cp scripts/systemd/*.slice /etc/systemd/system/
sudo cp scripts/systemd/*.service /etc/systemd/system/
sudo systemctl daemon-reload
```

### 2. Deploy Binaries

```bash
# Build services
make build

# Copy to deployment directory
sudo mkdir -p /opt/orchestrator/bin
sudo cp bin/* /opt/orchestrator/bin/
sudo chown -R orchestrator:orchestrator /opt/orchestrator
```

### 3. Configure Services

```bash
# Create config files
sudo mkdir -p /opt/orchestrator/etc

# Example: orchestrator.env
cat > /tmp/orchestrator.env <<EOF
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_DB=orchestrator
PORT=8080
GOMAXPROCS=8
EOF

sudo mv /tmp/orchestrator.env /opt/orchestrator/etc/
```

### 4. Start Services

```bash
# Start control plane first
sudo systemctl start orchestrator-orchestrator.service
sudo systemctl start orchestrator-router.service

# Then data plane
sudo systemctl start orchestrator-runner.service

# Then streaming
sudo systemctl start orchestrator-fanout.service

# Enable auto-start
sudo systemctl enable orchestrator-orchestrator.service
```

## Monitoring

### Service Status
```bash
# Individual service
systemctl status orchestrator-orchestrator.service

# All orchestrator services
systemctl status 'orchestrator-*'

# Logs
journalctl -u orchestrator-orchestrator -f
journalctl -u orchestrator-runner -f --since "10 minutes ago"
```

### Resource Usage
```bash
# Top-like view of cgroups
systemd-cgtop

# Slice details
systemctl show orchestrator-control.slice
systemctl show orchestrator-runner.slice

# CPU affinity verification
taskset -cp $(pidof orchestrator)
taskset -cp $(pidof runner)

# NUMA stats (if applicable)
numastat -p $(pidof orchestrator)
```

### Performance Metrics
```bash
# CPU usage per slice
systemctl show orchestrator-control.slice -p CPUUsageNSec

# Memory usage
systemctl show orchestrator-runner.slice -p MemoryCurrent

# Task count
systemctl show orchestrator-fanout.slice -p TasksCurrent
```

## Tuning

### For Systems with Fewer Cores (<16)

Edit slice files:

```bash
# Control plane: first half of cores
# orchestrator-control.slice
CPUAffinity=0 1 2 3

# Runner plane: second half
# orchestrator-runner.slice
CPUAffinity=4 5 6 7
```

### For Multi-Socket NUMA Systems

Uncomment NUMA settings in slice files:

```ini
# orchestrator-control.slice
NUMAPolicy=bind
NUMAMask=0

# orchestrator-runner.slice
NUMAPolicy=bind
NUMAMask=1
```

Verify with:
```bash
numactl --hardware
lscpu | grep NUMA
```

### Memory Limits

Adjust based on workload:

```ini
[Slice]
MemoryMin=4G      # Guaranteed minimum
MemoryHigh=8G     # Soft limit (throttle above this)
MemoryMax=12G     # Hard limit (OOM kill above this)
```

### I/O Priority

```ini
[Slice]
IOWeight=500      # 100-1000, higher = more I/O bandwidth
BlockIOWeight=500 # cgroups v1 equivalent
```

## Troubleshooting

### Services Won't Start

```bash
# Check journal for errors
journalctl -xe

# Check if binary exists
ls -la /opt/orchestrator/bin/

# Check permissions
sudo -u orchestrator /opt/orchestrator/bin/orchestrator --version
```

### CPU Affinity Not Applied

```bash
# Verify kernel support
cat /proc/sys/kernel/sched_domain/cpu0/domain0/flags

# Check systemd version (need 240+)
systemd --version

# Force reload
systemctl daemon-reexec
```

### Memory Limits Hit

```bash
# Check OOM kills
journalctl -k | grep -i oom

# Adjust limits in slice file
sudo systemctl edit orchestrator-runner.slice

# Or override per service
sudo systemctl edit orchestrator-runner.service
```

## Best Practices

1. **Monitor before tuning:** Use systemd-cgtop and metrics for 24h before adjusting
2. **Gradual rollout:** Enable slices on staging first
3. **Keep headroom:** Don't allocate 100% of CPU/memory
4. **Test failover:** Verify restart behavior under load
5. **Document changes:** Track slice modifications in git

## References

- [systemd.slice(5)](https://www.freedesktop.org/software/systemd/man/systemd.slice.html)
- [systemd.resource-control(5)](https://www.freedesktop.org/software/systemd/man/systemd.resource-control.html)
- [cgroups v2](https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html)
- [NUMA Best Practices](https://www.kernel.org/doc/html/latest/vm/numa.html)
