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
mod iouring;
mod protocol;

use anyhow::{Context, Result};
use config::MoverConfig;
use iouring::{BufferPool, IoUringPool};
use protocol::{MoverRequest, MoverResponse, OpCode, ResponseStatus};
use tokio_uring::net::UnixListener;
use tracing::{debug, error, info, warn};

#[tokio_uring::main]
async fn main() -> Result<()> {
    // Initialize tracing
    tracing_subscriber::fmt()
        .with_target(false)
        .with_thread_ids(true)
        .init();

    info!("===========================================");
    info!(" Mover Service - io_uring + mmap");
    info!("===========================================");

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

    info!("Initializing io_uring...");
    let iouring = IoUringPool::new(config.iouring_entries, config.iouring_flags)?;

    info!("Initializing buffer pool...");
    let mut buffer_pool = BufferPool::new(config.buffer_count, config.buffer_size);
    buffer_pool.register_with_ring()?;

    // Remove old socket if exists
    let _ = std::fs::remove_file(&config.socket_path);

    // Start Unix socket listener
    info!("Starting Unix socket listener on {}", config.socket_path);
    let listener = UnixListener::bind(&config.socket_path)
        .context("Failed to bind Unix socket")?;

    info!("Mover service ready!");
    info!("  Mode: io_uring I/O optimization (no caching)");
    info!("===========================================");

    // Accept connections
    loop {
        match listener.accept().await {
            Ok((stream, _addr)) => {
                info!("New connection");

                // Spawn handler (tokio-uring task)
                tokio_uring::spawn(handle_connection(stream, &iouring, &mut buffer_pool));
            }
            Err(e) => {
                error!("Accept error: {}", e);
            }
        }
    }
}

/// Handle a single connection from Go service
async fn handle_connection(
    mut stream: tokio_uring::net::UnixStream,
    _iouring: &IoUringPool,
    _buffer_pool: &mut BufferPool,
) -> Result<()> {
    // Read request
    let buffer = vec![0u8; 8192];  // Max request size
    let (result, buffer) = stream.read(buffer).await;
    let n = result.context("Failed to read request")?;

    // Parse request
    let req = MoverRequest::read_from(&mut &buffer[..n])
        .context("Failed to parse request")?;

    debug!("Received request: op={:?}, id_len={}", req.op, req.id.len());

    // Handle request (all via io_uring to Postgres, no caching)
    let response = match req.op {
        OpCode::Read => handle_read(&req).await,
        OpCode::Write => handle_write(&req).await,
        OpCode::SendZC => handle_send_zc(&req).await,
        OpCode::Recv => handle_recv(&req).await,
        OpCode::Batch => handle_batch(&req).await,
    };

    // Send response
    let mut response_buf = Vec::new();
    response.write_to(&mut response_buf)?;

    let (result, _) = stream.write(response_buf).await;
    result.context("Failed to send response")?;

    Ok(())
}

/// Handle READ operation - Query Postgres via io_uring
async fn handle_read(req: &MoverRequest) -> MoverResponse {
    let cas_id = String::from_utf8_lossy(&req.id);

    // TODO: Query Postgres via io_uring
    // For now, return not implemented
    warn!("READ from Postgres not yet implemented for cas_id: {}", cas_id);
    MoverResponse::error("READ not implemented - needs Postgres connection".to_string())
}

/// Handle WRITE operation (placeholder)
async fn handle_write(_req: &MoverRequest) -> MoverResponse {
    // TODO: Implement write-through to CAS
    warn!("WRITE not yet implemented");
    MoverResponse::error("WRITE not implemented".to_string())
}

/// Handle SEND_ZC operation (placeholder)
async fn handle_send_zc(_req: &MoverRequest) -> MoverResponse {
    // TODO: Implement zero-copy send to peer mover
    warn!("SEND_ZC not yet implemented");
    MoverResponse::error("SEND_ZC not implemented".to_string())
}

/// Handle RECV operation (placeholder)
async fn handle_recv(_req: &MoverRequest) -> MoverResponse {
    // TODO: Implement receive from peer
    warn!("RECV not yet implemented");
    MoverResponse::error("RECV not implemented".to_string())
}

/// Handle BATCH operation (placeholder)
async fn handle_batch(_req: &MoverRequest) -> MoverResponse {
    // TODO: Implement batch operations
    warn!("BATCH not yet implemented");
    MoverResponse::error("BATCH not implemented".to_string())
}
