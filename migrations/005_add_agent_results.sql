-- Migration: Add agent_results table for agentic service
-- Description: Store results from agent jobs for workflow execution

-- Agent Results Table
CREATE TABLE IF NOT EXISTS agent_results (
    result_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL UNIQUE,
    run_id TEXT NOT NULL,
    node_id TEXT NOT NULL,

    -- Result storage
    result_data JSONB,              -- For small results (<10MB)
    cas_id TEXT,                    -- Reference to CAS blob (for >=10MB)
    s3_ref TEXT,                    -- Future: S3 reference

    -- Metadata
    status TEXT NOT NULL,           -- 'completed', 'failed'
    error JSONB,                    -- Error details if failed
    tool_calls JSONB,               -- Array of tool invocations
    tokens_used INTEGER,
    cache_hit BOOLEAN DEFAULT false,
    execution_time_ms INTEGER,
    llm_model TEXT,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_agent_results_job_id ON agent_results(job_id);
CREATE INDEX IF NOT EXISTS idx_agent_results_run_id ON agent_results(run_id);
CREATE INDEX IF NOT EXISTS idx_agent_results_created_at ON agent_results(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_results_status ON agent_results(status);

-- Comments
COMMENT ON TABLE agent_results IS 'Stores results from agentic workflow nodes';
COMMENT ON COLUMN agent_results.result_data IS 'Direct JSONB storage for results <10MB';
COMMENT ON COLUMN agent_results.cas_id IS 'CAS reference for large results >=10MB';
COMMENT ON COLUMN agent_results.s3_ref IS 'Future S3 reference for very large results';
COMMENT ON COLUMN agent_results.tool_calls IS 'Array of LLM tool invocations made during execution';
