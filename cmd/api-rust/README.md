# API Gateway (Rust)

High-performance API gateway built with Rust, Axum, and Hyper.

## Features

- **High-Performance Reverse Proxy**: Forwards HTTP requests to backend services with connection pooling
- **Server-Sent Events (SSE)**: Real-time event streaming for thousands of concurrent clients
- **HITL Support**: Bidirectional communication for Human-in-the-Loop approvals
- **Rate Limiting**: Per-IP rate limiting to prevent abuse
- **Low Latency**: No GC pauses, predictable performance under load
- **Memory Efficient**: 50-70% less memory per SSE connection compared to Go

## Architecture

```
CLI/Browser
    |
    v
API Gateway (Rust) :8081
    |
    +---> Orchestrator (Go) :8080  [Workflows, Tags, Artifacts]
    |
    +---> Runner (Go) :8082          [Run Execution]
    |
    +---> HITL (Go) :8083            [Approvals, Questions]
```

## Configuration

Environment variables:

```bash
PORT=8081                                    # Gateway port
ORCHESTRATOR_URL=http://localhost:8080       # Orchestrator service
RUNNER_URL=http://localhost:8082             # Runner service
HITL_URL=http://localhost:8083               # HITL service
RATE_LIMIT=1000                              # Requests per minute
LOG_LEVEL=info                               # Log level
```

## Building

```bash
# Development build
cargo build

# Production build (optimized)
cargo build --release

# Or use the start script
./start.sh
```

## Running

```bash
# Using start script (recommended)
./start.sh

# Or directly
./bin/api-gateway
```

## API Endpoints

### Health Check
```
GET /health
```

### Generic Proxy (Pattern: `/api/v1/:service/*path`)

All requests are forwarded to backend services based on the service name:

**Orchestrator Service** (Port 8080)
```
POST   /api/v1/orchestrator/workflows
GET    /api/v1/orchestrator/workflows
GET    /api/v1/orchestrator/workflows/:tag
PUT    /api/v1/orchestrator/workflows/:tag
POST   /api/v1/orchestrator/tags
GET    /api/v1/orchestrator/tags/:name
DELETE /api/v1/orchestrator/tags/:name
```

**Runner Service** (Port 8082)
```
POST   /api/v1/runner/runs
GET    /api/v1/runner/runs/:id
GET    /api/v1/runner/runs/:id/status
POST   /api/v1/runner/runs/:id/cancel
```

**HITL Service** (Port 8083)
```
POST   /api/v1/hitl/approvals
GET    /api/v1/hitl/approvals/:id
PUT    /api/v1/hitl/approvals/:id/approve
PUT    /api/v1/hitl/approvals/:id/reject
```

**Parser Service** (Port 8084)
```
POST   /api/v1/parser/validate
POST   /api/v1/parser/compile
```

**Fanout Service** (Port 8085)
```
GET    /api/v1/fanout/stream/:run_id
```

### SSE Events (Special Routes - Not Proxied)
```
GET    /api/v1/events/:client_id         # SSE stream
POST   /api/v1/events/:client_id/response # Send response
```

### Adding New Services

Simply set the environment variable:

```bash
export MY_SERVICE_URL=http://localhost:9000
```

Then access via:
```
GET /api/v1/my_service/any/path
```

The gateway automatically routes to registered services!

## SSE Event Types

- `connection.established` - Initial connection
- `run.start` - Run started
- `run.progress` - Run progress update
- `run.complete` - Run completed
- `run.error` - Run error
- `question` - Question requiring response
- `node.complete` - Node completed
- `node.error` - Node error
- `heartbeat` - Keep-alive ping (every 30s)

## Performance

### Connection Pooling
- 100 idle connections per backend service
- 90s idle timeout
- TCP_NODELAY enabled
- Keep-alive enabled

### SSE Optimization
- Non-blocking channels (100 events buffered)
- Automatic client cleanup
- Heartbeat every 30s
- Memory-efficient streaming

### Rate Limiting
- In-memory token bucket
- Per-IP limiting
- Configurable limits

## Monitoring

Structured JSON logging with tracing:

```json
{
  "level": "info",
  "target_url": "http://localhost:8080",
  "path": "/workflows",
  "status": 200,
  "duration_ms": 45,
  "message": "Proxied request"
}
```

## Development

### Add new backend service

Just register it in `config.rs`:

```rust
services.insert(
    "myservice".to_string(),
    env::var("MYSERVICE_URL").unwrap_or_else(|_| "http://localhost:9000".to_string()),
);
```

Or set environment variable at runtime:
```bash
export MYSERVICE_URL=http://localhost:9000
```

No code changes needed for routing!

### Send SSE event

```rust
sse_manager.send_event("client_id", SSEEvent {
    id: Uuid::new_v4().to_string(),
    event_type: "custom.event".to_string(),
    timestamp: chrono::Utc::now(),
    data: serde_json::json!({"key": "value"}),
}).await?;
```

## Why Rust?

- **2-3x better latency**: No GC pauses
- **50-70% less memory**: Especially for SSE connections
- **Better concurrency**: Tokio async runtime
- **Predictable performance**: No GC-induced latency spikes
- **Ideal for boundary services**: Perfect for proxying and streaming

## License

MIT
