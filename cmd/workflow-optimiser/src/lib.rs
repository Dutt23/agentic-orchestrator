//! Workflow Optimizer - WASM Library
//!
//! Real-time workflow optimization engine that runs in the browser.
//! Analyzes workflow graphs and suggests performance optimizations.
//!
//! # Example (Browser)
//! ```javascript
//! import init, { optimize_workflow } from './workflow_optimiser.wasm';
//!
//! await init();
//! const workflow = { nodes: [...], edges: [...] };
//! const suggestions = optimize_workflow(JSON.stringify(workflow));
//! console.log(JSON.parse(suggestions));
//! ```

use wasm_bindgen::prelude::*;
use serde::{Deserialize, Serialize};

// Module declarations
pub mod optimizer;
pub mod types;
pub mod optimizers;

// Re-exports for convenience
pub use optimizer::{Optimize, OptimizationResult, OptimizerEngine};
pub use types::{WorkflowIR, Node, Edge};

/// Initialize the WASM module
/// Sets up panic hook for better error messages in browser console
#[wasm_bindgen(start)]
pub fn init() {
    #[cfg(feature = "console_error_panic_hook")]
    console_error_panic_hook::set_once();

    log("Workflow Optimizer initialized");
}

/// Analyze workflow and return optimization suggestions
///
/// # Arguments
/// * `workflow_json` - JSON string of workflow IR
///
/// # Returns
/// JSON string containing optimization suggestions
#[wasm_bindgen]
pub fn optimize_workflow(workflow_json: &str) -> Result<String, JsValue> {
    // Parse workflow
    let workflow: WorkflowIR = serde_json::from_str(workflow_json)
        .map_err(|e| JsValue::from_str(&format!("Failed to parse workflow: {}", e)))?;

    // Run optimization engine
    let engine = OptimizerEngine::new();
    let results = engine.analyze(&workflow);

    // Serialize results
    serde_json::to_string_pretty(&results)
        .map_err(|e| JsValue::from_str(&format!("Failed to serialize results: {}", e)))
}

/// Apply a specific optimization to the workflow
///
/// # Arguments
/// * `workflow_json` - JSON string of workflow IR
/// * `optimizer_id` - ID of optimizer to apply (e.g., "conditional_absorber")
///
/// # Returns
/// JSON string of optimized workflow
#[wasm_bindgen]
pub fn apply_optimization(workflow_json: &str, optimizer_id: &str) -> Result<String, JsValue> {
    // Parse workflow
    let workflow: WorkflowIR = serde_json::from_str(workflow_json)
        .map_err(|e| JsValue::from_str(&format!("Failed to parse workflow: {}", e)))?;

    // Get optimizer
    let engine = OptimizerEngine::new();
    let optimizer = engine.get_optimizer(optimizer_id)
        .ok_or_else(|| JsValue::from_str(&format!("Unknown optimizer: {}", optimizer_id)))?;

    // Apply optimization
    let optimized = optimizer.apply(&workflow)
        .map_err(|e| JsValue::from_str(&format!("Optimization failed: {}", e)))?;

    // Serialize optimized workflow
    serde_json::to_string_pretty(&optimized)
        .map_err(|e| JsValue::from_str(&format!("Failed to serialize workflow: {}", e)))
}

/// Get list of available optimizers with descriptions
#[wasm_bindgen]
pub fn list_optimizers() -> String {
    let engine = OptimizerEngine::new();
    let optimizers: Vec<_> = engine.list_optimizers()
        .into_iter()
        .map(|(id, desc)| OptimizerInfo { id, description: desc })
        .collect();

    serde_json::to_string_pretty(&optimizers).unwrap_or_else(|_| "[]".to_string())
}

/// Estimate performance improvement for a workflow
#[wasm_bindgen]
pub fn estimate_improvement(workflow_json: &str) -> Result<f32, JsValue> {
    let workflow: WorkflowIR = serde_json::from_str(workflow_json)
        .map_err(|e| JsValue::from_str(&format!("Failed to parse workflow: {}", e)))?;

    let engine = OptimizerEngine::new();
    Ok(engine.estimate_total_improvement(&workflow))
}

// Helper types for WASM exports
#[derive(Serialize, Deserialize)]
struct OptimizerInfo {
    id: String,
    description: String,
}

/// Log message to browser console (for debugging)
fn log(msg: &str) {
    web_sys::console::log_1(&JsValue::from_str(msg));
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_workflow() {
        let json = r#"{
            "version": "1.0",
            "nodes": [],
            "edges": []
        }"#;

        let workflow: WorkflowIR = serde_json::from_str(json).unwrap();
        assert_eq!(workflow.version, "1.0");
    }
}
