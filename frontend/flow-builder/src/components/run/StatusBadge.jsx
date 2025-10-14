import { Badge } from '@chakra-ui/react';

/**
 * StatusBadge displays a color-coded badge for workflow/node status
 */
export default function StatusBadge({ status, size = 'md' }) {
  const statusLower = status?.toLowerCase() || '';

  const colorScheme =
    statusLower === 'completed'
      ? 'green'
      : statusLower === 'failed'
      ? 'red'
      : statusLower === 'running'
      ? 'blue'
      : 'gray';

  return (
    <Badge colorScheme={colorScheme} fontSize={size === 'sm' ? 'xs' : 'md'}>
      {status}
    </Badge>
  );
}
