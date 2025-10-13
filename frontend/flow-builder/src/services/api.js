/**
 * API Service Layer
 * Handles all HTTP requests to the orchestrator backend
 */

// Get API base URL from environment variable or default to localhost
const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8081/api/v1';

/**
 * Get username from environment variable or default
 * For development, defaults to 'sdutt'
 */
function getUsername() {
  return import.meta.env.VITE_DEV_USERNAME || 'sdutt';
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
 * @param {string} tag - Workflow tag name (e.g., 'main')
 * @param {boolean} materialize - Whether to materialize the workflow (default: true)
 * @returns {Promise<Object>} Workflow details
 */
export async function getWorkflow(tag, materialize = true) {
  return await apiRequest(`/workflows/${tag}?materialize=${materialize}`);
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
 * Delete a workflow tag
 * @param {string} tag - Workflow tag name to delete
 * @returns {Promise<Object>} Deletion confirmation
 */
export async function deleteWorkflow(tag) {
  return await apiRequest(`/workflows/${tag}`, {
    method: 'DELETE',
  });
}

export default {
  listWorkflows,
  getWorkflow,
  createWorkflow,
  deleteWorkflow,
};
