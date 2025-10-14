"""Workflow patch client - forwards patches to orchestrator API."""
import requests
from typing import Dict, Any
import logging

logger = logging.getLogger(__name__)


class PatchClient:
    """Client for forwarding workflow patches to orchestrator."""

    def __init__(self, orchestrator_url: str):
        """Initialize patch client.

        Args:
            orchestrator_url: Base URL of orchestrator API
        """
        self.orchestrator_url = orchestrator_url.rstrip('/')
        logger.info(f"Patch client initialized for {self.orchestrator_url}")

    def apply_patch(self, workflow_tag: str, workflow_owner: str, patch_spec: Dict[str, Any]) -> Dict[str, Any]:
        """Apply patch to workflow via orchestrator API.

        Args:
            workflow_tag: Tag of workflow to patch
            workflow_owner: Owner of the workflow
            patch_spec: Patch specification with operations and description

        Returns:
            Response from orchestrator with patch_id
        """
        logger.info(f"Applying patch to workflow {workflow_owner}:{workflow_tag}")

        # Build API request - correct URL format with owner in header
        url = f"{self.orchestrator_url}/workflows/{workflow_tag}/patch"
        headers = {
            "X-User-ID": workflow_owner,
            "Content-Type": "application/json"
        }
        payload = {
            "operations": patch_spec.get('operations', []),
            "description": patch_spec.get('description', 'Agent-generated patch')
        }

        logger.info(f"Sending PATCH request to {url} with owner={workflow_owner}")

        try:
            response = requests.patch(url, json=payload, headers=headers, timeout=30)
            response.raise_for_status()

            result = response.json()
            logger.info(f"Patch applied successfully: artifact_id={result.get('artifact_id')}")

            return {
                "status": "success",
                "artifact_id": result.get('artifact_id'),
                "patch_id": result.get('artifact_id'),  # For backward compatibility
                "cas_id": result.get('cas_id'),
                "depth": result.get('depth'),
                "op_count": result.get('op_count'),
                "message": f"Patch applied to {workflow_owner}:{workflow_tag}"
            }

        except requests.exceptions.RequestException as e:
            logger.error(f"Failed to apply patch: {e}")
            if hasattr(e, 'response') and e.response is not None:
                logger.error(f"Response status: {e.response.status_code}")
                logger.error(f"Response body: {e.response.text}")
            raise ValueError(f"Patch request failed: {e}")

    def apply_run_patch(self, run_id: str, workflow_owner: str, patch_spec: Dict[str, Any], node_id: str = None) -> Dict[str, Any]:
        """Apply patch to a specific workflow run.

        Args:
            run_id: Workflow run ID
            workflow_owner: Owner of the workflow
            patch_spec: Patch specification with operations and description
            node_id: ID of the node generating this patch (for tracking)

        Returns:
            Response from orchestrator with patch details
        """
        logger.info(f"Applying run patch: run_id={run_id}, owner={workflow_owner}, node_id={node_id}")

        # Build API request for run-specific patches
        url = f"{self.orchestrator_url}/api/v1/runs/{run_id}/patches"
        headers = {
            "X-User-ID": workflow_owner,
            "Content-Type": "application/json"
        }
        payload = {
            "operations": patch_spec.get('operations', []),
            "description": patch_spec.get('description', 'Agent-generated patch during run')
        }

        # Add node_id if provided
        if node_id:
            payload["node_id"] = node_id

        logger.info(f"Sending POST request to {url} with owner={workflow_owner}")

        try:
            response = requests.post(url, json=payload, headers=headers, timeout=30)
            response.raise_for_status()

            result = response.json()
            logger.info(f"Run patch applied successfully: id={result.get('id')}, seq={result.get('seq')}")

            return {
                "status": "success",
                "id": result.get('id'),
                "run_id": result.get('run_id'),
                "artifact_id": result.get('artifact_id'),
                "cas_id": result.get('cas_id'),
                "seq": result.get('seq'),
                "op_count": result.get('op_count'),
                "message": f"Run patch applied to run {run_id} (seq: {result.get('seq')})"
            }

        except requests.exceptions.RequestException as e:
            logger.error(f"Failed to apply run patch: {e}")
            if hasattr(e, 'response') and e.response is not None:
                logger.error(f"Response status: {e.response.status_code}")
                logger.error(f"Response body: {e.response.text}")
            raise ValueError(f"Run patch request failed: {e}")


def patch_workflow_tool(args: Dict[str, Any], orchestrator_url: str, run_id: str = None, node_id: str = None) -> Dict[str, Any]:
    """Execute patch_workflow tool.

    Args:
        args: Tool arguments with patch_spec and optionally run_id
        orchestrator_url: Orchestrator API URL
        run_id: Workflow run ID (for run-specific patches)
        node_id: Node ID (which node is generating this patch)

    Returns:
        Result dictionary with patch_id and status
    """
    patch_spec = args.get('patch_spec')
    workflow_owner = args.get('workflow_owner')

    # Get run_id from args or parameter
    if not run_id:
        run_id = args.get('run_id')

    # Get node_id from args or parameter
    if not node_id:
        node_id = args.get('node_id')

    if not patch_spec:
        raise ValueError("patch_workflow requires 'patch_spec'")

    if not workflow_owner:
        raise ValueError("patch_workflow requires 'workflow_owner'")

    if not run_id:
        raise ValueError("patch_workflow requires 'run_id' for run-specific patches")

    client = PatchClient(orchestrator_url)
    return client.apply_run_patch(run_id, workflow_owner, patch_spec, node_id=node_id)
