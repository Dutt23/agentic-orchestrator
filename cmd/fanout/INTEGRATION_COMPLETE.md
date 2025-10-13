# Coordinator Integration Complete

## Overview

The coordinator has been fully integrated with the fanout service for real-time workflow event broadcasting. All workflow lifecycle events are now published to Redis PubSub channels.

## Event Flow

```
Workflow Execution → Redis PubSub → Fanout Service → WebSocket Clients
                     (workflow:events:{username})
```

## Published Events

### 1. workflow_started
**Published by:** `cmd/workflow-runner/executor/run_request_consumer.go:232`
**When:** After workflow initialization completes (IR stored, counter initialized, entry tokens emitted)

**Payload:**
```json
{
  "type": "workflow_started",
  "run_id": "01234567-89ab-cdef-0123-456789abcdef",
  "tag": "flight-search",
  "nodes": 5,
  "entry_nodes": 1,
  "timestamp": 1697234567
}
```

### 2. node_completed
**Published by:** `cmd/workflow-runner/coordinator/coordinator.go:144`
**When:** After a node completes execution and token is consumed

**Payload:**
```json
{
  "type": "node_completed",
  "run_id": "01234567-89ab-cdef-0123-456789abcdef",
  "node_id": "search_flights",
  "status": "completed",
  "counter": 2,
  "result_ref": "sha256:abc123...",
  "timestamp": 1697234568
}
```

### 3. workflow_completed
**Published by:** `cmd/workflow-runner/coordinator/coordinator.go:519`
**When:** When counter reaches 0 (all nodes finished)

**Payload:**
```json
{
  "type": "workflow_completed",
  "run_id": "01234567-89ab-cdef-0123-456789abcdef",
  "counter": 0,
  "timestamp": 1697234580
}
```

## Redis Channels

Events are published to username-specific channels:

```
workflow:events:{username}
```

**Examples:**
- `workflow:events:test-user`
- `workflow:events:alice`
- `workflow:events:bob@example.com`

This enables multi-tenant event isolation.

## Code Changes

### 1. Run Request Consumer
**File:** `cmd/workflow-runner/executor/run_request_consumer.go`

**Changes:**
- Lines 156-161: Store username and tag in IR.Metadata
- Lines 232-239: Publish workflow_started event after initialization
- Lines 328-349: Added publishWorkflowEvent() helper method

### 2. Coordinator
**File:** `cmd/workflow-runner/coordinator/coordinator.go`

**Changes:**
- Lines 142-154: Publish node_completed event after token consumption
- Lines 515-525: Publish workflow_completed event when counter reaches 0
- Lines 533-554: Added publishWorkflowEvent() helper method

## Testing

### 1. Start Fanout Service

```bash
cd cmd/fanout
./start.sh
```

### 2. Connect WebSocket Client

```javascript
const ws = new WebSocket('ws://localhost:8084/ws?username=test-user');

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Event:', data.type, data);
};
```

### 3. Execute Workflow

```bash
curl -X POST http://localhost:8081/api/v1/runs \
  -H "X-User-ID: test-user" \
  -H "Content-Type: application/json" \
  -d '{
    "tag": "flight-search",
    "inputs": {"city": "NYC"}
  }'
```

### 4. Expected Output

```
Event: workflow_started {type: "workflow_started", run_id: "...", tag: "flight-search", nodes: 2, entry_nodes: 1}
Event: node_completed {type: "node_completed", run_id: "...", node_id: "search", status: "completed", counter: 1}
Event: node_completed {type: "node_completed", run_id: "...", node_id: "format", status: "completed", counter: 0}
Event: workflow_completed {type: "workflow_completed", run_id: "...", counter: 0}
```

## Architecture Benefits

1. **Separation of Concerns**: Workflow execution logic doesn't handle WebSocket connections
2. **Scalability**: Fanout service can be scaled independently
3. **Multi-tenancy**: User isolation via username-based channels
4. **Real-time Updates**: Frontend receives immediate workflow status updates
5. **Flexibility**: Easy to add new event types or consumers

## Future Enhancements

1. **Event Persistence**: Store events in database for history/replay
2. **Event Filtering**: Allow clients to subscribe to specific event types
3. **Batch Events**: Combine multiple node completions for high-throughput workflows
4. **Error Events**: Publish node_failed, workflow_failed events
5. **Progress Events**: Publish progress updates for long-running nodes
6. **Scaling**: Migrate to nchan for >5K concurrent connections (see NCHAN_ARCHITECTURE.md)

## Related Documentation

- [Fanout Service README](./README.md)
- [nchan Architecture](./docs/NCHAN_ARCHITECTURE.md)
- [Service Management](../../docs/SERVICE_MANAGEMENT.md)
- [Coordinator Code](../workflow-runner/coordinator/coordinator.go)
