/// Buffered HTTP Handler
///
/// Uses monoio's async buffered I/O with io_uring under the hood.
/// This is the safe, reliable approach that works with HTTP keep-alive.
///
/// Performance: Good (io_uring provides async I/O)
/// Memory: One copy per chunk (kernel → userspace → kernel)
/// Compatibility: Works with all HTTP responses (chunked, keep-alive, etc.)

use crate::dma_pool;
use crate::http_handler::HttpHandlerResult;
use monoio::io::{AsyncReadRent, AsyncWriteRentExt};
use std::time::{Duration, Instant};
use tracing::{debug, error};

pub struct BufferedHttpHandler;

impl BufferedHttpHandler {
    pub async fn handle(
        &self,
        mut upstream: monoio::net::TcpStream,
        client: &mut monoio::net::UnixStream,
        http_request: Vec<u8>,
    ) -> Result<HttpHandlerResult, String> {
        let start = Instant::now();

        let modified_request = add_connection_close(http_request)?;
        let request_size = modified_request.len();

        let write_start = Instant::now();
        let (write_result, _) = upstream.write_all(modified_request).await;
        if let Err(e) = write_result {
            error!("Failed to write request to upstream: {}", e);
            return Err(format!("Failed to write request to upstream: {}", e));
        }
        let write_time = write_start.elapsed();

        let transfer_start = Instant::now();
        let mut total_bytes = 0;
        let mut chunk_count = 0;
        let read_timeout = Duration::from_millis(100);

        // Simple read loop until EOF (Connection: close ensures EOF)
        loop {
            let read_buf = dma_pool::get_buffer_4k();

            let read_result = match monoio::time::timeout(read_timeout, upstream.read(read_buf)).await {
                Ok((result, buf)) => (result, buf),
                Err(_) => {
                    error!("Read timeout");
                    return Err(format!("Read timeout after {:?}", read_timeout));
                }
            };

            let (read_result, buf) = read_result;

            match read_result {
                Ok(0) => {
                    // EOF - response complete (Connection: close)
                    dma_pool::return_buffer_4k(buf);
                    break;
                }
                Ok(n) => {
                    chunk_count += 1;
                    total_bytes += n;

                    let data = buf[..n].to_vec();
                    dma_pool::return_buffer_4k(buf);

                    let (write_result, _) = client.write_all(data).await;
                    if let Err(e) = write_result {
                        error!("Failed to write to client: {}", e);
                        return Err(format!("Write failed: {}", e));
                    }
                }
                Err(e) => {
                    dma_pool::return_buffer_4k(buf);
                    error!("Read error: {}", e);
                    return Err(format!("Read error: {}", e));
                }
            }
        }

        let transfer_time = transfer_start.elapsed();
        let total_time = start.elapsed();

        debug!("Buffered handler: {}b in {} chunks, {:?}",
               total_bytes, chunk_count, total_time);

        Ok(HttpHandlerResult {
            bytes_transferred: request_size + total_bytes,
            duration: total_time,
            method: "buffered".to_string(),
            chunks: chunk_count,
            connect_time: std::time::Duration::ZERO,
            write_time,
            transfer_time,
        })
    }
}

/// Add "Connection: close" header to HTTP request
/// This forces the server to close the connection after the response
fn add_connection_close(request: Vec<u8>) -> Result<Vec<u8>, String> {
    let request_str = String::from_utf8_lossy(&request);

    // Check if Connection header already exists
    let lines: Vec<&str> = request_str.lines().collect();
    let has_connection_header = lines
        .iter()
        .skip(1) // Skip request line
        .any(|line| line.to_lowercase().starts_with("connection:"));

    if has_connection_header {
        // Replace existing Connection header
        let modified = request_str
            .lines()
            .map(|line| {
                if line.to_lowercase().starts_with("connection:") {
                    "Connection: close"
                } else {
                    line
                }
            })
            .collect::<Vec<_>>()
            .join("\r\n");
        return Ok(modified.into_bytes());
    }

    // Find insertion point (before final \r\n\r\n)
    if let Some(pos) = request_str.find("\r\n\r\n") {
        let mut modified = String::from(&request_str[..pos]);
        modified.push_str("\r\nConnection: close\r\n\r\n");
        if pos + 4 < request_str.len() {
            // Append body if present
            modified.push_str(&request_str[pos + 4..]);
        }
        Ok(modified.into_bytes())
    } else {
        Err("Invalid HTTP request: no header terminator found".to_string())
    }
}
