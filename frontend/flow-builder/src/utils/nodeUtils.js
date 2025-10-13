// Helper functions for node styling
export const getHeaderBgColor = (type) => {
  const colors = {
    text: '#d1fae5',
    image: '#dbeafe',
    video: '#fee2e2',
    button: '#f3e8ff',
  };
  return colors[type] || '#f3f4f6';
};

export const getBorderColor = (type) => {
  const colors = {
    text: '#86efac',
    image: '#93c5fd',
    video: '#fca5a5',
    button: '#c4b5fd',
  };
  return colors[type] || '#e5e7eb';
};

export const getTextColor = (type) => {
  const colors = {
    text: '#166534',
    image: '#1e40af',
    video: '#991b1b',
    button: '#5b21b6',
  };
  return colors[type] || '#1f2937';
};

export const getHandleColor = (type) => {
  const colors = {
    text: '#22c55e',
    image: '#3b82f6',
    video: '#ef4444',
    button: '#8b5cf6',
  };
  return colors[type] || '#6b7280';
};
