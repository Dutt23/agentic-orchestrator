# Getting Started with Docker

## Easiest Way: Use Setup Script

```bash
cd docker
./setup.sh
```

The script will:
- ✅ Check if `.env` exists in project root
- ✅ Validate OPENAI_API_KEY is set
- ✅ Create symlink automatically
- ✅ Build and start all services
- ✅ Show you the status

---

## Manual Setup (3 Steps)

### 1. Configure Environment

The `.env` file must be in the **project root directory** (not in docker/ folder):

```bash
# Make sure you're in the project root
cd /path/to/orchestrator

# Copy example environment file
cp .env.example .env

# Edit .env and add your OpenAI API key
# Required: OPENAI_API_KEY=sk-...
```

**Important:** Docker Compose reads the `.env` file from the root directory via a symlink in `docker/.env`.

### 2. Build & Start

**Option A - From docker directory (recommended):**
```bash
cd docker
docker-compose up --build -d
```

**Option B - From project root:**
```bash
docker-compose -f docker/docker-compose.yml up --build -d
```

### 3. Verify

```bash
# Check all services are running
docker-compose ps

# Test the API
curl http://localhost:8081/health

# Open the UI
open http://localhost:3000
```

That's it! 🎉

---

## What Gets Started

When you run `docker-compose up`, these services start:

| Service | Description | Port |
|---------|-------------|------|
| **postgres** | Database | 5432 |
| **redis** | Cache/Queue | 6379 |
| **orchestrator** | Main API | 8081 |
| **workflow-runner** | Workflow execution | 8082 |
| **http-worker** | HTTP task executor | - |
| **hitl-worker** | Human-in-the-loop | 8084 |
| **agent-runner** | AI agent executor | 8086 |
| **fanout** | WebSocket streaming | 8085 |
| **frontend** | React UI | 3000 |

---

## Common Commands

### View Logs
```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f orchestrator
```

### Stop Services
```bash
# Stop (keeps data)
docker-compose down

# Stop and delete all data
docker-compose down -v
```

### Restart a Service
```bash
docker-compose restart orchestrator
```

### Rebuild a Service
```bash
docker-compose build orchestrator
docker-compose up -d orchestrator
```

---

## Troubleshooting

### Port Already in Use

```bash
# Kill process on port 8081
lsof -ti:8081 | xargs kill -9

# Or change port in .env file
```

### Build Fails Randomly

This is normal with Alpine Linux package manager. Just retry:

```bash
docker-compose build
```

The Dockerfile has built-in retry logic. If it still fails:

```bash
docker-compose build --no-cache
```

### Service Won't Start

```bash
# Check logs
docker-compose logs <service-name>

# Common issues:
# 1. Missing OPENAI_API_KEY in .env
# 2. Ports already in use
# 3. Need to rebuild: docker-compose build <service-name>
```

### Fresh Start

```bash
# Delete everything and start clean
docker-compose down -v
docker-compose up --build -d
```

---

## Testing the Setup

### Test Orchestrator API

```bash
# Health check
curl http://localhost:8081/health

# List workflows
curl http://localhost:8081/api/v1/workflows
```

### Test Frontend

Open http://localhost:3000 in your browser.

### Test WebSocket

```bash
# Connect to fanout service
curl http://localhost:8085/health
```

---

## Performance Tips

### Enable BuildKit (Faster Builds)

```bash
# Add to ~/.bashrc or ~/.zshrc
export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1
```

### Scale Workers

```bash
# Run more http-workers
docker-compose up -d --scale http-worker=5

# Or edit .env:
HTTP_WORKER_REPLICAS=5
```

---

## Next Steps

- **Architecture**: See `DOCKER_BUILD.md` for technical details
- **Development**: See `../BUILD_GUIDE.md` for advanced usage
- **API Documentation**: Check orchestrator service README

## Need Help?

- Check logs: `docker-compose logs -f`
- Verify services: `docker-compose ps`
- Read troubleshooting section above
