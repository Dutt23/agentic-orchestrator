# GitHub Codespaces Setup

This directory contains configuration for developing the orchestrator in GitHub Codespaces.

## Quick Start

1. **Open in Codespaces**
   - Click "Code" → "Codespaces" → "Create codespace on main"
   - Or visit: https://github.com/codespaces/new

2. **Wait for Setup**
   - Codespace will automatically run `.devcontainer/setup.sh`
   - This installs Go, Python, Rust, Node.js, Docker, and dev tools
   - Takes ~3-5 minutes on first launch

3. **Configure Environment**
   ```bash
   # Edit .env and add your OPENAI_API_KEY
   code .env
   ```

4. **Start Services**
   ```bash
   cd docker
   ./setup.sh
   ```

5. **Access Services**
   - Frontend: Port 3000 (will auto-forward)
   - Orchestrator API: Port 8081
   - See OBSERVABILITY.md for all endpoints

## What's Pre-Configured

### Languages & Runtimes
- ✅ Go 1.23
- ✅ Python 3.11
- ✅ Node.js 18
- ✅ Rust (latest)

### Tools
- ✅ Docker & Docker Compose (Docker-in-Docker)
- ✅ PostgreSQL client
- ✅ Redis client
- ✅ GitHub CLI
- ✅ jq, curl

### VS Code Extensions
- Go (gopls, delve debugger)
- Python (Pylance, linting)
- Rust (rust-analyzer)
- Docker
- GitLens
- REST Client (for API testing)
- Markdown tools

### Dev Tools
- Go: gopls, delve
- Python: black, pylint, pytest
- npm (latest)

## Ports Exposed

| Port | Service | Auto-Forward |
|------|---------|--------------|
| 3000 | Frontend | ✅ |
| 8081 | Orchestrator | ✅ |
| 8085 | Fanout | Silent |
| 5432 | PostgreSQL | Silent |
| 6379 | Redis | Silent |
| 8082-8086 | Workers | Silent |

## Performance

**Machine Specs (Recommended):**
- 4-core CPU minimum
- 8GB RAM minimum
- 32GB disk

**Codespace Size:**
- Use "4-core" or larger for best performance
- Building Docker images requires substantial resources

## Tips

### Faster Rebuilds
BuildKit is pre-configured:
```bash
# Already set via containerEnv
export DOCKER_BUILDKIT=1
```

### Persistent Storage
Go and Cargo caches are persisted in named volumes:
- `orchestrator-go-cache` - Go modules
- `orchestrator-cargo-cache` - Rust crates

### Debugging

**Go:**
```bash
# Set breakpoint in VS Code, press F5
# Or use delve directly:
dlv debug ./cmd/orchestrator
```

**Python:**
```bash
# Set breakpoint in VS Code, configure launch.json
# Or use debugpy
```

### Testing APIs

Use the REST Client extension:
1. Create `test.http` file
2. Add requests:
```http
### Health Check
GET http://localhost:8081/health

### List Workflows
GET http://localhost:8081/api/v1/workflows
```
3. Click "Send Request" above each request

## Troubleshooting

### Codespace Won't Start
- Check if setup.sh has errors in the logs
- Rebuild: Codespaces → Rebuild Container

### Docker Compose Fails
- Ensure you're using a 4-core+ codespace
- Check disk space: `df -h`
- Try: `docker system prune -a`

### Port Not Forwarding
- Go to "Ports" tab in VS Code
- Click "Forward Port" and enter port number
- Check "Public" if you need to share

## See Also

- [START.md](../docker/START.md) - Getting started guide
- [OBSERVABILITY.md](../docker/OBSERVABILITY.md) - Monitoring and debugging
- [BUILD_GUIDE.md](../docker/BUILD_GUIDE.md) - Build documentation
