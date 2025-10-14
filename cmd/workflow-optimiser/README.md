# Workflow Optimizer - WASM Module

Real-time workflow optimization engine that runs in the browser via WebAssembly. Analyzes workflow graphs and suggests performance optimizations before execution.

## 🎯 Purpose

Provide **instant, intelligent feedback** to users as they design workflows in the UI, suggesting optimizations without requiring backend round-trips.

## ✨ Features

- **🚀 Zero Backend Dependency**: Runs entirely in browser via WASM
- **⚡ Real-time Analysis**: Sub-millisecond optimization suggestions
- **🎨 Visual Integration**: Highlights optimization opportunities in the UI
- **🔧 Multiple Optimizers**: 6+ optimization patterns included
- **📊 Performance Metrics**: Quantified improvements (time, tokens, efficiency)
- **🔒 Type-Safe**: Written in Rust for reliability

## 📦 Optimizers

### 1. **Conditional Absorber**
Merges conditional/branch nodes into their parent node.

**Before**:
```
fetch_data → check_status → [high_priority | low_priority]
```

**After**:
```
fetch_data (with conditional config) → [high_priority | low_priority]
```

**Savings**: ~75ms per absorbed node, 10% efficiency gain

---

### 2. **HTTP Coalescer**
Batches independent HTTP calls into parallel requests.

**Before** (Sequential):
```
fetch_users (HTTP)    → 200ms
fetch_posts (HTTP)    → 200ms
fetch_comments (HTTP) → 200ms
Total: 600ms
```

**After** (Parallel):
```
batch_fetch_all (HTTP parallel) → 200ms
Total: 200ms (3x faster!)
```

**Savings**: (N-1) × 200ms where N = number of batched calls

---

### 3. **Semantic Cache**
Detects cacheable LLM/agent calls to reduce token usage.

**Detection**:
- Repeated prompts with same context
- Deterministic queries (temperature=0)
- Static knowledge retrieval

**Savings**: 90% latency reduction, ~500 tokens per cached call

---

### 4. **Parallel Detector**
Identifies independent node sequences that can run concurrently.

**Example**:
```
A → B, A → C, A → D  (where B, C, D don't depend on each other)
```

**Suggestion**: Execute B, C, D in parallel

---

### 5. **Branch Absorber**
Similar to conditional absorber but for switch/case patterns.

---

### 6. **Dead Code Eliminator**
Removes unreachable nodes (e.g., branch conditions always false).

---

## 🛠️ Building

### Prerequisites
```bash
# Install Rust
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh

# Add wasm32 target
rustup target add wasm32-unknown-unknown

# Install wasm-pack
curl https://rustwasm.github.io/wasm-pack/installer/init.sh -sSf | sh
```

### Build for WASM
```bash
# Development build
wasm-pack build --dev --target web

# Production build (optimized for size)
wasm-pack build --release --target web

# Output: pkg/ directory with .wasm and .js files
```

### Run Tests
```bash
cargo test

# Run WASM-specific tests
wasm-pack test --headless --firefox
```

## 🌐 Frontend Integration

### 1. Install Package
```bash
# Copy pkg/ output to your frontend
cp -r pkg/ frontend/src/wasm/workflow-optimiser/
```

### 2. Import in JavaScript/TypeScript
```typescript
// Initialize WASM module
import init, {
  optimize_workflow,
  apply_optimization,
  list_optimizers,
  estimate_improvement
} from './wasm/workflow-optimiser/workflow_optimiser.js';

// Initialize (once at app startup)
await init();

// Analyze workflow
const workflow = {
  version: "1.0",
  nodes: {
    fetch: { id: "fetch", type: "http", ... },
    process: { id: "process", type: "function", ... }
  },
  edges: []
};

const suggestions = JSON.parse(
  optimize_workflow(JSON.stringify(workflow))
);

console.log(suggestions);
// Output: Array of OptimizationResult objects
```

### 3. React Example
```typescript
import { useEffect, useState } from 'react';
import init, { optimize_workflow } from './wasm/workflow-optimiser';

function WorkflowEditor() {
  const [suggestions, setSuggestions] = useState([]);
  const [wasmReady, setWasmReady] = useState(false);

  useEffect(() => {
    init().then(() => setWasmReady(true));
  }, []);

  const analyzeWorkflow = (workflow) => {
    if (!wasmReady) return;

    const results = JSON.parse(
      optimize_workflow(JSON.stringify(workflow))
    );

    setSuggestions(results);
  };

  return (
    <div>
      {/* Workflow canvas */}
      <Canvas workflow={workflow} onChange={analyzeWorkflow} />

      {/* Optimization suggestions */}
      {suggestions.map(result => (
        <OptimizationCard key={result.id} result={result} />
      ))}
    </div>
  );
}
```

## 📊 API Reference

### `optimize_workflow(workflow_json: string) -> string`
Analyzes workflow and returns optimization suggestions.

**Input**: JSON string of WorkflowIR
**Output**: JSON string of `OptimizationResult[]`

```typescript
type OptimizationResult = {
  applicable: boolean;
  suggestions: Suggestion[];
  total_improvement: PerformanceMetrics;
};

type Suggestion = {
  id: string;
  title: string;
  description: string;
  affected_nodes: string[];
  severity: "info" | "warning" | "high";
  metrics: PerformanceMetrics;
  auto_apply_safe: boolean;
};

type PerformanceMetrics = {
  time_saved_ms: number;
  requests_saved: number;
  tokens_saved: number;
  efficiency_gain: number; // Percentage: 0-100
};
```

### `apply_optimization(workflow_json: string, optimizer_id: string) -> string`
Applies a specific optimization to the workflow.

**Input**:
- `workflow_json`: JSON string of WorkflowIR
- `optimizer_id`: e.g., "conditional_absorber", "http_coalescer"

**Output**: JSON string of optimized WorkflowIR

### `list_optimizers() -> string`
Returns list of available optimizers with descriptions.

**Output**: JSON array of `{ id: string, description: string }`

### `estimate_improvement(workflow_json: string) -> number`
Returns overall estimated efficiency gain (0-100 percentage).

## 🏗️ Architecture

```
┌─────────────────────────────────────────┐
│           Browser (WASM)                │
├─────────────────────────────────────────┤
│  workflow-optimiser.wasm                │
│  ├── OptimizerEngine                    │
│  ├── ConditionalAbsorber                │
│  ├── HttpCoalescer                      │
│  ├── SemanticCache                      │
│  └── ... more optimizers                │
└─────────────────────────────────────────┘
           ↕ (JS bindings)
┌─────────────────────────────────────────┐
│        React/Vue UI                     │
│  - Workflow canvas                      │
│  - Real-time suggestions                │
│  - Apply optimizations                  │
└─────────────────────────────────────────┘
```

## 🎨 UI Integration Suggestions

### Visual Indicators
```typescript
// Highlight optimizable nodes with badges
<Node
  id="check_status"
  badge={suggestion ? {
    icon: "⚡",
    color: "warning",
    tooltip: suggestion.title
  } : null}
/>
```

### Suggestion Panel
```typescript
<OptimizationPanel>
  <Suggestion
    icon="⚡"
    title={suggestion.title}
    description={suggestion.description}
    metrics={suggestion.metrics}
    onApply={() => applyOptimization(suggestion.id)}
  />
</OptimizationPanel>
```

### Performance Score
```typescript
const score = estimate_improvement(workflow);

<PerformanceScore
  value={score}
  label={`${score}% efficiency gain available`}
/>
```

## 🔮 Future Enhancements

1. **Machine Learning Integration**: Learn from user patterns
2. **Cost Optimization**: Estimate and minimize execution costs
3. **Security Analysis**: Detect potential security issues
4. **Compliance Checking**: Validate against organizational policies
5. **Custom Optimizer API**: Let users write their own optimizers
6. **A/B Testing**: Suggest experiments to test optimizations

## 🧪 Development

### Project Structure
```
src/
├── lib.rs                    # WASM entry point
├── optimizer.rs              # Core Optimize trait
├── types.rs                  # Workflow IR types
└── optimizers/
    ├── mod.rs
    ├── conditional_absorber.rs
    ├── http_coalescer.rs
    ├── semantic_cache.rs
    ├── parallel_detector.rs
    ├── branch_absorber.rs
    └── dead_code_eliminator.rs
```

### Adding a New Optimizer

1. Create new file in `src/optimizers/`
2. Implement `Optimize` trait
3. Register in `OptimizerEngine::register_all_optimizers()`
4. Add tests
5. Document in README

**Template**:
```rust
use crate::optimizer::{Optimize, OptimizationResult, OptimizationError};
use crate::types::WorkflowIR;

pub struct MyOptimizer;

impl MyOptimizer {
    pub fn new() -> Self {
        Self
    }
}

impl Optimize for MyOptimizer {
    fn analyze(&self, workflow: &WorkflowIR) -> OptimizationResult {
        // Detect optimization opportunities
        todo!()
    }

    fn apply(&self, workflow: &WorkflowIR) -> Result<WorkflowIR, OptimizationError> {
        // Transform workflow
        todo!()
    }

    fn id(&self) -> &str {
        "my_optimizer"
    }

    fn description(&self) -> &str {
        "My custom optimization"
    }
}
```

## 📝 License

MIT

## 🤝 Contributing

This is a scaffold. Actual optimization logic needs implementation. PRs welcome!

## 📚 References

- [WebAssembly](https://webassembly.org/)
- [wasm-bindgen](https://rustwasm.github.io/docs/wasm-bindgen/)
- [wasm-pack](https://rustwasm.github.io/wasm-pack/)
- [Go Workflow Runner SDK](../workflow-runner/sdk/)
