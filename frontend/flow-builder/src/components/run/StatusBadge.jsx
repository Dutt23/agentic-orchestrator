import { Badge, HStack, Spinner } from '@chakra-ui/react';
import { getStatusColorScheme, getStatusLabel } from '../../utils/statusConfig';

// Statuses that should show a spinner (active/in-progress states)
const ACTIVE_STATUSES = new Set([
  'queued',
  'running',
  'waiting_for_approval'
]);

/**
 * StatusBadge displays a color-coded badge for workflow/node status with spinner for active states
 */
export default function StatusBadge({ status, size = 'md', showLabel = false }) {
  const colorScheme = getStatusColorScheme(status);
  const displayText = showLabel ? getStatusLabel(status) : status;
  const normalizedStatus = status?.toLowerCase().trim();
  const showSpinner = ACTIVE_STATUSES.has(normalizedStatus);

  return (
    <HStack spacing={2}>
      <Badge colorScheme={colorScheme} fontSize={size === 'sm' ? 'xs' : 'md'}>
        {displayText}
      </Badge>
      {showSpinner && (
        <Spinner
          size={size === 'sm' ? 'xs' : 'sm'}
          color={`${colorScheme}.500`}
          thickness="2px"
        />
      )}
    </HStack>
  );
}
