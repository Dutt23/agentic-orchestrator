/**
 * API Service Layer
 * Handles all HTTP requests to the orchestrator backend
 */

// Get API base URL from environment variable or default to localhost
const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8081/api/v1';

/**
 * Get username from environment variable or default
 * For development, defaults to 'test-user'
 */
function getUsername() {
  return import.meta.env.VITE_DEV_USERNAME || 'test-user';
}

/**
 * Make an authenticated API request
 */
async function apiRequest(endpoint, options = {}) {
  const url = `${API_BASE_URL}${endpoint}`;
  const username = getUsername();

  const headers = {
    'Content-Type': 'application/json',
    'X-User-ID': username, // Backend uses X-User-ID for auth
    ...options.headers,
  };

  const config = {
    ...options,
    headers,
  };

  try {
    const response = await fetch(url, config);

    if (!response.ok) {
      const error = await response.json().catch(() => ({ error: 'Request failed' }));
      throw new Error(error.error || `HTTP ${response.status}`);
    }

    return await response.json();
  } catch (error) {
    console.error('API request failed:', error);
    throw error;
  }
}

/**
 * List workflows for the authenticated user
 * @param {string} scope - 'user', 'global', or 'all' (default: 'user')
 * @returns {Promise<Array>} List of workflows
 */
export async function listWorkflows(scope = 'user') {
  const data = await apiRequest(`/workflows?scope=${scope}`);
  return data.workflows || [];
}

/**
 * Get a specific workflow by tag
 * @param {string} tag - Workflow tag name (e.g., 'main' or 'release/v1.0')
 * @param {boolean} materialize - Whether to materialize the workflow (default: true)
 * @returns {Promise<Object>} Workflow details
 */
export async function getWorkflow(tag, materialize = true) {
  // URL-encode tag to handle slashes (e.g., "release/v1.0" -> "release%2Fv1.0")
  const encodedTag = encodeURIComponent(tag);
  return await apiRequest(`/workflows/${encodedTag}?materialize=${materialize}`);
}

/**
 * Create a new workflow
 * @param {string} tagName - Tag name for the workflow
 * @param {Object} workflow - Workflow definition (nodes, edges, metadata)
 * @returns {Promise<Object>} Created workflow details
 */
export async function createWorkflow(tagName, workflow) {
  return await apiRequest('/workflows', {
    method: 'POST',
    body: JSON.stringify({
      tag_name: tagName,
      workflow: workflow,
    }),
  });
}

/**
 * Get a specific workflow version by tag and sequence number
 * @param {string} tag - Workflow tag name (e.g., 'main' or 'release/v1.0')
 * @param {number} seq - Sequence number (1-indexed)
 * @param {boolean} materialize - Whether to materialize the workflow (default: true)
 * @returns {Promise<Object>} Workflow details at specific version
 */
export async function getWorkflowVersion(tag, seq, materialize = true) {
  // URL-encode tag to handle slashes (e.g., "release/v1.0" -> "release%2Fv1.0")
  const encodedTag = encodeURIComponent(tag);
  return await apiRequest(`/workflows/${encodedTag}/versions/${seq}?materialize=${materialize}`);
}

/**
 * Update a workflow by applying JSON Patch operations
 * @param {string} tag - Workflow tag name
 * @param {Array<Object>} operations - JSON Patch operations (add, remove, replace)
 * @param {string} description - Optional description of the changes
 * @returns {Promise<Object>} Updated workflow details
 */
export async function updateWorkflow(tag, operations, description = '') {
  const encodedTag = encodeURIComponent(tag);
  return await apiRequest(`/workflows/${encodedTag}/patch`, {
    method: 'PATCH',
    body: JSON.stringify({
      operations: operations,
      description: description,
    }),
  });
}

/**
 * Delete a workflow tag
 * @param {string} tag - Workflow tag name to delete
 * @returns {Promise<Object>} Deletion confirmation
 */
export async function deleteWorkflow(tag) {
  const encodedTag = encodeURIComponent(tag);
  return await apiRequest(`/workflows/${encodedTag}`, {
    method: 'DELETE',
  });
}

/**
 * Run a workflow
 * @param {string} tag - Workflow tag name to run
 * @param {Object} inputs - Input parameters for the workflow
 * @returns {Promise<Object>} Run details including run_id
 */
export async function runWorkflow(tag, inputs = {}) {
  const encodedTag = encodeURIComponent(tag);
  return await apiRequest(`/workflows/${encodedTag}/execute`, {
    method: 'POST',
    body: JSON.stringify({
      inputs: inputs,
    }),
  });
}

/**
 * List runs for a workflow tag
 * @param {string} tag - Workflow tag
 * @param {number} limit - Max runs to return (default: 20)
 * @returns {Promise<Array>} List of runs
 */
export async function listWorkflowRuns(tag, limit = 20) {
  const encodedTag = encodeURIComponent(tag);
  const data = await apiRequest(`/workflows/${encodedTag}/runs?limit=${limit}`);
  return data.runs || [];
}

/**
 * Get detailed run information
 * @param {string} runId - Run ID
 * @returns {Promise<Object>} Run details with node executions
 */
export async function getRunDetails(runId) {
  return await apiRequest(`/runs/${runId}/details`);
}

export default {
  listWorkflows,
  getWorkflow,
  getWorkflowVersion,
  createWorkflow,
  updateWorkflow,
  deleteWorkflow,
  runWorkflow,
  listWorkflowRuns,
  getRunDetails,
};
