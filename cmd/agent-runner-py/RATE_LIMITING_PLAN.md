# Agent Runner Rate Limiting Plan

## Problem Statement

Agent nodes make expensive OpenAI API calls. Without rate limiting:
- Malicious workflows could drain API quota
- Accidental loops could cause $$$$ charges
- No fairness between users/workflows
- No cost budgeting

## Proposed Solution

### Architecture: Multi-Level Rate Limiting

```
Request → Global Limiter → Per-User Limiter → Per-Workflow Limiter → LLM Call
```

### 1. Global Rate Limiter (Service Level)

**Purpose**: Protect OpenAI API quota and prevent service-wide overload

**Algorithm**: Token Bucket
- **Capacity**: 100 requests
- **Refill rate**: 10 requests/minute
- **Allows**: Bursts up to 100, then throttles to 10/min

**Implementation**:
```python
class GlobalRateLimiter:
    def __init__(self, capacity=100, refill_rate=10):
        self.capacity = capacity
        self.tokens = capacity
        self.refill_rate = refill_rate  # per minute
        self.last_refill = time.time()

    def acquire(self) -> bool:
        # Refill tokens based on elapsed time
        # Deduct 1 token if available
        # Return True if allowed, False if rate limited
```

### 2. Per-User Rate Limiter

**Purpose**: Fairness between users, prevent single user monopolizing

**Algorithm**: Sliding Window
- **Limit**: 20 requests per user per minute
- **Tracks**: Request count in last 60 seconds

**Implementation**:
```python
class PerUserRateLimiter:
    def __init__(self, redis_client, limit=20, window_sec=60):
        self.redis = redis_client
        self.limit = limit
        self.window = window_sec

    def acquire(self, username: str) -> bool:
        key = f"rate_limit:user:{username}"
        # Use Redis INCR + EXPIRE for distributed rate limiting
        count = redis.incr(key)
        if count == 1:
            redis.expire(key, window_sec)
        return count <= limit
```

### 3. Per-Workflow Rate Limiter

**Purpose**: Detect runaway workflows, prevent accidental loops

**Algorithm**: Circuit Breaker
- **Threshold**: 50 agent calls per workflow per hour
- **Action**: After threshold, reject with clear error
- **Reset**: After 1 hour

**Implementation**:
```python
class PerWorkflowRateLimiter:
    def __init__(self, redis_client, limit=50, window_sec=3600):
        self.redis = redis_client
        self.limit = limit
        self.window = window_sec

    def acquire(self, workflow_tag: str, username: str) -> bool:
        key = f"rate_limit:workflow:{username}:{workflow_tag}"
        # Track and limit
```

### 4. Cost-Based Limiter (Optional Phase 2)

**Purpose**: Budget control, track $ spent

**Tracks**:
- Tokens used per user/workflow
- Estimated cost (tokens × $0.005/1K)
- Daily/monthly budgets

**Implementation**:
```python
class CostTracker:
    def __init__(self, redis_client):
        self.redis = redis_client
        self.cost_per_1k_input = 0.005
        self.cost_per_1k_output = 0.015

    def track_usage(self, username, workflow_tag, tokens_in, tokens_out):
        cost = (tokens_in * self.cost_per_1k_input / 1000) +
               (tokens_out * self.cost_per_1k_output / 1000)
        # Store in Redis with daily/monthly rollup
```

## Implementation Strategy

### Phase 1: Basic Rate Limiting (High Priority)

1. **Global Limiter** - Protect OpenAI quota
2. **Per-User Limiter** - Fairness
3. **Logging** - Track who's rate limited and why

### Phase 2: Advanced (Lower Priority)

1. **Per-Workflow Limiter** - Detect loops
2. **Cost Tracking** - Budget monitoring
3. **Dashboard** - Show usage/costs

## Integration Points

**Where to add rate limiting:**

```python
# main.py, line 198 - Before LLM call
def process_job(...):
    # ...

    # Rate limiting check
    if not self.rate_limiter.acquire(username, workflow_tag):
        logger.warn(f"Rate limited: user={username}, workflow={workflow_tag}")
        self.signal_failure(job_id, "Rate limit exceeded. Try again later.")
        return

    # Proceed with LLM call
    llm_result = self.llm.chat(task, enhanced_context)

    # Track usage for cost monitoring
    self.cost_tracker.track(username, workflow_tag, llm_result['tokens_used'])
```

## Configuration

**config.yaml**:
```yaml
rate_limiting:
  enabled: true
  global:
    capacity: 100  # Max burst
    refill_rate: 10  # requests/minute
  per_user:
    limit: 20  # requests/minute per user
    window_sec: 60
  per_workflow:
    limit: 50  # requests/hour per workflow
    window_sec: 3600

cost_tracking:
  enabled: true
  daily_budget_usd: 100  # Per user
  monthly_budget_usd: 2000  # Service-wide
```

## Error Responses

When rate limited, return clear error:

```json
{
  "status": "failed",
  "error": "Rate limit exceeded",
  "error_type": "RateLimitError",
  "details": {
    "limit_type": "per_user",
    "limit": 20,
    "window": "60 seconds",
    "retry_after_seconds": 45,
    "message": "You've made 20 agent calls in the last minute. Please wait 45 seconds."
  }
}
```

## Benefits of Queue-Based Approach

Since workflows use queues:
- ✅ No HTTP 429 errors to handle
- ✅ Failed jobs can be retried later
- ✅ Backpressure naturally handled
- ✅ Can deprioritize rate-limited requests

## Metrics to Track

1. **Rate limit hits**: Counter per limit type
2. **Queue depth**: Monitor backlog when throttling
3. **Cost per user/workflow**: Track spending
4. **Token usage**: Input/output tokens
5. **Cache hit rate**: From prompt caching

## Recommendation

**Start with Phase 1**:
- Global limiter: 10 req/min (simple, effective)
- Per-user limiter: 20 req/min (fairness)
- Clear error messages

**Then add Phase 2** when needed:
- Per-workflow limiter (detect loops)
- Cost tracking (budget alerts)

This protects your OpenAI bill while allowing legitimate workflows to run smoothly!
