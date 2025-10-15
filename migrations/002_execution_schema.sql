-- ============================================================================
-- Workflow Execution Schema Extension
-- ============================================================================
-- Version: 1.0
-- Description: Add execution tables for workflow-runner service
-- Extends: 001_final_schema.sql (artifact management)
-- ============================================================================

-- ============================================================================
-- 1. EXTEND RUN TABLE WITH EXECUTION FIELDS
-- ============================================================================

-- Add execution tracking fields to existing run table
ALTER TABLE run ADD COLUMN IF NOT EXISTS counter_value INT DEFAULT 1;
ALTER TABLE run ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;
ALTER TABLE run ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ;
ALTER TABLE run ADD COLUMN IF NOT EXISTS error TEXT;

-- Update status check constraint to include new statuses
-- Note: This will fail if constraint exists, which is okay
DO $$
BEGIN
    ALTER TABLE run DROP CONSTRAINT IF EXISTS run_status_check;
    ALTER TABLE run ADD CONSTRAINT run_status_check
        CHECK (status IN ('QUEUED', 'RUNNING', 'COMPLETED', 'FAILED', 'CANCELLED'));
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- ============================================================================
-- 2. EVENT LOG (Execution Audit Trail)
-- ============================================================================

CREATE TABLE IF NOT EXISTS event_log (
    event_id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),

    -- Run reference (matches run table composite key)
    run_id UUID NOT NULL,
    run_submitted_at TIMESTAMPTZ NOT NULL,

    -- Event metadata
    event_type TEXT NOT NULL,             -- 'token.emitted', 'node.executed', etc.
    sequence_num BIGINT NOT NULL,         -- Strict ordering per run
    timestamp TIMESTAMPTZ DEFAULT now(),

    -- Event payload (small, no large data)
    event_data JSONB NOT NULL,

    -- Causality
    parent_event_id UUID,
    correlation_id UUID,

    -- Unique sequence per run
    UNIQUE (run_id, sequence_num),

    -- Foreign key to run table (composite key)
    FOREIGN KEY (run_id, run_submitted_at)
        REFERENCES run(run_id, submitted_at) ON DELETE CASCADE
);

-- Indexes for event log queries
CREATE INDEX IF NOT EXISTS idx_event_log_run
    ON event_log(run_id, sequence_num);

CREATE INDEX IF NOT EXISTS idx_event_log_type
    ON event_log(event_type);

CREATE INDEX IF NOT EXISTS idx_event_log_timestamp
    ON event_log(timestamp DESC);

COMMENT ON TABLE event_log IS 'Append-only event log for workflow execution audit trail';
COMMENT ON COLUMN event_log.event_type IS 'Event type: token.emitted, node.executed, node.failed, etc.';
COMMENT ON COLUMN event_log.sequence_num IS 'Monotonic sequence number per run for ordering';

-- ============================================================================
-- 3. APPLIED KEYS (Idempotency Tracking)
-- ============================================================================

CREATE TABLE IF NOT EXISTS applied_keys (
    -- Run reference
    run_id UUID NOT NULL,
    run_submitted_at TIMESTAMPTZ NOT NULL,

    -- Operation key
    op_key TEXT NOT NULL,                 -- "consume:run_123:A->B"
    delta INT NOT NULL,                   -- -1 or +N
    applied_at TIMESTAMPTZ DEFAULT now(),

    -- Primary key
    PRIMARY KEY (run_id, op_key),

    -- Foreign key to run table
    FOREIGN KEY (run_id, run_submitted_at)
        REFERENCES run(run_id, submitted_at) ON DELETE CASCADE
);

-- Index for queries by run
CREATE INDEX IF NOT EXISTS idx_applied_keys_run
    ON applied_keys(run_id, applied_at);

COMMENT ON TABLE applied_keys IS 'Track applied operations for idempotency (flushed from Redis on completion)';
COMMENT ON COLUMN applied_keys.op_key IS 'Unique operation key: consume:run_id:from->to or emit:run_id:node:uuid';
COMMENT ON COLUMN applied_keys.delta IS 'Counter delta: -1 for consume, +N for emit';

-- ============================================================================
-- 4. RUN COUNTER SNAPSHOTS (Recovery)
-- ============================================================================

CREATE TABLE IF NOT EXISTS run_counter_snapshots (
    run_id UUID NOT NULL,
    run_submitted_at TIMESTAMPTZ NOT NULL,

    -- Snapshot data
    value INT NOT NULL,
    snapshot_at TIMESTAMPTZ DEFAULT now(),

    -- Primary key
    PRIMARY KEY (run_id, snapshot_at),

    -- Foreign key to run table
    FOREIGN KEY (run_id, run_submitted_at)
        REFERENCES run(run_id, submitted_at) ON DELETE CASCADE
);

-- Index for latest snapshot queries
CREATE INDEX IF NOT EXISTS idx_counter_snapshots_latest
    ON run_counter_snapshots(run_id, snapshot_at DESC);

COMMENT ON TABLE run_counter_snapshots IS 'Periodic counter snapshots for recovery (every 10s)';

-- ============================================================================
-- 5. PENDING TOKENS (Join Pattern Support)
-- ============================================================================
-- Note: This table is primarily managed in Redis for performance
-- This is for persistence/recovery only

CREATE TABLE IF NOT EXISTS pending_tokens (
    run_id UUID NOT NULL,
    run_submitted_at TIMESTAMPTZ NOT NULL,

    -- Join node info
    node_id TEXT NOT NULL,
    from_node TEXT NOT NULL,              -- Which dependency sent this token

    -- Token metadata
    token_id UUID NOT NULL,
    payload_ref TEXT,
    received_at TIMESTAMPTZ DEFAULT now(),

    -- Primary key
    PRIMARY KEY (run_id, node_id, from_node),

    -- Foreign key to run table
    FOREIGN KEY (run_id, run_submitted_at)
        REFERENCES run(run_id, submitted_at) ON DELETE CASCADE
);

-- Index for node queries
CREATE INDEX IF NOT EXISTS idx_pending_tokens_node
    ON pending_tokens(run_id, node_id);

COMMENT ON TABLE pending_tokens IS 'Track pending tokens for join pattern (wait_for_all nodes)';
COMMENT ON COLUMN pending_tokens.from_node IS 'Which dependency sent this token (path tracking)';

-- ============================================================================
-- 6. NODE EXECUTIONS (Execution History)
-- ============================================================================

CREATE TABLE IF NOT EXISTS node_executions (
    execution_id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),

    -- Run reference
    run_id UUID NOT NULL,
    run_submitted_at TIMESTAMPTZ NOT NULL,

    -- Node info
    node_id TEXT NOT NULL,
    node_type TEXT NOT NULL,              -- 'task', 'agent', 'human', etc.

    -- Execution status
    status TEXT NOT NULL DEFAULT 'RUNNING' CHECK (status IN ('RUNNING', 'SUCCESS', 'FAILED')),

    -- Timing
    started_at TIMESTAMPTZ DEFAULT now(),
    ended_at TIMESTAMPTZ,
    duration_ms INT,

    -- I/O references (CAS)
    input_cas_ref TEXT,
    output_cas_ref TEXT,
    error_details JSONB,

    -- Attempt tracking
    attempt INT DEFAULT 1,
    retry_count INT DEFAULT 0,

    -- Metadata
    worker_id TEXT,                       -- Which worker executed this

    -- Foreign key to run table
    FOREIGN KEY (run_id, run_submitted_at)
        REFERENCES run(run_id, submitted_at) ON DELETE CASCADE
);

-- Indexes for execution queries
CREATE INDEX IF NOT EXISTS idx_node_executions_run
    ON node_executions(run_id, started_at DESC);

CREATE INDEX IF NOT EXISTS idx_node_executions_node
    ON node_executions(run_id, node_id, started_at DESC);

CREATE INDEX IF NOT EXISTS idx_node_executions_status
    ON node_executions(status) WHERE status = 'RUNNING';

COMMENT ON TABLE node_executions IS 'Track individual node executions for observability';
COMMENT ON COLUMN node_executions.worker_id IS 'Worker instance that executed this node';

-- ============================================================================
-- 7. LOOP STATE (Iteration Tracking)
-- ============================================================================
-- Note: Primarily managed in Redis, this is for persistence

CREATE TABLE IF NOT EXISTS loop_state (
    run_id UUID NOT NULL,
    run_submitted_at TIMESTAMPTZ NOT NULL,
    node_id TEXT NOT NULL,

    -- Loop tracking
    current_iteration INT NOT NULL DEFAULT 0,
    max_iterations INT NOT NULL,
    last_output_ref TEXT,

    -- State
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'COMPLETED', 'TIMEOUT')),
    started_at TIMESTAMPTZ DEFAULT now(),
    ended_at TIMESTAMPTZ,

    -- Primary key
    PRIMARY KEY (run_id, node_id),

    -- Foreign key to run table
    FOREIGN KEY (run_id, run_submitted_at)
        REFERENCES run(run_id, submitted_at) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_loop_state_run
    ON loop_state(run_id);

COMMENT ON TABLE loop_state IS 'Track loop iterations for nodes with loop config';

-- ============================================================================
-- 8. RUN STATISTICS (Aggregated Metrics)
-- ============================================================================

CREATE TABLE IF NOT EXISTS run_statistics (
    run_id UUID PRIMARY KEY,
    run_submitted_at TIMESTAMPTZ NOT NULL,

    -- Counter metrics
    total_tokens_emitted INT DEFAULT 0,
    total_tokens_consumed INT DEFAULT 0,
    final_counter_value INT,

    -- Node metrics
    total_nodes_executed INT DEFAULT 0,
    total_nodes_failed INT DEFAULT 0,
    total_retries INT DEFAULT 0,

    -- Timing
    total_execution_time_ms BIGINT,

    -- Updated timestamp
    last_updated_at TIMESTAMPTZ DEFAULT now(),

    -- Foreign key to run table
    FOREIGN KEY (run_id, run_submitted_at)
        REFERENCES run(run_id, submitted_at) ON DELETE CASCADE
);

COMMENT ON TABLE run_statistics IS 'Aggregated statistics per workflow run';

-- ============================================================================
-- 9. WORKER REGISTRY (Active Workers)
-- ============================================================================

CREATE TABLE IF NOT EXISTS worker_registry (
    worker_id TEXT PRIMARY KEY,
    worker_type TEXT NOT NULL,            -- 'task', 'agent', 'human'

    -- Status
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'IDLE', 'STOPPED')),

    -- Capacity
    max_concurrent INT DEFAULT 10,
    current_load INT DEFAULT 0,

    -- Metadata
    hostname TEXT,
    version TEXT,
    capabilities JSONB DEFAULT '{}'::jsonb,

    -- Heartbeat
    last_heartbeat_at TIMESTAMPTZ DEFAULT now(),
    started_at TIMESTAMPTZ DEFAULT now(),

    -- Redis stream
    stream_name TEXT                      -- Which Redis stream this worker listens to
);

-- Index for active workers
CREATE INDEX IF NOT EXISTS idx_worker_registry_active
    ON worker_registry(status, worker_type)
    WHERE status = 'ACTIVE';

-- Index for stale workers (no heartbeat) - can't use now() in predicate, query without predicate instead
CREATE INDEX IF NOT EXISTS idx_worker_registry_stale
    ON worker_registry(status, last_heartbeat_at)
    WHERE status = 'ACTIVE';

COMMENT ON TABLE worker_registry IS 'Track active workers for monitoring and load balancing';
COMMENT ON COLUMN worker_registry.last_heartbeat_at IS 'Workers send heartbeat every 30s';

-- ============================================================================
-- 10. VIEWS FOR COMMON QUERIES
-- ============================================================================

-- View: Active runs
CREATE OR REPLACE VIEW v_active_runs AS
SELECT
    r.run_id,
    r.status,
    r.counter_value,
    r.submitted_at,
    r.started_at,
    EXTRACT(EPOCH FROM (COALESCE(r.completed_at, now()) - r.started_at)) as duration_seconds,
    rs.total_nodes_executed,
    rs.total_nodes_failed
FROM run r
LEFT JOIN run_statistics rs ON r.run_id = rs.run_id
WHERE r.status IN ('RUNNING', 'QUEUED')
ORDER BY r.submitted_at DESC;

COMMENT ON VIEW v_active_runs IS 'Currently active workflow runs with statistics';

-- View: Failed executions
CREATE OR REPLACE VIEW v_failed_executions AS
SELECT
    ne.run_id,
    ne.node_id,
    ne.error_details,
    ne.started_at,
    ne.duration_ms,
    ne.retry_count,
    r.status as run_status
FROM node_executions ne
JOIN run r ON ne.run_id = r.run_id
WHERE ne.status = 'FAILED'
ORDER BY ne.started_at DESC;

COMMENT ON VIEW v_failed_executions IS 'Failed node executions for debugging';

-- ============================================================================
-- 11. FUNCTIONS FOR COMMON OPERATIONS
-- ============================================================================

-- Function: Get run counter from applied_keys
CREATE OR REPLACE FUNCTION get_run_counter(p_run_id UUID)
RETURNS INT AS $$
    SELECT COALESCE(SUM(delta), 0)::INT
    FROM applied_keys
    WHERE run_id = p_run_id;
$$ LANGUAGE SQL STABLE;

COMMENT ON FUNCTION get_run_counter(UUID) IS 'Calculate counter value from applied_keys';

-- Function: Check if run is complete
CREATE OR REPLACE FUNCTION is_run_complete(p_run_id UUID)
RETURNS BOOLEAN AS $$
    SELECT
        COALESCE(get_run_counter(p_run_id), 0) = 0
        AND NOT EXISTS (
            SELECT 1 FROM pending_tokens WHERE run_id = p_run_id
        )
        AND NOT EXISTS (
            SELECT 1 FROM node_executions
            WHERE run_id = p_run_id AND status = 'RUNNING'
        );
$$ LANGUAGE SQL STABLE;

COMMENT ON FUNCTION is_run_complete(UUID) IS 'Check if run is truly complete (counter=0, no pending work)';

-- ============================================================================
-- 12. TRIGGERS
-- ============================================================================

-- Trigger: Update run statistics on node execution
CREATE OR REPLACE FUNCTION update_run_statistics()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO run_statistics (run_id, run_submitted_at, total_nodes_executed, last_updated_at)
        VALUES (NEW.run_id, NEW.run_submitted_at, 1, now())
        ON CONFLICT (run_id) DO UPDATE
        SET total_nodes_executed = run_statistics.total_nodes_executed + 1,
            last_updated_at = now();
    ELSIF TG_OP = 'UPDATE' AND NEW.status = 'FAILED' THEN
        UPDATE run_statistics
        SET total_nodes_failed = total_nodes_failed + 1,
            total_retries = total_retries + NEW.retry_count,
            last_updated_at = now()
        WHERE run_id = NEW.run_id;
    END IF
;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_update_run_statistics
    AFTER INSERT OR UPDATE ON node_executions
    FOR EACH ROW
    EXECUTE FUNCTION update_run_statistics();

COMMENT ON FUNCTION update_run_statistics() IS 'Automatically update run statistics when nodes execute';

-- Trigger: Update run completed_at
CREATE OR REPLACE FUNCTION update_run_completed_at()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status IN ('COMPLETED', 'FAILED', 'CANCELLED') AND OLD.completed_at IS NULL THEN
        NEW.completed_at = now();
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_update_run_completed_at
    BEFORE UPDATE ON run
    FOR EACH ROW
    EXECUTE FUNCTION update_run_completed_at();

COMMENT ON FUNCTION update_run_completed_at() IS 'Automatically set completed_at when run finishes';

-- ============================================================================
-- END OF SCHEMA
-- ============================================================================

-- Grant permissions (adjust for your setup)
-- GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA public TO workflow_runner;
-- GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO workflow_runner;
