# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /build

RUN apk add --no-cache git make musl-dev

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY cmd/hitl-worker ./cmd/hitl-worker
COPY common ./common

# Build optimized
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags="-s -w -extldflags '-static'" \
    -trimpath \
    -tags netgo \
    -o hitl-worker \
    ./cmd/hitl-worker

# Runtime stage
FROM alpine:3.19

WORKDIR /app

RUN apk add --no-cache ca-certificates

COPY --from=builder /build/hitl-worker .

RUN addgroup -S -g 1000 app && \
    adduser -S -u 1000 -G app app && \
    chown app:app /app/hitl-worker

USER app

HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
  CMD pgrep -f hitl-worker || exit 1

CMD ["./hitl-worker"]
