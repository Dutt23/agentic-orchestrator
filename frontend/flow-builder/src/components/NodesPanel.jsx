import { Box, VStack, Text, Heading } from '@chakra-ui/react';
import { useState, useEffect } from 'react';
import SelectedNodeMessage from './SelectedNodeMessage';
import NodeItem from './NodeItem';
import nodeTypesData from '../data/nodeTypes.json';

export default function NodesPanel({ selectedNode, onNodeDeselect, onNodeUpdate }) {
  const [nodeTypes, setNodeTypes] = useState([]);

  useEffect(() => {
    // In a real app, you might fetch this from an API
    setNodeTypes(nodeTypesData.nodes);
  }, []);

  const handleDragStart = (event, nodeType) => {
    event.dataTransfer.setData('application/reactflow', JSON.stringify({
      type: nodeType.type,
      data: nodeType.defaultData
    }));
    event.dataTransfer.effectAllowed = 'move';
  };

  if (selectedNode) {
    return (
      <SelectedNodeMessage 
        node={selectedNode} 
        onBack={onNodeDeselect}
        onNodeUpdate={onNodeUpdate}
      />
    );
  }

  return (
    <Box p={4} overflowY="auto" height="100%">
      <Heading size="md" mb={4} color="gray.700">
        Nodes Panel
      </Heading>
      <Text fontSize="sm" color="gray.500" mb={4}>
        Drag and drop nodes onto the canvas
      </Text>
      
      <VStack spacing={3} align="stretch">
        {nodeTypes.map((nodeType) => (
          <NodeItem 
            key={nodeType.type}
            nodeType={nodeType}
            onDragStart={handleDragStart}
          />
        ))}
      </VStack>
    </Box>
  );
}
