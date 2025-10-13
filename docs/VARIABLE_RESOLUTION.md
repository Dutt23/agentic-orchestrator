# Variable Resolution Architecture

## Overview

This document describes how variable substitution works in the workflow execution system, the architectural decisions made, current tradeoffs, and future evolution path.

## Variable Syntax

The system supports referencing previous node outputs in workflow configurations:

### 1. Full Node Reference
```
$nodes.node_id
```
Returns the entire output from `node_id`.

**Example:**
```json
{
  "payload": "$nodes.fetch_flights"
}
```

### 2. Field Access
```
$nodes.node_id.field.path
```
Accesses a specific field using JSONPath notation.

**Example:**
```json
{
  "price": "$nodes.fetch_flights.body[0].price"
}
```

### 3. String Interpolation
```
"text ${$nodes.node_id.field} more text"
```
Embeds node outputs in strings.

**Example:**
```json
{
  "message": "Found ${$nodes.fetch_flights.count} flights at ${$nodes.fetch_flights.body[0].price}"
}
```

## Current Architecture: Centralized Resolution

### How It Works

1. **Node A completes** → sends completion signal to coordinator
2. **Coordinator receives signal** → stores Node A's output in Redis context
3. **Coordinator loads Node B's config** from IR
4. **Coordinator resolves variables** in Node B's config:
   - Can now access `$nodes.A` since A just completed
   - Uses resolver to substitute all `$nodes.*` references
5. **Coordinator emits token** to Node B with **pre-resolved config**
6. **Worker receives token** → uses config directly, no resolution needed

### Key Components

#### Coordinator (`cmd/workflow-runner/coordinator/`)
- Receives completion signals
- Loads node configs from IR
- **Resolves variables** using resolver before emitting tokens
- Emits tokens with resolved configs to worker streams

#### Resolver (`cmd/workflow-runner/resolver/`)
- Go implementation for coordinator
- Loads previous node outputs from Redis context
- Resolves `$nodes.*` expressions recursively
- Uses gjson for efficient field extraction

#### Workers (Go HTTP worker, Python Agent worker, etc.)
- Receive tokens with **pre-resolved configs**
- Execute tasks using configs directly
- **No resolver implementation needed**

## Current Tradeoffs

### ✅ Advantages

1. **Polyglot Support**
   - Single resolver implementation (Go in coordinator)
   - Workers in any language (Python, Rust, JS) don't need resolvers
   - Easy to add new worker types

2. **Centralized Logic**
   - One place to debug/fix variable resolution
   - Consistent behavior across all node types
   - Easier to add new expression types (CEL, JSONPath, etc.)

3. **No Redis Access in Workers**
   - Workers don't need to load context from Redis
   - Workers are stateless and portable
   - Can run workers on Lambda/Fargate/K8s without Redis access

4. **Security**
   - Workers can't access other nodes' data
   - Coordinator controls what data workers see
   - Easier to implement access control/data masking

5. **Performance**
   - Config resolved once, not per worker instance
   - Horizontal scaling of workers doesn't increase Redis load
   - Workers start faster (no Redis client initialization)

### ⚠️ Disadvantages

1. **Token Size**
   - Resolved configs included in every token
   - Larger messages in Redis streams
   - More memory per token

2. **Coordinator Load**
   - Coordinator must resolve configs for all nodes
   - Single point of contention (mitigated by goroutines)
   - CPU/memory usage concentrated in coordinator

3. **Dynamic Resolution Limitations**
   - Can't re-resolve during node execution
   - Workers can't access latest values if context changes mid-execution
   - No support for streaming/reactive updates

4. **Debugging**
   - Harder to see original vs resolved config
   - Need to trace back to coordinator for resolution errors
   - Workers don't know they're using resolved configs

## Future Evolution

### Phase 1: Current (MVP) ✅
- Centralized resolution in coordinator
- Simple `$nodes.*` syntax
- Inline config in IR as fallback
- Works for linear and branching workflows

### Phase 2: Near Term (3-6 months)
**Goal:** Optimize performance and add advanced expressions

1. **Config Compaction**
   - Store resolved configs in CAS instead of inline in tokens
   - Token contains only `resolved_config_ref` (SHA256 hash)
   - Reduces token size from ~5KB to ~50 bytes
   - Workers load from CAS (cached locally)

2. **CEL Expression Support**
   ```
   $cel{nodes.A.price * 1.1}
   $cel{nodes.A.status == "success" && nodes.B.count > 0}
   ```

3. **Conditional Resolution**
   - Resolve different configs based on branch taken
   - Support for `$branch.*` to access branch metadata

4. **Resolution Caching**
   - Cache resolved configs per node in Redis
   - Reuse for retry/loop iterations
   - Invalidate on context changes

### Phase 3: Service Mesh (6-12 months)
**Goal:** Distributed resolution with sidecar pattern

1. **Resolver Sidecar**
   ```
   ┌──────────┐     ┌──────────┐
   │  Worker  │────▶│ Resolver │
   │ (Python) │     │ Sidecar  │
   └──────────┘     └──────────┘
                         │
                         ▼
                    Redis Context
   ```
   - Each worker pod has resolver sidecar
   - Worker makes local gRPC call to sidecar
   - Sidecar handles Redis access and resolution

2. **Benefits**
   - Lower coordinator load
   - Dynamic resolution during execution
   - Support for streaming/reactive updates
   - Better for long-running nodes

3. **Deployment Models**
   - Kubernetes: Sidecar container in same pod
   - Lambda: Extension layer
   - Fargate: Additional container in task

### Phase 4: Edge Resolution (12+ months)
**Goal:** Push resolution to edge for geo-distributed execution

1. **Regional Resolvers**
   - Deploy resolvers in each region
   - Workers resolve locally with cached context
   - Coordinator sends diffs instead of full configs

2. **Context Replication**
   - Replicate workflow context to edge
   - Use CRDTs for conflict resolution
   - Support for multi-region workflows

## Migration Path

### Adding CEL Support (Phase 2)
```go
// In resolver/resolver.go
func (r *Resolver) resolveString(ctx context.Context, runID, str string) (interface{}, error) {
    // Existing: $nodes.* support
    if strings.HasPrefix(str, "$nodes.") {
        return r.resolveNodeReference(ctx, runID, str)
    }

    // NEW: CEL expression support
    if strings.HasPrefix(str, "$cel{") {
        return r.resolveCELExpression(ctx, runID, str)
    }

    // Existing: interpolation
    if strings.Contains(str, "${") {
        return r.resolveInterpolation(ctx, runID, str)
    }

    return str, nil
}
```

### Moving to Config CAS (Phase 2)
```go
// In coordinator/coordinator.go publishToken()
resolvedConfigRef, err := c.sdk.StoreConfig(ctx, resolvedConfig)
token["resolved_config_ref"] = resolvedConfigRef  // Instead of inline config

// In worker/http_worker.go
if token.ResolvedConfigRef != "" {
    config = w.loadConfig(ctx, token.ResolvedConfigRef)
}
```

### Adding Sidecar (Phase 3)
```python
# In agent-runner-py/main.py
from resolver_sidecar import ResolverClient

resolver = ResolverClient("localhost:50051")  # gRPC
config = resolver.resolve(run_id, raw_config)
```

## Implementation Notes

### Current Files
- `cmd/workflow-runner/coordinator/coordinator.go` - Resolution happens here
- `cmd/workflow-runner/resolver/resolver.go` - Go resolver implementation
- `cmd/agent-runner-py/resolver.py` - Python implementation (unused currently, for future)
- `cmd/workflow-runner/sdk/types.go` - Token.Config field

### Testing Strategy
1. **Unit Tests**: Test resolver with mock Redis
2. **Integration Tests**: Full workflow with variable refs
3. **Performance Tests**: Token size, resolution time, throughput

### Monitoring
- Track resolution time per node
- Track token size distribution
- Alert on resolution failures
- Dashboard showing resolution cache hit rate (Phase 2)

## FAQ

**Q: Why not resolve in workers?**
A: Polyglot system - would need to implement resolver in Go, Python, Rust, JS, etc.

**Q: What if resolution fails?**
A: Coordinator logs error and uses unresolved config as fallback. Worker may fail with better error.

**Q: Can I mix resolved and unresolved configs?**
A: Yes - workers check token.Config first, fall back to IR if missing (backward compatible).

**Q: How do I debug resolution?**
A: Check coordinator logs for "resolved config variables" or "failed to resolve config variables".

**Q: What about circular references?**
A: Workflow DAG prevents cycles. If detected, compiler rejects workflow.

**Q: Can I reference future nodes?**
A: No - only completed nodes are in context. Referencing incomplete node fails resolution.

## References

- [Workflow Execution Architecture](./EXECUTION.md)
- [Condition Evaluation](./CONDITION_EVALUATION.md)
- [Token-Based Orchestration](./TOKEN_ORCHESTRATION.md)
