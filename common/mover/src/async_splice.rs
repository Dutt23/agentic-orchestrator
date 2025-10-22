/// Robust async splice using io_uring readiness polling
///
/// This module implements zero-copy splice with proper async integration:
/// - Uses monoio's io_uring-based readiness polling (POLLIN)
/// - No busy waiting or thread sleeps
/// - Kernel manages all waiting efficiently
///
/// Flow:
/// 1. Poll source FD for readiness (IORING_OP_POLL_ADD internally)
/// 2. When ready, attempt splice with SPLICE_F_NONBLOCK
/// 3. Handle results: >0 = progress, -EAGAIN = re-poll, 0 = EOF

use monoio::net::{TcpStream, UnixStream};
use std::os::unix::io::{AsRawFd, RawFd};
use std::time::{Duration, Instant};
use tracing::{debug, error};

#[cfg(target_os = "linux")]
use nix::unistd::{close, pipe};

// Splice flags
#[cfg(target_os = "linux")]
const SPLICE_F_MOVE: u32 = 1;
#[cfg(target_os = "linux")]
const SPLICE_F_NONBLOCK: u32 = 2;

// Raw splice syscall
#[cfg(target_os = "linux")]
unsafe fn splice_raw(
    fd_in: RawFd,
    off_in: *mut libc::loff_t,
    fd_out: RawFd,
    off_out: *mut libc::loff_t,
    len: usize,
    flags: u32,
) -> Result<usize, nix::errno::Errno> {
    let result = libc::splice(fd_in, off_in, fd_out, off_out, len, flags);
    if result < 0 {
        Err(nix::errno::Errno::from_i32(-result as i32))
    } else {
        Ok(result as usize)
    }
}

/// RAII guard for pipe cleanup
#[cfg(target_os = "linux")]
struct PipeGuard {
    pipe_read: RawFd,
    pipe_write: RawFd,
}

#[cfg(target_os = "linux")]
impl Drop for PipeGuard {
    fn drop(&mut self) {
        let _ = close(self.pipe_read);
        let _ = close(self.pipe_write);
    }
}

/// Async splice exact bytes using io_uring readiness polling
///
/// This is the ROBUST approach:
/// - Wait for readiness using monoio (io_uring POLL_ADD)
/// - Splice when data is ready
/// - Handle EAGAIN by re-polling
#[cfg(target_os = "linux")]
pub async fn splice_exact_bytes_async(
    upstream: &mut TcpStream,
    client: &mut UnixStream,
    exact_bytes: usize,
) -> Result<usize, String> {
    let upstream_fd = upstream.as_raw_fd();
    let client_fd = client.as_raw_fd();

    let start = Instant::now();
    let mut total_transferred = 0;
    let timeout_duration = std::time::Duration::from_secs(30); // 30 second timeout

    // Create pipe once for all transfers
    let (pipe_read, pipe_write) = pipe()
        .map_err(|e| format!("Failed to create pipe: {}", e))?;
    let _pipe_guard = PipeGuard { pipe_read, pipe_write };

    let mut read_retry_count = 0;
    const MAX_READ_RETRIES: usize = 5;

    while total_transferred < exact_bytes {
        // Check timeout
        if start.elapsed() > timeout_duration {
            return Err(format!(
                "Timeout after {:?}: transferred {}/{} bytes",
                start.elapsed(),
                total_transferred,
                exact_bytes
            ));
        }

        let remaining = exact_bytes - total_transferred;
        let chunk_size = remaining.min(65536); // 64KB chunks

        // Step 1: Wait for upstream to be readable (io_uring POLL_ADD)
        // Use timeout to prevent indefinite waiting
        let readable_timeout = Duration::from_secs(5);
        match monoio::time::timeout(readable_timeout, upstream.readable(false)).await {
            Ok(Ok(_)) => {
                // Socket is readable, data should be available
                read_retry_count = 0; // Reset on successful readiness
            }
            Ok(Err(e)) => {
                return Err(format!("Readiness check failed: {}", e));
            }
            Err(_) => {
                return Err(format!("Timeout waiting for upstream to be readable (5s)"));
            }
        }

        // Step 2: Attempt splice from upstream to pipe (should succeed now)
        let to_pipe = match unsafe {
            splice_raw(
                upstream_fd,
                std::ptr::null_mut(),
                pipe_write,
                std::ptr::null_mut(),
                chunk_size,
                SPLICE_F_MOVE | SPLICE_F_NONBLOCK,
            )
        } {
            Ok(0) => {
                if total_transferred < exact_bytes {
                    error!("Splice EOF: got {} bytes, expected {}", total_transferred, exact_bytes);
                    return Err(format!(
                        "Unexpected EOF: got {} bytes, expected {}",
                        total_transferred, exact_bytes
                    ));
                }
                break;
            }
            Ok(n) => {
                read_retry_count = 0;
                n
            }
            Err(nix::errno::Errno::EAGAIN) => {
                read_retry_count += 1;
                if read_retry_count > MAX_READ_RETRIES {
                    error!("Too many EAGAIN retries after readable()");
                    return Err(format!(
                        "Too many EAGAIN retries ({}) after readable()",
                        MAX_READ_RETRIES
                    ));
                }
                debug!("EAGAIN after readable(), retry {}/{}", read_retry_count, MAX_READ_RETRIES);
                continue;
            }
            Err(e) => {
                return Err(format!("Splice from upstream failed: {}", e));
            }
        };

        // Step 3: Transfer from pipe to client (may need multiple writes)
        let mut written = 0;
        let mut write_retry_count = 0;
        const MAX_WRITE_RETRIES: usize = 5;

        while written < to_pipe {
            // Check timeout
            if start.elapsed() > timeout_duration {
                return Err(format!(
                    "Timeout during write after {:?}: written {}/{} bytes of chunk",
                    start.elapsed(),
                    written,
                    to_pipe
                ));
            }

            // Wait for client to be writable
            // Use timeout to prevent indefinite waiting
            let writable_timeout = Duration::from_secs(5);
            match monoio::time::timeout(writable_timeout, client.writable(false)).await {
                Ok(Ok(_)) => {
                    write_retry_count = 0; // Reset on successful readiness
                },
                Ok(Err(e)) => {
                    return Err(format!("Client writable check failed: {}", e));
                }
                Err(_) => {
                    return Err(format!("Timeout waiting for client to be writable (5s)"));
                }
            }

            match unsafe {
                splice_raw(
                    pipe_read,
                    std::ptr::null_mut(),
                    client_fd,
                    std::ptr::null_mut(),
                    to_pipe - written,
                    SPLICE_F_MOVE | SPLICE_F_NONBLOCK,
                )
            } {
                Ok(0) => {
                    error!("Client splice returned 0 with {} bytes in pipe", to_pipe - written);
                    return Err(format!(
                        "Client splice returned 0 (pipe has {} bytes)",
                        to_pipe - written
                    ));
                }
                Ok(n) => {
                    written += n;
                    total_transferred += n;
                    write_retry_count = 0;
                }
                Err(nix::errno::Errno::EAGAIN) => {
                    write_retry_count += 1;
                    if write_retry_count > MAX_WRITE_RETRIES {
                        error!("Too many write EAGAIN retries");
                        return Err(format!(
                            "Too many EAGAIN retries ({}) after writable()",
                            MAX_WRITE_RETRIES
                        ));
                    }
                    debug!("Write EAGAIN, retry {}/{}", write_retry_count, MAX_WRITE_RETRIES);
                    continue;
                }
                Err(e) => {
                    return Err(format!("Splice to client failed: {}", e));
                }
            }
        }
    }

    debug!("Async splice: {}b in {:?}", total_transferred, start.elapsed());
    Ok(total_transferred)
}

/// Non-Linux fallback
#[cfg(not(target_os = "linux"))]
pub async fn splice_exact_bytes_async(
    _upstream: &mut monoio::net::TcpStream,
    _client: &mut monoio::net::UnixStream,
    _exact_bytes: usize,
) -> Result<usize, String> {
    Err("Async splice only available on Linux".to_string())
}
