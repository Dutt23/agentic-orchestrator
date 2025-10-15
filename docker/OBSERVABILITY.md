# Observability Guide

Complete guide to monitoring, debugging, and observing the orchestrator system.

---

## Health Checks

All services expose health endpoints for monitoring:

### Check All Services

```bash
# From docker directory
docker-compose ps

# Expected: All services show "healthy" status
```

### Individual Health Endpoints

```bash
# Orchestrator API
curl http://localhost:8081/health
# Response: {"service":"orchestrator","status":"ok"}

# Fanout (WebSocket)
curl http://localhost:8085/health
# Response: OK

# Agent Runner
curl http://localhost:8086/health
# Response: {"status":"ok","service":"agent-runner","workers":4,"running":true}

# Check PostgreSQL
docker-compose exec postgres pg_isready -U orchestrator

# Check Redis
docker-compose exec redis redis-cli ping
```

---

## Metrics Collection

### What Metrics Are Captured

#### Agent Runner (Python)
Each job execution captures:

**Timing Metrics:**
- `queue_time_ms` - Time spent waiting in Redis queue
- `execution_time_ms` - Actual execution time
- `total_duration_ms` - End-to-end duration

**Resource Metrics:**
- `memory_start_mb` - Memory at job start
- `memory_peak_mb` - Peak memory during execution
- `memory_end_mb` - Memory at job end
- `cpu_percent` - CPU usage percentage
- `thread_count` - Number of threads

**System Info:**
- OS, architecture, hostname
- CPU cores (physical and logical)
- Total memory
- Python version
- Container runtime (Docker/K8s)

**LLM Metrics:**
- `tokens_used` - Total tokens consumed
- `cache_hit` - Whether prompt cache was hit
- `llm_model` - Model used
- Tool calls executed

**Location:** `cmd/agent-runner-py/metrics.py`

#### Go Services (Orchestrator, Workflow-Runner, Workers)

**Performance Profiling:**
- CPU profiling via pprof
- Memory profiling
- Goroutine profiling
- Mutex contention profiling

**Operation Metrics:**
- Request durations
- Database query times
- Redis operation latencies

**Location:** `common/telemetry/telemetry.go`, `common/metrics/`

### Accessing Metrics

#### pprof Endpoints (Go Services)

**Orchestrator** - Port 6060:
```bash
# CPU profile
curl http://localhost:6060/debug/pprof/profile?seconds=30 > cpu.prof

# Memory profile
curl http://localhost:6060/debug/pprof/heap > heap.prof

# Goroutines
curl http://localhost:6060/debug/pprof/goroutine

# All available profiles
curl http://localhost:6060/debug/pprof/
```

**Analyze with go tool:**
```bash
go tool pprof cpu.prof
go tool pprof -http=:8080 heap.prof  # Interactive web UI
```

#### Agent Metrics Endpoint

```bash
curl http://localhost:8086/metrics
# Response: {"workers":4,"status":"running"}
```

**Metrics are embedded in job results** - Check Redis or database for detailed per-job metrics.

---

## Logging

### View Logs

```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f orchestrator
docker-compose logs -f agent-runner
docker-compose logs -f workflow-runner

# Last N lines
docker-compose logs --tail 100 orchestrator

# Since timestamp
docker-compose logs --since 5m agent-runner

# Follow multiple services
docker-compose logs -f orchestrator workflow-runner agent-runner
```

### Log Levels

Set via `LOG_LEVEL` environment variable in `.env`:
```bash
LOG_LEVEL=debug   # Verbose (development)
LOG_LEVEL=info    # Normal (default)
LOG_LEVEL=warn    # Warnings only
LOG_LEVEL=error   # Errors only
```

### Log Format

**Go Services:** Structured logging with fields
```
[timestamp] [LEVEL] message key=value key=value
```

**Python Services:** Standard Python logging
```
timestamp - module - LEVEL - message
```

### Log Aggregation (Production)

For production, aggregate logs using:

**Option 1: Loki + Grafana**
```yaml
# Add to docker-compose.yml
loki:
  image: grafana/loki:latest
  ports:
    - "3100:3100"

promtail:
  image: grafana/promtail:latest
  volumes:
    - /var/lib/docker/containers:/var/lib/docker/containers:ro
  command: -config.file=/etc/promtail/config.yml
```

**Option 2: ELK Stack**
- Elasticsearch for storage
- Logstash for processing
- Kibana for visualization

**Option 3: Cloud Logging**
- AWS CloudWatch
- Google Cloud Logging
- Datadog

---

## Performance Monitoring

### Real-Time Resource Usage

```bash
# Container stats (all services)
docker stats

# Specific service
docker stats orchestrator-api

# CPU and memory usage
docker-compose exec orchestrator top
```

### Database Performance

```bash
# Active connections
docker-compose exec postgres psql -U orchestrator -d orchestrator -c "SELECT count(*) FROM pg_stat_activity;"

# Slow queries
docker-compose exec postgres psql -U orchestrator -d orchestrator -c "SELECT query, calls, total_time FROM pg_stat_statements ORDER BY total_time DESC LIMIT 10;"

# Database size
docker-compose exec postgres psql -U orchestrator -d orchestrator -c "SELECT pg_size_pretty(pg_database_size('orchestrator'));"
```

### Redis Performance

```bash
# Redis info
docker-compose exec redis redis-cli info stats

# Memory usage
docker-compose exec redis redis-cli info memory

# Check slow log
docker-compose exec redis redis-cli slowlog get 10

# Monitor commands in real-time
docker-compose exec redis redis-cli monitor
```

---

## Debugging

### Debug a Failing Service

```bash
# 1. Check if service is running
docker-compose ps <service-name>

# 2. View logs
docker-compose logs <service-name> --tail 100

# 3. Check health
curl http://localhost:<port>/health

# 4. Exec into container
docker-compose exec <service-name> sh

# 5. Check environment variables
docker-compose exec <service-name> env

# 6. Check network connectivity
docker-compose exec orchestrator ping postgres
docker-compose exec workflow-runner curl http://orchestrator:8081/health
```

### Common Issues & Solutions

#### Service Won't Start
```bash
# Check dependencies
docker-compose ps postgres redis

# Check logs for errors
docker-compose logs <service>

# Rebuild if code changed
docker-compose build <service>
docker-compose up -d <service>
```

#### Database Connection Errors
```bash
# Check postgres is healthy
docker-compose ps postgres

# Check connection from service
docker-compose exec orchestrator env | grep POSTGRES

# Test connection
docker-compose exec postgres psql -U orchestrator -d orchestrator -c "SELECT 1"
```

#### Redis Connection Errors
```bash
# Check redis is healthy
docker-compose exec redis redis-cli ping

# Check from service
docker-compose exec workflow-runner env | grep REDIS
```

### Debug Agent Runner SSL Issues

```bash
# Check certifi installation
docker-compose exec docker-agent-runner-1 python -c "import certifi; print(certifi.where())"

# Test OpenAI connection
docker-compose exec docker-agent-runner-1 python -c "
from openai import OpenAI
client = OpenAI()
print('Testing OpenAI connection...')
models = client.models.list()
print(f'Success! Found {len(models.data)} models')
"

# Check environment variables
docker-compose exec docker-agent-runner-1 env | grep -E "SSL|CERT|OPENAI"
```

---

## Performance Metrics

### Current Performance (MVP)

Based on integration tests and profiling:

**Throughput:**
- ~1,000 workflows/sec (single instance)
- ~5-10ms average latency per node execution
- ~100-300ms for LLM-based agent nodes

**Resource Usage:**
- Orchestrator: ~50-100MB RAM, ~5-10% CPU
- Workflow-runner: ~80-150MB RAM, ~10-20% CPU
- Agent-runner: ~200-500MB RAM per replica, ~20-40% CPU
- PostgreSQL: ~200MB RAM, ~10-15% CPU
- Redis: ~100MB RAM, ~5% CPU

### Benchmarking

```bash
# Run performance tests
cd cmd/orchestrator && go test -bench=. -benchmem

# Agent execution timing
docker-compose logs agent-runner | grep "execution_time_ms"

# Database query performance
docker-compose exec postgres psql -U orchestrator -d orchestrator -c "
SELECT query, calls, mean_exec_time, max_exec_time
FROM pg_stat_statements
ORDER BY mean_exec_time DESC
LIMIT 10;
"
```

---

## Monitoring Endpoints

### Service Endpoints

| Service | Health | Metrics | Profiling |
|---------|--------|---------|-----------|
| **Orchestrator** | :8081/health | :9090/metrics* | :6060/debug/pprof/ |
| **Workflow-Runner** | - | :9090/metrics* | :6060/debug/pprof/ |
| **Agent-Runner** | :8086/health | :8086/metrics | - |
| **Fanout** | :8085/health | - | - |
| **HTTP-Worker** | - | - | :6060/debug/pprof/ |
| **HITL-Worker** | - | - | :6060/debug/pprof/ |

*Prometheus metrics endpoints (not yet implemented - placeholder)

### Enable Profiling

pprof is enabled by default in development. Configure via environment:

```bash
# In .env file
ENABLE_PPROF=true
PPROF_PORT=6060
ENABLE_METRICS=true
METRICS_PORT=9090
```

---

## Production Monitoring Setup

### Prometheus + Grafana (Recommended)

Add to `docker-compose.yml`:

```yaml
prometheus:
  image: prom/prometheus:latest
  ports:
    - "9090:9090"
  volumes:
    - ./prometheus.yml:/etc/prometheus/prometheus.yml
    - prometheus_data:/prometheus
  command:
    - '--config.file=/etc/prometheus/prometheus.yml'

grafana:
  image: grafana/grafana:latest
  ports:
    - "3001:3000"
  environment:
    - GF_SECURITY_ADMIN_PASSWORD=admin
  volumes:
    - grafana_data:/var/lib/grafana
  depends_on:
    - prometheus

volumes:
  prometheus_data:
  grafana_data:
```

**Create `prometheus.yml`:**
```yaml
scrape_configs:
  - job_name: 'orchestrator'
    static_configs:
      - targets: ['orchestrator:9090']

  - job_name: 'workflow-runner'
    static_configs:
      - targets: ['workflow-runner:9090']
```

### Key Metrics to Monitor

**System Health:**
- Service uptime
- Health check status
- Container restart count

**Performance:**
- Request latency (p50, p95, p99)
- Throughput (workflows/sec)
- Error rate

**Resources:**
- CPU usage per service
- Memory usage per service
- Disk I/O
- Network I/O

**Business Metrics:**
- Workflows submitted
- Workflows completed
- Workflows failed
- Agent patch count
- HITL approval count

### Alerting

**Critical Alerts:**
- Service down > 1 minute
- Error rate > 5%
- Database connections exhausted
- Redis memory > 90%
- Disk space < 10%

**Warning Alerts:**
- High latency (p95 > 1s)
- Memory usage > 80%
- CPU usage > 80%
- Failed health checks

---

## Distributed Tracing (Future)

For production, add OpenTelemetry:

**Go Services:**
```go
import "go.opentelemetry.io/otel"
```

**Python Services:**
```python
from opentelemetry import trace
from opentelemetry.exporter.jaeger import JaegerExporter
```

**Jaeger UI:**
```yaml
jaeger:
  image: jaegertracing/all-in-one:latest
  ports:
    - "16686:16686"  # UI
    - "14268:14268"  # Collector
```

---

## Quick Reference

### Most Useful Commands

```bash
# View all service logs
docker-compose logs -f

# Check service health
docker-compose ps
curl http://localhost:8081/health

# Monitor resource usage
docker stats

# Database queries
docker-compose exec postgres psql -U orchestrator -d orchestrator

# Redis monitoring
docker-compose exec redis redis-cli monitor

# Service profiling
curl http://localhost:6060/debug/pprof/

# Restart unhealthy service
docker-compose restart <service-name>
```

### Debugging Workflow

1. **Check service status**: `docker-compose ps`
2. **View recent logs**: `docker-compose logs --tail 50 <service>`
3. **Test health endpoint**: `curl http://localhost:<port>/health`
4. **Check dependencies**: Postgres, Redis healthy?
5. **Review environment**: `docker-compose exec <service> env`
6. **Inspect network**: Can services reach each other?
7. **Check resources**: `docker stats` - out of memory/CPU?

---

## Log Analysis Examples

### Find Errors

```bash
# All services
docker-compose logs | grep -i error

# Specific service
docker-compose logs orchestrator | grep -i "error\|failed\|panic"

# Count errors
docker-compose logs --since 1h | grep -c "ERROR"
```

### Performance Analysis

```bash
# Agent execution times
docker-compose logs agent-runner | grep "execution_time_ms" | awk -F'execution_time_ms=' '{print $2}' | awk '{print $1}' | sort -n

# Database query times
docker-compose logs orchestrator | grep "duration_ms"

# Find slow operations
docker-compose logs | grep "duration_ms" | awk -F'duration_ms=' '{print $2}' | awk '{print $1}' | sort -rn | head -20
```

### Traffic Analysis

```bash
# Request count by endpoint
docker-compose logs orchestrator | grep "request" | awk '{print $NF}' | sort | uniq -c | sort -rn

# Error rate
TOTAL=$(docker-compose logs orchestrator | grep -c "request")
ERRORS=$(docker-compose logs orchestrator | grep -c "error")
echo "Error rate: $(echo "scale=2; $ERRORS / $TOTAL * 100" | bc)%"
```

---

## Production Recommendations

### Essential Monitoring

1. **Health Checks** - Every 30 seconds (already configured)
2. **Log Aggregation** - Centralized logging (Loki/ELK)
3. **Metrics Dashboard** - Grafana with Prometheus
4. **Alerting** - PagerDuty/Slack integration
5. **Distributed Tracing** - Jaeger/Tempo

### Metrics to Track

**Red Metrics (Requests, Errors, Duration):**
- Request rate per endpoint
- Error rate (%)
- Request duration (p50, p95, p99)

**USE Metrics (Utilization, Saturation, Errors):**
- CPU utilization (%)
- Memory utilization (%)
- Disk I/O saturation
- Network saturation
- Connection pool utilization

**Business Metrics:**
- Workflows/hour
- Success rate (%)
- Average workflow duration
- Agent patch frequency
- HITL approval rate

### Dashboard Panels

**System Overview:**
- Service health status (all green?)
- Total requests/sec
- Error rate %
- Average latency

**Performance:**
- CPU usage by service
- Memory usage by service
- Request latency histogram
- Database connection pool

**Business:**
- Active workflows
- Completed workflows (last hour)
- Failed workflows (last hour)
- Agent actions (patches, executions)

---

## Current Limitations

⚠️ **What's Not Yet Implemented:**
- Prometheus metrics endpoints (placeholder exists)
- Distributed tracing
- Centralized log aggregation
- Auto-scaling based on metrics
- Performance SLO tracking

✅ **What Works Now:**
- Health checks on all services
- pprof profiling for Go services
- Per-job metrics in agent-runner
- Docker container monitoring
- Log streaming via docker-compose

---

## Next Steps for Production

1. **Implement Prometheus Exporters** in Go services
2. **Add Grafana Dashboards** for visualization
3. **Setup Centralized Logging** (Loki or ELK)
4. **Configure Alerting** (critical alerts first)
5. **Add Distributed Tracing** (OpenTelemetry + Jaeger)
6. **Create Runbooks** for common incidents
7. **Setup On-Call Rotation** and escalation

---

## See Also

- [SCALABILITY.md](../submission_doc/operations/SCALABILITY.md) - Performance tuning
- [START.md](START.md) - Getting started guide
- [BUILD_GUIDE.md](BUILD_GUIDE.md) - Build and deployment
