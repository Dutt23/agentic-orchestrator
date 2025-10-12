# DAG Optimizer

A Rust library for analyzing and optimizing workflow DAGs (Directed Acyclic Graphs).

## Features

- **Node Merging**: Detect opportunities to batch operations (e.g., multiple API calls)
- **Parallelization**: Identify independent paths that can run concurrently
- **Dead Code Elimination**: Remove unused nodes and edges
- **Performance Analysis**: Estimate execution time and resource usage
- **WASM Support**: Run in the browser for real-time feedback

## Usage

### As a Library

```rust
use dag_optimizer::{Workflow, Optimizer};

// Parse a workflow
let workflow: Workflow = serde_json::from_str(json_str)?;

// Create an optimizer
let optimizer = Optimizer::new();

// Analyze the workflow
let suggestions = optimizer.analyze(&workflow)?;

// Apply an optimization
let optimized = optimizer.optimize(&workflow, &suggestions[0])?;
```

### With CLI

```bash
# From aob-cli
cargo run -- optimize workflow.json
```

### As WASM (in browser)

```javascript
import init, { analyze_workflow } from 'dag-optimizer';

await init();
const suggestions = analyze_workflow(workflowJson);
```

## Type Generation

Types are automatically generated from JSON Schema:

```bash
# Generate types from schema
make generate-rust-types

# Types are generated in: src/types.rs
```

## Development

```bash
# Build
cargo build

# Test
cargo test

# Build for WASM
cargo build --target wasm32-unknown-unknown --features wasm
```

## Architecture

```
dag-optimizer/
├── src/
│   ├── lib.rs          # Main library entry
│   ├── types.rs        # Generated from JSON Schema
│   ├── analyzer.rs     # DAG analysis (future)
│   ├── optimizer.rs    # Optimization logic (future)
│   └── wasm.rs         # WASM bindings (future)
├── Cargo.toml
└── README.md
```

## License

MIT
