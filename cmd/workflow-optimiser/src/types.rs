//! Workflow IR types (mirrors Go SDK types)

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// Workflow Intermediate Representation
/// Mirrors: github.com/lyzr/orchestrator/cmd/workflow-runner/sdk.IR
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WorkflowIR {
    /// IR version
    pub version: String,

    /// Map of node_id -> Node
    pub nodes: HashMap<String, Node>,

    /// Edges (derived from node dependencies/dependents)
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub edges: Vec<Edge>,
}

/// Workflow node
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Node {
    /// Unique node identifier
    pub id: String,

    /// Node type: "task", "agent", "conditional", "loop", etc.
    #[serde(rename = "type")]
    pub node_type: String,

    /// CAS reference to node configuration
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub config_ref: Option<String>,

    /// Inline config (alternative to config_ref)
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub config: Option<serde_json::Value>,

    /// Node dependencies (nodes that must complete before this)
    #[serde(default)]
    pub dependencies: Vec<String>,

    /// Dependent nodes (nodes that depend on this)
    #[serde(default)]
    pub dependents: Vec<String>,

    /// Whether this is a terminal node
    #[serde(default)]
    pub is_terminal: bool,

    /// Loop configuration
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub loop_config: Option<LoopConfig>,

    /// Branch configuration
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub branch: Option<BranchConfig>,
}

/// Loop configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LoopConfig {
    pub enabled: bool,
    pub max_iterations: usize,
    pub loop_back_to: String,

    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub condition: Option<Condition>,

    #[serde(default)]
    pub break_path: Vec<String>,

    #[serde(default)]
    pub timeout_path: Vec<String>,
}

/// Branch/conditional configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BranchConfig {
    pub enabled: bool,

    #[serde(rename = "type")]
    pub branch_type: String, // "conditional", "switch", etc.

    #[serde(default)]
    pub rules: Vec<BranchRule>,

    #[serde(default)]
    pub default: Vec<String>,
}

/// Branch rule (if-then)
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BranchRule {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub condition: Option<Condition>,

    pub next_nodes: Vec<String>,
}

/// Condition (CEL expression)
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Condition {
    #[serde(rename = "type")]
    pub condition_type: String, // "cel", "simple", etc.

    pub expression: String,
}

/// Edge between nodes
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Edge {
    pub from: String,
    pub to: String,

    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub condition: Option<Condition>,
}

impl WorkflowIR {
    /// Get node by ID
    pub fn get_node(&self, node_id: &str) -> Option<&Node> {
        self.nodes.get(node_id)
    }

    /// Get all entry nodes (no dependencies)
    pub fn entry_nodes(&self) -> Vec<&Node> {
        self.nodes
            .values()
            .filter(|node| node.dependencies.is_empty())
            .collect()
    }

    /// Get all terminal nodes
    pub fn terminal_nodes(&self) -> Vec<&Node> {
        self.nodes
            .values()
            .filter(|node| node.is_terminal)
            .collect()
    }

    /// Count nodes by type
    pub fn node_count_by_type(&self) -> HashMap<String, usize> {
        let mut counts = HashMap::new();
        for node in self.nodes.values() {
            *counts.entry(node.node_type.clone()).or_insert(0) += 1;
        }
        counts
    }

    /// Get nodes that can run in parallel (same dependency level)
    pub fn parallel_candidate_nodes(&self) -> Vec<Vec<String>> {
        // TODO: Implement topological sorting and level detection
        vec![]
    }
}

impl Node {
    /// Check if this node has a conditional/branch config
    pub fn is_conditional(&self) -> bool {
        self.branch.as_ref().map_or(false, |b| b.enabled)
    }

    /// Check if this node has a loop config
    pub fn is_loop(&self) -> bool {
        self.loop_config.as_ref().map_or(false, |l| l.enabled)
    }

    /// Check if this node is an HTTP node
    pub fn is_http(&self) -> bool {
        self.node_type == "http" ||
        self.config.as_ref()
            .and_then(|c| c.get("type"))
            .and_then(|t| t.as_str())
            .map_or(false, |t| t == "http" || t == "http_request")
    }

    /// Check if this node is an LLM/agent node
    pub fn is_llm(&self) -> bool {
        self.node_type == "agent" || self.node_type == "llm"
    }

    /// Get estimated execution time (milliseconds) based on node type
    pub fn estimated_execution_time(&self) -> f32 {
        match self.node_type.as_str() {
            "http" => 200.0,    // Average HTTP request
            "agent" | "llm" => 2000.0, // Average LLM call
            "function" => 50.0, // Function execution
            _ => 100.0,         // Default
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_workflow_parsing() {
        let json = r#"{
            "version": "1.0",
            "nodes": {
                "A": {
                    "id": "A",
                    "type": "function",
                    "dependencies": [],
                    "dependents": ["B"],
                    "is_terminal": false
                },
                "B": {
                    "id": "B",
                    "type": "function",
                    "dependencies": ["A"],
                    "dependents": [],
                    "is_terminal": true
                }
            },
            "edges": []
        }"#;

        let workflow: WorkflowIR = serde_json::from_str(json).unwrap();
        assert_eq!(workflow.nodes.len(), 2);
        assert_eq!(workflow.entry_nodes().len(), 1);
        assert_eq!(workflow.terminal_nodes().len(), 1);
    }

    #[test]
    fn test_node_type_detection() {
        let mut node = Node {
            id: "test".to_string(),
            node_type: "http".to_string(),
            config_ref: None,
            config: None,
            dependencies: vec![],
            dependents: vec![],
            is_terminal: false,
            loop_config: None,
            branch: None,
        };

        assert!(node.is_http());
        assert!(!node.is_llm());

        node.node_type = "agent".to_string();
        assert!(!node.is_http());
        assert!(node.is_llm());
    }
}
