# Mover HTTP Proxy Protocol

## Overview

The Go client now supports routing HTTP requests through the mover service for io_uring optimization. This document describes the protocol that needs to be implemented on the Rust mover side.

## OpCode

```rust
const OP_HTTP: u8 = 0x06;
```

## Request Format

When the Go client sends an HTTP proxy request, it uses the standard mover protocol with `OpCode = 0x06 (OpHTTP)`.

The `Data` field contains a JSON payload:

```json
{
  "method": "GET",
  "url": "http://orchestrator:8081/api/v1/test/fetch-workflow/test-123",
  "headers": {
    "X-User-ID": "user-123",
    "X-Test-Token": "token-456",
    "X-Internal-Service": "test"
  },
  "body": null
}
```

## Response Format

The mover should return a standard mover response with `Status = 0x00` (success) and the `Data` field containing a JSON payload:

```json
{
  "status_code": 200,
  "headers": {
    "Content-Type": "application/json",
    "Content-Length": "352"
  },
  "body": <raw bytes>
}
```

## Rust Implementation Steps

### 1. Add OpCode

```rust
// src/protocol.rs or src/main.rs
const OP_READ: u8 = 0x01;
const OP_WRITE: u8 = 0x02;
const OP_SENDZC: u8 = 0x03;
const OP_RECV: u8 = 0x04;
const OP_BATCH: u8 = 0x05;
const OP_HTTP: u8 = 0x06;  // Add this
```

### 2. Define HTTP Request/Response Structs

```rust
use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Deserialize)]
struct HttpProxyRequest {
    method: String,
    url: String,
    headers: HashMap<String, String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    body: Option<Vec<u8>>,
}

#[derive(Serialize)]
struct HttpProxyResponse {
    status_code: u16,
    headers: HashMap<String, String>,
    body: Vec<u8>,
}
```

### 3. Handle OpHTTP in Main Loop

```rust
// In your command handler
OP_HTTP => {
    let request: HttpProxyRequest = serde_json::from_slice(&data)?;

    // Make HTTP request using io_uring
    let response = handle_http_proxy(ring, request).await?;

    // Serialize response
    let response_data = serde_json::to_vec(&response)?;

    // Send back through mover response protocol
    send_response(stream, 0x00, response_data).await?;
}
```

### 4. Implement HTTP Request Handler

```rust
async fn handle_http_proxy(
    ring: &IoUring,
    request: HttpProxyRequest,
) -> Result<HttpProxyResponse> {
    // Parse URL
    let url = Url::parse(&request.url)?;

    // Connect to host using io_uring
    let socket = connect_with_uring(ring, url.host_str().unwrap(), url.port().unwrap_or(80)).await?;

    // Build HTTP request
    let mut http_request = format!(
        "{} {} HTTP/1.1\r\nHost: {}\r\n",
        request.method,
        url.path(),
        url.host_str().unwrap()
    );

    // Add headers
    for (key, value) in request.headers {
        http_request.push_str(&format!("{}: {}\r\n", key, value));
    }

    // Add body if present
    if let Some(body) = request.body {
        http_request.push_str(&format!("Content-Length: {}\r\n\r\n", body.len()));
        // Append body bytes
    } else {
        http_request.push_str("\r\n");
    }

    // Send request via io_uring
    send_with_uring(ring, &socket, http_request.as_bytes()).await?;

    // Receive response via io_uring
    let response_bytes = recv_with_uring(ring, &socket).await?;

    // Parse HTTP response
    let (status_code, headers, body) = parse_http_response(&response_bytes)?;

    Ok(HttpProxyResponse {
        status_code,
        headers,
        body,
    })
}
```

## Benefits

1. **Zero-copy I/O**: Uses io_uring for HTTP requests
2. **Connection pooling**: Can reuse connections across requests
3. **Transparent**: Service code doesn't know it's using mover
4. **Fallback**: Automatically falls back to direct HTTP if mover fails

## Testing

Once implemented, the Go tests will automatically use the mover HTTP proxy when `USE_MOVER=true`:

```bash
USE_MOVER=true go test -v ./perf_tests/workflows/
```

## Current Status

- ✅ Go client implementation complete
- ⏳ Rust mover service implementation pending
- ✅ Automatic fallback to direct HTTP working

## Notes

- The Go client will gracefully fall back to direct HTTP if mover returns an error
- HTTP response headers from the actual server can be preserved and forwarded
- Consider implementing connection pooling in the mover for better performance
