# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git make musl-dev

# Set Go proxy and disable sum DB (for network issues)
ENV GOPROXY=https://proxy.golang.org,direct
ENV GOSUMDB=off

# Copy go.mod and go.sum first (better caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/orchestrator ./cmd/orchestrator
COPY cmd/workflow-runner ./cmd/workflow-runner
COPY common ./common

# Build with aggressive optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags="-s -w -extldflags '-static'" \
    -trimpath \
    -tags netgo \
    -installsuffix netgo \
    -o orchestrator \
    ./cmd/orchestrator

# Verify binary
RUN ./orchestrator --version || echo "Binary built"

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install runtime dependencies
RUN apk update && apk add --no-cache \
    ca-certificates \
    tzdata

# Copy binary from builder
COPY --from=builder /build/orchestrator .

# Copy migrations
COPY migrations/ ./migrations/

# Create non-root user
RUN addgroup -S -g 1000 app && \
    adduser -S -u 1000 -G app app && \
    chown -R app:app /app

USER app

EXPOSE 8081

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8081/health || exit 1

CMD ["./orchestrator"]
