# Agent Runner Documentation

Complete documentation for the Agent Runner service.

## Core Documentation

### AGENT_SERVICE.md
Complete implementation guide:
- Architecture overview
- Message protocol
- Tool specifications (5 tools)
- LLM integration
- Token caching strategy
- Implementation phases
- Testing strategy

**Start here for understanding the agent system!**

### LLM_OPTIMIZATIONS.md
Performance optimizations:
- Prompt caching strategy
- HTTP connection pooling
- Benchmarks and results

### RATE_LIMITING_PLAN.md
Multi-level rate limiting:
- Global limiter (service-wide)
- Per-user limiter (fairness)
- Per-workflow limiter (loop detection)
- Cost tracking

## Testing Documentation

### TEST_PATCH_FLOW.md
Testing agent patch functionality

### RUN_TESTS.md
Test execution guide

## Implementation Guides

### PATCH_WORKFLOW_GUIDE.md
Guide for workflow patching

### WORKFLOW_FETCHING_FIX.md
Workflow resolution fixes

---

**Also see:**
- [../README.md](../README.md) - Service overview
- [../tests/README.md](../tests/README.md) - Test suite
