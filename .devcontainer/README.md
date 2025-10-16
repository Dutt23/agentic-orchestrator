# GitHub Codespaces Configuration

This directory contains optional configuration for GitHub Codespaces. **It does not affect local development.**

## Local Development (Default)

The standard setup works out of the box:

```bash
cd docker
./setup.sh
```

The `docker-compose.yml` defaults to `localhost` URLs which work for local development.

## GitHub Codespaces (Optional)

If you open this repo in Codespaces, the environment will be pre-configured with Go, Python, Rust, Node.js, and Docker.

### Note on Codespaces Timeout

Codespaces stop after 30 minutes of inactivity (default). Your code is saved, but Docker containers stop. To resume:

```bash
cd docker
docker-compose up -d
```

### Codespaces URLs

When running in Codespaces, the frontend needs to use forwarded URLs instead of localhost. This requires manual configuration since Codespace names change each time.

**Not automated** - this avoids interfering with local development where `localhost` is correct.

## Does This Affect Local Development?

**No.** The `.devcontainer/` directory is only used by GitHub Codespaces. When you run Docker locally:
- Docker Compose uses default `localhost` URLs
- All services communicate via Docker's internal network
- Frontend connects to `http://localhost:8081` (correct for local)

## When to Use Codespaces

- Quick demo without local Docker setup
- Testing on different OS
- Sharing a live environment temporarily

**Limitation:** 4-hour timeout means it's not ideal for long-running demos.

## Files in This Directory

- `devcontainer.json` - Codespaces configuration (languages, extensions, ports)
- `setup.sh` - Runs on Codespace creation
- `README.md` - This file

**None of these files affect `docker-compose up` or local development.**
