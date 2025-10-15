# Schema-Based Type Generation Setup

## ✅ What's Been Created

### 1. Schema Infrastructure
- **Location**: `common/schema/`
- **Files**:
  - `workflow.schema.json` - Core workflow DAG schema (nodes, edges, metadata)
  - `examples/simple-workflow.json` - Example workflow
  - `README.md` - Documentation

### 2. DAG Optimizer Crate
- **Location**: `crates/dag-optimizer/`
- **Purpose**: Shared Rust library for workflow optimization
- **Features**:
  - Type definitions (generated from schema)
  - Optimization algorithms (stubbed for now)
  - WASM support (optional feature)
- **Can be used by**:
  - aob-cli (command-line tool)
  - WASM (browser integration)
  - Future: Go backend via FFI

### 3. Makefile Targets
New commands added to root Makefile:
```bash
make generate-types      # Generate Rust + Go types
make watch-schema        # Auto-regenerate on changes
make validate-schema     # Validate schemas
make clean-generated     # Remove generated files
```

## 📋 Next Steps

### Step 1: Install Code Generation Tools

```bash
# Easy way: Run the setup script
./scripts/setup-codegen.sh

# Or install manually:

# Install quicktype (multi-language code generator)
npm install -g quicktype

# Install go-jsonschema for Go type generation (optional)
go install github.com/atombender/go-jsonschema@latest
```

### Step 2: Generate Types

```bash
# Generate types for both Rust and Go
make generate-types
```

This will create:
- `crates/dag-optimizer/src/types.rs` (Rust)
- `cmd/orchestrator/models/generated/workflow.go` (Go)

### Step 3: Update CLI to Use Generated Types

The CLI currently has manual structs in:
- `cmd/aob-cli/src/commands/workflow.rs`

These need to be replaced with:
```rust
use dag_optimizer::{Workflow, Node, Edge};
```

### Step 4: Test Integration

```bash
# Test the dag-optimizer crate
cd crates/dag-optimizer
cargo test

# Test CLI with generated types
cd cmd/aob-cli
cargo run -- workflow list
```

## 🎯 Benefits of This Setup

### Single Source of Truth
```
common/schema/workflow.schema.json
         │
         ├──> [typify] ──> Rust types (dag-optimizer)
         │                      │
         │                      ├──> CLI (aob-cli)
         │                      └──> WASM (browser)
         │
         └──> [go-jsonschema] ──> Go types (orchestrator)
```

### Type Safety Across Languages
- **Rust**: Compile-time type checking
- **Go**: Struct tags for JSON validation
- **Guaranteed compatibility**: Schema ensures both match

### Easy Evolution
1. Update `workflow.schema.json`
2. Run `make generate-types`
3. Both Rust and Go types update automatically
4. Compiler catches any breaking changes

## 🔧 Code Generation Tools

### quicktype (Recommended)
- **What**: Multi-language code generator
- **Install**: `npm install -g quicktype`
- **Why**: Standalone CLI, supports 20+ languages, actively maintained
- **Usage**: Automatically used by `make generate-types`

### Alternative: typify (Library-based)
If you prefer to use `typify` (a Rust library), you can:
1. Add it to `crates/dag-optimizer/Cargo.toml` as a build dependency
2. Use it in `build.rs` to generate types at compile time

We use `quicktype` for simplicity since it's a standalone CLI tool.

## 📚 Schema Design

### Current Schema Supports

**Node Types:**
- `function` - Execute a function
- `http` - HTTP request
- `conditional` - Branching logic
- `loop` - Iteration
- `parallel` - Concurrent execution
- `transform` - Data transformation
- `aggregate` - Combine data
- `filter` - Filter data

**Features:**
- Timeout configuration per node
- Retry policies with backoff
- Conditional edges
- Workflow metadata (name, description, version, tags)

### Adding New Node Types

1. Edit `common/schema/workflow.schema.json`
2. Add to the `Node.type` enum:
   ```json
   "enum": [
     "function",
     "http",
     "your_new_type"  // Add here
   ]
   ```
3. Run `make generate-types`
4. Types automatically include the new variant

## 🔧 Development Workflow

### Watch Mode (Recommended)
```bash
# Terminal 1: Watch schemas
make watch-schema

# Terminal 2: Develop
# Edit schema files
# Types auto-regenerate!
```

### Manual Mode
```bash
# 1. Edit schema
vim common/schema/workflow.schema.json

# 2. Regenerate types
make generate-types

# 3. Build
make build
```

## 📦 Crate Structure

```
crates/dag-optimizer/
├── Cargo.toml          # Crate configuration
├── README.md           # Documentation
└── src/
    ├── lib.rs          # Main library
    └── types.rs        # Generated from schema (after make generate-types)
```

**Add to CLI's Cargo.toml:**
```toml
[dependencies]
dag-optimizer = { path = "../../crates/dag-optimizer" }
```

## 🌐 Future: WASM Integration

The dag-optimizer crate supports WASM:

```bash
# Build for WASM
cd crates/dag-optimizer
cargo build --target wasm32-unknown-unknown --features wasm
wasm-bindgen target/wasm32-unknown-unknown/release/dag_optimizer.wasm --out-dir pkg
```

Then use in browser:
```javascript
import init, { Optimizer } from './pkg/dag_optimizer';

await init();
const optimizer = new Optimizer();
const suggestions = optimizer.analyze(workflowJson);
```

## 🧪 Testing

### Validate Example Workflows
```bash
# Requires: npm install -g ajv-cli
make validate-schema
```

### Test Type Compatibility
```bash
# Create test in crates/dag-optimizer/src/lib.rs
#[test]
fn test_workflow_parse() {
    let json = include_str!("../../../common/schema/examples/simple-workflow.json");
    let workflow: Workflow = serde_json::from_str(json).unwrap();
    assert!(!workflow.nodes.is_empty());
}
```

## 🚀 Ready to Use

Once you run `make generate-types`, you'll have:

1. ✅ Shared type definitions
2. ✅ Rust library (dag-optimizer)
3. ✅ Types available in CLI
4. ✅ Types available in Go backend
5. ✅ Single schema to maintain

Just install the tools and run the command!

```bash
# Install code generation tools
./scripts/setup-codegen.sh

# Or manually:
npm install -g quicktype

# Generate types
make generate-types
```
