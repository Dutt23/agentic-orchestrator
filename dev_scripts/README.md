# Development Scripts

Shell scripts for local development and testing. For production deployment, use Docker/Kubernetes.

## Core Scripts

### dev.sh
Complete development environment startup - starts all services

```bash
./dev_scripts/dev.sh
```

### migrate.sh
Run database migrations

```bash
./dev_scripts/migrate.sh
```

### start.sh / stop.sh / restart.sh
Control all services

```bash
./dev_scripts/start.sh    # Start all
./dev_scripts/stop.sh     # Stop all
./dev_scripts/restart.sh  # Restart all
```

### status.sh
Check status of all services

```bash
./dev_scripts/status.sh
```

## Testing Scripts

### test_run.sh
Integration test - complete workflow execution

```bash
./dev_scripts/test_run.sh
```

### test_rate_limit.sh
Test workflow-aware rate limiting

```bash
./dev_scripts/test_rate_limit.sh
```

## Service-Specific Scripts

Each service has its own start.sh in its directory:

```bash
cmd/orchestrator/start.sh
cmd/workflow-runner/start.sh
cmd/agent-runner-py/start.sh
cmd/http-worker/start.sh
cmd/hitl-worker/start.sh
cmd/fanout/start.sh
```

## Production Deployment

For production, use:
- **Docker Compose:** `docker-compose up`
- **Kubernetes:** See `scripts/k8s/` (future)
- **Systemd:** See `scripts/systemd/`

These dev scripts are **not** for production use.

## Prerequisites

```bash
# Install dependencies
make deps

# Build all services
make build
```

## Environment Variables

All scripts read from `.env` file. Copy `.env.example`:

```bash
cp .env.example .env
# Edit .env with your settings
```

## Notes

- Scripts use relative paths (run from repo root)
- Services run in foreground (use tmux/screen for multiple)
- Logs go to `logs/` directory
- PIDs tracked in `.pids/` directory
