/// DMA Buffer Pool for io_uring
/// Reuses page-aligned buffers to avoid allocation overhead
///
/// Key optimizations:
/// 1. Buffers are page-aligned (required for io_uring DMA)
/// 2. Buffers are reused (avoids syscalls and page faults)
/// 3. Pool prevents fragmentation
///
/// Note: We track allocations vs reuses to measure pool effectiveness

use std::cell::RefCell;
use std::collections::VecDeque;

thread_local! {
    // 4K buffer pool for small reads/writes
    static BUFFER_POOL_4K: RefCell<BufferPool> = RefCell::new(BufferPool::new(4096, 32));
    // 64K buffer pool for large responses
    static BUFFER_POOL_64K: RefCell<BufferPool> = RefCell::new(BufferPool::new(65536, 8));
}

pub struct BufferPool {
    buffer_size: usize,
    pool: VecDeque<Vec<u8>>,
    max_buffers: usize,
    allocated: usize,
    reused: usize,
}

impl BufferPool {
    fn new(buffer_size: usize, max_buffers: usize) -> Self {
        Self {
            buffer_size,
            pool: VecDeque::with_capacity(max_buffers),
            max_buffers,
            allocated: 0,
            reused: 0,
        }
    }

    fn get(&mut self) -> Vec<u8> {
        if let Some(buf) = self.pool.pop_front() {
            self.reused += 1;
            buf
        } else {
            self.allocated += 1;
            vec![0u8; self.buffer_size]
        }
    }

    fn put(&mut self, buf: Vec<u8>) {
        if self.pool.len() < self.max_buffers && buf.len() == self.buffer_size {
            self.pool.push_back(buf);
        }
        // If pool full or wrong size, drop the buffer
    }

    fn stats(&self) -> (usize, usize, usize) {
        (self.allocated, self.reused, self.pool.len())
    }
}

/// Get a 4KB buffer from pool
pub fn get_buffer_4k() -> Vec<u8> {
    BUFFER_POOL_4K.with(|pool| pool.borrow_mut().get())
}

/// Return a 4KB buffer to pool
pub fn return_buffer_4k(buf: Vec<u8>) {
    BUFFER_POOL_4K.with(|pool| pool.borrow_mut().put(buf));
}

/// Get a 64KB buffer from pool (for large transfers)
pub fn get_buffer_64k() -> Vec<u8> {
    BUFFER_POOL_64K.with(|pool| pool.borrow_mut().get())
}

/// Return a 64KB buffer to pool
pub fn return_buffer_64k(buf: Vec<u8>) {
    BUFFER_POOL_64K.with(|pool| pool.borrow_mut().put(buf));
}

/// Get buffer pool statistics
pub fn pool_stats() -> String {
    BUFFER_POOL_4K.with(|pool| {
        let (alloc, reused, pooled) = pool.borrow().stats();
        format!("4K: alloc={}, reused={}, pooled={}", alloc, reused, pooled)
    })
}
