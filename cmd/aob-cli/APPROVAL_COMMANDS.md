# HITL Approval Commands

Quick reference for approve/reject commands in aob CLI.

## Commands

### aob approve

Approve a HITL (Human-in-the-Loop) request.

```bash
aob approve <run_id> <node_id> [--comment "reason"]
```

**Arguments:**
- `<run_id>` - Run ID (required)
- `<node_id>` - Node ID requiring approval (required)

**Options:**
- `--comment <text>` - Optional approval comment

**Example:**
```bash
# Approve without comment
aob approve run_7f3e4a manager_approval

# Approve with comment
aob approve run_7f3e4a manager_approval --comment "Budget approved, proceed"

# Using environment variables
export AOB_API_URL=http://localhost:8084
aob approve run_7f3e4a manager_approval
```

**Output:**
```
✓ Approval granted
  Run: run_7f3e4a
  Node: manager_approval
  Comment: Budget approved, proceed
  Workflow resumed
```

---

### aob reject

Reject a HITL (Human-in-the-Loop) request.

```bash
aob reject <run_id> <node_id> --comment "reason"
```

**Arguments:**
- `<run_id>` - Run ID (required)
- `<node_id>` - Node ID requiring approval (required)

**Options:**
- `--comment <text>` - Rejection reason (REQUIRED for reject)

**Example:**
```bash
# Reject with reason
aob reject run_7f3e4a manager_approval --comment "Need more documentation"

# Multiple word comment
aob reject run_7f3e4a high_value_deal --comment "Budget exceeded for Q4, defer to next quarter"
```

**Output:**
```
✓ Approval rejected
  Run: run_7f3e4a
  Node: manager_approval
  Reason: Need more documentation
  Workflow routed to rejection path
```

---

## API Details

**Endpoint:** `POST /api/approval`

**Request:**
```json
{
  "run_id": "run_7f3e4a",
  "node_id": "manager_approval",
  "approved": true,
  "comment": "Looks good"
}
```

**Headers:**
- `Content-Type: application/json`
- `X-User-ID: <username>` (automatically set from $USER env var)

**Response:**
```json
{
  "success": true,
  "message": "Approval processed, workflow resumed"
}
```

---

## Environment Variables

```bash
# API endpoint (fanout service)
export AOB_API_URL=http://localhost:8084

# Username (optional, defaults to $USER)
export AOB_USER_ID=john.doe
```

---

## Error Handling

**Run not found:**
```
Error: Approval failed (404): Run not found
```

**Node not waiting for approval:**
```
Error: Approval failed (400): Node is not in pending approval state
```

**Network error:**
```
Error: Connection refused (is fanout service running?)
```

---

## Workflow Behavior

### On Approve (approved: true)
- HITL worker processes approval
- Removes from pending_approvals set
- Publishes completion signal
- Coordinator routes to next nodes (from config.dependents)
- Workflow resumes

### On Reject (approved: false)
- HITL worker processes rejection
- Removes from pending_approvals set
- Publishes completion signal
- Coordinator routes to rejection path (from config.rejection_path)
- Workflow continues on alternate path

---

## Usage Examples

### Approve simple request
```bash
aob approve run_abc123 approval_node_1
```

### Reject with detailed reason
```bash
aob reject run_abc123 budget_approval \
  --comment "Amount exceeds quarterly budget. Please resubmit next quarter."
```

### Check status after approval
```bash
aob approve run_abc123 manager_review --comment "LGTM"
aob logs stream run_abc123  # Watch workflow resume
```

---

## Integration with UI

The CLI and UI both call the same `/api/approval` endpoint, so approvals can be done from either interface:

- **CLI**: Fast, scriptable, automation-friendly
- **UI**: Visual, context-rich, easier for non-technical users

Both update the same workflow state.

---

## Testing

```bash
# Start a workflow with HITL node
aob run start examples/hitl_workflow.json

# Get run_id from output
RUN_ID=run_abc123

# Approve from CLI
aob approve $RUN_ID manager_approval --comment "Approved via CLI"

# Verify workflow completed
aob run status $RUN_ID
```

---

**Quick approval commands for efficient HITL workflow management.**