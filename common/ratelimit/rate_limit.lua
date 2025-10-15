-- Atomic rate limiting with sliding window
--
-- KEYS[1]: Redis key for rate limit counter
-- ARGV[1]: Limit (max requests allowed)
-- ARGV[2]: Window in seconds
--
-- Returns: {allowed (1/0), current_count, limit, retry_after_seconds}

local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window = tonumber(ARGV[2])

-- Increment counter atomically
local count = redis.call('INCR', key)

-- Set expiry only on first increment (avoids race condition)
if count == 1 then
    redis.call('EXPIRE', key, window)
end

-- Check if limit exceeded
if count > limit then
    -- Get TTL to tell user when they can retry
    local ttl = redis.call('TTL', key)
    if ttl < 0 then
        -- Key expired between INCR and TTL, retry immediately
        ttl = 0
    end

    -- Return: not allowed, current count, limit, retry after
    return {0, count, limit, ttl}
else
    -- Return: allowed, current count, limit, no retry needed
    return {1, count, limit, 0}
end
