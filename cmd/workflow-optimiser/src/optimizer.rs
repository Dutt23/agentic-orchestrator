//! Core optimizer trait and engine

use crate::types::{WorkflowIR, Node};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use thiserror::Error;

/// Optimization error types
#[derive(Error, Debug)]
pub enum OptimizationError {
    #[error("Invalid workflow structure: {0}")]
    InvalidWorkflow(String),

    #[error("Optimization not applicable: {0}")]
    NotApplicable(String),

    #[error("Analysis failed: {0}")]
    AnalysisFailed(String),
}

/// Performance metrics for optimization
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PerformanceMetrics {
    /// Estimated time saved (milliseconds)
    pub time_saved_ms: f32,

    /// Estimated network requests saved
    pub requests_saved: usize,

    /// Estimated token usage reduced (for LLM calls)
    pub tokens_saved: usize,

    /// Overall efficiency improvement (percentage: 0-100)
    pub efficiency_gain: f32,
}

/// A single optimization suggestion
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Suggestion {
    /// Unique identifier for this suggestion
    pub id: String,

    /// Human-readable title
    pub title: String,

    /// Detailed description
    pub description: String,

    /// Affected node IDs
    pub affected_nodes: Vec<String>,

    /// Severity: "info", "warning", "high"
    pub severity: String,

    /// Estimated improvement metrics
    pub metrics: PerformanceMetrics,

    /// Whether auto-apply is safe
    pub auto_apply_safe: bool,
}

/// Result of optimization analysis
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OptimizationResult {
    /// Whether this optimization is applicable
    pub applicable: bool,

    /// List of suggestions
    pub suggestions: Vec<Suggestion>,

    /// Overall estimated improvement
    pub total_improvement: PerformanceMetrics,
}

/// Core trait that all optimizers must implement
pub trait Optimize {
    /// Analyze workflow and identify optimization opportunities
    ///
    /// # Arguments
    /// * `workflow` - The workflow to analyze
    ///
    /// # Returns
    /// Result containing applicable suggestions
    fn analyze(&self, workflow: &WorkflowIR) -> OptimizationResult;

    /// Apply this optimization to the workflow
    ///
    /// # Arguments
    /// * `workflow` - The workflow to optimize
    ///
    /// # Returns
    /// New optimized workflow or error
    fn apply(&self, workflow: &WorkflowIR) -> Result<WorkflowIR, OptimizationError>;

    /// Get unique identifier for this optimizer
    fn id(&self) -> &str;

    /// Get human-readable description
    fn description(&self) -> &str;

    /// Estimate performance improvement (0-100 percentage)
    fn estimated_improvement(&self, workflow: &WorkflowIR) -> f32 {
        let result = self.analyze(workflow);
        result.total_improvement.efficiency_gain
    }
}

/// Main optimization engine that coordinates all optimizers
pub struct OptimizerEngine {
    optimizers: HashMap<String, Box<dyn Optimize>>,
}

impl OptimizerEngine {
    /// Create new optimizer engine with all available optimizers
    pub fn new() -> Self {
        let mut engine = Self {
            optimizers: HashMap::new(),
        };

        // Register optimizers (stub implementations for now)
        engine.register_all_optimizers();
        engine
    }

    /// Register all available optimizers
    fn register_all_optimizers(&mut self) {
        use crate::optimizers::*;

        // Register each optimizer
        self.register(Box::new(ConditionalAbsorber::new()));
        self.register(Box::new(BranchAbsorber::new()));
        self.register(Box::new(HttpCoalescer::new()));
        self.register(Box::new(SemanticCache::new()));
        self.register(Box::new(ParallelDetector::new()));
        self.register(Box::new(DeadCodeEliminator::new()));
    }

    /// Register a custom optimizer
    pub fn register(&mut self, optimizer: Box<dyn Optimize>) {
        self.optimizers.insert(optimizer.id().to_string(), optimizer);
    }

    /// Get optimizer by ID
    pub fn get_optimizer(&self, id: &str) -> Option<&Box<dyn Optimize>> {
        self.optimizers.get(id)
    }

    /// Analyze workflow with all optimizers
    pub fn analyze(&self, workflow: &WorkflowIR) -> Vec<OptimizationResult> {
        self.optimizers
            .values()
            .map(|opt| opt.analyze(workflow))
            .filter(|result| result.applicable)
            .collect()
    }

    /// Get list of all registered optimizers
    pub fn list_optimizers(&self) -> Vec<(String, String)> {
        self.optimizers
            .values()
            .map(|opt| (opt.id().to_string(), opt.description().to_string()))
            .collect()
    }

    /// Estimate total improvement across all optimizers
    pub fn estimate_total_improvement(&self, workflow: &WorkflowIR) -> f32 {
        let results = self.analyze(workflow);

        // Average efficiency gain across all applicable optimizations
        if results.is_empty() {
            return 0.0;
        }

        let total: f32 = results
            .iter()
            .map(|r| r.total_improvement.efficiency_gain)
            .sum();

        total / results.len() as f32
    }
}

impl Default for OptimizerEngine {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::types::WorkflowIR;

    #[test]
    fn test_engine_initialization() {
        let engine = OptimizerEngine::new();
        assert!(!engine.optimizers.is_empty());
    }

    #[test]
    fn test_list_optimizers() {
        let engine = OptimizerEngine::new();
        let optimizers = engine.list_optimizers();
        assert!(optimizers.len() >= 4); // At least 4 stub optimizers
    }
}
