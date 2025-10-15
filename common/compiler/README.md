# Workflow IR Compiler

This package compiles workflow definitions into an executable Intermediate Representation (IR) format used by the workflow-runner service.

## Overview

The compiler performs the following transformations:

1. **Type Mapping**: Converts workflow.schema.json node types to IR task types
2. **Dependency Analysis**: Builds dependency and dependent relationships
3. **Terminal Detection**: Pre-computes which nodes are terminal (optimization)
4. **Join Pattern Detection**: Identifies nodes that need wait_for_all behavior
5. **Branch/Loop Config**: Creates branch and loop configurations from node types
6. **Validation**: Ensures workflow is valid (has entry/terminal nodes, no cycles)

## Type Mapping

| workflow.schema.json Type | IR Type | Additional Config |
|---------------------------|---------|-------------------|
| `function`                | `task`  | None              |
| `http`                    | `task`  | None              |
| `transform`               | `task`  | None              |
| `aggregate`               | `task`  | None              |
| `filter`                  | `task`  | None              |
| `conditional`             | `task`  | + `branch` config |
| `loop`                    | `task`  | + `loop` config   |
| `parallel`                | `task`  | None (handled by edges) |

## Code Generation

### Current State

The types in `ir.go` are **manually defined** to match `common/schema/workflow.schema.json`.

### Future: Auto-Generated Types

To ensure types stay in sync with the schema, we should auto-generate them using a tool like:

- **[quicktype](https://github.com/quicktype/quicktype)**: Multi-language type generation
- **[go-jsonschema](https://github.com/atombender/go-jsonschema)**: Go-specific JSON Schema generator
- **[typify](https://github.com/oxidecomputer/typify)**: Rust-style type generation

### Recommended Setup

1. Install code generation tool:
```bash
# Using quicktype
npm install -g quicktype

# OR using go-jsonschema
go install github.com/atombender/go-jsonschema@latest
```

2. Add generation script to Makefile:
```makefile
.PHONY: generate-types
generate-types:
	quicktype \
		--src common/schema/workflow.schema.json \
		--lang go \
		--package compiler \
		--out cmd/workflow-runner/compiler/workflow_schema_gen.go \
		--just-types
```

3. Add to CI/CD pipeline:
```yaml
# .github/workflows/generate-types.yml
name: Generate Types
on:
  push:
    paths:
      - 'common/schema/workflow.schema.json'
jobs:
  generate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Generate types
        run: make generate-types
      - name: Commit changes
        run: |
          git config --global user.name "GitHub Actions"
          git add cmd/workflow-runner/compiler/workflow_schema_gen.go
          git commit -m "chore: regenerate workflow schema types"
          git push
```

## Usage

### Compile from WorkflowSchema

```go
import (
    "github.com/lyzr/orchestrator/cmd/workflow-runner/compiler"
    "github.com/lyzr/orchestrator/cmd/workflow-runner/sdk"
)

// Parse workflow.schema.json format
var schema compiler.WorkflowSchema
json.Unmarshal(workflowJSON, &schema)

// Create CAS client (for storing configs)
casClient := NewCASClient()

// Compile to IR
ir, err := compiler.CompileWorkflowSchema(&schema, casClient)
if err != nil {
    return fmt.Errorf("compilation failed: %w", err)
}

// IR is now ready for execution
```

### Compile from Legacy DSL (backward compatibility)

```go
// Parse legacy DSL format
var dsl compiler.DSL
json.Unmarshal(dslJSON, &dsl)

// Compile to IR
ir, err := compiler.Compile(&dsl)
if err != nil {
    return fmt.Errorf("compilation failed: %w", err)
}
```

## IR Structure

The compiled IR has the following structure:

```json
{
  "version": "1.0",
  "nodes": {
    "fetch": {
      "id": "fetch",
      "type": "task",
      "config_ref": "cas://sha256:abc123...",
      "dependencies": [],
      "dependents": ["check"],
      "is_terminal": false
    },
    "check": {
      "id": "check",
      "type": "task",
      "config_ref": "cas://sha256:def456...",
      "dependencies": ["fetch"],
      "dependents": [],
      "is_terminal": true,
      "branch": {
        "enabled": true,
        "type": "conditional",
        "rules": [
          {
            "condition": {
              "type": "cel",
              "expression": "output.score > 80"
            },
            "next_nodes": ["process"]
          }
        ],
        "default": []
      }
    }
  }
}
```

## Validation Rules

The compiler enforces the following validation rules:

1. **Has Entry Nodes**: At least one node with no dependencies
2. **Has Terminal Nodes**: At least one node with no outgoing edges
3. **No Dangling Edges**: All edges reference existing nodes
4. **No Invalid Cycles**: Cycles are only allowed with loop config
5. **Valid Loop Config**: Loop nodes must have `loop_back_to` and `max_iterations`
6. **Valid Branch Config**: Branch nodes must have rules or default path

## Examples

See `ir_test.go` for comprehensive examples:

- `TestCompileWorkflowSchema_SimpleSequential`: A→B→C sequential flow
- `TestCompileWorkflowSchema_ParallelFanOut`: A→(B,C)→D parallel with join
- `TestCompileWorkflowSchema_ConditionalBranch`: Conditional branching
- `TestCompileWorkflowSchema_Loop`: Loop with break/timeout paths
- `TestCompileWorkflowSchema_TypeMapping`: All type mappings
- `TestCompileWorkflowSchema_Validation`: Validation error cases

## Performance

The compiler is designed to be fast:

- **O(N+E)** complexity for graph traversal
- **Single pass** for terminal node detection
- **In-memory** processing, no disk I/O
- **Concurrent safe**: Each compilation is independent

Typical compilation times:
- Small workflow (10 nodes): < 1ms
- Medium workflow (100 nodes): < 5ms
- Large workflow (1000 nodes): < 50ms

## Future Improvements

1. **Type Generation**: Auto-generate types from schema
2. **Schema Validation**: Validate input against JSON Schema
3. **Optimization Passes**: Dead code elimination, constant folding
4. **Static Analysis**: Detect potential runtime issues at compile time
5. **Incremental Compilation**: Only recompile changed nodes
6. **Parallel Compilation**: Compile independent subgraphs concurrently
