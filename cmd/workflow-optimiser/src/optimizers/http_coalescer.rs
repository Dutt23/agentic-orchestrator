//! HTTP Coalescer Optimizer
//!
//! **Purpose**: Batch multiple independent HTTP calls into parallel requests.
//!
//! # Example
//!
//! **Before**:
//! ```text
//! fetch_users (HTTP) → 200ms
//! fetch_posts (HTTP) → 200ms
//! fetch_comments (HTTP) → 200ms
//! Total: 600ms sequential
//! ```
//!
//! **After**:
//! ```text
//! batch_fetch_all (HTTP parallel) → 200ms
//! Total: 200ms (3x faster!)
//! ```
//!
//! # Detection Logic
//! - Find HTTP nodes that don't depend on each other
//! - Same dependency level (can run in parallel)
//! - Similar endpoints or can be batched
//! - Suggest combining into one parallel request

use crate::optimizer::{Optimize, OptimizationResult, OptimizationError, Suggestion, PerformanceMetrics};
use crate::types::WorkflowIR;

pub struct HttpCoalescer;

impl HttpCoalescer {
    pub fn new() -> Self {
        Self
    }

    /// Find groups of HTTP nodes that can be batched
    fn find_batchable_http_nodes(&self, workflow: &WorkflowIR) -> Vec<Vec<String>> {
        let mut batches = vec![];

        // Find all HTTP nodes
        let http_nodes: Vec<_> = workflow.nodes.iter()
            .filter(|(_, node)| node.is_http())
            .map(|(id, _)| id.clone())
            .collect();

        if http_nodes.len() < 2 {
            return batches;
        }

        // TODO: Group nodes by dependency level (topological sort)
        // For now, simple stub: if multiple HTTP nodes share same dependencies, they can be batched
        for i in 0..http_nodes.len() {
            for j in (i + 1)..http_nodes.len() {
                let node_i = workflow.get_node(&http_nodes[i]).unwrap();
                let node_j = workflow.get_node(&http_nodes[j]).unwrap();

                // If they have same dependencies, they can run in parallel
                if node_i.dependencies == node_j.dependencies {
                    batches.push(vec![http_nodes[i].clone(), http_nodes[j].clone()]);
                }
            }
        }

        batches
    }
}

impl Optimize for HttpCoalescer {
    fn analyze(&self, workflow: &WorkflowIR) -> OptimizationResult {
        let batches = self.find_batchable_http_nodes(workflow);

        if batches.is_empty() {
            return OptimizationResult {
                applicable: false,
                suggestions: vec![],
                total_improvement: PerformanceMetrics {
                    time_saved_ms: 0.0,
                    requests_saved: 0,
                    tokens_saved: 0,
                    efficiency_gain: 0.0,
                },
            };
        }

        let suggestions: Vec<Suggestion> = batches.iter().enumerate().map(|(idx, batch)| {
            let time_saved = (batch.len() - 1) as f32 * 200.0; // Save ~200ms per eliminated HTTP call

            Suggestion {
                id: format!("http_coalescer_{}", idx),
                title: format!("Batch {} HTTP calls into parallel request", batch.len()),
                description: format!(
                    "Combine independent HTTP calls ({}) into a single parallel batch request. \
                    This reduces sequential wait time significantly.",
                    batch.join(", ")
                ),
                affected_nodes: batch.clone(),
                severity: "high".to_string(),
                metrics: PerformanceMetrics {
                    time_saved_ms: time_saved,
                    requests_saved: 0, // Technically same number, but parallel
                    tokens_saved: 0,
                    efficiency_gain: (time_saved / (batch.len() as f32 * 200.0)) * 100.0,
                },
                auto_apply_safe: false, // Requires manual review
            }
        }).collect();

        let total_time_saved: f32 = suggestions.iter().map(|s| s.metrics.time_saved_ms).sum();
        let avg_efficiency: f32 = suggestions.iter().map(|s| s.metrics.efficiency_gain).sum::<f32>() / suggestions.len() as f32;

        OptimizationResult {
            applicable: true,
            suggestions,
            total_improvement: PerformanceMetrics {
                time_saved_ms: total_time_saved,
                requests_saved: 0,
                tokens_saved: 0,
                efficiency_gain: avg_efficiency,
            },
        }
    }

    fn apply(&self, workflow: &WorkflowIR) -> Result<WorkflowIR, OptimizationError> {
        // TODO: Implement batching transformation
        Ok(workflow.clone())
    }

    fn id(&self) -> &str {
        "http_coalescer"
    }

    fn description(&self) -> &str {
        "Batches independent HTTP calls into parallel requests for faster execution"
    }
}

impl Default for HttpCoalescer {
    fn default() -> Self {
        Self::new()
    }
}
