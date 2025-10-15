package validation

import (
	"fmt"
)

// PatchValidator validates JSON Patch operations for workflows
type PatchValidator struct{}

// NewPatchValidator creates a new patch validator
func NewPatchValidator() *PatchValidator {
	return &PatchValidator{}
}

// ValidateOperations validates all patch operations
func (v *PatchValidator) ValidateOperations(operations []map[string]interface{}) error {
	agentCount := 0

	for i, op := range operations {
		// Validate operation structure
		if err := v.validateOperation(op, i); err != nil {
			return err
		}

		// Count agent nodes being added
		if op["op"] == "add" && op["path"] == "/nodes/-" {
			if value, ok := op["value"].(map[string]interface{}); ok {
				if nodeType, ok := value["type"].(string); ok && nodeType == "agent" {
					agentCount++
				}
			}
		}
	}

	// Enforce limit: max 5 agent nodes per patch
	if agentCount > 5 {
		return fmt.Errorf("patch validation failed: cannot add more than 5 agent nodes per patch (attempted: %d)", agentCount)
	}

	return nil
}

// validateOperation validates a single operation
func (v *PatchValidator) validateOperation(op map[string]interface{}, index int) error {
	// Check required fields
	opType, ok := op["op"].(string)
	if !ok {
		return fmt.Errorf("operation %d: missing or invalid 'op' field", index)
	}

	path, ok := op["path"].(string)
	if !ok {
		return fmt.Errorf("operation %d: missing or invalid 'path' field", index)
	}

	// Validate based on operation type
	switch opType {
	case "add", "replace":
		if _, ok := op["value"]; !ok {
			return fmt.Errorf("operation %d: 'value' required for %s operation", index, opType)
		}

		// Special validation for node additions
		if path == "/nodes/-" {
			if err := v.validateNodeValue(op["value"], index); err != nil {
				return err
			}
		}

	case "remove":
		// Remove doesn't need value
		return nil

	default:
		return fmt.Errorf("operation %d: unsupported operation type: %s", index, opType)
	}

	return nil
}

// validateNodeValue validates a node value in a patch
func (v *PatchValidator) validateNodeValue(value interface{}, opIndex int) error {
	nodeValue, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("operation %d: node value must be an object, got %T", opIndex, value)
	}

	// Check required fields
	if _, ok := nodeValue["id"].(string); !ok {
		return fmt.Errorf("operation %d: node must have 'id' field (string)", opIndex)
	}

	if _, ok := nodeValue["type"].(string); !ok {
		return fmt.Errorf("operation %d: node must have 'type' field (string)", opIndex)
	}

	// Validate config if present
	if config, exists := nodeValue["config"]; exists {
		// Config MUST be an object, not array/string
		if _, ok := config.(map[string]interface{}); !ok {
			return fmt.Errorf("operation %d: node 'config' must be an object, got %T (hint: use {\"key\": \"value\"}, not [\"key\"])", opIndex, config)
		}
	}

	return nil
}
