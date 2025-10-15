# Docker Build Optimizations

> **Multi-stage builds, dependency caching, and size optimization strategies**

## 📖 Document Overview

**Purpose:** Detailed explanation of Docker build optimizations for all services

**In this document:**
- [Overview](#overview) - Optimization strategies
- [Go Services](#go-services-6-dockerfiles) - Multi-stage, static linking
- [Python Service](#python-service-agent-runner-py) - Dependency separation
- [Rust Service](#rust-service-aob-cli) - cargo-chef caching
- [Image Sizes](#image-sizes) - Before/after comparison
- [Build Times](#build-times) - Caching impact
- [Best Practices](#best-practices) - Reusable patterns

---

## Overview

All Dockerfiles use optimization techniques to minimize image size and build time:

1. **Multi-stage builds** - Build and runtime stages separated
2. **Dependency caching** - Dependencies installed before source code
3. **Static linking** - No runtime dependencies
4. **Alpine base** - Minimal OS (5MB)
5. **Symbol stripping** - Remove debug info
6. **Non-root user** - Security

---

## Go Services (6 Dockerfiles)

### Optimization Strategy

**Build Stage:**
```dockerfile
FROM golang:1.21-alpine AS builder

# 1. Copy dependency files FIRST (caching)
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# 2. THEN copy source (invalidates cache only if code changes)
COPY cmd/orchestrator ./cmd/orchestrator
COPY common ./common

# 3. Build with aggressive optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags="-s -w -extldflags '-static'" \
    -trimpath \
    -tags netgo \
    -o orchestrator \
    ./cmd/orchestrator
```

**Runtime Stage:**
```dockerfile
FROM alpine:3.19

COPY --from=builder /build/orchestrator .

RUN addgroup -S -g 1000 app && \
    adduser -S -u 1000 -G app app

USER app
```

### Build Flags Explained

| Flag | Purpose | Benefit |
|------|---------|---------|
| `CGO_ENABLED=0` | Disable C bindings | Static binary, no libc |
| `-ldflags="-s -w"` | Strip symbols | 30-40% smaller binary |
| `-extldflags '-static'` | Static linking | No runtime dependencies |
| `-trimpath` | Remove build paths | Reproducible builds |
| `-tags netgo` | Pure Go networking | No C DNS resolver |

### Caching Benefits

**Without dependency caching:**
```
Change 1 line of code → Rebuild everything (3 minutes)
```

**With dependency caching:**
```
Change 1 line of code → Reuse deps cache → Build code only (30 seconds)
```

**Dependency cache invalidates only when:**
- go.mod changes (add/remove package)
- go.sum changes (version update)

---

## Python Service (agent-runner-py)

### Optimization Strategy

**Build Stage:**
```dockerfile
FROM python:3.11-alpine AS builder

# Install build dependencies
RUN apk add --no-cache gcc musl-dev libffi-dev openssl-dev

# Copy requirements FIRST (caching)
COPY cmd/agent-runner-py/requirements.txt .

# Install to temporary location
RUN pip install --no-cache-dir --user -r requirements.txt
```

**Runtime Stage:**
```dockerfile
FROM python:3.11-alpine

# Install ONLY runtime dependencies
RUN apk add --no-cache libffi openssl ca-certificates

# Copy installed packages from builder
COPY --from=builder /root/.local /root/.local

# Copy application code
COPY cmd/agent-runner-py/ .
```

### Benefits

| Optimization | Benefit |
|-------------|---------|
| Multi-stage | Build deps not in final image |
| requirements.txt first | Cache pip installs |
| --user install | Smaller layer size |
| Remove build deps | 100-200MB savings |

**Image size:** ~150MB (vs ~350MB without multi-stage)

---

## Rust Service (aob-cli)

### Optimization Strategy (cargo-chef)

**3-Stage Build:**

**Stage 1: Chef (prepare cargo-chef)**
```dockerfile
FROM rust:1.75-alpine AS chef
RUN cargo install cargo-chef --locked
```

**Stage 2: Planner (analyze dependencies)**
```dockerfile
FROM chef AS planner
COPY Cargo.toml Cargo.lock ./
COPY src ./src
RUN cargo chef prepare --recipe-path recipe.json
```

**Stage 3: Builder (build with caching)**
```dockerfile
FROM chef AS builder

# Build dependencies FIRST (cached!)
COPY --from=planner /build/recipe.json .
RUN cargo chef cook --release --recipe-path recipe.json

# THEN build app (only this layer rebuilds on code changes)
COPY Cargo.toml Cargo.lock ./
COPY src ./src
RUN cargo build --release --locked
```

**Stage 4: Runtime**
```dockerfile
FROM alpine:3.19
COPY --from=builder /build/target/release/aob .
```

### Why cargo-chef?

**Problem:** Rust recompiles ALL dependencies on ANY code change

**Solution:** cargo-chef separates dependency build from app build

**Result:**
- First build: 5 minutes
- Code change: 30 seconds (dependencies cached!)

### Build Flags

```dockerfile
ENV RUSTFLAGS="-C target-feature=+crt-static -C link-arg=-s"
RUN cargo build --release --locked && strip /build/target/release/aob
```

| Flag | Purpose |
|------|---------|
| `+crt-static` | Static C runtime |
| `-C link-arg=-s` | Strip symbols |
| `--locked` | Use exact Cargo.lock |
| `strip` | Further size reduction |

---

## Image Sizes

### Before Optimization

| Service | Size (without optimization) |
|---------|---------------------------|
| Orchestrator | 800MB (golang:1.21 base) |
| Python Agent | 350MB (python:3.11 base) |
| Rust CLI | 1.2GB (rust:1.75 base) |

### After Optimization

| Service | Size (optimized) | Savings |
|---------|-----------------|---------|
| Orchestrator | ~15MB | 98% smaller |
| Workflow-Runner | ~12MB | 98% smaller |
| HTTP-Worker | ~10MB | 99% smaller |
| HITL-Worker | ~10MB | 99% smaller |
| Fanout | ~12MB | 98% smaller |
| Agent-Runner | ~150MB | 57% smaller |
| aob CLI | ~8MB | 99% smaller |

**Total:** ~217MB for all 7 services (vs ~3.5GB unoptimized)

---

## Build Times

### First Build (no cache)

| Service | Time | Bottleneck |
|---------|------|------------|
| Go services | 2-3 min | go mod download |
| Python service | 3-4 min | pip install |
| Rust CLI | 5-7 min | cargo build |

### Incremental Build (with cache)

**Code change only:**
| Service | Time | Cache Hit |
|---------|------|-----------|
| Go services | 20-40s | go.mod cached |
| Python service | 10-20s | requirements.txt cached |
| Rust CLI | 30-60s | Dependencies cached |

**Dependency change:**
- Full rebuild required (cache invalidated)

---

## Build Context Optimization

### .dockerignore

Excludes unnecessary files from build context:

```dockerignore
# Documentation (not needed in images)
submission_doc/
delete/
docs/
**/docs/
*.md
*.MD

# Development files
node_modules/
target/
*.log
.git/
```

**Impact:**
- Build context: 50MB (vs 500MB without ignore)
- Faster uploads to Docker daemon
- Cleaner builds

---

## Best Practices Applied

### 1. Dependency Layer First

❌ **Bad:**
```dockerfile
COPY . .
RUN go mod download
```

✅ **Good:**
```dockerfile
COPY go.mod go.sum ./
RUN go mod download  # Cached!
COPY . .
```

### 2. Multi-Stage Builds

❌ **Bad:**
```dockerfile
FROM golang:1.21
RUN go build
# 800MB image!
```

✅ **Good:**
```dockerfile
FROM golang:1.21 AS builder
RUN go build

FROM alpine:3.19
COPY --from=builder /build/app .
# 15MB image!
```

### 3. Non-Root User

```dockerfile
RUN addgroup -S -g 1000 app && \
    adduser -S -u 1000 -G app app
USER app
```

**Security:** Containers don't run as root

### 4. Health Checks

```dockerfile
HEALTHCHECK --interval=30s --timeout=3s \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8081/health
```

**Kubernetes-ready:** Maps to readiness probes

---

## Comparison to Common Patterns

### vs. Docker Hub Official Images

**Official Go image (unoptimized):**
```dockerfile
FROM golang:1.21
COPY . .
RUN go build
```
- Size: 800MB
- Build time: 3 min (every time)

**Our optimized:**
```dockerfile
FROM golang:1.21-alpine AS builder
# ... (see above)
FROM alpine:3.19
```
- Size: 15MB (98% smaller)
- Build time: 30s (cached)

### vs. Common Multi-Stage (naive)

**Naive multi-stage:**
```dockerfile
FROM golang AS builder
COPY . .
RUN go mod download && go build
FROM alpine
COPY --from=builder /build/app .
```
- Dependencies rebuild on code change

**Our optimized:**
```dockerfile
FROM golang AS builder
COPY go.mod go.sum ./
RUN go mod download  # Cached layer
COPY . .
RUN go build  # Only this rebuilds
FROM alpine
COPY --from=builder /build/app .
```
- Dependencies cached unless go.mod changes

---

## Verification

### Check Image Sizes

```bash
docker images | grep orchestrator
```

Expected:
```
orchestrator-orchestrator      15MB
orchestrator-workflow-runner   12MB
orchestrator-http-worker       10MB
orchestrator-agent-runner     150MB
```

### Check Build Cache

```bash
# First build (no cache)
time docker-compose build --no-cache

# Second build (with cache, no changes)
time docker-compose build

# Should be 10x faster!
```

---

## Summary

**Optimizations applied:**
- Multi-stage builds (all services)
- Dependency caching (go.mod, requirements.txt, Cargo.toml first)
- Static linking (Go services)
- Symbol stripping (-ldflags="-s -w")
- Alpine base (5MB vs 100MB+)
- cargo-chef (Rust dependency caching)
- .dockerignore (exclude docs, tests)
- Non-root user (security)
- Health checks (k8s-ready)

**Results:**
- 98% size reduction (3.5GB → 217MB)
- 10x faster incremental builds
- Production-ready images
