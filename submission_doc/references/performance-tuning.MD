AlChange # OS & Network Optimizations (Fast Path)

These settings make the platform efficient without kernel bypass. Apply thoughtfully; test in staging.

---

## 1) CPU, Scheduling, and Affinity

* **Pin control-plane** (Orchestrator, Projectors, Command Router) to dedicated cores; pin **Runners** separately.

  * systemd: `CPUAffinity=1 2 3 4` and `GOMAXPROCS` per service.
  * K8s: `resources.requests/limits`, `topologySpreadConstraints`, and `nodeSelector` for runner pools.
* **Align Kafka partitions** to worker threads for cache locality (key by `run_id`).
* Set `GOMAXPROCS` explicitly; avoid noisy-neighbor thrash.

---

## 2) Network Stack (SSE/WS friendly)

* Keep **GRO/GSO ON** (default) to reduce CPU per packet for long-lived streams.
* Enable **SO_REUSEPORT** on listener sockets to distribute accepts across threads.
* Prefer **HTTP/2** (multiplexing) for Edge→Fanout/API; set conservative idle timeouts.
* **Busy-polling** only for ultra-low latency endpoints; otherwise default (it burns CPU).

**sysctl baseline**

```bash
# /etc/sysctl.d/99-orchestrator.conf
net.core.somaxconn=4096
net.core.netdev_max_backlog=32768
net.ipv4.tcp_max_syn_backlog=8192
net.ipv4.tcp_fin_timeout=30
net.ipv4.tcp_tw_reuse=1
net.ipv4.ip_local_port_range=20000 60999
net.ipv4.tcp_mtu_probing=1
```

**ulimits**

```bash
# Enough FDs for many SSE/WS conns
* soft nofile 262144
* hard nofile 262144
```

---

## 3) Conntrack Guidance (be cautious)

* **Default: keep conntrack** (K8s, NAT, debugging depend on it).
* If you fully control both ends and need raw L4 performance, you may bypass **selectively** with RAW `NOTRACK` for a specific port/interface (e.g., internal Kafka). Prefer using an eBPF CNI (Cilium) for safer reductions.
* Tune `nf_conntrack_max`, `hashsize`, and timeouts if under high connection churn.

---

## 4) Edge/Proxy (HAProxy/Envoy) Notes

* Terminate TLS and **disable response buffering** for SSE; long client timeouts.
* Example HAProxy snippets:

```haproxy
defaults
  option http-keep-alive
  timeout client  5m
  timeout server  5m
  timeout http-keep-alive 1m
  timeout connect 5s

frontend https
  bind :443 ssl crt /etc/ssl/certs/bundle.pem
  mode http
  option http-use-htx
  http-response set-header Connection keep-alive
  default_backend api

backend api
  balance roundrobin
  server s1 api-1:8080 check
  server s2 api-2:8080 check

backend fanout
  balance leastconn
  option http-keep-alive
  server f1 fanout-1:8090 check
  server f2 fanout-2:8090 check
```

---

## 5) Kafka & Postgres Tuning

**Kafka**

* Producers: `acks=all`, idempotence on, `linger.ms=2-5`, batch size tuned to NIC/CPU.
* Partitions: 64–256 for `node.jobs.*`, key by `run_id` for per-run order.
* Consumer: enable `max.poll.records` to batch; monitor lag; scale horizontally.

**Postgres**

* WAL: `wal_compression=on`; tune `shared_buffers` and `checkpoint_timeout`.
* Use outbox pattern; consumers query with `FOR UPDATE SKIP LOCKED` for timers/outbox.
* Keep read models off the hot write path; batch projector upserts.

---

## 6) Fanout Service (SSE/WS) Patterns

* Non-blocking writes; **bounded per-conn buffer** (64–256KB). Drop oldest or backpressure slow clients.
* Heartbeats every 30s; detect half-open connections.
* Sticky routing by `run_id` (optional) to keep cache locality.
* Metrics: open conns, write queue depth, events/sec, drop rate, per-client lag.

---

## 7) CAS and Artifacts

* Use **content-addressed keys** (`sha256/…`); enable S3 multipart uploads and server-side compression for large JSON.
* Keep payloads in CAS; cache only metadata + pointers in Dragonfly.

---

## 8) eBPF for Observability (optional but powerful)

* **Uprobes** on Go `net/http` RoundTripper to collect per-upstream latency histograms (P50/P95/P99) without code changes.
* **cgroup-bpf**: tag sockets with `run_id` to attribute bytes/RTT per run.
* **bpftrace** off-CPU/lock hotspots to guide optimizations.
* **Selective packet capture** for one run (TC filter to userspace ring) for debugging.

---

## 9) Modes: Fast Path vs Full Fidelity

* **Fast Path:** minimal tracing, fused steps, aggressive caching; for high-throughput deterministic flows.
* **Full Fidelity:** deep traces, HITL, overlay diffs; for debugging, demos, or regulated workloads.

---

## 10) Checklist

* [ ] Set sysctls + ulimits
* [ ] Pin control-plane vs runners on separate cores
* [ ] Enable REUSEPORT on listeners
* [ ] Configure HAProxy/Envoy for SSE/WS
* [ ] Tune Kafka producers/consumers; size partitions
* [ ] Optimize Postgres outbox/timers
* [ ] Implement bounded buffers + heartbeats in Fanout
* [ ] Add eBPF uprobes for HTTP latency (staging)
