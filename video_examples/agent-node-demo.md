# Agent Node Demo

This demo shows how agent nodes can modify workflows during execution while staying within system guardrails.

## What You'll See

**Agent Behavior:**
- Agent analyzes the task and decides it needs additional nodes to complete its work
- It generates patches to add new nodes (HTTP calls, data processing, etc.)
- System validates patches and applies them mid-execution
- Workflow continues with the new topology

**Guardrails in Action:**
- Agent spawn limits enforced (max 5 agents per workflow)
- All external API calls go through our HTTP worker (SSRF protection)
- Invalid patches are rejected with clear error messages
- System switches between deterministic and non-deterministic execution as needed

**Post-Execution Analysis:**
- Execution path visualization showing which nodes ran
- All patches applied by the agent (JSON Patch operations)
- Output from each node (stored in CAS)
- Metrics: queue time, execution time, memory usage, CPU usage
- Full audit trail of agent decisions

## Key Takeaways

- Agents can safely modify workflows mid-execution
- Triple-layer validation prevents runaway agent spawning
- Complete observability: every decision is logged and auditable
- Workflow stays deterministic despite dynamic modifications

[▶️ Watch Demo](https://github.com/Dutt23/agentic-orchestrator/releases/download/Release-v.1/AgentNodeDemo.mp4)
