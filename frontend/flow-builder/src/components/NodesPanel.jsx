import { Box, VStack, Text, Heading, Tabs, TabList, TabPanels, Tab, TabPanel } from '@chakra-ui/react';
import { useState, useEffect } from 'react';
import SelectedNodeMessage from './SelectedNodeMessage';
import NodeItem from './NodeItem';
import CodeView from './CodeView';
import VersionDiff from './VersionDiff';
import nodeTypesData from '../data/nodeTypes.json';

export default function NodesPanel({
  selectedNode,
  onNodeDeselect,
  onNodeUpdate,
  currentWorkflow,
  workflowVersions = [],
  selectedVersionIndex = 0,
  onVersionChange
}) {
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
    <Box overflowY="auto" height="100%">
      <Tabs size="sm" variant="enclosed" colorScheme="blue">
        <TabList px={4} pt={2}>
          <Tab fontSize="xs">Nodes</Tab>
          <Tab fontSize="xs">Code</Tab>
          <Tab fontSize="xs">Versions</Tab>
        </TabList>

        <TabPanels>
          {/* Nodes Tab */}
          <TabPanel>
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
          </TabPanel>

          {/* Code View Tab */}
          <TabPanel>
            <CodeView
              workflow={currentWorkflow}
              selectedNode={selectedNode}
            />
          </TabPanel>

          {/* Version Diff Tab */}
          <TabPanel>
            <VersionDiff
              versions={workflowVersions}
              selectedVersionIndex={selectedVersionIndex}
              onVersionChange={onVersionChange}
            />
          </TabPanel>
        </TabPanels>
      </Tabs>
    </Box>
  );
}
