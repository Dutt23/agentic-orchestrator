/// Mover Service - Ultra-fast data mover with io_uring and zero-copy
///
/// Provides low-level primitives for Go services:
/// - READ: Zero-copy reads from mmap'd CAS
/// - WRITE: Write-through to CAS
/// - SEND_ZC: Zero-copy network send
/// - RECV: Receive into registered buffers
///
/// Communication: Unix Domain Socket
/// I/O: io_uring for all operations
/// Storage: Memory-mapped CAS files

mod config;
mod connection_pool;
mod dma_pool;
mod glommio_http;
// mod iouring; // Disabled - uses tokio-uring
// mod postgres; // Disabled - tokio-postgres doesn't work with glommio
mod protocol;
mod splice;
mod true_splice;

use anyhow::{Context, Result};
use config::MoverConfig;
use futures_lite::io::{AsyncReadExt, AsyncWriteExt};
use glommio::net::UnixListener;
use glommio::LocalExecutorBuilder;
use protocol::{MoverRequest, MoverResponse, OpCode};
use std::sync::Arc;
use tracing::{debug, error, info, warn};

// PostgresManager placeholder since we disabled it
struct PostgresManager;

fn main() -> Result<()> {
    // Initialize tracing first
    tracing_subscriber::fmt()
        .with_target(false)
        .with_thread_ids(true)
        .init();

    info!("===========================================");
    info!(" Mover Service - Glommio (io_uring)");
    info!("===========================================");

    // Create glommio executor (pure io_uring!)
    LocalExecutorBuilder::default()
        .name("mover-main")
        .spawn(|| async move {
            if let Err(e) = run_mover().await {
                eprintln!("FATAL: Mover failed: {}", e);
                std::process::exit(1);
            }
        })
        .expect("Failed to spawn glommio executor")
        .join()
        .expect("Glommio executor panicked");

    Ok(())
}

async fn run_mover() -> Result<()> {
    // Load and validate configuration
    let config = MoverConfig::load_from_env()
        .and_then(|c| c.validate())
        .context("Failed to load configuration")?;

    info!("Configuration:");
    info!("  Socket: {}", config.socket_path);
    info!("  Database: {}", config.db_url);
    info!("  io_uring entries: {}", config.iouring_entries);
    info!("  io_uring flags: {} ({})", config.iouring_flags, config.flags_description());
    info!("  Buffer pool: {} x {}KB", config.buffer_count, config.buffer_size / 1024);
    info!("  Features: SEND_ZC={}, Huge pages={}", config.enable_send_zc, config.enable_huge_pages);

    // Note: Postgres disabled for now with glommio (would need async-postgres or similar)
    info!("Postgres disabled (not yet supported with glommio)");
    let postgres: Option<Arc<PostgresManager>> = None;

    info!("Using Glommio - pure io_uring for all I/O");
    info!("HTTP requests will use glommio TcpStream (io_uring)");

    // Remove old socket if exists
    let _ = std::fs::remove_file(&config.socket_path);

    // Start Unix socket listener (glommio, io_uring!)
    info!("Starting Unix socket listener on {}", config.socket_path);
    let listener = UnixListener::bind(&config.socket_path)
        .map_err(|e| anyhow::anyhow!("Failed to bind Unix socket: {}", e))?;

    info!("Mover service ready!");
    info!("  Mode: Glommio io_uring (pure io_uring for all I/O)");
    info!("===========================================");

    // Accept connections loop (silent for performance)
    loop {
        match listener.accept().await {
            Ok(stream) => {
                let postgres_clone = postgres.clone();
                // Spawn handler using glommio
                glommio::spawn_local(async move {
                    if let Err(e) = handle_connection(stream, postgres_clone).await {
                        error!("Connection handler error: {}", e);
                    }
                })
                .detach(); // Detach so it runs independently
            }
            Err(e) => {
                error!("Accept error: {}", e);
            }
        }
    }
}

/// Handle a single connection from Go service
/// Keeps connection alive and handles multiple requests (connection pooling)
/// Uses buffer pool for reduced allocation overhead
async fn handle_connection(
    mut stream: glommio::net::UnixStream,
    postgres: Option<Arc<PostgresManager>>,
) -> Result<()> {
    let mut request_count = 0;

    // Handle multiple requests on this connection (keep-alive)
    loop {
        request_count += 1;

        // Read request using DMA buffer (page-aligned for io_uring)
        let mut accumulated_data = Vec::new();

        // Keep reading until we can successfully parse
        let req = loop {
            // Get buffer from pool (amortizes allocation cost)
            let mut read_buf = dma_pool::get_buffer_4k();

            match stream.read(&mut read_buf).await {
                Ok(0) => {
                    // Return buffer to pool before exiting
                    dma_pool::return_buffer_4k(read_buf);

                    // EOF - connection closed by client
                    if accumulated_data.is_empty() {
                        return Ok(());
                    }
                    error!("Incomplete request (EOF after {} bytes)", accumulated_data.len());
                    return Ok(());
                }
                Ok(n) => {
                    accumulated_data.extend_from_slice(&read_buf[..n]);

                    // Return buffer to pool immediately after use
                    dma_pool::return_buffer_4k(read_buf);

                    // Try to parse
                    match MoverRequest::read_from(&mut &accumulated_data[..]) {
                        Ok(req) => {
                            break req; // Successfully parsed!
                        }
                        Err(_e) => {
                            // Need more data

                            if accumulated_data.len() > 1_000_000 {
                                error!("Request too large: {} bytes", accumulated_data.len());
                                return Ok(());
                            }
                        }
                    }
                }
                Err(e) => {
                    error!("Read error: {}", e);
                    return Ok(());
                }
            }
        };

        // Handle HttpSplice - zero-copy socket relay
        // Must happen AFTER parsing request header but works with raw stream
        if req.op == OpCode::HttpSplice {
            if let Ok((host, port)) = parse_splice_target(&req.data) {
                // Note: relay_bidirectional consumes stream, so we return after
                match splice::relay_bidirectional(stream, &host, port).await {
                    Ok((req_bytes, resp_bytes)) => {
                        debug!("Zero-copy relay: {}b → {}b", req_bytes, resp_bytes);
                    }
                    Err(e) => {
                        error!("Splice relay failed: {}", e);
                    }
                }
                return Ok(()); // Stream consumed, exit handler
            } else {
                error!("Invalid splice target");
                return Ok(());
            }
        }

        // Handle other operations (including OpCode::Http with JSON)
        let response = match req.op {
            OpCode::Read => handle_read(&postgres, &req).await,
            OpCode::Write => handle_write(&postgres, &req).await,
            OpCode::SendZC => handle_send_zc(&req).await,
            OpCode::Recv => handle_recv(&req).await,
            OpCode::Batch => handle_batch(&req).await,
            OpCode::Http => handle_http(&req).await,
            OpCode::HttpSplice => unreachable!(), // Handled above
        };

        // Send response
        let mut response_buf = Vec::new();
        if let Err(e) = response.write_to(&mut response_buf) {
            error!("Failed to serialize response: {}", e);
            return Ok(());
        }

        // Write response - just write directly (glommio handles the DMA internally)
        stream.write_all(&response_buf).await
            .map_err(|e| anyhow::anyhow!("Write failed: {}", e))?;

        // Log pool stats every 100 requests
        if request_count % 100 == 0 {
            debug!("Buffer pool stats: {}", dma_pool::pool_stats());
        }
    }
}

/// Parse splice target from request data
/// Format: host:port (e.g. "orchestrator:8081" or "localhost:8082")
fn parse_splice_target(data: &[u8]) -> Result<(String, u16)> {
    let target = String::from_utf8(data.to_vec())
        .map_err(|e| anyhow::anyhow!("Invalid UTF-8 in splice target: {}", e))?;

    let parts: Vec<&str> = target.split(':').collect();
    if parts.len() != 2 {
        return Err(anyhow::anyhow!("Invalid target format, expected host:port"));
    }

    let host = parts[0].to_string();
    let port = parts[1].parse::<u16>()
        .map_err(|e| anyhow::anyhow!("Invalid port: {}", e))?;

    Ok((host, port))
}

/// Handle READ operation - Query Postgres via io_uring (disabled for glommio)
///
/// NOTE: This COULD use splice too! Here's how:
/// 1. Get cas_id from req.id
/// 2. Connect to Postgres (postgres:5432)
/// 3. Send: "SELECT content FROM cas_blob WHERE cas_id = 'xxx'"
/// 4. splice(postgres_socket → unix_socket)  ← ZERO-COPY!
///
/// Same relay_bidirectional() primitive as HTTP!
/// Just forwarding Postgres wire protocol bytes instead of HTTP bytes.
async fn handle_read(_postgres: &Option<Arc<PostgresManager>>, _req: &MoverRequest) -> MoverResponse {
    MoverResponse::error("Postgres not yet supported with glommio".to_string())
}

/// Handle WRITE operation - Write to Postgres via io_uring (disabled for glommio)
async fn handle_write(_postgres: &Option<Arc<PostgresManager>>, _req: &MoverRequest) -> MoverResponse {
    MoverResponse::error("Postgres not yet supported with glommio".to_string())
}

/// Handle SEND_ZC operation (placeholder)
async fn handle_send_zc(_req: &MoverRequest) -> MoverResponse {
    MoverResponse::error("SEND_ZC not implemented".to_string())
}

/// Handle RECV operation (placeholder)
async fn handle_recv(_req: &MoverRequest) -> MoverResponse {
    MoverResponse::error("RECV not implemented".to_string())
}

/// Handle BATCH operation (placeholder)
async fn handle_batch(_req: &MoverRequest) -> MoverResponse {
    MoverResponse::error("BATCH not implemented".to_string())
}

/// Handle HTTP proxy operation - Proxy HTTP requests via io_uring
async fn handle_http(req: &MoverRequest) -> MoverResponse {
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

    // Deserialize the HTTP request from JSON
    let http_req: HttpProxyRequest = match serde_json::from_slice(&req.data) {
        Ok(r) => r,
        Err(e) => {
            error!("Failed to parse HTTP request: {}", e);
            return MoverResponse::error(format!("Invalid HTTP request: {}", e));
        }
    };

    // Execute HTTP request using glommio (PURE io_uring!)
    // No more runtime conflicts - everything uses glommio consistently
    let result = glommio_http::GlommioHttpClient::request(
        &http_req.method,
        &http_req.url,
        http_req.headers,
        http_req.body,
    )
    .await;

    match result {
        Ok((status_code, headers, body)) => {
            let http_resp = HttpProxyResponse {
                status_code,
                headers,
                body,
            };

            match serde_json::to_vec(&http_resp) {
                Ok(data) => MoverResponse::ok(data),
                Err(e) => {
                    error!("Failed to serialize response: {}", e);
                    MoverResponse::error(format!("Serialization failed: {}", e))
                }
            }
        }
        Err(e) => {
            error!("HTTP proxy error (glommio): {}", e);
            MoverResponse::error(e)
        }
    }
}
