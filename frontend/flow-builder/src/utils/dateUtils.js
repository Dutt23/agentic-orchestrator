/**
 * Format ISO timestamp for display
 */
export function formatTimestamp(isoString) {
  if (!isoString) return 'N/A';
  try {
    return new Date(isoString).toLocaleString();
  } catch {
    return 'Invalid date';
  }
}

/**
 * Format date string for display
 */
export function formatDate(dateString) {
  if (!dateString) return 'N/A';
  return new Date(dateString).toLocaleString();
}
