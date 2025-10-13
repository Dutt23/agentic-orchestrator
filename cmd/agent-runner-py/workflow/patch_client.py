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

        # Build API request
        url = f"{self.orchestrator_url}/workflows/{workflow_owner}/{workflow_tag}/patch"
        payload = {
            "operations": patch_spec.get('operations', []),
            "description": patch_spec.get('description', 'Agent-generated patch')
        }

        try:
            response = requests.post(url, json=payload, timeout=30)
            response.raise_for_status()

            result = response.json()
            logger.info(f"Patch applied successfully: {result.get('patch_id')}")

            return {
                "status": "success",
                "patch_id": result.get('patch_id'),
                "message": f"Patch applied to {workflow_owner}:{workflow_tag}"
            }

        except requests.exceptions.RequestException as e:
            logger.error(f"Failed to apply patch: {e}")
            raise ValueError(f"Patch request failed: {e}")


def patch_workflow_tool(args: Dict[str, Any], orchestrator_url: str) -> Dict[str, Any]:
    """Execute patch_workflow tool.

    Args:
        args: Tool arguments with workflow_tag, workflow_owner, and patch_spec
        orchestrator_url: Orchestrator API URL

    Returns:
        Result dictionary with patch_id and status
    """
    workflow_tag = args.get('workflow_tag')
    workflow_owner = args.get('workflow_owner')
    patch_spec = args.get('patch_spec')

    if not workflow_tag or not patch_spec:
        raise ValueError("patch_workflow requires 'workflow_tag' and 'patch_spec'")

    if not workflow_owner:
        raise ValueError("patch_workflow requires 'workflow_owner'")

    client = PatchClient(orchestrator_url)
    return client.apply_patch(workflow_tag, workflow_owner, patch_spec)
