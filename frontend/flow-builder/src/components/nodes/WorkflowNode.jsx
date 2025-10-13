import { Handle, Position } from '@xyflow/react';
import { Box, HStack, Text, Badge } from '@chakra-ui/react';
import {
  FiCode,
  FiGlobe,
  FiGitBranch,
  FiRepeat,
  FiLayers,
  FiFilter,
  FiZap,
  FiPackage,
  FiSearch,
  FiUser
} from 'react-icons/fi';
import { MdSmartToy, MdHttp } from 'react-icons/md';

// Map orchestrator node types to icons and colors
const nodeTypeConfig = {
  // Core
  agent: {
    icon: MdSmartToy,
    color: 'purple',
    bgColor: 'purple.50',
    borderColor: 'purple.400',
    label: 'Agent'
  },
  // Tools
  http: {
    icon: MdHttp,
    color: 'teal',
    bgColor: 'teal.50',
    borderColor: 'teal.400',
    label: 'HTTP'
  },
  'file-search': {
    icon: FiSearch,
    color: 'blue',
    bgColor: 'blue.50',
    borderColor: 'blue.400',
    label: 'File Search'
  },
  // Data
  transform: {
    icon: FiZap,
    color: 'yellow',
    bgColor: 'yellow.50',
    borderColor: 'yellow.400',
    label: 'Transform'
  },
  aggregate: {
    icon: FiPackage,
    color: 'pink',
    bgColor: 'pink.50',
    borderColor: 'pink.400',
    label: 'Aggregate'
  },
  filter: {
    icon: FiFilter,
    color: 'cyan',
    bgColor: 'cyan.50',
    borderColor: 'cyan.400',
    label: 'Filter'
  },
  // Logic
  conditional: {
    icon: FiGitBranch,
    color: 'orange',
    bgColor: 'orange.50',
    borderColor: 'orange.400',
    label: 'Conditional'
  },
  loop: {
    icon: FiRepeat,
    color: 'red',
    bgColor: 'red.50',
    borderColor: 'red.400',
    label: 'Loop'
  },
  hitl: {
    icon: FiUser,
    color: 'green',
    bgColor: 'green.50',
    borderColor: 'green.400',
    label: 'Human Review'
  },
  // Legacy (for backward compatibility)
  function: {
    icon: FiCode,
    color: 'blue',
    bgColor: 'blue.50',
    borderColor: 'blue.400',
    label: 'Function'
  },
  http: {
    icon: FiGlobe,
    color: 'green',
    bgColor: 'green.50',
    borderColor: 'green.400',
    label: 'HTTP'
  },
  parallel: {
    icon: FiLayers,
    color: 'teal',
    bgColor: 'teal.50',
    borderColor: 'teal.400',
    label: 'Parallel'
  }
};

export default function WorkflowNode({ data, selected }) {
  const nodeType = data?.type || 'function';
  const config = nodeTypeConfig[nodeType] || nodeTypeConfig.function;
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
