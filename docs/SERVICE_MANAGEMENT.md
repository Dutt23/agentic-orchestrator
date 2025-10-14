# Service Management Guide

## Quick Start

```bash
# Start all services
./start.sh

# Check status
./status.sh

# Stop all services
./stop.sh
```

## Service Architecture

| Service | Port | Purpose |
|---------|------|---------|
| **orchestrator** | 8081 | HTTP REST API for workflows |
| **workflow-runner** | 8082 | Coordinator + Executor |
| **fanout** | 8084 | WebSocket server for realtime events |
| **frontend** | 5173 | React UI (Vite dev server) |
| **agent-runner-py** | 8085 | Python agent worker (optional) |

## Management Scripts

### `./start.sh`
Starts all enabled services via supervisord.

**Features:**
- Checks if supervisord is installed
- Creates log and pid directories
- Loads environment from `.env`
- Shows service status after start

**Prerequisites:**
```bash
# macOS
brew install supervisor

# Ubuntu/Debian
sudo apt-get install supervisor

# Python
pip install supervisor
```

### `./stop.sh`
Gracefully stops all services.

### `./restart.sh`
Stops then starts all services.

### `./status.sh`
Shows current status of all services.

### `./dev.sh`
Interactive menu for managing services during development.

**Features:**
- Start/stop services
- View logs in real-time
- Restart individual services
- Start only core services

## Configuration

### Environment Variables

Create `.env` from `.env.example`:

```bash
cp .env.example .env
```

Edit `.env`:
```bash
DB_USER=your_username
DB_PASSWORD=your_password
REDIS_HOST=localhost
```

### supervisord.conf

Main configuration file. Edit to:
- Enable/disable services (`autostart=true/false`)
- Change ports
- Add new services
- Configure logging

## Service Groups

**Core Services** (essential):
- orchestrator
- workflow-runner
- fanout

**Full Stack**:
- core + frontend + agent-runner-py

**Backend Only**:
- core + agent-runner-py

## Common Operations

### Start Only Core Services

```bash
supervisorctl -c supervisord.conf start orchestrator workflow-runner fanout
```

### Restart Single Service

```bash
supervisorctl -c supervisord.conf restart orchestrator
```

### View Real-Time Logs

```bash
# Single service
tail -f logs/orchestrator.log

# All services
tail -f logs/*.log

# With filtering
tail -f logs/orchestrator.log | grep ERROR
```

### Manual Service Control

```bash
# Start supervisor
supervisord -c supervisord.conf

# Control services
supervisorctl -c supervisord.conf status
supervisorctl -c supervisord.conf start <service>
supervisorctl -c supervisord.conf stop <service>
supervisorctl -c supervisord.conf restart <service>

# Shutdown supervisor
supervisorctl -c supervisord.conf shutdown
```

## Troubleshooting

### Services Won't Start

1. **Check supervisord is installed:**
   ```bash
   which supervisord
   ```

2. **Check logs:**
   ```bash
   cat logs/supervisord.log
   cat logs/<service>-error.log
   ```

3. **Check if ports are in use:**
   ```bash
   lsof -i :8081  # orchestrator
   lsof -i :8084  # fanout
   lsof -i :5173  # frontend
   ```

4. **Clean up stale PIDs:**
   ```bash
   ./stop.sh
   rm -rf pids/*
   ```

### Port Already in Use

Edit `supervisord.conf` and change the port for the service:

```ini
[program:orchestrator]
environment=PORT="8091",...  # Changed from 8081
```

### Service Keeps Crashing

Check service-specific logs:
```bash
tail -f logs/orchestrator-error.log
```

Common issues:
- Database not running
- Redis not running
- Missing environment variables
- Incorrect file permissions

### Can't Stop Services

```bash
# Force kill supervisor
pkill -f supervisord

# Clean up
rm -rf pids/*

# Restart fresh
./start.sh
```

## Log Files

All logs are in `./logs/`:

- `supervisord.log` - Service manager log
- `orchestrator.log` - Orchestrator output
- `orchestrator-error.log` - Orchestrator errors
- `workflow-runner.log` - Workflow runner output
- `fanout.log` - Fanout service output
- `frontend.log` - Frontend dev server output
- `agent-runner-py.log` - Python agent worker output

## Development Workflow

### Typical Development Day

```bash
# Morning: Start everything
./start.sh

# Work on code...

# Check if services are healthy
./status.sh

# Restart after code changes (if not hot-reloading)
supervisorctl -c supervisord.conf restart orchestrator

# View logs while debugging
tail -f logs/orchestrator.log

# Evening: Stop everything
./stop.sh
```

### Frontend-Only Development

```bash
# Start only frontend
supervisorctl -c supervisord.conf start frontend

# Or use interactive menu
./dev.sh
# Select option 4
```

### Backend-Only Development

```bash
./dev.sh
# Select option 3: Start backend only
```

## Production Deployment

⚠️ **Note**: supervisord is designed for development/testing. For production, use:

- **Docker Compose** - Container orchestration
- **Kubernetes** - Cloud-native deployment
- **systemd** - Linux service management
- **PM2** - Node.js process manager

See deployment documentation for production setup.

## Alternative: Simple Process Manager

If supervisord is not available, use `dev-simple.sh` (basic process management without dependencies).

## Tips

1. **Use tmux/screen** for persistent terminal sessions:
   ```bash
   tmux new -s orchestrator
   ./start.sh
   # Detach: Ctrl+B, D
   # Reattach: tmux attach -t orchestrator
   ```

2. **Auto-restart on file changes** (for Go services):
   ```bash
   # Install air
   go install github.com/cosmtrek/air@latest
   
   # Use in service directories
   cd cmd/orchestrator && air
   ```

3. **Monitor resource usage**:
   ```bash
   # CPU/Memory
   ps aux | grep -E 'orchestrator|fanout|workflow'
   
   # Network
   lsof -i -P | grep LISTEN
   ```

## Reference

- [supervisord documentation](http://supervisord.org/)
- [Architecture Overview](../readme.MD)
- [Fanout Service](../cmd/fanout/docs/README.md)
