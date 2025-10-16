# Video Demonstrations

This directory contains demo videos showing key features of the orchestrator platform.

## Available Demos

### 1. [Agent Node Demo](./agent-node-demo.md)
**Runtime workflow patching in action**

Watch an agent node modify a workflow mid-execution by adding new nodes. Shows validation, guardrails, execution path, and metrics collection.

**Duration:** ~3 minutes
**Covers:** Agent patching, validation layers, audit trail, metrics

---

### 2. [Dynamic Rate Limiting](./dynamic-rate-limiting.md)
**Workflow-aware rate limiting and agent spawn protection**

See how the system protects itself from resource exhaustion when users try to create agent-heavy workflows or agents attempt to spawn unlimited additional agents.

**Duration:** ~2 minutes
**Covers:** Complexity-based rate limits, agent spawn limits, cost protection

---

### 3. [HITL Node Demo](./hitl-node-demo.md)
**Human-in-the-loop approval workflow**

Watch a workflow pause for human approval, then resume based on the decision. Shows the web UI for reviewing and approving HITL requests.

**Duration:** ~2 minutes
**Covers:** HITL nodes, pause/resume, approval UI, execution branching

---

### 4. [CLI HITL Approval](./cli-approval-hitl.md)
**Command-line interface for approvals**

See how to approve/reject HITL nodes from the terminal using the aob CLI tool. Faster than web UI for power users.

**Duration:** ~1 minute
**Covers:** CLI tool, scriptable approvals, terminal workflow

_Note: Video pending - CLI is implemented but demo not yet recorded_

---

## Quick Links

All demos are hosted on GitHub Releases:
https://github.com/Dutt23/agentic-orchestrator/releases/tag/Release-v.1

## Topics Covered Across Demos

- ✅ Runtime workflow patching
- ✅ Agent spawn limits and cost protection
- ✅ Workflow-aware rate limiting
- ✅ Human-in-the-loop approvals
- ✅ Execution path visualization
- ✅ Metrics collection (queue time, execution time, resources)
- ✅ Audit trail and observability
- ✅ CLI tool for terminal-based workflows

## See Also

- [Core Innovation Documentation](../submission_doc/innovation/UNIQUENESS.md) - Detailed explanation of unique features
- [Agent Service Documentation](../cmd/agent-runner-py/docs/AGENT_SERVICE.md) - Agent implementation details
- [HITL Architecture](../cmd/hitl-worker/README.md) - How HITL nodes work
- [Rate Limiting](../submission_doc/technical/RATE_LIMITING.md) - Rate limiting implementation
