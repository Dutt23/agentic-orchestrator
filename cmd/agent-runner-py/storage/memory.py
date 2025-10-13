"""In-memory storage for agent results."""
import logging
from typing import Dict, Any, Optional
from datetime import datetime
import uuid

logger = logging.getLogger(__name__)


class MemoryStorage:
    """In-memory storage for agent results."""

    def __init__(self):
        """Initialize in-memory storage."""
        self.results = {}  # result_id -> result_data
        logger.info("In-memory storage initialized")

    def store_result(
        self,
        job_id: str,
        run_id: str,
        node_id: str,
        result_data: Optional[Dict[str, Any]] = None,
        status: str = "completed",
        error: Optional[Dict[str, Any]] = None,
        tool_calls: Optional[list] = None,
        tokens_used: Optional[int] = None,
        cache_hit: bool = False,
        execution_time_ms: Optional[int] = None,
        llm_model: Optional[str] = None
    ) -> str:
        """Store agent result in memory.

        Args:
            job_id: Unique job identifier
            run_id: Workflow run identifier
            node_id: Node that triggered the job
            result_data: Result data
            status: 'completed' or 'failed'
            error: Error details if failed
            tool_calls: Array of tool invocations
            tokens_used: Number of LLM tokens used
            cache_hit: Whether prompt cache was hit
            execution_time_ms: Execution time in milliseconds
            llm_model: LLM model used

        Returns:
            result_id: UUID of stored result
        """
        result_id = str(uuid.uuid4())

        self.results[result_id] = {
            "result_id": result_id,
            "job_id": job_id,
            "run_id": run_id,
            "node_id": node_id,
            "result_data": result_data,
            "status": status,
            "error": error,
            "tool_calls": tool_calls,
            "tokens_used": tokens_used,
            "cache_hit": cache_hit,
            "execution_time_ms": execution_time_ms,
            "llm_model": llm_model,
            "created_at": datetime.utcnow().isoformat(),
            "completed_at": datetime.utcnow().isoformat()
        }

        logger.info(f"Stored result for job {job_id}: {result_id}")
        return result_id

    def get_result(self, result_id: str) -> Optional[Dict[str, Any]]:
        """Retrieve result by result_id.

        Args:
            result_id: UUID of result to retrieve

        Returns:
            Result dictionary or None if not found
        """
        return self.results.get(result_id)

    def get_result_by_job_id(self, job_id: str) -> Optional[Dict[str, Any]]:
        """Retrieve result by job_id.

        Args:
            job_id: Job identifier

        Returns:
            Result dictionary or None if not found
        """
        for result in self.results.values():
            if result.get("job_id") == job_id:
                return result
        return None

    def close(self):
        """Close storage (no-op for memory storage)."""
        logger.info("Memory storage closed")
