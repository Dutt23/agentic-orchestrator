# Fanout Service

Real-time event broadcasting service for workflow execution updates.

## Overview

The fanout service provides WebSocket connections for frontends to receive real-time workflow execution events. It subscribes to Redis PubSub channels and broadcasts messages to connected clients.

## Architecture

```
Frontend (WebSocket) → Fanout Service → Redis PubSub → Coordinator
```

### Key Components

1. **Hub**: Manages WebSocket connections, grouped by username
2. **Client**: Individual WebSocket connection handler
3. **Server**: HTTP server with WebSocket upgrade handler
4. **Redis Subscriber**: Listens to Redis PubSub and forwards to Hub

## Quick Start

### Start the Service

```bash
./start.sh
```

### Connect from Browser

```javascript
const ws = new WebSocket('ws://localhost:8084/ws?username=test-user');

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Event:', data);
};
```

### Test with curl + websocat

```bash
# Install websocat
brew install websocat

# Connect
websocat "ws://localhost:8084/ws?username=test-user"

# In another terminal, publish test event
redis-cli PUBLISH workflow:events:test-user '{"type":"test","message":"hello"}'
```

## Configuration

Environment variables:

- `REDIS_HOST`: Redis hostname (default: localhost)
- `REDIS_PORT`: Redis port (default: 6379)
- `REDIS_PASSWORD`: Redis password (default: empty)
- `PORT`: HTTP server port (default: 8084)

## Event Format

All events follow this structure:

```json
{
  "type": "workflow_started|node_completed|workflow_completed",
  "run_id": "flight-search:186e185fcb360f00",
  "timestamp": 1697234567,
  "username": "test-user",
  ...additional fields based on type
}
```

### Event Types

#### workflow_started
```json
{
  "type": "workflow_started",
  "run_id": "flight-search:186e185fcb360f00",
  "tag": "flight-search",
  "nodes": 2,
  "entry_nodes": 1,
  "timestamp": 1697234567
}
```

#### node_completed
```json
{
  "type": "node_completed",
  "run_id": "flight-search:186e185fcb360f00",
  "node_id": "fetch_flights",
  "status": "completed",
  "counter": 1,
  "result_ref": "sha256:...",
  "timestamp": 1697234568
}
```

#### workflow_completed
```json
{
  "type": "workflow_completed",
  "run_id": "flight-search:186e185fcb360f00",
  "counter": 0,
  "duration_ms": 1234,
  "timestamp": 1697234570
}
```

## API Endpoints

### WebSocket Connection

**URL:** `ws://localhost:8084/ws?username={username}`

**Query Parameters:**
- `username` (required): User identifier for routing events

**Example:**
```
ws://localhost:8084/ws?username=test-user
```

### Health Check

**URL:** `GET http://localhost:8084/health`

**Response:** `200 OK`

## Redis Channel Convention

Events are published to channels following this pattern:

```
workflow:events:{username}
```

Examples:
- `workflow:events:test-user`
- `workflow:events:sdutt`
- `workflow:events:org_acme`

The fanout service subscribes to `workflow:events:*` to receive all events.

## Multi-Tenancy

Isolation is achieved through username-based channels:
- Each user connects with their username
- Events are routed only to connections with matching username
- No cross-user event leakage

## Scaling

### Horizontal Scaling

Multiple fanout instances can run simultaneously:

```bash
# Instance 1
PORT=8084 ./start.sh

# Instance 2
PORT=8085 ./start.sh

# Instance 3
PORT=8086 ./start.sh
```

Load balancer configuration:
```nginx
upstream fanout_cluster {
    least_conn;
    server localhost:8084;
    server localhost:8085;
    server localhost:8086;
}

server {
    location /ws {
        proxy_pass http://fanout_cluster;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

### High-Scale Alternative: nchan

For >10K concurrent connections, see [NCHAN_ARCHITECTURE.md](./NCHAN_ARCHITECTURE.md) for a battle-tested Nginx module alternative.

## Monitoring

### Connection Stats

Add an admin endpoint to show stats:

```bash
# TODO: Implement /admin/stats endpoint
curl http://localhost:8084/admin/stats
```

### Logs

Service logs connection events:
```
2025-01-13 10:30:15 New WebSocket connection: username=test-user, remote=127.0.0.1
2025-01-13 10:30:20 Broadcasting to username=test-user, client_count=3
2025-01-13 10:30:45 Client unregistered: username=test-user, remaining_for_user=2
```

## Development

### Build

```bash
go build -o fanout .
```

### Run Tests

```bash
go test ./...
```

### Dependencies

```bash
go get github.com/redis/go-redis/v9
go get github.com/gorilla/websocket
```

## Deployment

### Docker

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o fanout .

FROM alpine:latest
COPY --from=builder /app/fanout /fanout
EXPOSE 8084
CMD ["/fanout"]
```

### Kubernetes

See nchan documentation for production Kubernetes deployment examples.

## Troubleshooting

### Connections not receiving events

1. Check Redis PubSub is working:
```bash
redis-cli PSUBSCRIBE workflow:events:*
# In another terminal:
redis-cli PUBLISH workflow:events:test-user '{"test":"data"}'
```

2. Check fanout logs for subscription confirmation

3. Verify username matches between connection and Redis channel

### High memory usage

- Each connection uses ~1MB memory
- 5K connections = ~5GB memory
- Consider scaling horizontally or migrating to nchan

### Connection drops

- Check firewall/proxy timeout settings
- Ensure ping/pong is working (default: 54s interval)
- Check Redis connection stability

## Future Improvements

- [ ] Add authentication/authorization
- [ ] Implement event history (buffer last N messages)
- [ ] Add Prometheus metrics endpoint
- [ ] Add connection statistics endpoint
- [ ] Implement rate limiting per user
- [ ] Add support for Redis Streams (for event replay)
