package compiler

import (
	"encoding/json"
	"fmt"

	"github.com/lyzr/orchestrator/cmd/workflow-runner/sdk"
)

// Node type constants
const (
	NodeTypeConditional = "conditional"
	NodeTypeLoop        = "loop"
	NodeTypeParallel    = "parallel"
	NodeTypeTask        = "task"
	NodeTypeFunction    = "function"
	NodeTypeHTTP        = "http"
	NodeTypeAgent       = "agent"
	NodeTypeTransform   = "transform"
	NodeTypeAggregate   = "aggregate"
	NodeTypeFilter      = "filter"
)

// Condition type constants
const (
	ConditionTypeCEL = "cel"
)

// validExecutableTypes defines the set of valid executable node types
// These types are preserved for specialized routing by the coordinator
var validExecutableTypes = map[string]bool{
	NodeTypeFunction:  true,
	NodeTypeHTTP:      true,
	NodeTypeAgent:     true,
	NodeTypeTransform: true,
	NodeTypeAggregate: true,
	NodeTypeFilter:    true,
}

// ============================================================================
// Workflow Schema Types
// ============================================================================
// NOTE: These types should be auto-generated from common/schema/workflow.schema.json
// using a tool like typify, quicktype, or go-jsonschema to ensure they stay in sync
// with the schema definition.
//
// For now, these are manually defined to match the schema structure.
// TODO: Set up code generation pipeline for automatic type generation.
// ============================================================================

// WorkflowSchema represents the input workflow definition from workflow.schema.json
type WorkflowSchema struct {
	Nodes    []WorkflowNode         `json:"nodes"`
	Edges    []WorkflowEdge         `json:"edges"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// WorkflowNode represents a node in workflow.schema.json format
// Schema: common/schema/workflow.schema.json#/definitions/Node
type WorkflowNode struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"` // function, http, agent, conditional, loop, parallel, transform, aggregate, filter
	Config    map[string]interface{} `json:"config,omitempty"`
	TimeoutMS int                    `json:"timeout_ms,omitempty"`
	Retry     *RetryPolicy           `json:"retry,omitempty"`
}

// WorkflowEdge represents an edge in workflow.schema.json format
// Schema: common/schema/workflow.schema.json#/definitions/Edge
type WorkflowEdge struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Condition string `json:"condition,omitempty"` // Optional condition for edge traversal
}

// RetryPolicy from workflow.schema.json
// Schema: common/schema/workflow.schema.json#/definitions/RetryPolicy
type RetryPolicy struct {
	MaxAttempts       int     `json:"max_attempts,omitempty"`
	BackoffMS         int     `json:"backoff_ms,omitempty"`
	BackoffMultiplier float64 `json:"backoff_multiplier,omitempty"`
}

// DSL represents the source workflow definition (legacy, for backward compatibility)
type DSL struct {
	Version string    `json:"version"`
	Nodes   []DSLNode `json:"nodes"`
	Edges   []DSLEdge `json:"edges"`
}

// DSLNode represents a node in the DSL (legacy)
type DSLNode struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	ConfigRef string            `json:"config_ref"`
	Loop      *sdk.LoopConfig   `json:"loop,omitempty"`
	Branch    *sdk.BranchConfig `json:"branch,omitempty"`
}

// DSLEdge represents an edge in the DSL (legacy)
type DSLEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// CompileWorkflowSchema converts workflow.schema.json format to executable IR
func CompileWorkflowSchema(schema *WorkflowSchema, casClient sdk.CASClient) (*sdk.IR, error) {
	ir := &sdk.IR{
		Version: "1.0",
		Nodes:   make(map[string]*sdk.Node),
	}

	// Track conditional edges for branch config creation
	conditionalEdges := make(map[string][]WorkflowEdge)
	for _, edge := range schema.Edges {
		if edge.Condition != "" {
			conditionalEdges[edge.From] = append(conditionalEdges[edge.From], edge)
		}
	}

	// 1. Convert nodes with type mapping
	for _, wfNode := range schema.Nodes {
		node, err := convertWorkflowNode(&wfNode, conditionalEdges, casClient)
		if err != nil {
			return nil, fmt.Errorf("failed to convert node %s: %w", wfNode.ID, err)
		}
		ir.Nodes[node.ID] = node
	}

	// 2. Build edges (dependencies and dependents)
	for _, edge := range schema.Edges {
		fromNode, exists := ir.Nodes[edge.From]
		if !exists {
			return nil, fmt.Errorf("edge references non-existent node: %s", edge.From)
		}

		toNode, exists := ir.Nodes[edge.To]
		if !exists {
			return nil, fmt.Errorf("edge references non-existent node: %s", edge.To)
		}

		// Skip if this is handled by branch config
		if edge.Condition == "" || fromNode.Branch == nil {
			// Add to dependents of from_node (only for unconditional edges or non-branch nodes)
			if fromNode.Branch == nil {
				fromNode.Dependents = append(fromNode.Dependents, edge.To)
			}
			// Always add to dependencies of to_node
			toNode.Dependencies = append(toNode.Dependencies, edge.From)
		} else {
			// Conditional edge - dependency already set, branch config handles next nodes
			toNode.Dependencies = append(toNode.Dependencies, edge.From)
		}
	}

	// 3. Set wait_for_all flag for join nodes
	for _, node := range ir.Nodes {
		if len(node.Dependencies) > 1 {
			node.WaitForAll = true
		}
	}

	// 4. Compute terminal nodes
	computeTerminalNodes(ir)

	// 5. Validate IR
	if err := validate(ir); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return ir, nil
}

// convertWorkflowNode converts workflow.schema.json node to IR node with type mapping
func convertWorkflowNode(wfNode *WorkflowNode, conditionalEdges map[string][]WorkflowEdge, casClient sdk.CASClient) (*sdk.Node, error) {
	node := &sdk.Node{
		ID:           wfNode.ID,
		Dependencies: []string{},
		Dependents:   []string{},
	}

	// Store config in CAS
	if len(wfNode.Config) > 0 {
		configJSON, err := json.Marshal(wfNode.Config)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal config: %w", err)
		}
		casID, err := casClient.Put(configJSON, "application/json;type=node_config")
		if err != nil {
			return nil, fmt.Errorf("failed to store config in CAS: %w", err)
		}
		node.ConfigRef = casID
	}

	// Type mapping: workflow.schema.json type → IR type + additional config
	// Executable node types are preserved for specialized routing
	// Control flow nodes (conditional, loop, parallel) are mapped to "task" with special config

	switch wfNode.Type {
	case NodeTypeConditional:
		// Map to task with branch config (routing happens through branch rules)
		node.Type = NodeTypeTask
		branchConfig, err := createBranchConfig(wfNode, conditionalEdges[wfNode.ID])
		if err != nil {
			return nil, fmt.Errorf("failed to create branch config: %w", err)
		}
		node.Branch = branchConfig

	case NodeTypeLoop:
		// Map to task with loop config (routing happens through loop logic)
		node.Type = NodeTypeTask
		loopConfig, err := createLoopConfig(wfNode)
		if err != nil {
			return nil, fmt.Errorf("failed to create loop config: %w", err)
		}
		node.Loop = loopConfig

	case NodeTypeParallel:
		// Parallel is handled at edge level (multiple edges from same source)
		// Just mark as task
		node.Type = NodeTypeTask

	default:
		// All other types (function, http, agent, transform, aggregate, filter, etc.)
		// are preserved as-is for specialized routing by the coordinator
		// This allows for extensibility without modifying the compiler
		if !isValidExecutableType(wfNode.Type) {
			return nil, fmt.Errorf("unknown node type: %s", wfNode.Type)
		}
		node.Type = wfNode.Type
	}

	return node, nil
}

// isValidExecutableType checks if a node type is a valid executable type
// Executable types are those that can be routed to specific streams for execution
func isValidExecutableType(nodeType string) bool {
	return validExecutableTypes[nodeType]
}

// createBranchConfig creates branch config from conditional node
func createBranchConfig(wfNode *WorkflowNode, edges []WorkflowEdge) (*sdk.BranchConfig, error) {
	branchConfig := &sdk.BranchConfig{
		Enabled: true,
		Type:    NodeTypeConditional,
		Rules:   []sdk.BranchRule{},
	}

	// Group edges by condition
	conditionMap := make(map[string][]string)
	var defaultNodes []string

	for _, edge := range edges {
		if edge.Condition != "" {
			conditionMap[edge.Condition] = append(conditionMap[edge.Condition], edge.To)
		} else {
			defaultNodes = append(defaultNodes, edge.To)
		}
	}

	// Create rules for each unique condition
	for condExpr, nextNodes := range conditionMap {
		rule := sdk.BranchRule{
			Condition: createCELCondition(condExpr),
			NextNodes: nextNodes,
		}
		branchConfig.Rules = append(branchConfig.Rules, rule)
	}

	// Set default path
	branchConfig.Default = defaultNodes

	return branchConfig, nil
}

// createLoopConfig creates loop config from loop node
func createLoopConfig(wfNode *WorkflowNode) (*sdk.LoopConfig, error) {
	config := wfNode.Config

	// Extract loop parameters from config
	maxIterations, ok := config["max_iterations"].(float64)
	if !ok || maxIterations <= 0 {
		return nil, fmt.Errorf("loop node missing valid max_iterations in config")
	}

	loopBackTo, ok := config["loop_back_to"].(string)
	if !ok || loopBackTo == "" {
		return nil, fmt.Errorf("loop node missing loop_back_to in config")
	}

	loopConfig := &sdk.LoopConfig{
		Enabled:       true,
		MaxIterations: int(maxIterations),
		LoopBackTo:    loopBackTo,
		BreakPath:     extractStringArray(config, "break_path"),
		TimeoutPath:   extractStringArray(config, "timeout_path"),
	}

	// Optional: condition for breaking loop
	if condExpr, ok := config["condition"].(string); ok && condExpr != "" {
		loopConfig.Condition = createCELCondition(condExpr)
	}

	return loopConfig, nil
}

// createCELCondition creates a CEL condition from an expression string
func createCELCondition(expression string) *sdk.Condition {
	return &sdk.Condition{
		Type:       ConditionTypeCEL,
		Expression: expression,
	}
}

// extractStringArray extracts a string array from a config map
// Returns empty slice if key doesn't exist or is not a valid string array
func extractStringArray(config map[string]interface{}, key string) []string {
	result := []string{}
	if arr, ok := config[key].([]interface{}); ok {
		for _, item := range arr {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
	}
	return result
}

// Compile converts DSL to executable IR (legacy support)
func Compile(dsl *DSL) (*sdk.IR, error) {
	ir := &sdk.IR{
		Version: dsl.Version,
		Nodes:   make(map[string]*sdk.Node),
	}

	// 1. Build nodes
	for _, dslNode := range dsl.Nodes {
		ir.Nodes[dslNode.ID] = &sdk.Node{
			ID:           dslNode.ID,
			Type:         dslNode.Type,
			ConfigRef:    dslNode.ConfigRef,
			Dependencies: []string{},
			Dependents:   []string{},
			Loop:         dslNode.Loop,
			Branch:       dslNode.Branch,
		}
	}

	// 2. Build edges (dependencies and dependents)
	for _, edge := range dsl.Edges {
		fromNode, exists := ir.Nodes[edge.From]
		if !exists {
			return nil, fmt.Errorf("edge references non-existent node: %s", edge.From)
		}

		toNode, exists := ir.Nodes[edge.To]
		if !exists {
			return nil, fmt.Errorf("edge references non-existent node: %s", edge.To)
		}

		// Add to dependents of from_node
		fromNode.Dependents = append(fromNode.Dependents, edge.To)

		// Add to dependencies of to_node
		toNode.Dependencies = append(toNode.Dependencies, edge.From)
	}

	// 3. Set wait_for_all flag for join nodes
	for _, node := range ir.Nodes {
		if len(node.Dependencies) > 1 {
			node.WaitForAll = true
		}
	}

	// 4. Compute terminal nodes
	computeTerminalNodes(ir)

	// 5. Validate IR
	if err := validate(ir); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return ir, nil
}

// computeTerminalNodes marks nodes with no outgoing edges as terminal
func computeTerminalNodes(ir *sdk.IR) {
	for _, node := range ir.Nodes {
		node.IsTerminal = isTerminal(node)
	}
}

// isTerminal checks if a node is terminal (has no outgoing edges)
func isTerminal(node *sdk.Node) bool {
	// 1. Has static dependents?
	if len(node.Dependents) > 0 {
		return false
	}

	// 2. Has branch that can emit?
	if node.Branch != nil && node.Branch.Enabled {
		// Check if any rule has next_nodes
		for _, rule := range node.Branch.Rules {
			if len(rule.NextNodes) > 0 {
				return false
			}
		}
		// Check if default has nodes
		if len(node.Branch.Default) > 0 {
			return false
		}
	}

	// 3. Has loop that can emit?
	if node.Loop != nil && node.Loop.Enabled {
		// Loop can exit to break_path or timeout_path
		if len(node.Loop.BreakPath) > 0 {
			return false
		}
		if len(node.Loop.TimeoutPath) > 0 {
			return false
		}
		// Loop can emit back to loop_back_to
		if node.Loop.LoopBackTo != "" {
			return false
		}
	}

	// No outgoing edges found
	return true
}

// validate checks the IR for correctness
func validate(ir *sdk.IR) error {
	// 1. Check for terminal nodes
	terminalCount := 0
	for _, node := range ir.Nodes {
		if node.IsTerminal {
			terminalCount++
		}
	}

	if terminalCount == 0 {
		return fmt.Errorf("workflow has no terminal nodes (would run forever)")
	}

	// 2. Check for entry nodes (nodes with no dependencies)
	entryCount := 0
	for _, node := range ir.Nodes {
		if len(node.Dependencies) == 0 {
			entryCount++
		}
	}

	if entryCount == 0 {
		return fmt.Errorf("workflow has no entry nodes (no place to start)")
	}

	// 3. Validate loop configs
	for _, node := range ir.Nodes {
		if node.Loop != nil && node.Loop.Enabled {
			if node.Loop.MaxIterations <= 0 {
				return fmt.Errorf("node %s: loop max_iterations must be > 0", node.ID)
			}
			if node.Loop.LoopBackTo == "" {
				return fmt.Errorf("node %s: loop loop_back_to is required", node.ID)
			}
			// Check loop_back_to target exists
			if _, exists := ir.Nodes[node.Loop.LoopBackTo]; !exists {
				return fmt.Errorf("node %s: loop_back_to references non-existent node: %s",
					node.ID, node.Loop.LoopBackTo)
			}
		}
	}

	// 4. Validate branch configs
	for _, node := range ir.Nodes {
		if node.Branch != nil && node.Branch.Enabled {
			// Check branch has rules or default
			if len(node.Branch.Rules) == 0 && len(node.Branch.Default) == 0 {
				return fmt.Errorf("node %s: branch must have rules or default", node.ID)
			}
			// Validate all next_nodes exist
			for i, rule := range node.Branch.Rules {
				for _, nextNode := range rule.NextNodes {
					if _, exists := ir.Nodes[nextNode]; !exists {
						return fmt.Errorf("node %s: branch rule %d references non-existent node: %s",
							node.ID, i, nextNode)
					}
				}
			}
			for _, nextNode := range node.Branch.Default {
				if _, exists := ir.Nodes[nextNode]; !exists {
					return fmt.Errorf("node %s: branch default references non-existent node: %s",
						node.ID, nextNode)
				}
			}
		}
	}

	// 5. Check for cycles (without loop config)
	// Simple DFS-based cycle detection
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var hasCycle func(nodeID string) bool
	hasCycle = func(nodeID string) bool {
		visited[nodeID] = true
		recStack[nodeID] = true

		node := ir.Nodes[nodeID]

		// Check dependents
		for _, dep := range node.Dependents {
			if !visited[dep] {
				if hasCycle(dep) {
					return true
				}
			} else if recStack[dep] {
				// Found cycle, check if it's a loop
				depNode := ir.Nodes[dep]
				if depNode.Loop == nil || !depNode.Loop.Enabled {
					return true
				}
			}
		}

		recStack[nodeID] = false
		return false
	}

	for nodeID := range ir.Nodes {
		if !visited[nodeID] {
			if hasCycle(nodeID) {
				return fmt.Errorf("workflow contains cycles without loop configuration")
			}
		}
	}

	return nil
}

// GetEntryNodes returns nodes with no dependencies (entry points)
func GetEntryNodes(ir *sdk.IR) []*sdk.Node {
	var entries []*sdk.Node
	for _, node := range ir.Nodes {
		if len(node.Dependencies) == 0 {
			entries = append(entries, node)
		}
	}
	return entries
}

// GetTerminalNodes returns nodes with no dependents (terminal nodes)
func GetTerminalNodes(ir *sdk.IR) []*sdk.Node {
	var terminals []*sdk.Node
	for _, node := range ir.Nodes {
		if node.IsTerminal {
			terminals = append(terminals, node)
		}
	}
	return terminals
}

// CountTerminalNodes returns the number of terminal nodes
func CountTerminalNodes(ir *sdk.IR) int {
	count := 0
	for _, node := range ir.Nodes {
		if node.IsTerminal {
			count++
		}
	}
	return count
}
