# Rate Limiting Architecture

> **Multi-level rate limiting for cost control and fairness**

## 📖 Document Overview

**Purpose:** Multi-level rate limiting strategy from workflow-aware to agent-specific

**In this document:**
- [Architecture](#architecture) - Multi-level flow diagram
- [Level 1: Workflow-Aware](#level-1-workflow-aware-rate-limiting) - Tier-based (Simple/Standard/Heavy)
- [Level 2: Agent-Specific](#level-2-agent-specific-rate-limiting) - Global, per-user, per-workflow
- [Level 3: Agent Spawn Protection](#level-3-agent-spawn-protection) - Triple validation
- [Cost Tracking](#cost-tracking-optional) - Budget monitoring
- [Error Handling](#error-handling) - Clear error messages
- [Configuration](#configuration) - Settings and tuning
- [Benefits](#benefits) - Cost protection, fairness, loop detection
- [Testing](#testing) - Verification approaches

---

## Overview

The platform implements **workflow-aware rate limiting** at multiple levels to prevent cost overruns, ensure fairness, and detect runaway workflows.

**Unique approach:** Rate limits based on workflow complexity, not just request count.

---

## Architecture

```
Request Flow:
  ↓
Global Limiter (service-wide protection)
  ↓
Per-User Limiter (fairness between users)
  ↓
Workflow Inspector (analyze complexity)
  ↓
Tier-Based Limiter (based on agent count)
  ↓
Request Processed
```

---

## Level 1: Workflow-Aware Rate Limiting

**Location:** `common/ratelimit/workflow_inspector.go`

**Innovation:** Different limits based on workflow complexity

### Tier Classification

```go
func InspectWorkflow(workflow *Workflow) RateLimitTier {
    agentCount := countNodesByType(workflow, "agent")

    switch {
    case agentCount == 0:
        return SimpleTier   // 100 requests/min
    case agentCount <= 2:
        return StandardTier // 20 requests/min
    case agentCount > 2:
        return HeavyTier    // 5 requests/min
    }
}
```

### Rate Limit Tiers

| Tier | Agent Count | Limit | Use Case |
|------|------------|-------|----------|
| **Simple** | 0 | 100/min | Pure HTTP/function workflows |
| **Standard** | 1-2 | 20/min | Light agent usage |
| **Heavy** | 3+ | 5/min | Heavy LLM usage |

**Why this works:**
- Simple workflows (no agents) aren't throttled by agent workflows
- Heavy agent workflows limited to prevent cost blowup
- Fair resource allocation

### Implementation (Redis Lua Script)

```lua
-- rate_limit.lua
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window = tonumber(ARGV[2])

local current = redis.call('INCR', key)

if current == 1 then
    redis.call('EXPIRE', key, window)
end

if current > limit then
    redis.call('DECR', key)
    return 0  -- Rate limited
end

return 1  -- Allowed
```

**Benefits:**
- Atomic operation (single round-trip)
- Distributed (works across instances)
- Simple (no complex state)

**Code:** [../../common/ratelimit/](../../common/ratelimit/)

---

## Level 2: Agent-Specific Rate Limiting

**Location:** `cmd/agent-runner-py/` rate limiting

### Multi-Level Protection

#### A. Global Limiter (Service-Wide)

**Purpose:** Protect OpenAI API quota

**Algorithm:** Token bucket
- Capacity: 100 requests
- Refill: 10 requests/minute
- Allows bursts, then throttles

```python
class GlobalRateLimiter:
    def __init__(self, capacity=100, refill_rate=10):
        self.capacity = capacity
        self.tokens = capacity
        self.refill_rate = refill_rate

    def acquire(self) -> bool:
        # Refill tokens based on elapsed time
        now = time.time()
        elapsed = now - self.last_refill
        refill = (elapsed / 60) * self.refill_rate
        self.tokens = min(self.capacity, self.tokens + refill)
        self.last_refill = now

        # Try to consume token
        if self.tokens >= 1:
            self.tokens -= 1
            return True

        return False  # Rate limited
```

#### B. Per-User Limiter

**Purpose:** Fairness between users

**Algorithm:** Sliding window (Redis)
- Limit: 20 requests/minute per user
- Distributed across agent instances

```python
class PerUserRateLimiter:
    def __init__(self, redis_client, limit=20, window_sec=60):
        self.redis = redis_client
        self.limit = limit
        self.window = window_sec

    def acquire(self, username: str) -> bool:
        key = f"rate_limit:user:{username}"

        # Lua script for atomic increment + check
        result = self.redis.eval(rate_limit_script,
            keys=[key],
            args=[self.limit, self.window])

        return result == 1
```

#### C. Per-Workflow Limiter (Circuit Breaker)

**Purpose:** Detect runaway workflows

**Algorithm:** Circuit breaker
- Threshold: 50 agent calls/hour per workflow
- Prevents infinite loops

```python
class PerWorkflowRateLimiter:
    def __init__(self, redis_client, limit=50, window_sec=3600):
        self.redis = redis_client
        self.limit = limit
        self.window = window_sec

    def acquire(self, workflow_tag: str, username: str) -> bool:
        key = f"rate_limit:workflow:{username}:{workflow_tag}"

        count = self.redis.incr(key)
        if count == 1:
            self.redis.expire(key, self.window)

        if count > self.limit:
            # Workflow exceeded limit (possible loop!)
            return False

        return True
```

---

## Level 3: Agent Spawn Protection

**Location:** Multiple layers

### Triple Validation

**Layer 1 - Python (agent-runner-py):**
```python
def validate_patch(patch: dict, current_workflow: dict):
    new_agents = count_agent_nodes(patch)
    existing_agents = count_agent_nodes(current_workflow)

    if existing_agents + new_agents > MAX_AGENTS:
        return ValidationResult(
            valid=False,
            error=f"Agent limit exceeded (max {MAX_AGENTS})"
        )
```

**Layer 2 - Go (orchestrator):**
```go
func (v *Validator) ValidateAgentLimit(workflow *Workflow, patch *Patch) error {
    count := countNodesByType(workflow, "agent")
    newCount := countNodesInPatch(patch, "agent")

    if count + newCount > MaxAgentsPerWorkflow {
        return ErrAgentLimitExceeded
    }

    return nil
}
```

**Layer 3 - Coordinator (workflow-runner):**
```go
func (c *Coordinator) checkAgentLimit(runID string) bool {
    ir := c.loadIR(runID)
    agentCount := countNodesByType(ir, "agent")
    return agentCount <= MaxAgentsPerWorkflow
}
```

**Max agents per workflow:** 5 (configurable)

---

## Cost Tracking (Optional)

### Track Usage

```python
class CostTracker:
    def __init__(self, redis_client):
        self.redis = redis_client
        # GPT-4o pricing
        self.cost_per_1k_input = 0.0025
        self.cost_per_1k_output = 0.01

    def track_usage(self, username, workflow_tag, tokens_in, tokens_out):
        cost_usd = (tokens_in * self.cost_per_1k_input / 1000) + \
                   (tokens_out * self.cost_per_1k_output / 1000)

        # Daily rollup
        day_key = f"cost:daily:{username}:{date.today()}"
        self.redis.incrbyfloat(day_key, cost_usd)
        self.redis.expire(day_key, 86400 * 7)  # Keep 7 days

        # Monthly rollup
        month_key = f"cost:monthly:{username}:{date.today().strftime('%Y-%m')}"
        self.redis.incrbyfloat(month_key, cost_usd)

        # Check budget
        daily_spend = self.redis.get(day_key)
        if daily_spend > DAILY_BUDGET:
            # Alert or throttle
            pass
```

### Query Usage

```python
# Get user's daily cost
daily_cost = redis.get(f"cost:daily:{username}:{today}")

# Get workflow cost
workflow_cost = redis.get(f"cost:workflow:{username}:{workflow_tag}")

# Get monthly total
monthly_cost = redis.get(f"cost:monthly:{username}:{month}")
```

---

## Error Handling

### Rate Limited Response

```json
{
  "status": "failed",
  "error": "Rate limit exceeded",
  "error_type": "RateLimitError",
  "details": {
    "limit_type": "per_workflow",
    "limit": 5,
    "window": "60 seconds",
    "retry_after_seconds": 30,
    "message": "Heavy workflow (3+ agents) limited to 5 req/min. Please wait 30s."
  },
  "help": "Reduce agent count or contact support to increase limit"
}
```

### Agent Spawn Blocked

```json
{
  "status": "failed",
  "error": "Agent spawn limit exceeded",
  "error_type": "ValidationError",
  "details": {
    "max_agents": 5,
    "current_agents": 5,
    "attempted_new_agents": 1,
    "message": "Workflow already has 5 agents (max allowed)"
  },
  "help": "Remove existing agent nodes or increase limit in config"
}
```

---

## Configuration

### Global Config

**Location:** `cmd/orchestrator/config.go` or environment variables

```yaml
rate_limiting:
  enabled: true

  # Workflow-aware tiers
  simple_tier:
    agent_count: 0
    limit: 100
    window_sec: 60

  standard_tier:
    agent_count_max: 2
    limit: 20
    window_sec: 60

  heavy_tier:
    agent_count_min: 3
    limit: 5
    window_sec: 60

  # Agent spawn protection
  max_agents_per_workflow: 5

  # Global service limits
  global:
    capacity: 100
    refill_rate: 10
```

### Agent Runner Config

**Location:** `cmd/agent-runner-py/config.yaml`

```yaml
rate_limiting:
  enabled: true

  global:
    capacity: 100
    refill_rate: 10  # per minute

  per_user:
    limit: 20
    window_sec: 60

  per_workflow:
    limit: 50
    window_sec: 3600

cost_tracking:
  enabled: true
  daily_budget_usd: 100
  monthly_budget_usd: 2000
```

---

## Metrics

### Prometheus Metrics

```go
var (
    rateLimitHits = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "rate_limit_hits_total"},
        []string{"tier", "type"},  // tier: simple/standard/heavy, type: global/user/workflow
    )

    rateLimitAllowed = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "rate_limit_allowed_total"},
        []string{"tier"},
    )

    agentSpawnBlocked = prometheus.NewCounter(
        prometheus.CounterOpts{Name: "agent_spawn_blocked_total"},
    )

    llmCostUSD = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "llm_cost_usd_total"},
        []string{"username", "workflow_tag"},
    )
)
```

### Dashboard Queries

```promql
# Rate limit hit rate
rate(rate_limit_hits_total[5m])

# Cost per user (daily)
sum by (username) (rate(llm_cost_usd_total[1d]))

# Agent spawn blocks
rate(agent_spawn_blocked_total[1h])
```

---

## Benefits

### Cost Protection

**Without rate limiting:**
```
User creates workflow with 10 agents
Each agent spawns 2 more agents (exponential!)
Total: 10 + 20 + 40 + 80 = 150 agent calls
Cost: $50-150 (depending on token usage)
```

**With rate limiting:**
```
Agent spawn limit: Max 5 agents
Workflow tier: Heavy (5 req/min)
Cost controlled: $5-15 max per workflow
```

### Fairness

**Without per-user limiting:**
```
User A submits 100 agent workflows → monopolizes queue
User B's simple workflows → starved
```

**With per-user limiting:**
```
User A: 20 req/min (fair share)
User B: 20 req/min (fair share)
Both can make progress
```

### Loop Detection

**Without per-workflow limiting:**
```
Agent accidentally creates infinite loop
→ 1000s of LLM calls
→ $$$$ bill
```

**With per-workflow limiting:**
```
50 agent calls/hour limit
→ Circuit breaker trips
→ Workflow fails with clear error
→ Cost contained
```

---

## Implementation Status

### Phase 1 (Implemented)
✅ Workflow-aware rate limiting (tier-based)
✅ Global service limiter
✅ Per-user fairness
✅ Agent spawn limits (max 5)
✅ Clear error messages

### Phase 2 (Future)
⏳ Per-workflow circuit breaker
⏳ Cost tracking and budgets
⏳ Dashboard for usage monitoring
⏳ Dynamic limit adjustment

---

## Testing

### Test Rate Limiting

```bash
# Run rate limit test
./dev_scripts/test_rate_limit.sh

# Expected: Simple workflows pass, heavy workflows throttled
```

### Test Agent Spawn Limits

```bash
# Try to create workflow with 6 agents
# Expected: Validation error

# Create 5 agents, then agent tries to spawn 1 more
# Expected: Spawn blocked
```

---

## References

**Implementation:**
- [../../common/ratelimit/workflow_inspector.go](../../common/ratelimit/workflow_inspector.go)
- [../../common/ratelimit/lua/rate_limit.lua](../../common/ratelimit/lua/)
- [../../cmd/agent-runner-py/docs/RATE_LIMITING_PLAN.md](../../cmd/agent-runner-py/docs/RATE_LIMITING_PLAN.md)

**Testing:**
- [../../dev_scripts/test_rate_limit.sh](../../dev_scripts/test_rate_limit.sh)

---

**Multi-level protection: workflow-aware, user-fair, cost-controlled.**
