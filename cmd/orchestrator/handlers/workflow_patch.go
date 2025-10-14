package handlers

import (
	"fmt"
	"strconv"
	"strings"
)

// WorkflowPatcher handles JSON Patch operations on workflows
type WorkflowPatcher struct{}

// ApplyJSONPatchToWorkflow applies JSON Patch operations to a workflow
func (p *WorkflowPatcher) ApplyJSONPatchToWorkflow(workflow map[string]interface{}, operations []map[string]interface{}) (map[string]interface{}, error) {
	// Create a deep copy of the workflow to avoid modifying the original
	patchedWorkflow := make(map[string]interface{})
	for k, v := range workflow {
		// Deep copy arrays
		if k == "nodes" || k == "edges" {
			if arr, ok := v.([]interface{}); ok {
				copyArr := make([]interface{}, len(arr))
				copy(copyArr, arr)
				patchedWorkflow[k] = copyArr
			} else {
				patchedWorkflow[k] = v
			}
		} else {
			patchedWorkflow[k] = v
		}
	}

	// Apply each operation
	for i, op := range operations {
		opType, ok := op["op"].(string)
		if !ok {
			return nil, fmt.Errorf("operation %d missing 'op' field", i)
		}

		path, ok := op["path"].(string)
		if !ok {
			return nil, fmt.Errorf("operation %d missing 'path' field", i)
		}

		switch opType {
		case "add":
			if err := p.applyAddOperation(patchedWorkflow, path, op["value"]); err != nil {
				return nil, fmt.Errorf("operation %d (add) failed: %w", i, err)
			}

		case "remove":
			if err := p.applyRemoveOperation(patchedWorkflow, path); err != nil {
				return nil, fmt.Errorf("operation %d (remove) failed: %w", i, err)
			}

		case "replace":
			if err := p.applyReplaceOperation(patchedWorkflow, path, op["value"]); err != nil {
				return nil, fmt.Errorf("operation %d (replace) failed: %w", i, err)
			}

		default:
			return nil, fmt.Errorf("unsupported operation type: %s", opType)
		}
	}

	return patchedWorkflow, nil
}

// applyAddOperation handles "add" operations
func (p *WorkflowPatcher) applyAddOperation(workflow map[string]interface{}, path string, value interface{}) error {
	if path == "/nodes/-" {
		// Add node to the end of nodes array
		nodes, ok := workflow["nodes"].([]interface{})
		if !ok {
			workflow["nodes"] = []interface{}{value}
			return nil
		}
		workflow["nodes"] = append(nodes, value)
		return nil
	}

	if path == "/edges/-" {
		// Add edge to the end of edges array
		edges, ok := workflow["edges"].([]interface{})
		if !ok {
			workflow["edges"] = []interface{}{value}
			return nil
		}
		workflow["edges"] = append(edges, value)
		return nil
	}

	return fmt.Errorf("unsupported add path: %s", path)
}

// applyRemoveOperation handles "remove" operations
func (p *WorkflowPatcher) applyRemoveOperation(workflow map[string]interface{}, path string) error {
	// Parse path like "/nodes/2" or "/edges/1"
	// Split by "/" and parse components
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid remove path format: %s (expected format: /collection/index)", path)
	}

	collection := parts[0]
	index, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("invalid index in path %s: %v", path, err)
	}

	if collection == "nodes" {
		nodes, ok := workflow["nodes"].([]interface{})
		if !ok || index < 0 || index >= len(nodes) {
			return fmt.Errorf("invalid node index: %d", index)
		}
		workflow["nodes"] = append(nodes[:index], nodes[index+1:]...)
		return nil
	}

	if collection == "edges" {
		edges, ok := workflow["edges"].([]interface{})
		if !ok || index < 0 || index >= len(edges) {
			return fmt.Errorf("invalid edge index: %d", index)
		}
		workflow["edges"] = append(edges[:index], edges[index+1:]...)
		return nil
	}

	return fmt.Errorf("unsupported remove collection: %s", collection)
}

// applyReplaceOperation handles "replace" operations
func (p *WorkflowPatcher) applyReplaceOperation(workflow map[string]interface{}, path string, value interface{}) error {
	// Parse path like "/nodes/2" or "/edges/1"
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid replace path format: %s (expected format: /collection/index)", path)
	}

	collection := parts[0]
	index, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("invalid index in path %s: %v", path, err)
	}

	if collection == "nodes" {
		nodes, ok := workflow["nodes"].([]interface{})
		if !ok || index < 0 || index >= len(nodes) {
			return fmt.Errorf("invalid node index: %d", index)
		}
		nodes[index] = value
		return nil
	}

	if collection == "edges" {
		edges, ok := workflow["edges"].([]interface{})
		if !ok || index < 0 || index >= len(edges) {
			return fmt.Errorf("invalid edge index: %d", index)
		}
		edges[index] = value
		return nil
	}

	return fmt.Errorf("unsupported replace collection: %s", collection)
}
