import { HStack, Badge } from '@chakra-ui/react';
import { getStatusColorScheme } from '../../utils/statusConfig';

/**
 * NodeSelectorBadges displays clickable badges for selecting nodes
 */
export default function NodeSelectorBadges({ nodeExecutions, selectedNode, onNodeSelect }) {
  return (
    <HStack spacing={2} flexWrap="wrap">
      {Object.keys(nodeExecutions).map((nodeId) => {
        const exec = nodeExecutions[nodeId];

        return (
          <Badge
            key={nodeId}
            colorScheme={getStatusColorScheme(exec.status)}
            cursor="pointer"
            p={2}
            variant={nodeId === selectedNode ? 'solid' : 'outline'}
            onClick={() => onNodeSelect(nodeId)}
            _hover={{ opacity: 0.8 }}
          >
            {nodeId}
          </Badge>
        );
      })}
    </HStack>
  );
}
