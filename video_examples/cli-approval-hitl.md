# CLI HITL Approval Demo

This demo shows how to approve/reject HITL nodes using the command-line interface instead of the web UI.

## What You'll See

**CLI Usage:**
- `aob approve <ticket-id> approve --reason "LGTM"` - Approve with reason
- `aob approve <ticket-id> reject --reason "Need more data"` - Reject with reason
- Real-time feedback in terminal

**Why CLI Matters:**
- Faster for power users (no need to open browser)
- Scriptable - can integrate into existing workflows
- Works over SSH - approve from anywhere
- JSON output for automation

**Workflow:**
1. HITL node pauses workflow
2. Manager runs `aob approve ticket_abc123 approve`
3. Workflow immediately resumes
4. Completion signal routed to next nodes

## Key Takeaways

- Full HITL functionality available from terminal
- Rust-based CLI is fast (<10ms startup)
- Can be scripted or integrated into CI/CD pipelines
- Same validation and audit trail as web UI

[¶ Watch Demo](https://github.com/Dutt23/agentic-orchestrator/releases/download/Release-v.1/CLI-HITL-approval.mov)

_Note: Video pending - CLI functionality implemented but demo video not yet recorded_
