# Human-in-the-Loop (HITL) Demo

This demo shows how workflows can pause for human approval and resume based on the decision.

## What You'll See

**HITL Node Execution:**
- Workflow reaches a HITL node (e.g., "approve payment" or "review agent decision")
- Execution pauses - workflow counter drops to zero
- Approval request appears in the UI with context

**UI Features:**
- Shows the workflow state at time of approval request
- Displays input data, previous node outputs
- Manager can approve or reject with reason
- Real-time status updates

**Execution Flow:**
- **Approve**: Workflow resumes, continues to next node
- **Reject**: Workflow takes rejection path (if configured) or marks as failed

**Resume Capability:**
- System can resume from any point in the execution
- No state loss - all data persisted in Redis and Postgres
- Coordinator loads IR and continues routing tokens

## Key Takeaways

- Workflows can pause indefinitely waiting for human input
- No polling - event-driven architecture (Redis pub/sub)
- Full context provided to approver (inputs, outputs, state)
- Audit trail: who approved/rejected, when, and why

[▶️ Watch Demo](https://github.com/Dutt23/agentic-orchestrator/releases/download/Release-v.1/HITL-approval.mov)
