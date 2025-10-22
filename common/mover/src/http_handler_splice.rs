/// Splice HTTP Handler - Zero-Copy with HTTP Header Parsing
///
/// This implementation uses Linux splice() syscall for true zero-copy transfer.
/// It adds "Connection: close" to force the upstream to close after response,
/// then uses splice() to transfer the response body without userspace copies.
///
/// Performance: Best for large responses (>1MB)
/// Memory: Zero copies for response body
/// Compatibility: Requires Content-Length header, doesn't support chunked encoding
///
/// Trade-offs:
/// - ✅ True zero-copy for response body
/// - ✅ No memory allocations for body transfer
/// - ❌ Requires new TCP connection per request (no keep-alive)
/// - ❌ Small overhead for header parsing
/// - ❌ Doesn't work with chunked encoding

use crate::async_splice;
use crate::dma_pool;
use crate::http_handler::HttpHandlerResult;
use monoio::io::{AsyncReadRent, AsyncWriteRentExt};
use std::time::Instant;
use tracing::{debug, error, warn};

use crate::protocol::HttpMetadata;

pub struct SpliceHttpHandler;

impl SpliceHttpHandler {
    pub async fn handle(
        &self,
        mut upstream: monoio::net::TcpStream,
        client: &mut monoio::net::UnixStream,
        metadata: HttpMetadata,
    ) -> Result<HttpHandlerResult, String> {
        let start = Instant::now();

        // Build HTTP request from metadata (headers only, body comes from socket)
        let http_headers = build_http_headers(&metadata)?;
        let request_size = http_headers.len();

        let write_start = Instant::now();

        // Write HTTP headers to upstream
        let (write_result, _) = upstream.write_all(http_headers).await;
        if let Err(e) = write_result {
            error!("Failed to write request headers to upstream: {}", e);
            return Err(format!("Failed to write request headers: {}", e));
        }

        // Splice request body from unix socket to TCP if present
        if metadata.content_length > 0 {
            debug!("Splicing request body: {} bytes from unix → TCP", metadata.content_length);
            match splice_request_body(client, &mut upstream, metadata.content_length).await {
                Ok(bytes) => {
                    debug!("Spliced request body: {} bytes", bytes);
                }
                Err(e) => {
                    error!("Failed to splice request body: {}", e);
                    return Err(format!("Failed to splice request body: {}", e));
                }
            }
        }

        let write_time = write_start.elapsed();

        let (header_bytes, content_length, excess_body_bytes) = parse_http_response_headers(&mut upstream).await?;

        debug!(
            "Parsed headers: {}b, Content-Length={:?}, excess={}b",
            header_bytes.len(),
            content_length,
            excess_body_bytes.len()
        );

        let (write_result, _) = client.write_all(header_bytes.clone()).await;
        if let Err(e) = write_result {
            error!("Failed to write headers to client: {}", e);
            return Err(format!("Failed to write headers to client: {}", e));
        }

        let mut body_bytes_written = 0;
        if !excess_body_bytes.is_empty() {
            debug!("Writing {}b excess body bytes from header read", excess_body_bytes.len());
            let (write_result, _) = client.write_all(excess_body_bytes.clone()).await;
            if let Err(e) = write_result {
                error!("Failed to write excess body bytes: {}", e);
                return Err(format!("Failed to write excess body bytes: {}", e));
            }
            body_bytes_written = excess_body_bytes.len();
        }

        let body_bytes = if let Some(length) = content_length {
            let remaining_bytes = length.saturating_sub(body_bytes_written);
            if remaining_bytes > 0 {
                debug!("Splicing remaining {}b ({}b already written)", remaining_bytes, body_bytes_written);

                match async_splice::splice_exact_bytes_async(&mut upstream, client, remaining_bytes).await {
                    Ok(bytes) => {
                        debug!("Spliced {}b successfully", bytes);
                        body_bytes_written + bytes
                    }
                    Err(e) => {
                        warn!("Splice failed ({}b remaining), falling back to buffered: {}", remaining_bytes, e);

                        let mut total_read = body_bytes_written;
                        let mut remaining = remaining_bytes;

                        while remaining > 0 {
                            let buf = dma_pool::get_buffer_4k();

                            let (result, buf) = upstream.read(buf).await;
                            match result {
                                Ok(0) => {
                                    dma_pool::return_buffer_4k(buf);
                                    error!("Buffered fallback EOF: got {} bytes, expected {} total", total_read, length);
                                    return Err(format!(
                                        "Buffered fallback: Unexpected EOF after {} bytes (expected {} total, {} remaining)",
                                        total_read, length, remaining
                                    ));
                                }
                            Ok(n) => {
                                let data = buf[..n].to_vec();
                                dma_pool::return_buffer_4k(buf);

                                let (write_result, _) = client.write_all(data).await;
                                if let Err(e) = write_result {
                                    error!("Buffered fallback write failed: {}", e);
                                    return Err(format!("Buffered fallback: Write failed: {}", e));
                                }

                                total_read += n;
                                remaining = remaining.saturating_sub(n);
                            }
                            Err(e) => {
                                dma_pool::return_buffer_4k(buf);
                                error!("Buffered fallback read failed: {}", e);
                                return Err(format!("Buffered fallback: Read failed: {}", e));
                            }
                        }
                    }

                        debug!("Buffered fallback succeeded: {}b total", total_read);
                        total_read
                    }
                }
            } else {
                debug!("All {}b body bytes read with headers", body_bytes_written);
                body_bytes_written
            }
        } else {
            warn!("No Content-Length header, cannot use splice mode");
            return Err("No Content-Length header".to_string());
        };

        let total_time = start.elapsed();
        let transfer_time = total_time - write_time;

        debug!("Splice handler completed: {}b in {:?}",
               request_size + header_bytes.len() + body_bytes,
               total_time);

        Ok(HttpHandlerResult {
            bytes_transferred: request_size + header_bytes.len() + body_bytes,
            duration: total_time,
            method: "splice".to_string(),
            chunks: 0,
            connect_time: std::time::Duration::ZERO,
            write_time,
            transfer_time,
        })
    }
}

/// Build HTTP request headers from metadata
/// Returns the complete HTTP request (request line + headers + \r\n\r\n)
/// Body is NOT included - it will be spliced separately
fn build_http_headers(metadata: &HttpMetadata) -> Result<Vec<u8>, String> {
    let mut buf = Vec::new();

    // Request line: METHOD /path HTTP/1.1\r\n
    buf.extend_from_slice(metadata.method.as_bytes());
    buf.push(b' ');
    buf.extend_from_slice(metadata.path.as_bytes());
    buf.extend_from_slice(b" HTTP/1.1\r\n");

    // Host header (required for HTTP/1.1)
    buf.extend_from_slice(b"Host: ");
    buf.extend_from_slice(metadata.host.as_bytes());
    buf.extend_from_slice(b"\r\n");

    // Custom headers from metadata
    for (key, value) in &metadata.headers {
        buf.extend_from_slice(key.as_bytes());
        buf.extend_from_slice(b": ");
        buf.extend_from_slice(value.as_bytes());
        buf.extend_from_slice(b"\r\n");
    }

    // Content-Length if body present
    if metadata.content_length > 0 {
        buf.extend_from_slice(b"Content-Length: ");
        buf.extend_from_slice(metadata.content_length.to_string().as_bytes());
        buf.extend_from_slice(b"\r\n");
    }

    // Force connection close (so we know when response is complete)
    buf.extend_from_slice(b"Connection: close\r\n");

    // End of headers
    buf.extend_from_slice(b"\r\n");

    Ok(buf)
}

/// Splice request body from unix socket to TCP stream
/// Returns number of bytes spliced
/// Note: The splice function signature expects (TcpStream, UnixStream, bytes)
/// but we're reading from Unix and writing to TCP, so we can't use splice directly
/// for this direction. We'll use a manual copy with buffers.
async fn splice_request_body(
    unix_sock: &mut monoio::net::UnixStream,
    tcp_sock: &mut monoio::net::TcpStream,
    content_length: u64,
) -> Result<usize, String> {
    // Manual copy with buffering (splice doesn't support Unix→TCP direction in our impl)
    // TODO: Implement true splice for this direction
    let mut total_written = 0;
    let mut remaining = content_length as usize;

    while remaining > 0 {
        let buf = dma_pool::get_buffer_4k();

        let (result, buf) = unix_sock.read(buf).await;
        match result {
            Ok(0) => {
                dma_pool::return_buffer_4k(buf);
                return Err(format!(
                    "Unexpected EOF while reading request body: got {} bytes, expected {}",
                    total_written, content_length
                ));
            }
            Ok(n) => {
                let data = buf[..n].to_vec();
                dma_pool::return_buffer_4k(buf);

                let (write_result, _) = tcp_sock.write_all(data).await;
                if let Err(e) = write_result {
                    return Err(format!("Failed to write request body to TCP: {}", e));
                }

                total_written += n;
                remaining = remaining.saturating_sub(n);
            }
            Err(e) => {
                dma_pool::return_buffer_4k(buf);
                return Err(format!("Failed to read request body from unix socket: {}", e));
            }
        }
    }

    debug!("Buffered copy of request body succeeded: {} bytes", total_written);
    Ok(total_written)
}

/// Parse HTTP response headers and extract Content-Length
/// Returns: (headers, content_length, excess_body_bytes)
/// The excess_body_bytes are body data that was read along with headers
pub async fn parse_http_response_headers(
    stream: &mut monoio::net::TcpStream,
) -> Result<(Vec<u8>, Option<usize>, Vec<u8>), String> {
    let mut header_buf = Vec::new();
    let max_header_size = 16384; // 16KB max headers

    // Read until we find "\r\n\r\n" (end of headers)
    loop {
        let read_buf = dma_pool::get_buffer_4k();
        let (result, buf) = stream.read(read_buf).await;

        match result {
            Ok(0) => {
                dma_pool::return_buffer_4k(buf);
                return Err("Unexpected EOF while reading headers".to_string());
            }
            Ok(n) => {
                header_buf.extend_from_slice(&buf[..n]);
                dma_pool::return_buffer_4k(buf);

                // Check if we have complete headers
                if header_buf.windows(4).any(|w| w == b"\r\n\r\n") {
                    break;
                }

                if header_buf.len() > max_header_size {
                    return Err(format!(
                        "Headers too large: {} bytes",
                        header_buf.len()
                    ));
                }
            }
            Err(e) => {
                dma_pool::return_buffer_4k(buf);
                return Err(format!("Failed to read headers: {}", e));
            }
        }
    }

    // Parse headers with httparse
    let mut headers = [httparse::EMPTY_HEADER; 64];
    let mut response = httparse::Response::new(&mut headers);

    match response.parse(&header_buf) {
        Ok(httparse::Status::Complete(header_len)) => {
            // Extract Content-Length
            let content_length = response
                .headers
                .iter()
                .find(|h| h.name.eq_ignore_ascii_case("Content-Length"))
                .and_then(|h| std::str::from_utf8(h.value).ok())
                .and_then(|v| v.parse::<usize>().ok());

            // Check for chunked encoding
            let is_chunked = response
                .headers
                .iter()
                .find(|h| h.name.eq_ignore_ascii_case("Transfer-Encoding"))
                .and_then(|h| std::str::from_utf8(h.value).ok())
                .map(|v| v.to_lowercase().contains("chunked"))
                .unwrap_or(false);

            if is_chunked {
                return Err(
                    "Chunked encoding not supported with splice (use buffered mode)"
                        .to_string(),
                );
            }

            // Extract excess body bytes that were read along with headers
            let excess_bytes = if header_buf.len() > header_len {
                header_buf[header_len..].to_vec()
            } else {
                Vec::new()
            };

            // Return headers, content_length, and any excess body bytes
            Ok((header_buf[..header_len].to_vec(), content_length, excess_bytes))
        }
        Ok(httparse::Status::Partial) => Err("Incomplete headers (partial parse)".to_string()),
        Err(e) => Err(format!("Failed to parse headers: {}", e)),
    }
}
