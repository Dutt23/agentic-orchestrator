package compiler

import (
	"context"
	"encoding/json"
	"testing"

	_ "github.com/lyzr/orchestrator/common/sdk"
)

// MockCASClient for testing
type MockCASClient struct {
	storage map[string][]byte
}

func NewMockCASClient() *MockCASClient {
	return &MockCASClient{
		storage: make(map[string][]byte),
	}
}

func (m *MockCASClient) Get(ctx context.Context, ref string) (interface{}, error) {
	data, exists := m.storage[ref]
	if !exists {
		return nil, nil
	}
	var result interface{}
	json.Unmarshal(data, &result)
	return result, nil
}

func (m *MockCASClient) Put(ctx context.Context, data []byte, mediaType string) (string, error) {
	ref := "cas://test-" + string(data[:10])
	m.storage[ref] = data
	return ref, nil
}

func (m *MockCASClient) Store(ctx context.Context, data interface{}) (string, error) {
	jsonData, _ := json.Marshal(data)
	return m.Put(ctx, jsonData, "application/json")
}

// TestCompileWorkflowSchema_SimpleSequential tests A->B->C sequential workflow
func TestCompileWorkflowSchema_SimpleSequential(t *testing.T) {
	schema := &WorkflowSchema{
		Nodes: []WorkflowNode{
			{ID: "A", Type: "function", Config: map[string]interface{}{"name": "taskA"}},
			{ID: "B", Type: "transform", Config: map[string]interface{}{"type": "uppercase"}},
			{ID: "C", Type: "http", Config: map[string]interface{}{"url": "http://example.com"}},
		},
		Edges: []WorkflowEdge{
			{From: "A", To: "B"},
			{From: "B", To: "C"},
		},
	}

	casClient := NewMockCASClient()
	ir, err := CompileWorkflowSchema(schema, casClient)
	if err != nil {
		t.Fatalf("CompileWorkflowSchema failed: %v", err)
	}

	// Verify nodes
	if len(ir.Nodes) != 3 {
		t.Errorf("Expected 3 nodes, got %d", len(ir.Nodes))
	}

	// Verify node types are preserved
	nodeA := ir.Nodes["A"]
	if nodeA.Type != "function" {
		t.Errorf("Node A: expected type 'function', got '%s'", nodeA.Type)
	}

	nodeB := ir.Nodes["B"]
	if nodeB.Type != "transform" {
		t.Errorf("Node B: expected type 'transform', got '%s'", nodeB.Type)
	}

	nodeC := ir.Nodes["C"]
	if nodeC.Type != "http" {
		t.Errorf("Node C: expected type 'http', got '%s'", nodeC.Type)
	}

	// Verify dependencies
	if len(nodeB.Dependencies) != 1 || nodeB.Dependencies[0] != "A" {
		t.Errorf("Node B: expected dependency [A], got %v", nodeB.Dependencies)
	}

	// Verify terminal node
	if !nodeC.IsTerminal {
		t.Errorf("Node C should be marked as terminal")
	}

	// Verify entry node
	if len(nodeA.Dependencies) != 0 {
		t.Errorf("Node A should have no dependencies (entry node)")
	}
}

// TestCompileWorkflowSchema_ParallelFanOut tests A->(B,C)->D parallel execution
func TestCompileWorkflowSchema_ParallelFanOut(t *testing.T) {
	schema := &WorkflowSchema{
		Nodes: []WorkflowNode{
			{ID: "A", Type: "function", Config: map[string]interface{}{"name": "prepare"}},
			{ID: "B", Type: "function", Config: map[string]interface{}{"name": "path1"}},
			{ID: "C", Type: "function", Config: map[string]interface{}{"name": "path2"}},
			{ID: "D", Type: "aggregate", Config: map[string]interface{}{"strategy": "merge"}},
		},
		Edges: []WorkflowEdge{
			{From: "A", To: "B"},
			{From: "A", To: "C"},
			{From: "B", To: "D"},
			{From: "C", To: "D"},
		},
	}

	casClient := NewMockCASClient()
	ir, err := CompileWorkflowSchema(schema, casClient)
	if err != nil {
		t.Fatalf("CompileWorkflowSchema failed: %v", err)
	}

	// Verify node D is a join node (wait_for_all)
	nodeD := ir.Nodes["D"]
	if !nodeD.WaitForAll {
		t.Errorf("Node D should have wait_for_all=true (join node)")
	}

	if len(nodeD.Dependencies) != 2 {
		t.Errorf("Node D: expected 2 dependencies, got %d", len(nodeD.Dependencies))
	}

	// Verify node A has 2 dependents
	nodeA := ir.Nodes["A"]
	if len(nodeA.Dependents) != 2 {
		t.Errorf("Node A: expected 2 dependents, got %d", len(nodeA.Dependents))
	}

	// Verify terminal node
	if !nodeD.IsTerminal {
		t.Errorf("Node D should be marked as terminal")
	}
}

// TestCompileWorkflowSchema_ConditionalBranch tests conditional branching
func TestCompileWorkflowSchema_ConditionalBranch(t *testing.T) {
	schema := &WorkflowSchema{
		Nodes: []WorkflowNode{
			{ID: "check", Type: "conditional", Config: map[string]interface{}{}},
			{ID: "high", Type: "function", Config: map[string]interface{}{"name": "high_path"}},
			{ID: "low", Type: "function", Config: map[string]interface{}{"name": "low_path"}},
		},
		Edges: []WorkflowEdge{
			{From: "check", To: "high", Condition: "output.score > 80"},
			{From: "check", To: "low", Condition: "output.score <= 80"},
		},
	}

	casClient := NewMockCASClient()
	ir, err := CompileWorkflowSchema(schema, casClient)
	if err != nil {
		t.Fatalf("CompileWorkflowSchema failed: %v", err)
	}

	// Verify conditional node has branch config
	nodeCheck := ir.Nodes["check"]
	if nodeCheck.Branch == nil {
		t.Fatalf("Node 'check' should have branch config")
	}

	if !nodeCheck.Branch.Enabled {
		t.Errorf("Branch config should be enabled")
	}

	if nodeCheck.Branch.Type != "conditional" {
		t.Errorf("Branch type should be 'conditional', got '%s'", nodeCheck.Branch.Type)
	}

	// Verify branch rules
	if len(nodeCheck.Branch.Rules) != 2 {
		t.Errorf("Expected 2 branch rules, got %d", len(nodeCheck.Branch.Rules))
	}

	// Verify dependencies are set correctly
	nodeHigh := ir.Nodes["high"]
	if len(nodeHigh.Dependencies) != 1 || nodeHigh.Dependencies[0] != "check" {
		t.Errorf("Node 'high': expected dependency [check], got %v", nodeHigh.Dependencies)
	}

	// Verify both paths are terminal
	if !nodeHigh.IsTerminal || !ir.Nodes["low"].IsTerminal {
		t.Errorf("Both high and low paths should be terminal")
	}
}

// TestCompileWorkflowSchema_Loop tests loop configuration
func TestCompileWorkflowSchema_Loop(t *testing.T) {
	schema := &WorkflowSchema{
		Nodes: []WorkflowNode{
			{ID: "start", Type: "function", Config: map[string]interface{}{"name": "init"}},
			{
				ID:   "retry",
				Type: "loop",
				Config: map[string]interface{}{
					"max_iterations": 5.0,
					"loop_back_to":   "retry",
					"condition":      "output.status != 'success'",
					"break_path":     []interface{}{"success"},
				},
			},
			{ID: "success", Type: "function", Config: map[string]interface{}{"name": "handle_success"}},
		},
		Edges: []WorkflowEdge{
			{From: "start", To: "retry"},
		},
	}

	casClient := NewMockCASClient()
	ir, err := CompileWorkflowSchema(schema, casClient)
	if err != nil {
		t.Fatalf("CompileWorkflowSchema failed: %v", err)
	}

	// Verify loop node has loop config
	nodeRetry := ir.Nodes["retry"]
	if nodeRetry.Loop == nil {
		t.Fatalf("Node 'retry' should have loop config")
	}

	if !nodeRetry.Loop.Enabled {
		t.Errorf("Loop config should be enabled")
	}

	if nodeRetry.Loop.MaxIterations != 5 {
		t.Errorf("Expected max_iterations=5, got %d", nodeRetry.Loop.MaxIterations)
	}

	if nodeRetry.Loop.LoopBackTo != "retry" {
		t.Errorf("Expected loop_back_to='retry', got '%s'", nodeRetry.Loop.LoopBackTo)
	}

	// Verify condition
	if nodeRetry.Loop.Condition == nil {
		t.Fatalf("Loop should have condition")
	}

	if nodeRetry.Loop.Condition.Type != "cel" {
		t.Errorf("Condition type should be 'cel', got '%s'", nodeRetry.Loop.Condition.Type)
	}

	// Verify break path
	if len(nodeRetry.Loop.BreakPath) != 1 || nodeRetry.Loop.BreakPath[0] != "success" {
		t.Errorf("Expected break_path=[success], got %v", nodeRetry.Loop.BreakPath)
	}

	// Verify terminal detection
	nodeSuccess := ir.Nodes["success"]
	if !nodeSuccess.IsTerminal {
		t.Errorf("Node 'success' should be terminal")
	}
}

// TestCompileWorkflowSchema_TypeMapping tests all type mappings
func TestCompileWorkflowSchema_TypeMapping(t *testing.T) {
	tests := []struct {
		inputType    string
		expectedType string
	}{
		{"function", "function"},   // Executable types are preserved
		{"http", "http"},
		{"transform", "transform"},
		{"aggregate", "aggregate"},
		{"filter", "filter"},
		{"parallel", "task"},       // Control flow type mapped to task
	}

	for _, tt := range tests {
		schema := &WorkflowSchema{
			Nodes: []WorkflowNode{
				{ID: "test", Type: tt.inputType, Config: map[string]interface{}{}},
			},
			Edges: []WorkflowEdge{},
		}

		casClient := NewMockCASClient()
		ir, err := CompileWorkflowSchema(schema, casClient)
		if err != nil {
			t.Errorf("Failed to compile node type '%s': %v", tt.inputType, err)
			continue
		}

		node := ir.Nodes["test"]
		// Verify the type mapping is correct
		if node.Type != tt.expectedType {
			t.Errorf("Type '%s': expected IR type '%s', got '%s'", tt.inputType, tt.expectedType, node.Type)
		}
	}
}

// TestCompileWorkflowSchema_Validation tests validation errors
func TestCompileWorkflowSchema_Validation(t *testing.T) {
	tests := []struct {
		name        string
		schema      *WorkflowSchema
		expectError bool
		errorMsg    string
	}{
		{
			name: "missing_node_in_edge",
			schema: &WorkflowSchema{
				Nodes: []WorkflowNode{{ID: "A", Type: "function"}},
				Edges: []WorkflowEdge{{From: "A", To: "B"}},
			},
			expectError: true,
			errorMsg:    "non-existent node",
		},
		{
			name: "no_terminal_nodes",
			schema: &WorkflowSchema{
				Nodes: []WorkflowNode{
					{ID: "A", Type: "function"},
					{ID: "B", Type: "function"},
				},
				Edges: []WorkflowEdge{
					{From: "A", To: "B"},
					{From: "B", To: "A"},
				},
			},
			expectError: true,
			errorMsg:    "no terminal nodes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			casClient := NewMockCASClient()
			_, err := CompileWorkflowSchema(tt.schema, casClient)

			if tt.expectError && err == nil {
				t.Errorf("Expected error containing '%s', got nil", tt.errorMsg)
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}
		})
	}
}
