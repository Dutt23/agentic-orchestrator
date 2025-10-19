/// Simple HTTP client using monoio (pure io_uring)
/// This gives us TRUE io_uring performance for HTTP requests

use monoio::io::{AsyncReadRent, AsyncWriteRentExt};
use monoio::net::TcpStream;
use std::collections::HashMap;

pub struct IoUringHttpClient;

impl IoUringHttpClient {
    /// Make an HTTP request using pure io_uring
    pub async fn request(
        method: &str,
        url: &str,
        headers: HashMap<String, String>,
        body: Option<Vec<u8>>,
    ) -> Result<(u16, HashMap<String, String>, Vec<u8>), String> {
        // Parse URL
        let url = url::Url::parse(url).map_err(|e| format!("Invalid URL: {}", e))?;

        let host = url.host_str().ok_or("No host in URL")?;
        let port = url.port().unwrap_or(if url.scheme() == "https" { 443 } else { 80 });
        let path = if url.path().is_empty() { "/" } else { url.path() };

        if url.scheme() == "https" {
            return Err("HTTPS not yet supported in io_uring client".to_string());
        }

        // Connect using io_uring
        let addr = format!("{}:{}", host, port);
        let mut stream = TcpStream::connect(&addr)
            .await
            .map_err(|e| format!("Connection failed: {}", e))?;

        // Build HTTP request
        let mut request = format!("{} {} HTTP/1.1\r\n", method, path);
        request.push_str(&format!("Host: {}\r\n", host));

        for (key, value) in headers {
            request.push_str(&format!("{}: {}\r\n", key, value));
        }

        if let Some(ref body_data) = body {
            request.push_str(&format!("Content-Length: {}\r\n", body_data.len()));
        }

        request.push_str("Connection: close\r\n");
        request.push_str("\r\n");

        // Write request (io_uring!) - convert to Vec<u8> for ownership
        let request_bytes = request.into_bytes();
        let (result, _buf) = stream.write_all(request_bytes).await;
        result.map_err(|e| format!("Write failed: {}", e))?;

        // Write body if present (monoio takes ownership)
        if let Some(body_data) = body {
            let (result, _buf) = stream.write_all(body_data).await;
            result.map_err(|e| format!("Body write failed: {}", e))?;
        }

        // Read response (io_uring!)
        let mut response_data = Vec::with_capacity(8192);
        let mut buffer = vec![0u8; 4096];

        loop {
            let (result, buf) = stream.read(buffer).await;
            buffer = buf;

            match result {
                Ok(0) => break, // EOF
                Ok(n) => {
                    response_data.extend_from_slice(&buffer[..n]);
                }
                Err(e) => return Err(format!("Read failed: {}", e)),
            }
        }

        // Parse HTTP response
        parse_http_response(&response_data)
    }
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
