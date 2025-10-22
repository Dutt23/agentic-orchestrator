/// HTTP Handler trait for pluggable HTTP proxy implementations
///
/// This allows easy switching between different HTTP handling strategies:
/// - Buffered I/O: Uses monoio async I/O (io_uring under the hood)
/// - Splice: Uses Linux splice() syscall for true zero-copy
///
/// Each implementation can be benchmarked and compared for learning purposes

use std::time::Duration;

/// Result of handling an HTTP request
pub struct HttpHandlerResult {
    /// Total bytes transferred (request + response)
    pub bytes_transferred: usize,

    /// Time taken to process the request
    pub duration: Duration,

    /// Method used: "buffered" or "splice"
    pub method: String,

    /// Number of chunks read (for buffered mode)
    pub chunks: usize,

    /// Connection time
    pub connect_time: Duration,

    /// Time to write request to upstream
    pub write_time: Duration,

    /// Time to read/splice response
    pub transfer_time: Duration,
}

impl HttpHandlerResult {
    /// Calculate throughput in MB/s
    pub fn throughput_mb_per_sec(&self) -> f64 {
        (self.bytes_transferred as f64 / 1_000_000.0) / self.duration.as_secs_f64()
    }
}

/// Trait for HTTP proxy handlers
pub trait HttpHandler {
    /// Handle an HTTP proxy request
    ///
    /// # Arguments
    /// * `upstream` - Connected TCP stream to upstream server
    /// * `client` - Unix socket stream to client (Go service)
    /// * `http_request` - Raw HTTP request bytes to send
    ///
    /// # Returns
    /// Result with (stats, Option<TcpStream>)
    /// - TcpStream is Some if connection is reusable (for pooling)
    /// - TcpStream is None if connection should be closed
    fn handle(
        &self,
        upstream: monoio::net::TcpStream,
        client: &mut monoio::net::UnixStream,
        http_request: Vec<u8>,
    ) -> impl std::future::Future<Output = Result<(HttpHandlerResult, Option<monoio::net::TcpStream>), String>>;

    /// Get the handler name for logging
    fn name(&self) -> &str;
}
