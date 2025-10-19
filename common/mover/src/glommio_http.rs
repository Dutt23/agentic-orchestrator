/// HTTP client using glommio (pure io_uring)
/// Glommio is DataDog's production-tested io_uring runtime
/// Uses HTTP/1.1 keep-alive with connection pooling for maximum performance

use crate::connection_pool;
use futures_lite::io::{AsyncReadExt, AsyncWriteExt};
use glommio::net::TcpStream;
use std::collections::HashMap;
use std::time::Instant;

pub struct GlommioHttpClient;

impl GlommioHttpClient {
    /// Make an HTTP request using pure io_uring via glommio with DMA buffers
    /// Returns: (status_code, headers, body)
    pub async fn request(
        method: &str,
        url: &str,
        headers: HashMap<String, String>,
        body: Option<Vec<u8>>,
    ) -> Result<(u16, HashMap<String, String>, Vec<u8>), String> {
        let total_start = Instant::now();

        // Parse URL
        let parse_start = Instant::now();
        let url = url::Url::parse(url).map_err(|e| format!("Invalid URL: {}", e))?;

        let host = url.host_str().ok_or("No host in URL")?;
        let port = url.port().unwrap_or(if url.scheme() == "https" { 443 } else { 80 });
        let path = if url.path().is_empty() { "/" } else { url.path() };

        if url.scheme() == "https" {
            return Err("HTTPS not yet supported".to_string());
        }
        let parse_elapsed = parse_start.elapsed();

        // Try to get connection from pool (avoids TCP handshake!)
        let connect_start = Instant::now();
        let addr = format!("{}:{}", host, port);
        let mut is_pooled = false;

        let mut stream = if let Some(pooled_stream) = connection_pool::get_connection(host, port) {
            // Reused connection - no TCP handshake needed!
            is_pooled = true;
            pooled_stream
        } else {
            // No pooled connection - create new one
            TcpStream::connect(&addr)
                .await
                .map_err(|e| format!("Connection failed: {}", e))?
        };
        let connect_elapsed = connect_start.elapsed();

        // Build HTTP request
        let mut request = format!("{} {} HTTP/1.1\r\n", method, path);
        request.push_str(&format!("Host: {}\r\n", host));

        for (key, value) in headers {
            request.push_str(&format!("{}: {}\r\n", key, value));
        }

        if let Some(ref body_data) = body {
            request.push_str(&format!("Content-Length: {}\r\n", body_data.len()));
        }

        // HTTP/1.1 keep-alive for connection reuse (eliminates TCP handshake!)
        request.push_str("Connection: keep-alive\r\n");
        request.push_str("\r\n");

        // Write request (glommio uses io_uring internally)
        let write_start = Instant::now();
        let request_bytes = request.into_bytes();

        stream
            .write_all(&request_bytes)
            .await
            .map_err(|e| format!("Write failed: {}", e))?;

        // Write body if present
        if let Some(body_data) = body {
            stream
                .write_all(&body_data)
                .await
                .map_err(|e| format!("Body write failed: {}", e))?;
        }

        stream.flush().await.map_err(|e| format!("Flush failed: {}", e))?;
        let write_elapsed = write_start.elapsed();

        // Read response using io_uring (glommio handles DMA internally)
        // With keep-alive, we must read headers first to get Content-Length
        let read_start = Instant::now();
        let mut response_data = Vec::new();
        let mut buffer = vec![0u8; 4096];
        let mut headers_complete = false;
        let mut content_length: Option<usize> = None;
        let mut body_start = 0;

        loop {
            match stream.read(&mut buffer).await {
                Ok(0) => break, // EOF (server closed - old HTTP/1.0 behavior)
                Ok(n) => {
                    response_data.extend_from_slice(&buffer[..n]);

                    // Parse headers if not done yet
                    if !headers_complete {
                        if let Some(header_end) = find_header_end(&response_data) {
                            headers_complete = true;
                            body_start = header_end + 4;

                            // Extract Content-Length from headers
                            let headers_str = String::from_utf8_lossy(&response_data[..header_end]);
                            content_length = extract_content_length(&headers_str);
                        }
                    }

                    // If we have Content-Length and all data, stop reading (keep-alive!)
                    if headers_complete {
                        if let Some(expected_len) = content_length {
                            let received_body = response_data.len() - body_start;
                            if received_body >= expected_len {
                                break; // Got all data, connection stays alive for reuse!
                            }
                        }
                    }
                }
                Err(e) => return Err(format!("Read failed: {}", e)),
            }
        }
        let read_elapsed = read_start.elapsed();
        let total_elapsed = total_start.elapsed();

        // Parse HTTP response
        let result = parse_http_response(&response_data);

        // If successful, return connection to pool for reuse (KEY OPTIMIZATION!)
        if result.is_ok() {
            connection_pool::return_connection(host, port, stream);
        } else {
            // On error, connection is broken - drop it immediately
            // Socket will close with SO_LINGER=0 if we had set it
            drop(stream);
        }

        // Log performance breakdown (this is key for understanding bottlenecks!)
        // eprintln!(
        //     "⏱️  HTTP: parse={:?}, connect={:?} [{}], write={:?}, read={:?}, total={:?} | {}",
        //     parse_elapsed,
        //     connect_elapsed,
        //     if is_pooled { "REUSED" } else { "NEW" },
        //     write_elapsed,
        //     read_elapsed,
        //     total_elapsed,
        //     connection_pool::pool_stats()
        // );

        result
    }
}

/// Find the end of HTTP headers (\r\n\r\n)
fn find_header_end(data: &[u8]) -> Option<usize> {
    data.windows(4).position(|w| w == b"\r\n\r\n")
}

/// Extract Content-Length from headers
fn extract_content_length(headers: &str) -> Option<usize> {
    for line in headers.lines() {
        let lower = line.to_lowercase();
        if lower.starts_with("content-length:") {
            if let Some(value) = line.split(':').nth(1) {
                return value.trim().parse::<usize>().ok();
            }
        }
    }
    None
}

fn parse_http_response(data: &[u8]) -> Result<(u16, HashMap<String, String>, Vec<u8>), String> {
    let response_str = String::from_utf8_lossy(data);

    // Find end of headers
    let header_end = response_str
        .find("\r\n\r\n")
        .ok_or("Invalid HTTP response")?;

    let headers_section = &response_str[..header_end];
    let body_start = header_end + 4;

    // Parse status line
    let mut lines = headers_section.lines();
    let status_line = lines.next().ok_or("No status line")?;

    let parts: Vec<&str> = status_line.split_whitespace().collect();
    if parts.len() < 2 {
        return Err("Invalid status line".to_string());
    }

    let status_code: u16 = parts[1]
        .parse()
        .map_err(|_| "Invalid status code")?;

    // Parse headers
    let mut headers = HashMap::new();
    for line in lines {
        if let Some(pos) = line.find(':') {
            let key = line[..pos].trim().to_lowercase();
            let value = line[pos + 1..].trim().to_string();
            headers.insert(key, value);
        }
    }

    // Extract body
    let body = data[body_start..].to_vec();

    Ok((status_code, headers, body))
}
