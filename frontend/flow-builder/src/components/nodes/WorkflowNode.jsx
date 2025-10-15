import { Handle, Position } from '@xyflow/react';
import { Box, HStack, Text, Badge } from '@chakra-ui/react';
import { getIconComponent } from '../../hooks/useNodeRegistry';

// Fallback config for when registry isn't loaded or node type not found
const fallbackConfig = {
  color: 'blue',
  label: 'Unknown',
  icon: 'FiCode'
};

// Get status badge color
const getStatusBadgeColor = (status) => {
  const colors = {
    active: 'green',
    coming_soon: 'yellow',
    experimental: 'orange',
    deprecated: 'red'
  };
  return colors[status] || 'gray';
};

export default function WorkflowNode({ data, selected, nodeRegistry }) {
  const nodeType = data?.type || 'function';

  // Get node definition from registry (passed as prop from parent)
  const registryNode = nodeRegistry?.nodes?.[nodeType];

  // Build config from registry or fallback
  const config = registryNode ? {
    color: registryNode.color,
    bgColor: `${registryNode.color}.50`,
    borderColor: `${registryNode.color}.400`,
    label: registryNode.label,
    icon: getIconComponent(registryNode.icon),
    status: registryNode.status,
    description: registryNode.description
  } : {
    ...fallbackConfig,
    bgColor: 'blue.50',
    borderColor: 'blue.400',
    icon: getIconComponent(fallbackConfig.icon),
    label: nodeType
  };

  const Icon = config.icon;
  const nodeName = data?.config?.name || data?.id || 'Unnamed Node';

  return (
    <Box
      minW="200px"
      maxW="280px"
      bg="white"
      borderRadius="lg"
      border="2px solid"
      borderColor={selected ? config.borderColor : 'gray.200'}
      boxShadow={selected ? 'lg' : 'md'}
      transition="all 0.2s"
      _hover={{
        boxShadow: 'lg',
        borderColor: config.borderColor
      }}
    >
      {/* Target Handle */}
      <Handle
        type="target"
        position={Position.Left}
        style={{
          background: `var(--chakra-colors-${config.color}-500)`,
          width: '12px',
          height: '12px',
          border: '2px solid white',
          left: '-6px'
        }}
      />

      {/* Node Header */}
      <Box bg={config.bgColor} px={3} py={2} borderTopRadius="lg">
        <HStack spacing={2} justify="space-between">
          <HStack spacing={2}>
            <Icon color={`var(--chakra-colors-${config.color}-600)`} size={14} />
            <Badge
              colorScheme={config.color}
              fontSize="xs"
              textTransform="uppercase"
            >
              {config.label}
            </Badge>
          </HStack>

          {/* Status Badge - show if not active */}
          {config.status && config.status !== 'active' && (
            <Badge
              colorScheme={getStatusBadgeColor(config.status)}
              fontSize="10px"
              variant="solid"
            >
              {config.status === 'coming_soon' ? 'Soon' : config.status}
            </Badge>
          )}
        </HStack>
      </Box>

      {/* Node Content */}
      <Box px={3} py={2}>
        <Text fontSize="sm" fontWeight="medium" color="gray.800" noOfLines={2}>
          {nodeName}
        </Text>

        {/* Optional config preview */}
        {data?.config?.url && (
          <Text fontSize="xs" color="gray.500" mt={1} noOfLines={1}>
            {data.config.url}
          </Text>
        )}
        {data?.config?.condition && (
          <Text fontSize="xs" color="gray.500" mt={1} noOfLines={1}>
            {data.config.condition}
          </Text>
        )}
        {data?.config?.handler && (
          <Text fontSize="xs" color="gray.500" mt={1} noOfLines={1}>
            {data.config.handler}
          </Text>
        )}
      </Box>

      {/* Source Handle(s) */}
      {data?.outputs && data.outputs.length > 1 ? (
        // Multiple outputs (HITL, If/Else, etc.)
        <>
          {data.outputs.map((output, index) => {
            const totalOutputs = data.outputs.length;
            const offsetPercent = ((index + 1) / (totalOutputs + 1)) * 100;

            return (
              <Handle
                key={output}
                type="source"
                position={Position.Right}
                id={output}
                style={{
                  background: `var(--chakra-colors-${config.color}-500)`,
                  width: '10px',
                  height: '10px',
                  border: '2px solid white',
                  right: '-5px',
                  top: `${offsetPercent}%`,
                  transform: 'translateY(-50%)'
                }}
              />
            );
          })}
          {/* Labels for outputs */}
          <Box position="absolute" right="-60px" top="0" bottom="0" display="flex" flexDirection="column" justifyContent="space-around" py={2}>
            {data.outputs.map((output) => (
              <Text key={output} fontSize="9px" color="gray.500" fontWeight="medium" textTransform="capitalize">
                {output}
              </Text>
            ))}
          </Box>
        </>
      ) : (
        // Single output (default)
        <Handle
          type="source"
          position={Position.Right}
          style={{
            background: `var(--chakra-colors-${config.color}-500)`,
            width: '12px',
            height: '12px',
            border: '2px solid white',
            right: '-6px'
          }}
        />
      )}
    </Box>
  );
}
