# aob CLI - Command Reference

Complete reference for all `aob` commands.

## Global Flags

```bash
--api-url <URL>        # API endpoint (default: http://localhost:8081, env: AOB_API_URL)
--output <format>      # Output format: pretty (default), json, compact
--help, -h             # Show help
--version, -V          # Show version
```

## Commands Overview

| Command | Description |
|---------|-------------|
| `help` | Show detailed help and examples |
| `run` | Start and manage workflow runs |
| `logs` | Stream and view logs |
| `approve` | Approve or reject HITL requests |
| `patch` | Manage agent-proposed patches |
| `workflow` | Manage workflows |
| `artifact` | Manage artifacts |
| `cache` | Manage cache |
| `replay` | Replay workflow runs |

---

## `aob help`

Shows comprehensive help with examples.

```bash
aob help
```

---

## `aob run` - Run Management

### `aob run start`

Start a new workflow run.

```bash
# Basic usage
aob run start workflow.json

# With inputs
aob run start workflow.json --inputs input.json

# Follow logs after starting
aob run start workflow.json -f
aob run start workflow.json --follow
```

**Arguments:**
- `<workflow>` - Path to workflow JSON file

**Options:**
- `--inputs <file>` - Path to inputs JSON file
- `-f, --follow` - Stream logs after starting

**Example:**
```bash
$ aob run start examples/lead-flow.json --inputs lead.json
✓ Run started: run_7f3e4a
Status: running
```

### `aob run status`

Get status of a specific run.

```bash
aob run status <run_id>
```

**Output:**
- Run ID
- Status (running, completed, failed, cancelled)
- Start/end times
- Active nodes

**Example:**
```bash
$ aob run status run_7f3e4a
• Run: run_7f3e4a
  Status: running
  Started: 2024-01-10T10:30:00Z
  Active nodes:
    - enrich
```

### `aob run list`

List recent runs.

```bash
# List all runs
aob run list

# Filter by status
aob run list --status running
aob run list --status completed
aob run list --status failed

# Limit results
aob run list --limit 50
```

**Options:**
- `--status <status>` - Filter by status
- `--limit <n>` - Limit number of results (default: 10)

**Example:**
```bash
$ aob run list --status running --limit 5
Runs (showing 3 of 3):

• run_7f3e4a - running (2024-01-10T10:30:00Z)
• run_8a2b9c - running (2024-01-10T10:25:00Z)
• run_3d4e5f - running (2024-01-10T10:20:00Z)
```

### `aob run cancel`

Cancel a running workflow.

```bash
aob run cancel <run_id>
```

**Example:**
```bash
$ aob run cancel run_7f3e4a
✓ Run cancelled: run_7f3e4a
```

---

## `aob logs` - Log Streaming

### `aob logs stream`

Stream logs from a run in real-time (SSE).

```bash
# Stream all logs
aob logs stream <run_id>

# Filter by node
aob logs stream <run_id> --node <node_id>

# Show only errors
aob logs stream <run_id> --filter errors
```

**Options:**
- `--node <node_id>` - Filter by specific node
- `--filter <level>` - Filter level: all (default), errors

**Example:**
```bash
$ aob logs stream run_7f3e4a --node enrich
• Streaming logs for run: run_7f3e4a
Press Ctrl+C to stop

2024-01-10T10:30:15Z INFO [enrich] Starting enrichment
2024-01-10T10:30:16Z INFO [enrich] Calling external API
2024-01-10T10:30:17Z INFO [enrich] Enrichment complete
```

---

## `aob approve` - HITL Approvals

Approve or reject human-in-the-loop requests.

```bash
# Approve
aob approve <ticket_id> approve [--reason "..."]

# Reject
aob approve <ticket_id> reject --reason "reason required"
```

**Arguments:**
- `<ticket_id>` - Approval ticket ID
- `<decision>` - Decision: approve or reject

**Options:**
- `--reason <text>` - Reason for decision (required for reject)

**Example:**
```bash
$ aob approve ticket_456 approve --reason "Data verified"
✓ Approval granted for ticket: ticket_456

$ aob approve ticket_789 reject --reason "Missing required field"
✓ Approval rejected for ticket: ticket_789
```

---

## `aob patch` - Patch Management

Manage agent-proposed patches (AIR overlays).

### `aob patch list`

List patches for a run.

```bash
aob patch list <run_id>
```

### `aob patch show`

Show patch details with diff.

```bash
aob patch show <patch_id>
```

**Example:**
```bash
$ aob patch show patch_abc123
Patch: patch_abc123
Justification: "Add S3 upload and notification"

Changes:
  + Node: upload_to_s3 (type: function)
  + Node: notify_email (type: function)
  + Edge: parse → upload_to_s3
  + Edge: upload_to_s3 → notify_email
```

### `aob patch approve`

Approve a patch.

```bash
aob patch approve <patch_id> [--reason "..."]
```

### `aob patch reject`

Reject a patch.

```bash
aob patch reject <patch_id> --reason "reason required"
```

---

## `aob workflow` - Workflow Management

### `aob workflow list`

List all workflows.

```bash
aob workflow list
```

### `aob workflow validate`

Validate a workflow file without running it.

```bash
aob workflow validate <file.json>
```

**Example:**
```bash
$ aob workflow validate workflow.json
✓ Workflow is valid
  Nodes: 5
  Edges: 4
```

### `aob workflow show`

Show workflow details.

```bash
aob workflow show <workflow_id>
```

---

## `aob artifact` - Artifact Management

### `aob artifact get`

Download an artifact.

```bash
# Print to stdout
aob artifact get cas://sha256:abc123...

# Save to file
aob artifact get cas://sha256:abc123... -o output.json
```

**Options:**
- `-o, --output <file>` - Output file path

### `aob artifact list`

List artifacts for a run.

```bash
aob artifact list <run_id>
```

---

## `aob cache` - Cache Management

### `aob cache invalidate`

Invalidate a cache entry.

```bash
aob cache invalidate <key>
```

**Example:**
```bash
$ aob cache invalidate enrich_A:sha256:abc123
✓ Cache entry invalidated
```

### `aob cache stats`

Show cache statistics.

```bash
aob cache stats
```

---

## `aob replay` - Replay Runs

Replay a workflow run from a checkpoint.

```bash
# Replay entire run
aob replay <run_id>

# Replay from specific node
aob replay <run_id> --from <node_id>

# Shadow mode (no side effects)
aob replay <run_id> --mode shadow
```

**Options:**
- `--from <node_id>` - Start replay from this node
- `--mode <mode>` - Replay mode: freeze (default), shadow

**Modes:**
- `freeze` - Use cached inputs (deterministic)
- `shadow` - Dry-run mode, no side effects

**Example:**
```bash
$ aob replay run_7f3e4a --from parse --mode freeze
✓ Replay started: run_9b2c1d
  Mode: freeze
  From: parse
  Original: run_7f3e4a
```

---

## Output Formats

### Pretty (default)

Human-readable colored output.

```bash
aob run status run_7f3e4a
```

### JSON

Machine-readable JSON for scripting.

```bash
aob run status run_7f3e4a --output json
```

```json
{
  "run_id": "run_7f3e4a",
  "status": "running",
  "started_at": "2024-01-10T10:30:00Z",
  "active_nodes": ["enrich"]
}
```

### Compact

Condensed output for pipelines.

```bash
aob run status run_7f3e4a --output compact
```

---

## Environment Variables

```bash
export AOB_API_URL=http://localhost:8081     # API endpoint
```

---

## Exit Codes

- `0` - Success
- `1` - General error
- `2` - API error
- `130` - Interrupted (Ctrl+C)

---

## Tips & Tricks

### Follow logs after starting

```bash
aob run start workflow.json -f
```

### Monitor multiple runs

```bash
# Terminal 1
aob logs stream run_1

# Terminal 2
aob logs stream run_2
```

### Filter errors only

```bash
aob logs stream run_7f3e4a --filter errors
```

### JSON for scripting

```bash
RUN_ID=$(aob run start workflow.json --output json | jq -r '.run_id')
aob run status "$RUN_ID"
```

### Validate before running

```bash
aob workflow validate workflow.json && aob run start workflow.json
```
