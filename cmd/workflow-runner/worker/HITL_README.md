# Human-in-the-Loop (HITL) Worker

The HITL worker enables workflows to pause and wait for human approval before proceeding.

## Architecture

```
┌─────────────┐        ┌──────────────┐        ┌───────────┐
│ Coordinator │───────>│ HITL Worker  │───────>│   Redis   │
└─────────────┘        └──────────────┘        └───────────┘
                              │                        │
                              │                        │
                              │ Polls for approval     │
                              │                        │
                              │                        │
                       ┌──────▼────────┐              │
                       │ User via      │              │
                       │ Fanout API    │──────────────┘
                       └───────────────┘
```

## How It Works

1. **Workflow Execution**: When a workflow reaches a `hitl` node, the coordinator routes the token to the `wf.tasks.hitl` stream

2. **HITL Worker**:
   - Picks up the token from the stream
   - Creates an approval request in Redis: `hitl:approval:{run_id}:{node_id}`
   - Publishes an `approval_required` event to the user via fanout service
   - Polls Redis every 2 seconds waiting for user decision

3. **User Decision**:
   - User receives the approval request via WebSocket from fanout service
   - User makes a decision (approve/reject) via the fanout API
   - Fanout service updates the approval status in Redis

4. **Completion**:
   - HITL worker detects the status change in Redis
   - Stores the approval result in CAS
   - Signals completion to coordinator with `approved` metadata
   - Coordinator routes to next nodes based on the result

## Node Configuration

```json
{
  "id": "approval_node",
  "type": "hitl",
  "config": {
    "message": "Please approve the transaction of $10,000",
    "timeout": 3600
  }
}
```

**Config Fields:**
- `message` (string): The approval message to show the user
- `timeout` (number, optional): Timeout in seconds (default: 24 hours)

## API Endpoint

### POST /api/approval

**Request:**
```json
{
  "run_id": "abc-123",
  "node_id": "approval_node",
  "approved": true,
  "comment": "Looks good",
  "data": {
    "custom_field": "value"
  }
}
```

**Headers:**
- `X-User-ID`: Username of the approver

**Response:**
```json
{
  "success": true,
  "message": "Approval recorded successfully",
  "run_id": "abc-123",
  "node_id": "approval_node",
  "status": "approved"
}
```

## Redis Data Structure

**Approval Request Key:** `hitl:approval:{run_id}:{node_id}`

**Value (JSON):**
```json
{
  "run_id": "abc-123",
  "node_id": "approval_node",
  "token_id": "token-xyz",
  "message": "Please approve...",
  "created_at": 1234567890,
  "status": "pending|approved|rejected",
  "approved_by": "user@example.com",
  "approved_at": 1234567900,
  "comment": "Optional comment"
}
```

## Events

### approval_required (published by HITL worker)
```json
{
  "type": "approval_required",
  "run_id": "abc-123",
  "node_id": "approval_node",
  "message": "Please approve...",
  "timestamp": 1234567890
}
```

## Result Format

The HITL worker stores results in CAS with this format:

```json
{
  "status": "completed",
  "approved": true,
  "approval_data": {
    "run_id": "abc-123",
    "node_id": "approval_node",
    "status": "approved",
    "approved_by": "user@example.com",
    "approved_at": 1234567900,
    "comment": "Looks good"
  },
  "node_id": "approval_node",
  "timestamp": 1234567900
}
```

## Usage Example

1. **Create a workflow with HITL node:**
```json
{
  "nodes": [
    {
      "id": "fetch_data",
      "type": "http",
      "config": {"url": "https://api.example.com/data"}
    },
    {
      "id": "approval",
      "type": "hitl",
      "config": {
        "message": "Approve this data?",
        "timeout": 3600
      }
    },
    {
      "id": "process_data",
      "type": "function",
      "config": {"name": "process"}
    }
  ],
  "edges": [
    {"from": "fetch_data", "to": "approval"},
    {"from": "approval", "to": "process_data"}
  ]
}
```

2. **User approves via API:**
```bash
curl -X POST http://localhost:8084/api/approval \
  -H "X-User-ID: user@example.com" \
  -H "Content-Type: application/json" \
  -d '{
    "run_id": "abc-123",
    "node_id": "approval",
    "approved": true,
    "comment": "Approved"
  }'
```

3. **Workflow continues** to `process_data` node

## Conditional Routing

You can use branch nodes after HITL to route based on approval:

```json
{
  "id": "approval",
  "type": "hitl",
  "config": {"message": "Approve?"}
},
{
  "id": "check_approval",
  "type": "branch",
  "config": {
    "rules": [
      {
        "condition": {"field": "approved", "operator": "==", "value": true},
        "next_nodes": ["approved_path"]
      }
    ],
    "default": ["rejected_path"]
  }
}
```
