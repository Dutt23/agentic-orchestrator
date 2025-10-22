/// Server and connection handling logic for mover service

use crate::config::MoverConfig;
use crate::dma_pool;
use crate::handlers::{self, PostgresManager};
use crate::http_handler_splice::SpliceHttpHandler;
use crate::protocol::{MoverRequest, MoverResponse, OpCode};
use anyhow::{Context, Result};
use monoio::io::{AsyncReadRent, AsyncWriteRentExt};
use monoio::net::UnixListener;
use std::sync::Arc;
use tracing::{debug, error, info};

/// Main server entry point - starts the mover service
pub async fn run_mover() -> Result<()> {
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
    info!("  HTTP handler mode: {}", config.http_handler_mode);

    // Note: Postgres disabled for now with glommio (would need async-postgres or similar)
    info!("Postgres disabled (not yet supported with glommio)");
    let postgres: Option<Arc<PostgresManager>> = None;

    info!("Using Monoio - pure io_uring with send_zc support");
    info!("HTTP requests will use monoio TcpStream (true zero-copy)");

    // Remove old socket if exists
    let _ = std::fs::remove_file(&config.socket_path);

    // Start Unix socket listener (monoio, io_uring!)
    info!("Starting Unix socket listener on {}", config.socket_path);
    let listener = UnixListener::bind(&config.socket_path)
        .map_err(|e| anyhow::anyhow!("Failed to bind Unix socket: {}", e))?;

    // Share config across connections
    let config = Arc::new(config);

    info!("Mover service ready!");
    info!("  Mode: Monoio io_uring (true zero-copy with send_zc)");
    info!("===========================================");

    // Accept connections loop (silent for performance)
    loop {
        match listener.accept().await {
            Ok((stream, _addr)) => {
                let postgres_clone = postgres.clone();
                let config_clone = config.clone();
                // Spawn handler using monoio
                monoio::spawn(async move {
                    if let Err(e) = handle_connection(stream, postgres_clone, config_clone).await {
                        error!("Connection handler error: {}", e);
                    }
                });
            }
            Err(e) => {
                error!("Accept error: {}", e);
            }
        }
    }
}

/// Handle HTTP splice request using pluggable handler
async fn handle_http_splice_request(
    config: &MoverConfig,
    client: &mut monoio::net::UnixStream,
    request_data: &[u8],
) -> Result<()> {
    use crate::protocol::parse_http_metadata;

    // Parse metadata from JSON
    let metadata = parse_http_metadata(request_data)
        .map_err(|e| anyhow::anyhow!("Failed to parse HTTP metadata: {}", e))?;

    let addr = format!("{}:{}", metadata.host, metadata.port);
    let upstream = monoio::net::TcpStream::connect(&addr).await
        .map_err(|e| anyhow::anyhow!("Connect failed {}:{}: {}", metadata.host, metadata.port, e))?;

    // Always use splice mode with the new streaming protocol
    // The old buffered mode is not compatible with the new metadata-based protocol
    let handler = SpliceHttpHandler;
    let result = handler.handle(upstream, client, metadata).await
        .map_err(|e| anyhow::anyhow!("Splice handler: {}", e))?;

    debug!("{} handler: {}b in {:?}",
           result.method, result.bytes_transferred, result.duration);

    Ok(())
}

/// Handle a single connection from Go service
/// Keeps connection alive and handles multiple requests (connection pooling)
/// Uses monoio's IoBuf pattern for proper buffer ownership
async fn handle_connection(
    mut stream: monoio::net::UnixStream,
    postgres: Option<Arc<PostgresManager>>,
    config: Arc<MoverConfig>,
) -> Result<()> {
    let mut request_count = 0;

    // Handle multiple requests on this connection (keep-alive)
    loop {
        request_count += 1;

        // Read request using monoio's IoBuf pattern (safe buffer ownership!)
        let mut accumulated_data = Vec::new();

        // Keep reading until we can successfully parse
        let req = loop {
            // Get buffer from pool (monoio will take ownership during I/O)
            let read_buf = dma_pool::get_buffer_4k();

            // Monoio's read pattern: takes ownership, returns (result, buf)
            let (result, buf) = stream.read(read_buf).await;

            match result {
                Ok(0) => {
                    // Return buffer to pool before exiting (monoio returned it to us!)
                    dma_pool::return_buffer_4k(buf);

                    // EOF - connection closed by client
                    if accumulated_data.is_empty() {
                        return Ok(());
                    }
                    error!("Incomplete request (EOF after {} bytes)", accumulated_data.len());
                    return Ok(());
                }
                Ok(n) => {
                    // Monoio returns buffer to us - extract data
                    accumulated_data.extend_from_slice(&buf[..n]);

                    // Return buffer to pool immediately after use
                    dma_pool::return_buffer_4k(buf);

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

        // Handle HttpSplice - Pluggable handler (buffered or splice)
        if req.op == OpCode::HttpSplice {
            match handle_http_splice_request(&config, &mut stream, &req.data).await {
                Ok(()) => {
                    return Ok(());
                }
                Err(e) => {
                    error!("HttpSplice failed: {}", e);
                    return Ok(());
                }
            }
        }

        // Handle other operations (including OpCode::Http with JSON)
        let response = match req.op {
            OpCode::Read => handlers::handle_read(&postgres, &req).await,
            OpCode::Write => handlers::handle_write(&postgres, &req).await,
            OpCode::SendZC => handlers::handle_send_zc(&req).await,
            OpCode::Recv => handlers::handle_recv(&req).await,
            OpCode::Batch => handlers::handle_batch(&req).await,
            OpCode::Http => handlers::handle_http(&req).await,
            OpCode::HttpSplice => unreachable!(), // Handled above
        };

        // Send response
        let mut response_buf = Vec::new();
        if let Err(e) = response.write_to(&mut response_buf) {
            error!("Failed to serialize response: {}", e);
            return Ok(());
        }

        // Write response using monoio (takes ownership of buffer)
        let (result, _buf) = stream.write_all(response_buf).await;
        result.map_err(|e| anyhow::anyhow!("Write failed: {}", e))?;

        // Log pool stats every 100 requests
        if request_count % 100 == 0 {
            debug!("Buffer pool stats: {}", dma_pool::pool_stats());
        }
    }
}
