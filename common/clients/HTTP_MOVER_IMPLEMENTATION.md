# HTTP Mover Proxy Implementation

## Summary

The HTTP client (`common/clients/http.go`) now supports routing HTTP requests through the mover service for io_uring optimization. This provides transparent performance improvements without requiring service code changes.

## Files Modified

### 1. `common/clients/mover_client.go`
- Added `OpHTTP = 0x06` operation code
- Added `ProxyHTTP()` method to send HTTP requests through mover socket
- Serializes HTTP request as JSON and sends via mover protocol
- Deserializes HTTP response from mover

### 2. `common/clients/http.go`
- Implemented `doRequestViaMover()` to route HTTP through mover
- Extracts headers from context (X-User-ID, X-Test-Token)
- Converts mover response to `http.Response`
- Automatic fallback to direct HTTP if mover fails

### 3. `common/clients/context.go`
- Added `TestTokenKey` for X-Test-Token header support
- Added `WithTestToken()` and `GetTestToken()` helper functions

### 4. `common/clients/orchestrator.go`
- Added `FetchWorkflowIR()` method for test endpoints
- Uses HTTPClient which supports mover routing

### 5. `cmd/workflow-runner/handlers/test.go`
- Refactored to use `OrchestratorClient` instead of custom HTTP
- Properly forwards X-Test-Token via context
- Automatically uses mover when enabled

## How It Works

### Request Flow (with mover)

```
Service Code
    ↓
OrchestratorClient.FetchWorkflowIR(ctx)
    ↓
HTTPClient.DoRequest(ctx, "GET", url, nil)
    ↓
[USE_MOVER=true detected]
    ↓
doRequestViaMover() - extracts context headers
    ↓
MoverCASClient.ProxyHTTP() - serializes to JSON
    ↓
Unix Domain Socket → Mover Service
    ↓
Mover makes HTTP request via io_uring
    ↓
Mover returns response
    ↓
Deserialize → http.Response
    ↓
Return to service code
```

### Request Flow (direct, fallback)

```
Service Code
    ↓
OrchestratorClient.FetchWorkflowIR(ctx)
    ↓
HTTPClient.DoRequest(ctx, "GET", url, nil)
    ↓
[USE_MOVER=false OR mover error]
    ↓
doRequestDirect() - standard http.Client
    ↓
Return http.Response
```

## Usage

### No Code Changes Required!

Services using `OrchestratorClient` automatically benefit:

```go
// This code works with or without mover
client := clients.NewOrchestratorClient(url, logger)
data, err := client.FetchWorkflowIR(ctx, runID)
```

### With Context Headers

```go
ctx := context.Background()
ctx = clients.WithUserID(ctx, "user-123")
ctx = clients.WithTestToken(ctx, "token-456")

// Headers automatically added to HTTP request
data, err := client.FetchWorkflowIR(ctx, runID)
```

## Configuration

Set `USE_MOVER=true` environment variable:

```bash
# In docker-compose.yml
environment:
  USE_MOVER: "true"
  MOVER_SOCKET: "/tmp/mover-workflow.sock"
```

## Testing

### With Mover (will fallback to direct until Rust implementation)
```bash
USE_MOVER=true \
  WORKFLOW_RUNNER_URL=http://localhost:8082 \
  ORCHESTRATOR_URL=http://localhost:8081 \
  go test -v ./perf_tests/workflows/ -run=TestFetchWorkflowsConcurrent
```

### Direct HTTP (current behavior)
```bash
USE_MOVER=false \
  go test -v ./perf_tests/workflows/ -run=TestFetchWorkflowsConcurrent
```

## Benefits

1. **Transparent**: No service code changes needed
2. **Automatic**: Enabled via environment variable
3. **Safe**: Automatic fallback to direct HTTP
4. **Fast**: io_uring optimization when mover is ready
5. **Consistent**: Same client for all orchestrator communication

## Current Status

- ✅ Go client implementation complete
- ✅ Protocol defined and documented
- ✅ Automatic fallback working
- ✅ Context header forwarding working
- ⏳ Rust mover service implementation pending

See `HTTP_PROXY_PROTOCOL.md` for Rust implementation details.

## Logs

When mover is enabled, you'll see:

```
HTTP client will use mover for external calls (io_uring) socket=/tmp/mover-workflow.sock
Routing HTTP request through mover (io_uring) method=GET url=http://orchestrator:8081/...
Mover HTTP proxy failed, falling back to direct error=<mover not implemented yet>
```

Once mover supports OpHTTP:

```
HTTP client will use mover for external calls (io_uring) socket=/tmp/mover-workflow.sock
Routing HTTP request through mover (io_uring) method=GET url=http://orchestrator:8081/...
Mover HTTP proxy succeeded status=200 body_size=352
```

## Performance Expectations

Once the Rust mover implementation is complete, expect:

- **Lower latency**: io_uring avoids syscall overhead
- **Higher throughput**: Batched I/O operations
- **Better CPU usage**: Async I/O without thread blocking
- **Connection pooling**: Reuse connections across requests

## Next Steps

1. Implement OpHTTP handler in Rust mover service (see `HTTP_PROXY_PROTOCOL.md`)
2. Add connection pooling in mover
3. Add HTTP/2 support
4. Add metrics for mover HTTP operations
