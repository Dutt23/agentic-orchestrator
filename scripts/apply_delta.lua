-- ============================================================================
-- apply_delta.lua - Atomic Counter Operation with Event Publishing
-- ============================================================================
-- Purpose: Apply counter delta (consume -1 or emit +N) with idempotency
-- Features:
--   - Idempotent: Each op_key can only be applied once
--   - Atomic: Counter update + applied set update in single operation
--   - Event-driven: Publishes to completion_events when counter hits 0
--
-- Usage:
--   EVAL script 3 applied_set_key counter_key run_id op_key delta
--
-- Example:
--   EVAL "..." 3 applied:run_123 counter:run_123 run_123 consume:run_123:A->B -1
--
-- Returns:
--   [new_counter_value, changed, hit_zero]
--   - new_counter_value: Current counter value after operation
--   - changed: 1 if operation was applied, 0 if already applied (idempotent)
--   - hit_zero: 1 if counter reached 0 after this operation, 0 otherwise
-- ============================================================================

local applied_set = KEYS[1]       -- "applied:run_123"
local counter_key = KEYS[2]       -- "counter:run_123"
local run_id = KEYS[3]            -- "run_123" (for publishing)
local op_key = ARGV[1]            -- "consume:run_123:A->B" or "emit:run_123:A:uuid"
local delta = tonumber(ARGV[2])   -- -1 for consume, +N for emit

-- 1. Check idempotency: Has this operation already been applied?
if redis.call('SISMEMBER', applied_set, op_key) == 1 then
    -- Already applied, return current counter without changing anything
    local current = redis.call('GET', counter_key)
    if current then
        return {tonumber(current), 0, 0}  -- value, not_changed, not_zero
    else
        return {0, 0, 1}  -- Counter doesn't exist, return 0, not_changed, hit_zero
    end
end

-- 2. Add to applied set (marks this operation as processed)
redis.call('SADD', applied_set, op_key)

-- 3. Update counter
local new_value = redis.call('INCRBY', counter_key, delta)

-- 4. Check if counter hit zero and publish completion event
if new_value == 0 then
    -- Publish to completion_events channel
    -- Supervisor listens to this channel for event-driven completion
    redis.call('PUBLISH', 'completion_events', run_id)
    return {new_value, 1, 1}  -- value=0, changed, hit_zero
else
    return {new_value, 1, 0}  -- value, changed, not_zero
end
