# Orchestrator Schema Definitions

This directory contains JSON Schema definitions that serve as the single source of truth for data structures across the orchestrator system.

## Schemas

- **workflow.schema.json** - Core workflow DAG definition (nodes, edges, metadata)
- More schemas will be added as needed (patch, run, etc.)

## Usage

### Generating Types

Types are automatically generated from these schemas for both Rust and Go:

```bash
# Generate all types
make generate-types

# Watch for changes and auto-regenerate
make watch-schema
```

### Adding a New Node Type

1. Edit `workflow.schema.json`
2. Add the new type to the `Node.type` enum
3. Run `make generate-types`
4. Types are automatically updated in:
   - `crates/dag-optimizer/src/types.rs` (Rust)
   - `cmd/orchestrator/models/workflow.go` (Go)

### Validation

JSON workflows are validated against these schemas at runtime. Invalid workflows will be rejected at the API boundary.

## Schema Design Principles

1. **Single Source of Truth** - Schema defines the contract
2. **Backward Compatible** - Use optional fields for new features
3. **Well-Documented** - Include descriptions for all fields
4. **Validated** - Use JSON Schema validation features
5. **Type-Safe** - Leverage strong typing in generated code

## Tools

- **Rust**: [quicktype](https://quicktype.io/)
- **Go**: [go-jsonschema](https://github.com/atombender/go-jsonschema)
- **Validation**: JSON Schema Draft 7

### Why quicktype?
- ✅ Standalone CLI binary (not a library)
- ✅ Supports 20+ languages
- ✅ Actively maintained
- ✅ Can infer schemas from JSON examples
- ✅ Excellent Rust code generation

## Examples

See `examples/` directory for sample workflows that conform to these schemas.
