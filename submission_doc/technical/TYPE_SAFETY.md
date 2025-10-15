# Type Safety & Single Source of Truth

> **JSON Schema as the contract - types generated for all languages**

## 📖 Document Overview

**Purpose:** How JSON Schema enables type safety across 4 languages (Rust, Go, TypeScript, Python)

**In this document:**
- [The Innovation](#the-innovation) - Single source of truth approach
- [JSON Schema as Contract](#json-schema-as-contract) - Schema examples
- [Type Generation](#type-generation) - Commands and tools
- [Benefits](#benefits) - Compile-time safety, zero drift
- [Migration Path](#migration-path) - Adding new fields
- [Validation](#validation-at-api-boundary) - Runtime validation
- [Comparison](#vs-competitors) - vs. other platforms
- [Future: Protobuf](#future-protobuf-migration) - Evolution path

---

## The Innovation

Instead of maintaining separate type definitions in Go, Rust, TypeScript, and Python, we use **JSON Schema as the single source of truth** and auto-generate types for all languages using `typify` and related tools.

## Why This Matters

### Traditional Approach (Broken)
```
workflow.go          ← Go team updates
workflow.rs          ← Rust team updates (out of sync!)
workflow.ts          ← Frontend updates (different fields!)
workflow.py          ← Python agent (completely different!)

Result: Runtime errors, mismatched fields, integration bugs
```

### Our Approach (Type-Safe)
```
workflow.schema.json  ← SINGLE SOURCE OF TRUTH
    ↓
make generate-types
    ↓
├─ workflow.go        (auto-generated, always in sync)
├─ workflow.rs        (auto-generated, always in sync)
├─ workflow.ts        (auto-generated, always in sync)
└─ workflow.py        (auto-generated, always in sync)

Result: Zero type mismatches, validated at compile time
```

---

## JSON Schema as Contract

**Location:** `common/schema/`

### Core Schemas

1. **workflow.schema.json** - Workflow DAG definition (nodes, edges, metadata)
2. **api-responses.schema.json** - API response formats
3. **patch.schema.json** - Agent patch operations (future)
4. **run.schema.json** - Run metadata (future)

### Example: Workflow Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://orchestrator.lyzr.ai/schemas/workflow.json",
  "title": "Workflow",
  "type": "object",
  "required": ["nodes", "edges"],
  "properties": {
    "nodes": {
      "type": "array",
      "items": {"$ref": "#/definitions/Node"}
    },
    "edges": {
      "type": "array",
      "items": {"$ref": "#/definitions/Edge"}
    }
  },
  "definitions": {
    "Node": {
      "type": "object",
      "required": ["id", "type"],
      "properties": {
        "id": {"type": "string", "pattern": "^[a-zA-Z0-9_-]+$"},
        "type": {
          "type": "string",
          "enum": ["function", "http", "agent", "hitl", "conditional"]
        },
        "config": {"type": "object"}
      }
    }
  }
}
```

**See:** [../../common/schema/workflow.schema.json](../../common/schema/workflow.schema.json)

---

## Type Generation

### Commands

```bash
# Generate types for all languages
make generate-types

# Or individually
make generate-rust-types
make generate-go-types
make generate-ts-types
make generate-python-types

# Watch for schema changes
make watch-schema
```

### Generated Artifacts

```
common/schema/workflow.schema.json
  ↓
Generated types:
  ├─ crates/dag-optimizer/src/types.rs      (Rust)
  ├─ cmd/orchestrator/models/workflow.go    (Go)
  ├─ frontend/flow-builder/src/types.ts    (TypeScript)
  └─ cmd/agent-runner-py/types.py          (Python)
```

---

## Tools Used

### 1. Rust - quicktype

```bash
# Install
npm install -g quicktype

# Generate
quicktype \
  --src common/schema/workflow.schema.json \
  --lang rust \
  --out crates/dag-optimizer/src/types.rs \
  --derive-debug \
  --visibility public
```

**Generated code:**
```rust
// Auto-generated from workflow.schema.json
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Workflow {
    pub nodes: Vec<Node>,
    pub edges: Vec<Edge>,
    pub metadata: Option<Metadata>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Node {
    pub id: String,
    #[serde(rename = "type")]
    pub node_type: NodeType,
    pub config: Option<serde_json::Value>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum NodeType {
    Function,
    Http,
    Agent,
    Hitl,
    Conditional,
}
```

**Why quicktype:**
- ✅ Standalone CLI (not a library)
- ✅ Supports 20+ languages
- ✅ Actively maintained
- ✅ Excellent Rust code generation
- ✅ Can infer schemas from JSON examples

---

### 2. Go - go-jsonschema

```bash
# Install
go install github.com/atombender/go-jsonschema@latest

# Generate
go-jsonschema \
  -p models \
  --schema-package=https://orchestrator.lyzr.ai/schemas=models \
  -o cmd/orchestrator/models/workflow.go \
  common/schema/workflow.schema.json
```

**Generated code:**
```go
// Auto-generated from workflow.schema.json
package models

type Workflow struct {
    Nodes    []Node   `json:"nodes"`
    Edges    []Edge   `json:"edges"`
    Metadata *Metadata `json:"metadata,omitempty"`
}

type Node struct {
    ID      string                 `json:"id"`
    Type    NodeType               `json:"type"`
    Config  map[string]interface{} `json:"config,omitempty"`
}

type NodeType string

const (
    NodeTypeFunction    NodeType = "function"
    NodeTypeHttp        NodeType = "http"
    NodeTypeAgent       NodeType = "agent"
    NodeTypeHitl        NodeType = "hitl"
    NodeTypeConditional NodeType = "conditional"
)
```

---

### 3. TypeScript - quicktype

```bash
# Generate
quicktype \
  --src common/schema/workflow.schema.json \
  --lang typescript \
  --out frontend/flow-builder/src/types.ts
```

**Generated code:**
```typescript
// Auto-generated from workflow.schema.json
export interface Workflow {
    nodes: Node[];
    edges: Edge[];
    metadata?: Metadata;
}

export interface Node {
    id: string;
    type: NodeType;
    config?: { [key: string]: any };
}

export type NodeType =
    | "function"
    | "http"
    | "agent"
    | "hitl"
    | "conditional";
```

---

### 4. Python - datamodel-code-generator

```bash
# Install
pip install datamodel-code-generator

# Generate
datamodel-codegen \
  --input common/schema/workflow.schema.json \
  --output cmd/agent-runner-py/types.py \
  --input-file-type jsonschema
```

**Generated code:**
```python
# Auto-generated from workflow.schema.json
from typing import List, Optional, Any, Literal
from pydantic import BaseModel, Field

class Node(BaseModel):
    id: str = Field(..., pattern=r'^[a-zA-Z0-9_-]+$')
    type: Literal["function", "http", "agent", "hitl", "conditional"]
    config: Optional[dict[str, Any]] = None

class Edge(BaseModel):
    from_: str = Field(..., alias="from")
    to: str
    condition: Optional[str] = None

class Workflow(BaseModel):
    nodes: List[Node]
    edges: List[Edge]
    metadata: Optional[dict[str, Any]] = None
```

---

## Benefits

### 1. Compile-Time Safety

**Before (without schemas):**
```python
# Python agent
workflow["nods"]  # Typo! Runtime error!
```

**After (with generated types):**
```python
# Python agent
workflow.nods  # Compile error! IDE catches it!
```

### 2. Automatic Validation

```go
// Go API
func (h *Handler) CreateWorkflow(c echo.Context) error {
    var req Workflow
    if err := c.Bind(&req); err != nil {
        return err  // Validation happens automatically!
    }

    // req is guaranteed to match schema
}
```

### 3. Self-Documenting

```typescript
// TypeScript - IDE autocomplete works!
const workflow: Workflow = {
    nodes: [/* IDE shows Node fields */],
    edges: [/* IDE shows Edge fields */]
};
```

### 4. Backward Compatibility

```json
// Add new optional field to schema
"timeout_ms": {
  "type": "integer",
  "description": "Execution timeout",
  "default": 30000
}

// Old clients still work (optional field)
// New clients get the field
```

### 5. Cross-Language Consistency

```
Frontend sends: {nodes: [...], edges: [...]}
    ↓
Backend receives: Workflow struct (validated)
    ↓
Rust optimizer: WorkflowIR struct (same shape)
    ↓
Python agent: Workflow class (same fields)

All generated from same schema → guaranteed compatibility!
```

---

## Workflow Example

### Define Schema Once

```json
// common/schema/workflow.schema.json
{
  "definitions": {
    "Node": {
      "properties": {
        "id": {"type": "string"},
        "type": {"enum": ["function", "http", "agent"]}
      }
    }
  }
}
```

### Use in All Languages

**Go (Orchestrator):**
```go
import "cmd/orchestrator/models"

func processWorkflow(w *models.Workflow) {
    for _, node := range w.Nodes {
        switch node.Type {
        case models.NodeTypeAgent:
            // Handle agent
        }
    }
}
```

**Rust (Optimizer):**
```rust
use dag_optimizer::types::*;

fn optimize(workflow: &Workflow) -> Workflow {
    workflow.nodes.iter()
        .filter(|n| n.node_type == NodeType::Http)
        .collect()
}
```

**TypeScript (Frontend):**
```typescript
import { Workflow, NodeType } from './types';

function validateWorkflow(w: Workflow) {
    w.nodes.forEach(node => {
        if (node.type === NodeType.Agent) {
            // Handle agent
        }
    });
}
```

**Python (Agent):**
```python
from types import Workflow, NodeType

def analyze_workflow(workflow: Workflow):
    for node in workflow.nodes:
        if node.type == NodeType.AGENT:
            # Handle agent
```

**All type-safe, all guaranteed to match!**

---

## Migration Path

### Phase 1: Add New Field

```json
// common/schema/workflow.schema.json
"Node": {
  "properties": {
    "cache_policy": {
      "type": "string",
      "enum": ["off", "read_only", "read_through"],
      "default": "off"
    }
  }
}
```

### Phase 2: Regenerate Types

```bash
make generate-types
```

### Phase 3: Use New Field

**All languages get the new field automatically:**

```rust
// Rust
node.cache_policy  // ✅ Works!

// Go
node.CachePolicy  // ✅ Works!

// TypeScript
node.cache_policy  // ✅ Works!

// Python
node.cache_policy  // ✅ Works!
```

**Zero manual sync needed!**

---

## Validation at API Boundary

```go
// cmd/orchestrator/handlers/workflow.go
func (h *Handler) CreateWorkflow(c echo.Context) error {
    var workflow models.Workflow

    // Bind + validate against schema
    if err := c.Bind(&workflow); err != nil {
        return c.JSON(400, map[string]string{
            "error": "Invalid workflow",
            "details": err.Error(),
        })
    }

    // Additional business logic validation
    if len(workflow.Nodes) == 0 {
        return c.JSON(400, map[string]string{
            "error": "Workflow must have at least one node",
        })
    }

    // Guaranteed type-safe from here on!
    return h.service.CreateWorkflow(ctx, &workflow)
}
```

---

## Documentation Integration

### Schema README

**Location:** [../../common/schema/README.md](../../common/schema/README.md)

**Key sections:**
- Schema design principles
- Adding new node types
- Validation rules
- Type generation commands

---

## Innovation Summary

### What This Gives Us

1. **Type safety across 4 languages** (Rust, Go, TypeScript, Python)
2. **Single source of truth** (JSON Schema)
3. **Compile-time error detection** (not runtime!)
4. **IDE autocomplete** (all languages)
5. **Automatic validation** (at API boundary)
6. **Zero sync overhead** (generate from schema)
7. **Backward compatibility** (optional fields)
8. **Self-documenting** (schema descriptions → doc comments)

### vs. Competitors

| Platform | Type System | Languages | Source of Truth |
|----------|------------|-----------|----------------|
| Temporal | Go protobuf | Go only | Protobuf |
| Airflow | Python (weak) | Python only | Code |
| n8n | TypeScript | TypeScript only | Code |
| **Ours** | **JSON Schema** | **4 languages** | **JSON Schema** |


---

## Future: Protobuf Migration

Currently: JSON Schema → JSON (runtime)
Future: JSON Schema → Protobuf → Binary (runtime)

**Migration strategy:**
```json
// workflow.schema.json unchanged (source of truth)

// Generate both JSON types AND protobuf
make generate-types        # → JSON types
make generate-proto-types  # → .proto files
make generate-proto-code   # → protobuf types

// Runtime: Accept both JSON and protobuf
Content-Type: application/json       → Use JSON types
Content-Type: application/protobuf   → Use protobuf types
```

**Benefits:**
- Schema remains source of truth
- No code changes needed
- Backward compatible (JSON still works)
- Forward compatible (protobuf opt-in)

---

## Documentation

**Schema docs:** [../../common/schema/README.md](../../common/schema/README.md)
**Vision:** [../architecture/VISION.md](../architecture/VISION.md) - Protobuf migration

---

## Adding to UNIQUENESS.md

This should be added as unique feature #10!
