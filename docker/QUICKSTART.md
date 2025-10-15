# Quick Start Guide

## Prerequisites

- Docker & Docker Compose
- 8GB+ RAM recommended
- Ports available: 3000, 5432, 6379, 8081, 8085

## Setup

### 1. Configure Environment

Copy and edit the environment file:

```bash
cp .env.example .env
# Edit .env and add your OPENAI_API_KEY
```

### 2. Build & Start

**Recommended (most reliable):**

```bash
# Enable BuildKit for better performance
export DOCKER_BUILDKIT=1

# Build and start everything
docker-compose up --build -d
```

**If build fails randomly:**

```bash
# Clean build
docker-compose build --no-cache
docker-compose up -d
```

### 3. Verify Services

```bash
# Check all services are running
docker-compose ps

# Test orchestrator API
curl http://localhost:8081/health
# Expected: {"service":"orchestrator","status":"ok"}

# Access frontend
open http://localhost:3000
```

## Service URLs

- **Frontend UI**: http://localhost:3000
- **Orchestrator API**: http://localhost:8081
- **WebSocket (Fanout)**: ws://localhost:8085/ws
- **PostgreSQL**: localhost:5432
- **Redis**: localhost:6379

## Common Commands

```bash
# View all logs
docker-compose logs -f

# View specific service logs
docker-compose logs -f orchestrator

# Restart a service
docker-compose restart orchestrator

# Stop everything
docker-compose down

# Stop and clean volumes (fresh start)
docker-compose down -v
```

## Troubleshooting

### "Port already in use"
```bash
# Kill process on port
lsof -ti:8081 | xargs kill -9
```

### "Connection refused" or services restarting
```bash
# Check logs for the failing service
docker-compose logs <service-name>

# Common fixes:
# 1. Ensure .env has all required variables
# 2. Rebuild the service: docker-compose build <service-name>
# 3. Check if dependencies are healthy: docker-compose ps
```

### Random build failures
```bash
# Just retry - built-in retry logic will handle it
docker-compose build

# If it keeps failing, clean build:
docker-compose build --no-cache
```

### Database connection errors
```bash
# Fresh database
docker-compose down -v
docker-compose up -d
```

## Architecture Overview

The orchestrator follows a microservices architecture:

- **Stateless services** - All can be horizontally scaled
- **Shared database** - PostgreSQL for persistence
- **Message queue** - Redis for async communication
- **Event streaming** - WebSocket fanout for real-time updates

## Next Steps

- Read `BUILD_GUIDE.md` for detailed information
- Read `DOCKER_BUILD.md` for Docker architecture
- Check individual service READMEs in `cmd/*/`
