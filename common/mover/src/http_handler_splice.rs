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

        debug!("Reading response headers from upstream...");
        let (header_bytes, content_length, excess_body_bytes, is_chunked) = parse_http_response_headers(&mut upstream).await?;

        debug!(
            "Parsed headers: {}b, Content-Length={:?}, excess={}b, chunked={}",
            header_bytes.len(),
            content_length,
            excess_body_bytes.len(),
            is_chunked
        );

        debug!("Writing headers to client...");

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

        let body_bytes = if is_chunked {
            // Handle chunked encoding
            debug!("Response uses chunked encoding, processing chunks with splice");
            match splice_chunked_response(&mut upstream, client, excess_body_bytes).await {
                Ok(bytes) => bytes,
                Err(e) => {
                    error!("Chunked splice failed: {}", e);
                    return Err(format!("Failed to splice chunked response: {}", e));
                }
            }
        } else if let Some(length) = content_length {
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
            // No Content-Length, no chunked - read until EOF (HTTP/1.0 behavior)
            debug!("No Content-Length or chunked, reading until EOF (HTTP/1.0)");

            // Write any excess bytes from header read
            if !excess_body_bytes.is_empty() {
                let (write_result, _) = client.write_all(excess_body_bytes.clone()).await;
                if let Err(e) = write_result {
                    error!("Failed to write excess bytes: {}", e);
                    return Err(format!("Failed to write excess bytes: {}", e));
                }
            }

            let mut total_read = excess_body_bytes.len();

            // Read until EOF
            loop {
                let buf = dma_pool::get_buffer_4k();
                let (result, buf) = upstream.read(buf).await;

                match result {
                    Ok(0) => {
                        dma_pool::return_buffer_4k(buf);
                        debug!("EOF reached, read {} total bytes", total_read);
                        break; // Normal EOF
                    }
                    Ok(n) => {
                        let data = buf[..n].to_vec();
                        dma_pool::return_buffer_4k(buf);

                        let (write_result, _) = client.write_all(data).await;
                        if let Err(e) = write_result {
                            error!("Failed to write to client: {}", e);
                            return Err(format!("Failed to write to client: {}", e));
                        }

                        total_read += n;
                    }
                    Err(e) => {
                        dma_pool::return_buffer_4k(buf);
                        error!("Read error: {}", e);
                        return Err(format!("Read error: {}", e));
                    }
                }
            }

            total_read
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

    // Request line: METHOD /path HTTP/1.0\r\n
    // Use HTTP/1.0 to implicitly close connection after response
    // This works with both chunked and Content-Length responses
    buf.extend_from_slice(metadata.method.as_bytes());
    buf.push(b' ');
    buf.extend_from_slice(metadata.path.as_bytes());
    buf.extend_from_slice(b" HTTP/1.0\r\n");

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

    // Use HTTP/1.0 to force connection close without explicit header
    // HTTP/1.0 defaults to close after response (no keep-alive)
    // This works better than "Connection: close" with chunked encoding

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

/// Parse HTTP response headers and extract Content-Length or chunked flag
/// Returns: (headers, content_length, excess_body_bytes, is_chunked)
/// The excess_body_bytes are body data that was read along with headers
pub async fn parse_http_response_headers(
    stream: &mut monoio::net::TcpStream,
) -> Result<(Vec<u8>, Option<usize>, Vec<u8>, bool), String> {
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

            // Extract excess body bytes that were read along with headers
            let excess_bytes = if header_buf.len() > header_len {
                header_buf[header_len..].to_vec()
            } else {
                Vec::new()
            };

            // Return headers, content_length, excess body bytes, and chunked flag
            Ok((header_buf[..header_len].to_vec(), content_length, excess_bytes, is_chunked))
        }
        Ok(httparse::Status::Partial) => Err("Incomplete headers (partial parse)".to_string()),
        Err(e) => Err(format!("Failed to parse headers: {}", e)),
    }
}

/// Read until we find \r\n, return the line including \r\n
async fn read_until_crlf(stream: &mut monoio::net::TcpStream) -> Result<Vec<u8>, String> {
    let mut line_buf = Vec::new();
    let max_line_size = 1024; // Max 1KB for chunk size line

    loop {
        let read_buf = dma_pool::get_buffer_4k();
        let (result, buf) = stream.read(read_buf).await;

        match result {
            Ok(0) => {
                dma_pool::return_buffer_4k(buf);
                return Err("Unexpected EOF while reading chunk line".to_string());
            }
            Ok(n) => {
                line_buf.extend_from_slice(&buf[..n]);
                dma_pool::return_buffer_4k(buf);

                // Check for \r\n
                if line_buf.windows(2).any(|w| w == b"\r\n") {
                    break;
                }

                if line_buf.len() > max_line_size {
                    return Err(format!("Chunk line too large: {} bytes", line_buf.len()));
                }
            }
            Err(e) => {
                dma_pool::return_buffer_4k(buf);
                return Err(format!("Failed to read chunk line: {}", e));
            }
        }
    }

    Ok(line_buf)
}

/// Read exact number of bytes from stream
async fn read_exact_bytes(stream: &mut monoio::net::TcpStream, count: usize) -> Result<Vec<u8>, String> {
    let mut result_buf = Vec::with_capacity(count);
    let mut remaining = count;

    while remaining > 0 {
        let read_buf = dma_pool::get_buffer_4k();
        let (result, buf) = stream.read(read_buf).await;

        match result {
            Ok(0) => {
                dma_pool::return_buffer_4k(buf);
                return Err(format!(
                    "Unexpected EOF: read {} bytes, expected {}",
                    result_buf.len(),
                    count
                ));
            }
            Ok(n) => {
                let to_copy = n.min(remaining);
                result_buf.extend_from_slice(&buf[..to_copy]);
                dma_pool::return_buffer_4k(buf);
                remaining -= to_copy;
            }
            Err(e) => {
                dma_pool::return_buffer_4k(buf);
                return Err(format!("Failed to read: {}", e));
            }
        }
    }

    Ok(result_buf)
}

/// Parse hex chunk size from line like "5a3\r\n"
fn parse_chunk_size(line: &[u8]) -> Result<usize, String> {
    // Remove \r\n if present
    let line = if line.ends_with(b"\r\n") {
        &line[..line.len() - 2]
    } else {
        line
    };

    let size_str = String::from_utf8_lossy(line);
    let size_str = size_str.trim();

    // Parse hex
    usize::from_str_radix(size_str, 16)
        .map_err(|e| format!("Invalid chunk size '{}': {}", size_str, e))
}

/// Handle chunked encoding response with zero-copy splice for chunk bodies
/// Returns total bytes transferred (including chunk metadata)
async fn splice_chunked_response(
    upstream: &mut monoio::net::TcpStream,
    client: &mut monoio::net::UnixStream,
    initial_data: Vec<u8>,
) -> Result<usize, String> {
    let mut total_bytes = 0;
    let mut buffer = initial_data;

    loop {
        // 1. Try to read chunk size line from buffer first
        let (chunk_size, size_line_len) = if let Some(crlf_pos) = buffer.windows(2).position(|w| w == b"\r\n") {
            // We have a complete line in buffer
            let line = &buffer[..crlf_pos + 2];
            let size = parse_chunk_size(line)?;
            (size, crlf_pos + 2)
        } else {
            // Need to read more data
            let line = read_until_crlf(upstream).await?;
            let size = parse_chunk_size(&line)?;

            // Write size line to client
            let (write_result, _) = client.write_all(line.clone()).await;
            if let Err(e) = write_result {
                return Err(format!("Failed to write chunk size: {}", e));
            }

            total_bytes += line.len();
            (size, 0) // size_line_len = 0 means we already wrote it
        };

        // If we read size from buffer, write it to client
        if size_line_len > 0 {
            let size_line = buffer[..size_line_len].to_vec();
            let (write_result, _) = client.write_all(size_line).await;
            if let Err(e) = write_result {
                return Err(format!("Failed to write chunk size from buffer: {}", e));
            }
            total_bytes += size_line_len;
            buffer = buffer[size_line_len..].to_vec();
        }

        // 2. Check for final chunk (size = 0)
        if chunk_size == 0 {
            debug!("Final chunk received (size=0)");

            // Read final \r\n (end of chunked message)
            let final_crlf = read_exact_bytes(upstream, 2).await?;
            let (write_result, _) = client.write_all(final_crlf).await;
            if let Err(e) = write_result {
                return Err(format!("Failed to write final CRLF: {}", e));
            }
            total_bytes += 2;
            break;
        }

        debug!("Processing chunk: {} bytes", chunk_size);

        // 3. Transfer chunk data - check if we have it in buffer first
        let mut chunk_transferred = 0;

        if !buffer.is_empty() {
            let from_buffer = buffer.len().min(chunk_size);
            let data = buffer[..from_buffer].to_vec();
            let (write_result, _) = client.write_all(data).await;
            if let Err(e) = write_result {
                return Err(format!("Failed to write chunk data from buffer: {}", e));
            }
            chunk_transferred += from_buffer;
            buffer = buffer[from_buffer..].to_vec();
            total_bytes += from_buffer;
        }

        // 4. Splice remaining chunk data if any
        let remaining_chunk = chunk_size - chunk_transferred;
        if remaining_chunk > 0 {
            match async_splice::splice_exact_bytes_async(upstream, client, remaining_chunk).await {
                Ok(bytes) => {
                    debug!("Spliced chunk body: {}b", bytes);
                    total_bytes += bytes;
                }
                Err(e) => {
                    warn!("Chunk splice failed, falling back to buffered for remaining {}b: {}", remaining_chunk, e);
                    // Buffered fallback for this chunk
                    let chunk_data = read_exact_bytes(upstream, remaining_chunk).await?;
                    let (write_result, _) = client.write_all(chunk_data).await;
                    if let Err(e) = write_result {
                        return Err(format!("Failed to write chunk data: {}", e));
                    }
                    total_bytes += remaining_chunk;
                }
            }
        }

        // 5. Read and forward chunk trailing \r\n
        let delimiter = read_exact_bytes(upstream, 2).await?;
        let (write_result, _) = client.write_all(delimiter).await;
        if let Err(e) = write_result {
            return Err(format!("Failed to write chunk delimiter: {}", e));
        }
        total_bytes += 2;
    }

    debug!("Chunked response complete: {} total bytes", total_bytes);
    Ok(total_bytes)
}
