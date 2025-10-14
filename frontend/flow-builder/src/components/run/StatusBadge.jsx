import { Badge } from '@chakra-ui/react';
import { getStatusColorScheme, getStatusLabel } from '../../utils/statusConfig';

/**
 * StatusBadge displays a color-coded badge for workflow/node status
 */
export default function StatusBadge({ status, size = 'md', showLabel = false }) {
  const colorScheme = getStatusColorScheme(status);
  const displayText = showLabel ? getStatusLabel(status) : status;

  return (
    <Badge colorScheme={colorScheme} fontSize={size === 'sm' ? 'xs' : 'md'}>
      {displayText}
    </Badge>
  );
}
