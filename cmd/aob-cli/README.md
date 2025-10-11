# aob - Agentic Orchestration Builder CLI

Fast, lightweight command-line interface for the Orchestrator platform.

## Installation

### From Source

```bash
cd cmd/aob-cli
cargo build --release
sudo cp target/release/aob /usr/local/bin/
```

### Verify Installation

```bash
aob --version
```

## Usage

### Configuration

Set the API endpoint:

```bash
export AOB_API_URL=http://localhost:8081
```

Or use the `--api-url` flag on each command.

### Commands

#### Run Management

```bash
# Start a workflow
aob run start workflow.json --inputs inputs.json

# Start and follow logs
aob run start workflow.json -f

# Check run status
aob run status run_7f3e4a

# List runs
aob run list
aob run list --status running --limit 20

# Cancel a run
aob run cancel run_7f3e4a
```

#### Log Streaming

```bash
# Stream all logs
aob logs stream run_7f3e4a

# Filter by node
aob logs stream run_7f3e4a --node parse

# Show only errors
aob logs stream run_7f3e4a --filter errors
```

#### HITL Approvals

```bash
# Approve a request
aob approve ticket_456 approve --reason "LGTM"

# Reject a request
aob approve ticket_456 reject --reason "Need more testing"
```

#### Patch Management

```bash
# List patches
aob patch list run_7f3e4a

# Show patch details
aob patch show patch_abc123

# Approve patch
aob patch approve patch_abc123 --reason "Safe to apply"

# Reject patch
aob patch reject patch_abc123 --reason "Too risky"
```

#### Replay

```bash
# Replay entire run
aob replay run_7f3e4a

# Replay from specific node
aob replay run_7f3e4a --from parse

# Shadow mode (no side effects)
aob replay run_7f3e4a --mode shadow
```

#### Workflow Management

```bash
# List workflows
aob workflow list

# Validate workflow file
aob workflow validate workflow.json

# Show workflow details
aob workflow show lead_flow
```

#### Artifacts

```bash
# Get artifact
aob artifact get cas://sha256:abc123... -o output.json

# List run artifacts
aob artifact list run_7f3e4a
```

#### Cache

```bash
# Invalidate cache entry
aob cache invalidate enrich_A:sha256:...

# Show cache stats
aob cache stats
```

### Output Formats

```bash
# Pretty (default)
aob run status run_7f3e4a

# JSON
aob run status run_7f3e4a --output json

# Compact
aob run status run_7f3e4a --output compact
```

## Performance

### Binary Size

Optimized for size with LTO and stripping:

```bash
$ ls -lh target/release/aob
-rwxr-xr-x  1 user  staff   2.4M  aob
```

### Startup Time

```bash
$ time aob --version
aob 0.1.0
aob --version  0.00s user 0.00s system 89% cpu 0.008 total
```

### Memory Usage

Minimal memory footprint (~5MB at idle).

## Development

### Build

```bash
cargo build
```

### Run

```bash
cargo run -- run start examples/workflow.json
```

### Test

```bash
cargo test
```

### Release Build

```bash
cargo build --release
```

The release build is optimized for size and performance:
- LTO enabled
- Single codegen unit
- Symbols stripped
- Panic=abort

## Architecture

```
src/
├── main.rs           # CLI entry point, command parsing
├── client/
│   └── mod.rs        # HTTP client wrapper
├── commands/
│   ├── run.rs        # Run management
│   ├── logs.rs       # Log streaming (SSE)
│   ├── approve.rs    # HITL approvals
│   ├── patch.rs      # Patch management
│   ├── workflow.rs   # Workflow commands
│   ├── artifact.rs   # Artifact management
│   ├── cache.rs      # Cache commands
│   └── replay.rs     # Replay functionality
└── utils/
    ├── spinner.rs    # Progress indicators
    └── mod.rs        # Shared utilities
```

## Dependencies

- **clap** - CLI argument parsing
- **tokio** - Async runtime (minimal features)
- **reqwest** - HTTP client (rustls for smaller binary)
- **serde** - JSON serialization
- **eventsource-stream** - SSE streaming
- **colored** - Terminal colors
- **indicatif** - Progress bars and spinners
- **anyhow** - Error handling
