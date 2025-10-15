/**
 * Status configuration for consistent styling across the app
 * Easy to extend with new statuses
 */

export const STATUS_CONFIG = {
  completed: {
    colorScheme: 'green',
    label: 'Completed',
  },
  failed: {
    colorScheme: 'red',
    label: 'Failed',
  },
  error: {
    colorScheme: 'red',
    label: 'Error',
  },
  running: {
    colorScheme: 'blue',
    label: 'Running',
  },
  waiting_for_approval: {
    colorScheme: 'orange',
    label: 'Waiting for Approval',
  },
  pending: {
    colorScheme: 'gray',
    label: 'Pending',
  },
  not_executed: {
    colorScheme: 'gray',
    label: 'Not Executed',
  },
  queued: {
    colorScheme: 'yellow',
    label: 'Queued',
  },
  'completed_with_errors': {
    colorScheme: 'orange',
    label: 'Completed with Errors',
  },
  cancelled: {
    colorScheme: 'purple',
    label: 'Cancelled',
  },
  timeout: {
    colorScheme: 'red',
    label: 'Timeout',
  },
};

/**
 * Get color scheme for a status (case-insensitive)
 * @param {string} status - The status string
 * @returns {string} - Chakra UI color scheme
 */
export function getStatusColorScheme(status) {
  if (!status) return 'gray';

  const normalizedStatus = status.toLowerCase().trim();
  const config = STATUS_CONFIG[normalizedStatus];

  return config ? config.colorScheme : 'gray';
}

/**
 * Get display label for a status (case-insensitive)
 * @param {string} status - The status string
 * @returns {string} - Display label
 */
export function getStatusLabel(status) {
  if (!status) return 'Unknown';

  const normalizedStatus = status.toLowerCase().trim();
  const config = STATUS_CONFIG[normalizedStatus];

  return config ? config.label : status;
}
