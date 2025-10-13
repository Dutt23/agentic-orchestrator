import { Box, Text, VStack } from '@chakra-ui/react';

export default function DefaultNodeEditor({ node }) {
  return (
    <VStack spacing={3} align="stretch">
      <Text fontSize="md" fontWeight="bold" mb={2}>
        {node.data.label || 'Node Details'}
      </Text>
      
      <Box>
        <Text fontSize="xs" color="gray.600" mb={1}>Node Type:</Text>
        <Text fontSize="sm" fontWeight="medium">{node.data.type || 'Unknown'}</Text>
      </Box>
      
      <Box>
        <Text fontSize="xs" color="gray.600" mb={1}>Node ID:</Text>
        <Text fontSize="xs" fontFamily="mono" color="gray.700">{node.id}</Text>
      </Box>
      
      {node.data.text && (
        <Box>
          <Text fontSize="xs" color="gray.600" mb={1}>Content:</Text>
          <Text fontSize="sm" whiteSpace="pre-wrap">{node.data.text}</Text>
        </Box>
      )}
    </VStack>
  );
}
