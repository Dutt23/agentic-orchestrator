/// Configuration for mover service
/// Loads from environment variables with validation and defaults

use anyhow::{Context, Result};
use std::env;

// ============================================================================
// Environment Variable Helpers (Static Dispatch)
// ============================================================================

/// Parse string from environment with default (no allocation if default used)
fn get_env_string(key: &str, default: &str) -> String {
    env::var(key).unwrap_or_else(|_| default.to_string())
}

/// Parse u32 from environment with default
fn get_env_u32(key: &str, default: u32) -> u32 {
    env::var(key)
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(default)
}

/// Parse usize from environment with default
fn get_env_usize(key: &str, default: usize) -> usize {
    env::var(key)
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(default)
}

/// Parse u64 from environment with default
fn get_env_u64(key: &str, default: u64) -> u64 {
    env::var(key)
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(default)
}

/// Parse boolean from environment with default
fn get_env_bool(key: &str, default: bool) -> bool {
    env::var(key)
        .ok()
        .map(|v| v.to_lowercase() == "true")
        .unwrap_or(default)
}

// ============================================================================
// io_uring Setup Flags (from Linux kernel)
// ============================================================================

// io_uring setup flags (from liburing.h)
pub const IORING_SETUP_IOPOLL: u32 = 1 << 0;  // 0x01 - Poll for I/O completion
pub const IORING_SETUP_SQPOLL: u32 = 1 << 1;  // 0x02 - Kernel polling thread
pub const IORING_SETUP_SQ_AFF: u32 = 1 << 2;  // 0x04 - SQ poll CPU affinity
pub const IORING_SETUP_CQSIZE: u32 = 1 << 3;  // 0x08 - CQ size specified
pub const IORING_SETUP_CLAMP: u32 = 1 << 4;   // 0x10 - Clamp sizes to max
pub const IORING_SETUP_ATTACH_WQ: u32 = 1 << 5; // 0x20 - Attach to existing workqueue

pub struct MoverConfig {
    // Network
    pub socket_path: String,
    pub db_url: String,

    // io_uring settings
    pub iouring_entries: u32,
    pub iouring_flags: u32,

    // Buffer pool
    pub buffer_count: usize,
    pub buffer_size: usize,

    // Performance
    pub batch_timeout_ms: u64,
    pub max_connections: usize,

    // Features
    pub enable_huge_pages: bool,
    pub enable_send_zc: bool,
}

impl MoverConfig {
    /// Load configuration from environment variables
    pub fn load_from_env() -> Result<Self> {
        Ok(Self {
            socket_path: get_env_string("MOVER_SOCKET", "/tmp/mover.sock"),
            db_url: get_env_string("DATABASE_URL", "postgres://localhost/orchestrator"),
            iouring_entries: get_env_u32("IOURING_ENTRIES", 4096),
            iouring_flags: Self::parse_iouring_flags()?,
            buffer_count: get_env_usize("MOVER_BUFFER_COUNT", 256),
            buffer_size: get_env_usize("MOVER_BUFFER_SIZE", 4096),
            batch_timeout_ms: get_env_u64("MOVER_BATCH_TIMEOUT_MS", 10),
            max_connections: get_env_usize("MOVER_MAX_CONNECTIONS", 32),
            enable_huge_pages: get_env_bool("MOVER_ENABLE_HUGE_PAGES", false),
            enable_send_zc: get_env_bool("MOVER_ENABLE_SEND_ZC", true),
        })
    }

    /// Parse io_uring flags from environment
    /// Supports numeric (16) or string ("clamp,sqpoll")
    fn parse_iouring_flags() -> Result<u32> {
        let flags_str = env::var("IOURING_FLAGS").unwrap_or_else(|_| "clamp".to_string());

        // Try parse as number first
        if let Ok(num) = flags_str.parse::<u32>() {
            return Ok(num);
        }

        // Parse as comma-separated flags
        let mut flags = 0u32;
        for flag_name in flags_str.split(',') {
            let flag_name = flag_name.trim().to_lowercase();
            flags |= match flag_name.as_str() {
                "iopoll" => IORING_SETUP_IOPOLL,
                "sqpoll" => IORING_SETUP_SQPOLL,
                "sq_aff" => IORING_SETUP_SQ_AFF,
                "cqsize" => IORING_SETUP_CQSIZE,
                "clamp" => IORING_SETUP_CLAMP,
                "attach_wq" => IORING_SETUP_ATTACH_WQ,
                "" => 0,  // Empty string, skip
                _ => anyhow::bail!("Unknown io_uring flag: {}", flag_name),
            };
        }

        Ok(flags)
    }

    /// Create config with default values
    pub fn with_defaults() -> Self {
        Self {
            socket_path: "/tmp/mover.sock".to_string(),
            db_url: "postgres://localhost/orchestrator".to_string(),
            iouring_entries: 4096,
            iouring_flags: IORING_SETUP_CLAMP,  // Safe default
            buffer_count: 256,
            buffer_size: 4096,
            batch_timeout_ms: 10,
            max_connections: 32,
            enable_huge_pages: false,
            enable_send_zc: true,
        }
    }

    /// Validate configuration values
    pub fn validate(self) -> Result<Self> {
        // io_uring entries must be power of 2
        if !self.iouring_entries.is_power_of_two() {
            anyhow::bail!("iouring_entries must be power of 2, got {}", self.iouring_entries);
        }

        if self.iouring_entries < 256 || self.iouring_entries > 8192 {
            anyhow::bail!("iouring_entries must be 256-8192, got {}", self.iouring_entries);
        }

        // Buffer size must be multiple of page size (4096)
        if self.buffer_size % 4096 != 0 {
            anyhow::bail!("buffer_size must be multiple of 4096, got {}", self.buffer_size);
        }

        if self.buffer_count == 0 || self.buffer_count > 1024 {
            anyhow::bail!("buffer_count must be 1-1024, got {}", self.buffer_count);
        }

        if self.batch_timeout_ms == 0 || self.batch_timeout_ms > 1000 {
            anyhow::bail!("batch_timeout_ms must be 1-1000, got {}", self.batch_timeout_ms);
        }

        if !self.socket_path.starts_with('/') {
            anyhow::bail!("socket_path must be absolute, got {}", self.socket_path);
        }

        Ok(self)
    }

    /// Get human-readable flag description
    pub fn flags_description(&self) -> String {
        let mut flags = Vec::new();

        if self.iouring_flags & IORING_SETUP_IOPOLL != 0 {
            flags.push("IOPOLL");
        }
        if self.iouring_flags & IORING_SETUP_SQPOLL != 0 {
            flags.push("SQPOLL");
        }
        if self.iouring_flags & IORING_SETUP_CLAMP != 0 {
            flags.push("CLAMP");
        }
        if self.iouring_flags & IORING_SETUP_CQSIZE != 0 {
            flags.push("CQSIZE");
        }

        if flags.is_empty() {
            "None".to_string()
        } else {
            flags.join("|")
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_default_config() {
        let config = MoverConfig::with_defaults();
        assert_eq!(config.iouring_entries, 4096);
        assert!(config.iouring_entries.is_power_of_two());
    }

    #[test]
    fn test_validate_power_of_two() {
        let mut config = MoverConfig::with_defaults();
        config.iouring_entries = 1000;  // Not power of 2
        assert!(config.validate().is_err());
    }

    #[test]
    fn test_validate_buffer_size() {
        let mut config = MoverConfig::with_defaults();
        config.buffer_size = 3000;  // Not multiple of 4096
        assert!(config.validate().is_err());
    }

    #[test]
    fn test_parse_flags_string() {
        std::env::set_var("IOURING_FLAGS", "clamp,sqpoll");
        let flags = MoverConfig::parse_iouring_flags().unwrap();
        assert_eq!(flags, IORING_SETUP_CLAMP | IORING_SETUP_SQPOLL);
    }

    #[test]
    fn test_parse_flags_numeric() {
        std::env::set_var("IOURING_FLAGS", "18");  // CLAMP | SQPOLL
        let flags = MoverConfig::parse_iouring_flags().unwrap();
        assert_eq!(flags, 18);
    }
}
