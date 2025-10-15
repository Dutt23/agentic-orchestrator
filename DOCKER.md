# Docker Deployment Guide

Complete containerization for all services using Alpine images with build optimizations.

## Quick Start

```bash
# 1. Configure environment
cp .env.docker .env
# Edit .env with your settings (especially OPENAI_API_KEY)

# 2. Build all images
docker-compose build

# 3. Start all services
docker-compose up -d

# 4. Check status
docker-compose ps

# 5. View logs
docker-compose logs -f

# 6. Stop all
docker-compose down
```

---

## Services

### Infrastructure (2 services)

**postgres** - PostgreSQL 15 (Alpine)
- Port: 5432
- Volume: postgres_data
- Healthcheck: pg_isready

**redis** - Redis 7 (Alpine)
- Port: 6379
- Volume: redis_data
- Persistence: AOF enabled
- Healthcheck: ping

### Control Plane (2 services)

**orchestrator** - API + workflow management (Go)
- Port: 8081
- Depends: postgres, redis
- Build: Multi-stage with optimizations

**workflow-runner** - Coordinator (Go)
- Internal only
- Depends: redis, orchestrator
- Stateless (can scale horizontally)

### Worker Plane (3 services)

**http-worker** - HTTP execution (Go)
- Internal only
- Depends: redis
- Replicas: 2 (configurable)

**hitl-worker** - Human approvals (Go)
- Internal only
- Depends: redis

**agent-runner** - LLM integration (Python)
- Internal only
- Depends: redis
- Requires: OPENAI_API_KEY
- Replicas: 2 (configurable)

### Streaming Plane (1 service)

**fanout** - WebSocket streaming (Go)
- Port: 8085
- Depends: redis

---

## Build Optimizations

All Go services use:
```dockerfile
CGO_ENABLED=0              # Static linking
-ldflags="-s -w"          # Strip symbols
-trimpath                 # Remove build paths
-extldflags '-static'     # Fully static
```

**Benefits:**
- Smaller images (10-20MB per service)
- No libc dependencies
- Faster startup
- Better security (minimal attack surface)

All services use:
- **Alpine base** (5MB base image)
- **Multi-stage builds** (build artifacts not in final image)
- **Non-root user** (security)
- **Health checks** (k8s-ready)

---

## Environment Configuration

### .env.docker

```bash
# Database
DB_USER=orchestrator
DB_PASSWORD=strong_password_here
DB_NAME=orchestrator

# OpenAI API Key (required for agent-runner)
OPENAI_API_KEY=sk-...

# Scaling
HTTP_WORKER_REPLICAS=2
AGENT_WORKER_REPLICAS=2

# Logging
LOG_LEVEL=info
```

### Override per service

```bash
# Scale workers
HTTP_WORKER_REPLICAS=5 docker-compose up -d --scale http-worker=5
```

---

## Development vs. Production

### Development (docker-compose.dev.yml)

```bash
# Start only infrastructure
docker-compose -f docker-compose.dev.yml up -d

# Run services locally (hot reload)
make start-orchestrator
make start-workflow-runner
...
```

**Benefits:**
- Hot reload (no rebuild)
- Easy debugging
- Fast iteration

### Production (docker-compose.yml)

```bash
# Start everything
docker-compose up -d
```

**Benefits:**
- Complete isolation
- Consistent environment
- Easy deployment
- Resource limits

---

## Networking

All services in `orchestrator-net` bridge network.

**Internal DNS:**
- postgres:5432
- redis:6379
- orchestrator:8081
- workflow-runner:8082
- http-worker:8083
- hitl-worker:8084
- fanout:8085
- agent-runner:8086

**External ports:**
- 8081 (orchestrator API)
- 8085 (fanout WebSocket)
- 5432 (postgres - optional)
- 6379 (redis - optional)

---

## Resource Limits

| Service | CPU Limit | Memory Limit | Replicas |
|---------|-----------|--------------|----------|
| postgres | 2 cores | 2GB | 1 |
| redis | 2 cores | 2GB | 1 |
| orchestrator | 2 cores | 1GB | 1 |
| workflow-runner | 2 cores | 1GB | 1 |
| http-worker | 1 core | 512MB | 2 |
| hitl-worker | 1 core | 512MB | 1 |
| agent-runner | 2 cores | 2GB | 2 |
| fanout | 2 cores | 1GB | 1 |

**Total:** 14 cores, 10GB memory (with replicas)

---

## Health Checks

All services have health checks:

```bash
# Check all services
docker-compose ps

# Check specific service
docker inspect --format='{{.State.Health.Status}}' orchestrator-api
```

**Healthcheck endpoints:**
- orchestrator: http://localhost:8081/health
- fanout: http://localhost:8085/health
- Others: internal health checks

---

## Volumes & Persistence

**postgres_data** - Database files
- Persistent across restarts
- Backup: `docker run --rm -v orchestrator_postgres_data:/data -v $(pwd):/backup alpine tar czf /backup/postgres-backup.tar.gz /data`

**redis_data** - Redis AOF files
- Persistent across restarts
- Backup: `docker run --rm -v orchestrator_redis_data:/data -v $(pwd):/backup alpine tar czf /backup/redis-backup.tar.gz /data`

---

## Scaling Workers

```bash
# Scale HTTP workers
docker-compose up -d --scale http-worker=5

# Scale agent workers
docker-compose up -d --scale agent-runner=3

# Check
docker-compose ps
```

---

## Monitoring

### Logs

```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f orchestrator

# Last 100 lines
docker-compose logs --tail=100 agent-runner
```

### Resource usage

```bash
# Docker stats
docker stats

# Per service
docker stats orchestrator-api
```

### Database

```bash
# Connect to postgres
docker-compose exec postgres psql -U orchestrator

# Run query
docker-compose exec postgres psql -U orchestrator -c "SELECT COUNT(*) FROM runs;"
```

### Redis

```bash
# Connect to redis
docker-compose exec redis redis-cli

# Check memory
docker-compose exec redis redis-cli INFO memory

# Monitor commands
docker-compose exec redis redis-cli MONITOR
```

---

## Troubleshooting

### Service won't start

```bash
# Check logs
docker-compose logs orchestrator

# Check health
docker inspect orchestrator-api | grep -A 10 Health

# Restart service
docker-compose restart orchestrator
```

### Out of memory

```bash
# Check memory usage
docker stats

# Increase limit in docker-compose.yml
deploy:
  resources:
    limits:
      memory: 2G  # Increase this
```

### Database connection errors

```bash
# Check postgres is healthy
docker-compose ps postgres

# Check environment variables
docker-compose exec orchestrator env | grep DB_

# Test connection
docker-compose exec postgres pg_isready -U orchestrator
```

---

## Production Deployment

### 1. Use external database

```yaml
# docker-compose.prod.yml
services:
  orchestrator:
    environment:
      DB_HOST: your-rds-endpoint.amazonaws.com
      DB_PORT: 5432
```

### 2. Use secrets management

```yaml
services:
  orchestrator:
    secrets:
      - db_password
      - openai_api_key

secrets:
  db_password:
    external: true
  openai_api_key:
    external: true
```

### 3. Add reverse proxy

```yaml
services:
  nginx:
    image: nginx:alpine
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf
      - ./certs:/etc/nginx/certs
```

---

## Kubernetes Migration (Future)

These Dockerfiles are k8s-ready:
- Health checks → readiness probes
- Resource limits → pod resources
- Non-root user → security contexts
- Environment variables → ConfigMaps/Secrets

See `scripts/k8s/` for manifests (future).

---

## Commands Reference

```bash
# Build
docker-compose build
docker-compose build --no-cache  # Force rebuild

# Start
docker-compose up -d
docker-compose up orchestrator redis postgres  # Specific services

# Stop
docker-compose down
docker-compose down -v  # Remove volumes too

# Restart
docker-compose restart
docker-compose restart orchestrator

# Logs
docker-compose logs -f
docker-compose logs -f --tail=100 orchestrator

# Scale
docker-compose up -d --scale http-worker=5

# Status
docker-compose ps
docker-compose top

# Exec
docker-compose exec orchestrator sh
docker-compose exec postgres psql -U orchestrator

# Clean
docker-compose down -v --rmi all  # Remove everything
```

---

**All services containerized and ready for deployment!**
