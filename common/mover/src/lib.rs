// Cross-platform modules
pub mod config;
pub mod protocol;

// Linux-only modules (require io_uring, splice syscalls)
#[cfg(target_os = "linux")]
pub mod async_splice;
#[cfg(target_os = "linux")]
pub mod connection_pool;
#[cfg(target_os = "linux")]
pub mod dma_pool;
#[cfg(target_os = "linux")]
pub mod handlers;
#[cfg(target_os = "linux")]
pub mod http_handler;
#[cfg(target_os = "linux")]
pub mod http_handler_buffered;
#[cfg(target_os = "linux")]
pub mod http_handler_splice;
#[cfg(target_os = "linux")]
pub mod monoio_http;
#[cfg(target_os = "linux")]
pub mod server;
#[cfg(target_os = "linux")]
pub mod splice;
#[cfg(target_os = "linux")]
pub mod true_splice;

// pub mod iouring; // Disabled - uses tokio-uring
// pub mod postgres; // Disabled - tokio-postgres incompatible with monoio

