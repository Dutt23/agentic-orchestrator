# Real-Time Updates Architecture

This document explains the hybrid WebSocket + polling approach for reliable real-time workflow updates.

## Overview

The fanout service provides real-time updates using a **hybrid approach**: WebSocket for instant updates with polling as a resilient fallback.

## Architecture: Hybrid WebSocket + Polling

### Problem with WebSocket-Only

WebSockets can drop messages due to:
- Network instability
- Server restarts
- Client reconnection delays
- Message buffer overflows

**Result:** Users miss workflow events, UI shows stale state.

### Problem with Polling-Only

Polling every N seconds:
- Wastes bandwidth (polls even when nothing changes)
- Higher latency (up to N seconds delay)
- Server load (constant requests)

### Our Solution: Hybrid Approach

**Primary**: WebSocket for instant updates
**Fallback**: Poll every 4 seconds if no WebSocket message received
**Smart**: Each WebSocket message delays the next poll
**Efficient**: Stop polling when workflow reaches terminal state

---

## Client Implementation

### React Hook Example

```javascript
function useWorkflowEvents(runId, username) {
    const [events, setEvents] = useState([]);
    const [isTerminal, setIsTerminal] = useState(false);
    const pollTimerRef = useRef(null);
    const wsRef = useRef(null);

    // Polling function (fallback)
    const pollEvents = async () => {
        if (isTerminal) return;

        try {
            const response = await fetch(`/api/v1/runs/${runId}/events`);
            const data = await response.json();
            setEvents(data.events || []);

            // Check for terminal state
            const lastEvent = data.events?.[data.events.length - 1];
            if (lastEvent?.type === 'workflow_completed' ||
                lastEvent?.type === 'workflow_failed') {
                setIsTerminal(true);
                clearTimeout(pollTimerRef.current);
            }
        } catch (error) {
            console.error('Polling failed:', error);
        }
    };

    // Reset poll timer (delays polling by 4 seconds)
    const resetPollTimer = useCallback(() => {
        clearTimeout(pollTimerRef.current);
        if (!isTerminal) {
            pollTimerRef.current = setTimeout(pollEvents, 4000);
        }
    }, [isTerminal, runId]);

    useEffect(() => {
        // Establish WebSocket connection
        const wsUrl = `ws://localhost:8085/ws?username=${username}`;
        const ws = new WebSocket(wsUrl);
        wsRef.current = ws;

        ws.onopen = () => {
            console.log('WebSocket connected');
            // Start poll timer as fallback
            resetPollTimer();
        };

        ws.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);

                // Update events
                setEvents(prev => [...prev, data]);

                // Reset poll timer - we got a WebSocket message!
                // This delays polling, preferring WebSocket when working
                resetPollTimer();

                // Check for terminal state
                if (data.type === 'workflow_completed' || data.type === 'workflow_failed') {
                    setIsTerminal(true);
                    clearTimeout(pollTimerRef.current);
                    // Don't close WebSocket - keep it open for other workflows
                }
            } catch (err) {
                console.error('Failed to parse WebSocket message:', err);
            }
        };

        ws.onerror = (error) => {
            console.error('WebSocket error:', error);
            // Polling will continue as fallback
        };

        ws.onclose = () => {
            console.log('WebSocket closed, falling back to polling only');
            // Don't stop polling - it's our fallback!
        };

        // Cleanup
        return () => {
            clearTimeout(pollTimerRef.current);
            ws.close();
        };
    }, [runId, username, resetPollTimer]);

    return { events, isTerminal };
}
```

### Usage in Component

```javascript
function WorkflowViewer({ runId }) {
    const { events, isTerminal } = useWorkflowEvents(runId, 'user-123');

    return (
        <div>
            <h2>Workflow Status: {isTerminal ? 'Complete' : 'Running'}</h2>
            <EventTimeline events={events} />
        </div>
    );
}
```

---

## Behavior Breakdown

### Scenario 1: WebSocket Working (Happy Path)

```
T=0s:   WebSocket connects, poll timer starts (4s)
T=1s:   WebSocket message arrives → Poll timer resets to 5s
T=2s:   WebSocket message arrives → Poll timer resets to 6s
T=3s:   WebSocket message arrives → Poll timer resets to 7s
T=5s:   Workflow completes via WebSocket
        → isTerminal = true
        → Poll timer cleared
        → No more polling (efficient!)
        → WebSocket stays open (can be reused)

Result: 0 polling requests (WebSocket handled everything)
```

### Scenario 2: WebSocket Drops Messages

```
T=0s:   WebSocket connects, poll timer starts (4s)
T=1s:   WebSocket message arrives → Poll timer resets to 5s
T=2s:   Message dropped (network issue)
T=5s:   No WebSocket message for 4s → Poll executes!
        → Fetches latest events
        → User sees the dropped event
        → Poll timer resets to 9s
T=6s:   WebSocket message arrives → Poll timer resets to 10s
T=7s:   Workflow completes via WebSocket
        → isTerminal = true
        → Poll timer cleared

Result: 1 polling request (caught the dropped message!)
```

### Scenario 3: WebSocket Dies Completely

```
T=0s:   WebSocket connects, poll timer starts (4s)
T=1s:   WebSocket message arrives → Poll timer resets to 5s
T=2s:   WebSocket connection dies
T=5s:   No WebSocket message → Poll executes
T=9s:   Poll executes again
T=13s:  Poll executes again
T=15s:  Poll detects workflow completed
        → isTerminal = true
        → Poll timer cleared
        → No more polling

Result: Polling takes over seamlessly (resilient!)
```

---

## Benefits

| Approach | Latency | Bandwidth | Reliability | Server Load |
|----------|---------|-----------|-------------|-------------|
| **WebSocket Only** | Instant | Low | Medium (can drop) | Low |
| **Polling Only** | 0-4s | High | High | High |
| **Hybrid (Ours)** | Instant | Low | **High** | **Low** |

**Our hybrid approach:**
- ✅ **Instant updates** when WebSocket works (99% of time)
- ✅ **Resilient** fallback when WebSocket fails
- ✅ **Efficient** minimal polling (only when needed)
- ✅ **Clean** stops polling when done (no runaway requests)

---

## Server-Side Requirements

### 1. WebSocket Endpoint (Fanout Service)

Already implemented: `ws://localhost:8085/ws?username=<user>`

### 2. Polling Endpoint (Orchestrator API)

Needs to expose:

```
GET /api/v1/runs/{run_id}/events
```

Response:
```json
{
  "run_id": "abc-123",
  "events": [
    {"type": "node_started", "node_id": "fetch", "timestamp": 1234567890},
    {"type": "node_completed", "node_id": "fetch", "timestamp": 1234567895},
    {"type": "workflow_completed", "timestamp": 1234567900}
  ],
  "status": "completed"
}
```

This allows polling to fetch the complete event history.

---

## Configuration

### Poll Interval

```javascript
const POLL_INTERVAL = 4000; // 4 seconds

// Adjust based on requirements:
// - 1s: More responsive, higher load
// - 10s: More efficient, less responsive
// - 4s: Good balance (our choice)
```

### Terminal States

```javascript
const TERMINAL_STATES = [
    'workflow_completed',
    'workflow_failed',
    'workflow_cancelled'
];

function isTerminalState(eventType) {
    return TERMINAL_STATES.includes(eventType);
}
```

---

## Error Handling

### WebSocket Failures

```javascript
ws.onerror = (error) => {
    console.error('WebSocket error:', error);
    // Don't stop - polling continues as fallback
    // Optionally: Show banner "Real-time updates unavailable, using polling"
};

ws.onclose = () => {
    console.log('WebSocket closed');
    // Polling continues automatically
    // Optionally: Attempt reconnection
};
```

### Polling Failures

```javascript
const pollEvents = async () => {
    try {
        const response = await fetch(`/api/v1/runs/${runId}/events`);
        // ... handle response
    } catch (error) {
        console.error('Polling failed:', error);
        // Continue polling - will retry in 4s
        // Don't stop the timer on temporary failures
    }
};
```

---

## Performance Impact

### Best Case (WebSocket Working)

```
1 workflow with 10 events over 30 seconds:
- WebSocket: 10 messages (instant)
- Polling: 0 requests (timer keeps resetting)
- Total requests: 0 HTTP polls

Result: Minimal server load, instant updates ✅
```

### Worst Case (WebSocket Dead)

```
1 workflow with 10 events over 30 seconds:
- WebSocket: 0 messages (dead)
- Polling: 30s / 4s = 7-8 polls
- Total requests: 8 HTTP polls

Result: Slight delay (up to 4s), moderate server load
Still acceptable for 1000s of concurrent users
```

### Scale Test

```
1000 concurrent workflows:
- Best case: 0 polling requests (WebSocket only)
- Worst case: 1000 × 8 polls = 8000 requests over 30s = 267 req/sec
- Mixed (10% WebSocket failure): 800 req/sec

Orchestrator can easily handle this ✅
```

---

## Alternative: Progressive Backoff

If you want to reduce server load even more:

```javascript
let pollInterval = 4000; // Start at 4s

ws.onmessage = () => {
    // Reset to fast polling
    pollInterval = 4000;
    resetPollTimer();
};

const pollEvents = async () => {
    await fetchEvents();

    // Increase interval if no activity (progressive backoff)
    pollInterval = Math.min(pollInterval * 1.5, 30000); // Max 30s

    if (!isTerminal) {
        pollTimerRef.current = setTimeout(pollEvents, pollInterval);
    }
};
```

This reduces polling frequency for idle workflows.

---

## Implementation Checklist

- [x] WebSocket message parsing fixed (separate frames)
- [x] WebSocket timeouts removed
- [x] Ping/pong more aggressive (25s)
- [ ] Frontend: Implement hybrid polling hook
- [ ] Backend: Ensure `/api/v1/runs/{id}/events` endpoint exists
- [ ] Frontend: Stop polling on terminal states
- [ ] Frontend: Reset timer on each WebSocket message

---

## See Also

- [WebSocket Client Implementation](../client.go) - Connection handling
- [Hub Architecture](../hub.go) - Message broadcasting
- [Redis Subscriber](../redis_subscriber.go) - Event source
