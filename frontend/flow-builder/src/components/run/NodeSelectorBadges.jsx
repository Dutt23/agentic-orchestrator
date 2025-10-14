import { HStack, Badge } from '@chakra-ui/react';

/**
 * NodeSelectorBadges displays clickable badges for selecting nodes
 */
export default function NodeSelectorBadges({ nodeExecutions, selectedNode, onNodeSelect }) {
  return (
    <HStack spacing={2} flexWrap="wrap">
      {Object.keys(nodeExecutions).map((nodeId) => {
        const exec = nodeExecutions[nodeId];
        const statusLower = exec.status?.toLowerCase() || '';

        return (
          <Badge
            key={nodeId}
            colorScheme={
              statusLower === 'completed'
                ? 'green'
                : statusLower === 'failed'
                ? 'red'
                : statusLower === 'running'
                ? 'blue'
                : 'gray'
            }
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
