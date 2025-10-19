/// HTTP Connection Pool for io_uring
/// Maintains persistent connections to avoid TCP handshake overhead
///
/// Key optimizations:
/// 1. Connection reuse eliminates TCP 3-way handshake (~1ms)
/// 2. HTTP/1.1 keep-alive support
/// 3. Automatic cleanup of stale connections
/// 4. Per-host connection tracking

use glommio::net::TcpStream;
use std::cell::RefCell;
use std::collections::HashMap;
use std::time::{Duration, Instant};

thread_local! {
    static HTTP_CONNECTION_POOL: RefCell<ConnectionPool> = RefCell::new(ConnectionPool::new());
}

/// Connection pool entry with timestamp for staleness detection
struct PooledConnection {
    stream: TcpStream,
    last_used: Instant,
}

pub struct ConnectionPool {
    connections: HashMap<String, PooledConnection>,
    max_idle_duration: Duration,
    hits: usize,
    misses: usize,
    evictions: usize,
}

impl ConnectionPool {
    fn new() -> Self {
        Self {
            connections: HashMap::new(),
            max_idle_duration: Duration::from_secs(30), // Close idle connections after 30s
            hits: 0,
            misses: 0,
            evictions: 0,
        }
    }

    /// Get a connection from pool or None if not available
    fn get(&mut self, key: &str) -> Option<TcpStream> {
        if let Some(pooled) = self.connections.remove(key) {
            // Check if connection is too old
            if pooled.last_used.elapsed() > self.max_idle_duration {
                self.evictions += 1;
                return None;
            }

            self.hits += 1;
            Some(pooled.stream)
        } else {
            self.misses += 1;
            None
        }
    }

    /// Return connection to pool
    fn put(&mut self, key: String, stream: TcpStream) {
        // Limit pool size per host to match expected concurrency
        // Higher limit = fewer NEW connections under load
        let max_per_host = 50; // Support up to 50 concurrent requests per host

        // Count connections for this host
        let host_key_prefix = key.split(':').next().unwrap_or("");
        let host_count = self.connections.keys()
            .filter(|k| k.starts_with(host_key_prefix))
            .count();

        if host_count < max_per_host && self.connections.len() < 500 {
            self.connections.insert(
                key,
                PooledConnection {
                    stream,
                    last_used: Instant::now(),
                },
            );
        }
        // If limits exceeded, drop connection (will go to TIME_WAIT)
    }

    /// Get pool statistics
    fn stats(&self) -> (usize, usize, usize, usize) {
        (self.hits, self.misses, self.evictions, self.connections.len())
    }
}

/// Get connection from pool
pub fn get_connection(host: &str, port: u16) -> Option<TcpStream> {
    let key = format!("{}:{}", host, port);
    HTTP_CONNECTION_POOL.with(|pool| pool.borrow_mut().get(&key))
}

/// Return connection to pool for reuse
pub fn return_connection(host: &str, port: u16, stream: TcpStream) {
    let key = format!("{}:{}", host, port);
    HTTP_CONNECTION_POOL.with(|pool| pool.borrow_mut().put(key, stream));
}

/// Get connection pool statistics
pub fn pool_stats() -> String {
    HTTP_CONNECTION_POOL.with(|pool| {
        let (hits, misses, evictions, pooled) = pool.borrow().stats();
        format!(
            "HTTP conns: hits={}, misses={}, evicted={}, pooled={}",
            hits, misses, evictions, pooled
        )
    })
}
