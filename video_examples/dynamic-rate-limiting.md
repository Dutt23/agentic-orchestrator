# Dynamic Rate Limiting Demo

This demo shows workflow-aware rate limiting in action, protecting the system from resource exhaustion.

## What You'll See

**Attack Scenario:**
- User attempts to create a workflow with an excessive number of agent nodes
- Or an agent tries to spawn unlimited additional agents
- System could be overwhelmed by LLM API calls and processing

**Protection Layers:**

1. **Workflow Complexity Analysis:**
   - System inspects workflow before execution
   - Counts agent nodes, classifies into tier (simple/standard/heavy)
   - Applies appropriate rate limit (100/20/5 req/min)

2. **Agent Spawn Limits:**
   - Python validation: Rejects patches exceeding 5 agents
   - Go validation: Schema check catches invalid patches
   - Coordinator: Final security check during routing

3. **Runtime Enforcement:**
   - System stops malicious attempts before they consume resources
   - Clear error messages explain why request was rejected
   - Rate limits reset after cooldown period

## Key Takeaways

- Rate limits based on workflow complexity (not one-size-fits-all)
- Multiple independent validation layers
- System can't be DDoS'd by creating agent-heavy workflows
- Prevents $1000+ OpenAI bills from runaway agents

[▶️ Watch Demo](https://github.com/Dutt23/agentic-orchestrator/releases/download/Release-v.1/Dynamic.rate.limiting.mov)
