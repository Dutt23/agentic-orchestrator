import { Box, Button, VStack, Icon } from '@chakra-ui/react';
import { FiArrowLeft } from 'react-icons/fi';
import NodeEditor from './editors/NodeEditorFactory';

export default function SelectedNodeMessage({ node, onBack, onNodeUpdate }) {
  if (!node?.data) return null;

  return (
    <VStack spacing={4} p={4} align="stretch">
      <Button 
        leftIcon={<Icon as={FiArrowLeft} />} 
        variant="ghost" 
        onClick={onBack}
        alignSelf="flex-start"
        colorScheme="blue"
        size="sm"
        mb={2}
      >
        Back to Nodes
      </Button>
      
      <Box
        border="1px solid"
        borderColor="blue.200"
        borderRadius="md"
        p={4}
        bg="blue.50"
      >
    <NodeEditor 
      node={node} 
      onUpdate={onNodeUpdate} 
    />
      </Box>
    </VStack>
  );
}
