"""PostgreSQL database client for agent service."""
import psycopg2
from psycopg2.extras import RealDictCursor
import json
from typing import Optional, Dict, Any
import logging

logger = logging.getLogger(__name__)


class DatabaseClient:
    """PostgreSQL client for storing agent results."""

    def __init__(self, config: Dict[str, Any]):
        """Initialize database connection."""
        self.config = config
        self.conn = None
        self._connect()

    def _connect(self):
        """Establish database connection."""
        try:
            self.conn = psycopg2.connect(
                host=self.config['host'],
                port=self.config['port'],
                dbname=self.config['dbname'],
                user=self.config['user'],
                password=self.config['password']
            )
            logger.info("Database connection established")
        except Exception as e:
            logger.error(f"Failed to connect to database: {e}")
            raise

    def store_result(
        self,
        job_id: str,
        run_id: str,
        node_id: str,
        result_data: Optional[Dict[str, Any]] = None,
        cas_id: Optional[str] = None,
        status: str = "completed",
        error: Optional[Dict[str, Any]] = None,
        tool_calls: Optional[list] = None,
        tokens_used: Optional[int] = None,
        cache_hit: bool = False,
        execution_time_ms: Optional[int] = None,
        llm_model: Optional[str] = None
    ) -> str:
        """Store agent result in database.

        Args:
            job_id: Unique job identifier
            run_id: Workflow run identifier
            node_id: Node that triggered the job
            result_data: Result JSON (for small results <10MB)
            cas_id: CAS reference (for large results >=10MB)
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
        try:
            with self.conn.cursor(cursor_factory=RealDictCursor) as cur:
                cur.execute("""
                    INSERT INTO agent_results (
                        job_id, run_id, node_id,
                        result_data, cas_id,
                        status, error, tool_calls,
                        tokens_used, cache_hit, execution_time_ms, llm_model,
                        completed_at
                    ) VALUES (
                        %s, %s, %s,
                        %s, %s,
                        %s, %s, %s,
                        %s, %s, %s, %s,
                        NOW()
                    )
                    RETURNING result_id
                """, (
                    job_id, run_id, node_id,
                    json.dumps(result_data) if result_data else None, cas_id,
                    status, json.dumps(error) if error else None, json.dumps(tool_calls) if tool_calls else None,
                    tokens_used, cache_hit, execution_time_ms, llm_model
                ))

                result = cur.fetchone()
                self.conn.commit()

                result_id = str(result['result_id'])
                logger.info(f"Stored result for job {job_id}: {result_id}")
                return result_id

        except Exception as e:
            self.conn.rollback()
            logger.error(f"Failed to store result: {e}")
            raise

    def get_result(self, result_id: str) -> Optional[Dict[str, Any]]:
        """Retrieve result by result_id.

        Args:
            result_id: UUID of result to retrieve

        Returns:
            Result dictionary or None if not found
        """
        try:
            with self.conn.cursor(cursor_factory=RealDictCursor) as cur:
                cur.execute("""
                    SELECT * FROM agent_results WHERE result_id = %s
                """, (result_id,))

                row = cur.fetchone()
                if not row:
                    return None

                # Convert to dict and parse JSON fields
                result = dict(row)
                if result.get('result_data'):
                    result['result_data'] = json.loads(result['result_data']) if isinstance(result['result_data'], str) else result['result_data']
                if result.get('error'):
                    result['error'] = json.loads(result['error']) if isinstance(result['error'], str) else result['error']
                if result.get('tool_calls'):
                    result['tool_calls'] = json.loads(result['tool_calls']) if isinstance(result['tool_calls'], str) else result['tool_calls']

                return result

        except Exception as e:
            logger.error(f"Failed to retrieve result: {e}")
            raise

    def close(self):
        """Close database connection."""
        if self.conn:
            self.conn.close()
            logger.info("Database connection closed")
