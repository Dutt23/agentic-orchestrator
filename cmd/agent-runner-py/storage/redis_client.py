"""Redis client for agent service."""
import redis
import json
from typing import Optional, Dict, Any
import logging

logger = logging.getLogger(__name__)


class RedisClient:
    """Redis client for job queue and result publishing."""

    def __init__(self, config: Dict[str, Any]):
        """Initialize Redis connection."""
        self.config = config
        self.client = redis.Redis(
            host=config['host'],
            port=config['port'],
            db=config['db'],
            decode_responses=True
        )
        self.job_queue = config['job_queue']
        self.result_queue_prefix = config['result_queue_prefix']
        self.timeout = config.get('timeout', 5)

        # Test connection
        try:
            self.client.ping()
            logger.info("Redis connection established")
        except Exception as e:
            logger.error(f"Failed to connect to Redis: {e}")
            raise

    def pop_job(self) -> Optional[Dict[str, Any]]:
        """Pop a job from the job queue (blocking).

        Returns:
            Job dictionary or None if timeout
        """
        try:
            result = self.client.blpop(self.job_queue, timeout=self.timeout)
            if result:
                queue, payload = result
                job = json.loads(payload)
                logger.info(f"Popped job: {job.get('job_id')}")
                return job
            return None
        except Exception as e:
            logger.error(f"Failed to pop job: {e}")
            raise

    def publish_result(self, job_id: str, result: Dict[str, Any]):
        """Publish result to job-specific result queue.

        Args:
            job_id: Job identifier
            result: Result dictionary
        """
        try:
            queue = f"{self.result_queue_prefix}:{job_id}"
            payload = json.dumps(result)
            self.client.rpush(queue, payload)
            logger.info(f"Published result for job {job_id} to {queue}")
        except Exception as e:
            logger.error(f"Failed to publish result: {e}")
            raise

    def signal_completion(self, completion_signal: Dict[str, Any]):
        """Signal completion to workflow coordinator.

        This publishes to the shared completion_signals queue that the
        coordinator consumes for choreography.

        Args:
            completion_signal: CompletionSignal matching coordinator schema
        """
        try:
            payload = json.dumps(completion_signal)
            self.client.rpush("completion_signals", payload)
            logger.info(f"Signaled completion to coordinator: run={completion_signal.get('run_id')}, "
                       f"node={completion_signal.get('node_id')}, "
                       f"status={completion_signal.get('status')}")
        except Exception as e:
            logger.error(f"Failed to signal completion: {e}")
            raise

    def close(self):
        """Close Redis connection."""
        if self.client:
            self.client.close()
            logger.info("Redis connection closed")
