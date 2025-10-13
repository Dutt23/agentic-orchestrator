import { Box, Flex, Text, Select, HStack, Divider } from '@chakra-ui/react';
import SaveButton from '../buttons/SaveButton';

export default function Header({
  onSave,
  workflows = [],
  selectedWorkflowId,
  onWorkflowChange,
  branches = [],
  selectedBranch,
  onBranchChange,
  workflowName
}) {
  return (
    <Box
      width="100%"
      bg="gray.50"
      borderBottom="1px solid"
      borderColor="gray.200"
      px={4}
      h="40px"
      position="fixed"
      top={0}
      left={0}
      right={0}
      zIndex={1000}
      boxShadow="sm"
    >
      <Flex
        justify="space-between"
        align="center"
        maxW="container.xl"
        mx="auto"
        h="100%"
      >
        <HStack spacing={3} h="100%">
          <Text fontSize="sm" fontWeight="medium" color="gray.700">
            Flow Builder
          </Text>

          <Divider orientation="vertical" h="20px" />

          {/* Workflow Selector */}
          <Select
            size="sm"
            value={selectedWorkflowId}
            onChange={(e) => onWorkflowChange(e.target.value)}
            maxW="200px"
            bg="white"
            fontSize="sm"
            borderRadius="md"
          >
            <option value="">Select Workflow</option>
            {workflows.map(wf => (
              <option key={wf.id} value={wf.id}>
                {wf.name}
              </option>
            ))}
          </Select>

          {/* Branch Switcher */}
          {selectedWorkflowId && branches.length > 0 && (
            <Select
              size="sm"
              value={selectedBranch}
              onChange={(e) => onBranchChange(e.target.value)}
              maxW="120px"
              bg="white"
              fontSize="sm"
              borderRadius="md"
              fontWeight="medium"
            >
              {branches.map(branch => (
                <option key={branch.tag} value={branch.tag}>
                  {branch.tag} ({branch.versionsCount})
                </option>
              ))}
            </Select>
          )}
        </HStack>

        <Box display="flex" alignItems="center" height="100%">
          <SaveButton size="sm" onClick={onSave} />
        </Box>
      </Flex>
    </Box>
  );
}
