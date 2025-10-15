//! Optimizer implementations
//!
//! This module contains all concrete optimizer implementations.
//! Each optimizer focuses on a specific optimization pattern.

pub mod conditional_absorber;
pub mod branch_absorber;
pub mod http_coalescer;
pub mod semantic_cache;
pub mod parallel_detector;
pub mod parallel_executor;
pub mod dead_code_eliminator;

// Re-exports
pub use conditional_absorber::ConditionalAbsorber;
pub use branch_absorber::BranchAbsorber;
pub use http_coalescer::HttpCoalescer;
pub use semantic_cache::SemanticCache;
pub use parallel_detector::ParallelDetector;
pub use parallel_executor::ParallelExecutor;
pub use dead_code_eliminator::DeadCodeEliminator;
