Okay# Build & Deployment Guide

## Quick Start

### Prerequisites
- Docker & Docker Compose installed
- `.env` file configured (see `.env.example`)

### Build & Run Everything

```bash
# Method 1: Build and start in one command
docker-compose up --build -d

# Method 2: Build first, then start
docker-compose build
docker-compose up -d

# Check status
docker-compose ps

# View logs
docker-compose logs -f
```

## Recommended Build Settings

For the most reliable builds, use BuildKit:

```bash
# Enable BuildKit (better caching and performance)
export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1

# Then build
docker-compose build
```

### Add to your shell profile (~/.bashrc or ~/.zshrc):
```bash
export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1
```

## Troubleshooting

### Intermittent Build Failures

If you encounter random package manager errors (especially with Alpine):

```bash
# Clean build without cache
docker-compose build --no-cache

# Or rebuild specific service
docker-compose build --no-cache orchestrator
```

### Port Already in Use

If port 8081, 5432, or 6379 is in use:

```bash
# Find and kill the process
lsof -ti:8081 | xargs kill -9

# Or change ports in .env file
```

### Database Issues

If services can't connect to the database:

```bash
# Fresh start with clean database
docker-compose down -v  # WARNING: Deletes all data!
docker-compose up -d
```

### Service Restart Loop

Check logs for the specific service:

```bash
docker-compose logs <service-name>
```

Common fixes:
- Missing environment variables
- Wrong service hostnames (use service names, not localhost)
- Missing files/directories

## Architecture

### Services

**Infrastructure:**
- `postgres` - Database (port 5432)
- `redis` - Cache/Queue (port 6379)

**Control Plane:**
- `orchestrator` - Main API (port 8081)
- `workflow-runner` - Workflow execution (port 8082)

**Worker Plane:**
- `http-worker` - HTTP task worker (2 replicas)
- `hitl-worker` - Human-in-the-loop worker
- `agent-runner` - Python agent runner (2 replicas)

**Streaming:**
- `fanout` - WebSocket fanout (port 8085)

**Frontend:**
- `frontend` - React UI (port 3000)

### Service Communication

Services communicate using **Docker service names**, not localhost:
- `http://orchestrator:8081` (not localhost:8081)
- `postgres:5432` (not localhost:5432)
- `redis:6379` (not localhost:6379)

## Development

### Rebuild Specific Service

```bash
docker-compose build <service-name>
docker-compose up -d <service-name>
```

### View Logs

```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f orchestrator

# Last N lines
docker-compose logs --tail 50 orchestrator
```

### Stop Everything

```bash
docker-compose down

# With volume cleanup (deletes data!)
docker-compose down -v
```

## Production Considerations

1. **Use specific versions** - Pin all dependencies
2. **Multi-stage builds** - Already implemented for efficiency
3. **Security** - Non-root users configured for all services
4. **Health checks** - Already implemented for all services
5. **Resource limits** - Already configured in docker-compose.yml

## Common Commands Reference

```bash
# Build everything
docker-compose build

# Build without cache
docker-compose build --no-cache

# Start all services
docker-compose up -d

# Stop all services
docker-compose down

# View status
docker-compose ps

# View logs
docker-compose logs -f

# Restart a service
docker-compose restart orchestrator

# Scale workers
docker-compose up -d --scale http-worker=5

# Execute command in container
docker-compose exec orchestrator sh

# Clean everything (including volumes)
docker-compose down -v
docker system prune -a
```
