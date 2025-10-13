import { Box, Text, VStack, Code, Tabs, TabList, TabPanels, Tab, TabPanel } from '@chakra-ui/react';

export default function CodeView({ workflow, selectedNode }) {
  // Format JSON with indentation
  const formatJSON = (obj) => {
    return JSON.stringify(obj, null, 2);
  };

  return (
    <VStack align="stretch" spacing={4} w="100%">
      <Text fontSize="md" fontWeight="semibold" color="gray.700">
        Code View
      </Text>

      <Tabs size="sm" variant="enclosed">
        <TabList>
          <Tab fontSize="xs">Workflow</Tab>
          {selectedNode && <Tab fontSize="xs">Node</Tab>}
        </TabList>

        <TabPanels>
          {/* Full Workflow Tab */}
          <TabPanel p={0} pt={2}>
            <Box
              maxH="400px"
              overflowY="auto"
              bg="gray.900"
              borderRadius="md"
              p={3}
            >
              <Code
                display="block"
                whiteSpace="pre"
                fontSize="xs"
                color="green.300"
                bg="transparent"
                fontFamily="monospace"
              >
                {workflow ? formatJSON({
                  metadata: workflow.metadata,
                  nodes: workflow.nodes,
                  edges: workflow.edges
                }) : 'No workflow loaded'}
              </Code>
            </Box>
          </TabPanel>

          {/* Selected Node Tab */}
          {selectedNode && (
            <TabPanel p={0} pt={2}>
              <Box
                maxH="400px"
                overflowY="auto"
                bg="gray.900"
                borderRadius="md"
                p={3}
              >
                <Code
                  display="block"
                  whiteSpace="pre"
                  fontSize="xs"
                  color="cyan.300"
                  bg="transparent"
                  fontFamily="monospace"
                >
                  {formatJSON({
                    id: selectedNode.id,
                    type: selectedNode.data?.type || selectedNode.type,
                    config: selectedNode.data?.config || selectedNode.config
                  })}
                </Code>
              </Box>
            </TabPanel>
          )}
        </TabPanels>
      </Tabs>
    </VStack>
  );
}
