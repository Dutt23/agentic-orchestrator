package resolver

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/lyzr/orchestrator/common/sdk"
	"github.com/tidwall/gjson"
)

// Resolver handles variable substitution in node configs
type Resolver struct {
	sdk    *sdk.SDK
	logger sdk.Logger
}

// NewResolver creates a new expression resolver
func NewResolver(workflowSDK *sdk.SDK, logger sdk.Logger) *Resolver {
	return &Resolver{
		sdk:    workflowSDK,
		logger: logger,
	}
}

// ResolveConfig resolves all variable expressions in a config map
// Supports:
// - $nodes.node_id - entire node output
// - $nodes.node_id.field - specific field access
// - ${$nodes.node_id.field} - string interpolation
func (r *Resolver) ResolveConfig(ctx context.Context, runID string, config map[string]interface{}) (map[string]interface{}, error) {
	resolved := make(map[string]interface{})

	for key, value := range config {
		resolvedValue, err := r.resolveValue(ctx, runID, value)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve config key %s: %w", key, err)
		}
		resolved[key] = resolvedValue
	}

	return resolved, nil
}

// resolveValue recursively resolves a value (string, map, array, etc.)
func (r *Resolver) resolveValue(ctx context.Context, runID string, value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case string:
		return r.resolveString(ctx, runID, v)
	case map[string]interface{}:
		return r.resolveMap(ctx, runID, v)
	case []interface{}:
		return r.resolveArray(ctx, runID, v)
	default:
		// Primitives (int, bool, etc.) pass through
		return value, nil
	}
}

// resolveString handles string expressions
func (r *Resolver) resolveString(ctx context.Context, runID, str string) (interface{}, error) {
	// Case 1: Full node reference: "$nodes.node_id" or "$nodes.node_id.field"
	if strings.HasPrefix(str, "$nodes.") {
		return r.resolveNodeReference(ctx, runID, str)
	}

	// Case 2: String interpolation: "text ${$nodes.node_id} more text"
	if strings.Contains(str, "${") {
		return r.resolveInterpolation(ctx, runID, str)
	}

	// Case 3: Plain string, no substitution needed
	return str, nil
}

// resolveMap recursively resolves all values in a map
func (r *Resolver) resolveMap(ctx context.Context, runID string, m map[string]interface{}) (map[string]interface{}, error) {
	resolved := make(map[string]interface{})
	for key, value := range m {
		resolvedValue, err := r.resolveValue(ctx, runID, value)
		if err != nil {
			return nil, err
		}
		resolved[key] = resolvedValue
	}
	return resolved, nil
}

// resolveArray recursively resolves all items in an array
func (r *Resolver) resolveArray(ctx context.Context, runID string, arr []interface{}) ([]interface{}, error) {
	resolved := make([]interface{}, len(arr))
	for i, value := range arr {
		resolvedValue, err := r.resolveValue(ctx, runID, value)
		if err != nil {
			return nil, err
		}
		resolved[i] = resolvedValue
	}
	return resolved, nil
}

// resolveNodeReference resolves "$nodes.node_id" or "$nodes.node_id.field.path"
func (r *Resolver) resolveNodeReference(ctx context.Context, runID, expr string) (interface{}, error) {
	// Remove "$nodes." prefix
	expr = strings.TrimPrefix(expr, "$nodes.")

	// Split into node_id and path
	parts := strings.SplitN(expr, ".", 2)
	nodeID := parts[0]

	// Load node output
	output, err := r.sdk.LoadNodeOutput(ctx, runID, nodeID)
	if err != nil {
		r.logger.Error("failed to load node output", "node_id", nodeID, "error", err)
		return nil, fmt.Errorf("node output not found: %s", nodeID)
	}

	// If no field path, return entire output
	if len(parts) == 1 {
		return output, nil
	}

	// Extract specific field using gjson
	fieldPath := parts[1]
	outputJSON, err := json.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal node output: %w", err)
	}

	result := gjson.GetBytes(outputJSON, fieldPath)
	if !result.Exists() {
		return nil, fmt.Errorf("field not found: %s in node %s", fieldPath, nodeID)
	}

	return result.Value(), nil
}

// resolveInterpolation handles string interpolation "${$nodes.node_id.field}"
func (r *Resolver) resolveInterpolation(ctx context.Context, runID, str string) (string, error) {
	// Pattern: ${$nodes.node_id.field.path}
	pattern := regexp.MustCompile(`\$\{([^}]+)\}`)

	result := str
	matches := pattern.FindAllStringSubmatch(str, -1)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		placeholder := match[0] // Full match: ${...}
		expr := match[1]        // Inner expression: $nodes.node_id.field

		// Resolve the expression
		value, err := r.resolveString(ctx, runID, expr)
		if err != nil {
			return "", fmt.Errorf("failed to resolve interpolation %s: %w", placeholder, err)
		}

		// Convert value to string
		var valueStr string
		switch v := value.(type) {
		case string:
			valueStr = v
		case []byte:
			valueStr = string(v)
		default:
			// For complex types, marshal to JSON
			jsonBytes, err := json.Marshal(v)
			if err != nil {
				return "", fmt.Errorf("failed to marshal interpolated value: %w", err)
			}
			valueStr = string(jsonBytes)
		}

		// Replace placeholder with value
		result = strings.Replace(result, placeholder, valueStr, 1)
	}

	return result, nil
}
