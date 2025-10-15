# Docker Build System

## Common Dockerfile Approach

All Go services (orchestrator, workflow-runner, http-worker, hitl-worker, fanout) now use a single common Dockerfile: **`Dockerfile.go-service`**

This eliminates duplication and makes it easier to maintain consistent build configurations across services.

## How It Works

### Build Arguments

The common Dockerfile accepts these build arguments:

- **`SERVICE_NAME`** (required): The name of the service to build (e.g., `orchestrator`, `workflow-runner`)
- **`NEEDS_SCRIPTS`** (optional): Set to `"true"` if the service needs the `scripts/` directory (e.g., workflow-runner needs Lua scripts)

### Docker Compose Configuration

Each service specifies its build args in `docker-compose.yml`:

```yaml
orchestrator:
  build:
    context: .
    dockerfile: Dockerfile.go-service
    args:
      SERVICE_NAME: orchestrator

workflow-runner:
  build:
    context: .
    dockerfile: Dockerfile.go-service
    args:
      SERVICE_NAME: workflow-runner
      NEEDS_SCRIPTS: "true"
```

### What Gets Copied

The common Dockerfile automatically copies:
- `common/` - Always (all services need it)
- `cmd/orchestrator/` - Always (other services import from it)
- `cmd/${SERVICE_NAME}/` - The specific service being built
- `scripts/` - Only if `NEEDS_SCRIPTS=true`

## Adding a New Service

To add a new Go service:

1. Create your service directory: `cmd/my-service/`
2. Add it to `docker-compose.yml`:

```yaml
my-service:
  build:
    context: .
    dockerfile: Dockerfile.go-service
    args:
      SERVICE_NAME: my-service
```

3. That's it! No need to create a separate Dockerfile.

## Rebuilding Services

```bash
# Rebuild all services
docker-compose build

# Rebuild a specific service
docker-compose build orchestrator

# Rebuild without cache
docker-compose build --no-cache orchestrator
```

## Benefits

- **DRY**: Single source of truth for build configuration
- **Consistency**: All services use the same base images and build process
- **Maintainability**: Update once, applies to all services
- **Easy to extend**: Add new services without duplicating Dockerfiles

## Legacy Dockerfiles

The old individual Dockerfiles in `cmd/*/Dockerfile` can be removed or kept as backups. They are no longer used by docker-compose.
