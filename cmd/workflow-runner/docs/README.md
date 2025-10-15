# Workflow Runner Documentation

Comprehensive documentation for the stateless coordinator service.

## Documents

### CHOREOGRAPHY_EXECUTION_DESIGN.md (98KB)

Complete execution model covering:
- Token-based choreography
- Counter system (idempotency)
- Join patterns (wait_for_all)
- Terminal node optimization
- Agent integration
- HITL (pause/resume)
- Event sourcing & replay

**Essential reading for understanding the execution engine!**

### RUN_LIFECYCLE_ARCHITECTURE.md (16KB)

Run lifecycle management:
- Run states
- State transitions
- Completion detection
- Cleanup

### NODE_REPLAY.md (19KB)

Replay system:
- Replay modes (freeze, shadow)
- Checkpoint recovery
- Deterministic replay

---

**Also see:**
- Coordinator implementation: [../coordinator/](../coordinator/)
