"""Redis client for agent service."""
import redis
import json
from typing import Optional, Dict, Any
import logging
import uuid

logger = logging.getLogger(__name__)


class RedisClient:
    """Redis client for job queue (streams) and result publishing."""

    def __init__(self, config: Dict[str, Any]):
        """Initialize Redis connection."""
        self.config = config
        self.client = redis.Redis(
            host=config['host'],
            port=config['port'],
            db=config['db'],
            decode_responses=True
        )
        # Use Redis streams for new architecture
        self.stream = config.get('stream', 'wf.tasks.agent')
        self.consumer_group = config.get('consumer_group', 'agent_workers')
        self.consumer_name = f"agent_worker_{uuid.uuid4().hex[:8]}"
        self.timeout = config.get('timeout', 5) * 1000  # Convert to milliseconds

        # Backward compatibility: legacy queue names
        self.job_queue = config.get('job_queue', 'agent:jobs')
        self.result_queue_prefix = config.get('result_queue_prefix', 'agent:results')

        # Test connection
        try:
            self.client.ping()
            logger.info("Redis connection established")
        except Exception as e:
            logger.error(f"Failed to connect to Redis: {e}")
            raise

        # Create consumer group if it doesn't exist
        try:
            self.client.xgroup_create(self.stream, self.consumer_group, id='0', mkstream=True)
            logger.info(f"Created consumer group {self.consumer_group} for stream {self.stream}")
        except redis.ResponseError as e:
            if "BUSYGROUP" not in str(e):
                logger.error(f"Failed to create consumer group: {e}")
                raise
            # Group already exists, continue
            logger.info(f"Consumer group {self.consumer_group} already exists")

    def pop_job(self) -> Optional[Dict[str, Any]]:
        """Pop a job from the stream (blocking with XREADGROUP).

        Returns:
            Job dictionary or None if timeout
        """
        try:
            # Read from stream using consumer group
            messages = self.client.xreadgroup(
                groupname=self.consumer_group,
                consumername=self.consumer_name,
                streams={self.stream: '>'},
                count=1,
                block=self.timeout
            )

            if not messages:
                return None

            # Extract message from stream
            stream_name, message_list = messages[0]
            if not message_list:
                return None

            message_id, message_data = message_list[0]

            # Parse token from message
            token_json = message_data.get('token')
            if not token_json:
                logger.error(f"Message {message_id} missing token field")
                # ACK the message to remove it from pending
                self.client.xack(self.stream, self.consumer_group, message_id)
                return None

            token = json.loads(token_json)

            # Log the raw token received from coordinator for debugging
            logger.info(f"Raw token from coordinator: {json.dumps(token, indent=2)}")
            logger.info(f"Token metadata: {token.get('metadata', 'NO METADATA FIELD')}")

            # Fetch current workflow IR from Redis
            run_id = token.get('run_id')
            current_workflow = None
            if run_id:
                ir_key = f"ir:{run_id}"
                try:
                    ir_data = self.client.get(ir_key)
                    if ir_data:
                        current_workflow = json.loads(ir_data)
                        logger.info(f"Fetched workflow IR from Redis: {ir_key}, nodes={len(current_workflow.get('nodes', {}))}")
                    else:
                        logger.warning(f"No IR found in Redis for run_id {run_id} at key {ir_key}")
                except Exception as e:
                    logger.error(f"Failed to fetch workflow IR from Redis: {e}")
            else:
                logger.warning("Token missing run_id, cannot fetch workflow IR")

            # Convert token to job format expected by main.py
            metadata = token.get('metadata', {})
            job = {
                'job_id': token.get('id'),
                'run_id': run_id,
                'node_id': token.get('to_node'),
                'task': metadata.get('task', ''),
                'context': metadata.get('context', {}),
                'workflow_owner': token.get('workflow_owner', 'test-user'),  # From coordinator, with fallback
                'workflow_tag': metadata.get('workflow_tag', ''),  # Optional, for context
                'current_workflow': current_workflow,  # Fetched from Redis IR
                'current_node_id': token.get('to_node'),  # The node that will execute (for patch edge creation)
                'token': token,  # Store full token for later
                'message_id': message_id  # Store for ACK
            }

            logger.info(f"Converted job: job_id={job.get('job_id')}, task='{job.get('task')}', "
                       f"workflow_owner='{job.get('workflow_owner')}', "
                       f"current_node_id='{job.get('current_node_id')}', "
                       f"has_workflow={current_workflow is not None}")
            logger.info(f"Received job from stream: {job.get('job_id')}")
            return job

        except Exception as e:
            logger.error(f"Failed to read from stream: {e}")
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

    def ack_message(self, message_id: str):
        """Acknowledge a message from the stream.

        Args:
            message_id: Message ID to acknowledge
        """
        try:
            self.client.xack(self.stream, self.consumer_group, message_id)
            logger.info(f"ACKed message: {message_id}")
        except Exception as e:
            logger.error(f"Failed to ACK message {message_id}: {e}")
            raise

    def close(self):
        """Close Redis connection."""
        if self.client:
            self.client.close()
            logger.info("Redis connection closed")
