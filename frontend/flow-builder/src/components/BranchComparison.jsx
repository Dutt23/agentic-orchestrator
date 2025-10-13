import {
  Box,
  VStack,
  HStack,
  Text,
  Select,
  Button,
  Badge,
  Divider,
  Alert,
  AlertIcon,
  AlertTitle,
  AlertDescription,
  ButtonGroup,
} from '@chakra-ui/react';
import { useState, useEffect } from 'react';
import { FiGitBranch, FiX, FiColumns, FiLayers } from 'react-icons/fi';
import { computeWorkflowDiff, getDiffSummaryText } from '../utils/workflowDiff';

export default function BranchComparison({
  branches = [],
  workflowId,
  onCompare,
  onClose,
  getWorkflowForBranch
}) {
  const [branchA, setBranchA] = useState('');
  const [branchB, setBranchB] = useState('');
  const [comparing, setComparing] = useState(false);
  const [diff, setDiff] = useState(null);
  const [viewMode, setViewMode] = useState('side-by-side'); // 'side-by-side' or 'overlay'

  // Auto-select first two branches if available
  useEffect(() => {
    if (branches.length >= 2 && !branchA && !branchB) {
      setBranchA(branches[0].tag);
      setBranchB(branches[1].tag);
    }
  }, [branches, branchA, branchB]);

  const handleCompare = async () => {
    if (!branchA || !branchB) {
      return;
    }

    setComparing(true);

    try {
      // Fetch both workflows
      const workflowA = await getWorkflowForBranch(workflowId, branchA);
      const workflowB = await getWorkflowForBranch(workflowId, branchB);

      // Compute diff
      const diffResult = computeWorkflowDiff(workflowA, workflowB);
      setDiff(diffResult);

      // Notify parent component
      if (onCompare) {
        onCompare({
          branchA,
          branchB,
          workflowA,
          workflowB,
          diff: diffResult,
          viewMode // Pass view mode to parent
        });
      }
    } catch (error) {
      console.error('Failed to compare branches:', error);
    } finally {
      setComparing(false);
    }
  };

  const handleSwap = () => {
    const temp = branchA;
    setBranchA(branchB);
    setBranchB(temp);

    // If already compared, recompute with swapped branches
    if (diff) {
      handleCompare();
    }
  };

  const canCompare = branchA && branchB && branchA !== branchB;

  return (
    <Box w="100%">
      <VStack align="stretch" spacing={4}>
        {/* Header */}
        <HStack justify="space-between">
          <HStack>
            <FiGitBranch size={20} />
            <Text fontSize="lg" fontWeight="semibold">
              Compare Branches
            </Text>
          </HStack>
          {onClose && (
            <Button
              size="sm"
              variant="ghost"
              onClick={onClose}
              leftIcon={<FiX />}
            >
              Close
            </Button>
          )}
        </HStack>

        <Divider />

        {/* Branch Selectors */}
        <VStack align="stretch" spacing={3}>
          <Box>
            <Text fontSize="sm" fontWeight="medium" mb={2}>
              Base Branch
            </Text>
            <Select
              value={branchA}
              onChange={(e) => setBranchA(e.target.value)}
              placeholder="Select base branch"
              size="sm"
            >
              {branches.map((branch) => (
                <option key={branch.tag} value={branch.tag}>
                  {branch.tag} ({branch.versionsCount} versions)
                </option>
              ))}
            </Select>
          </Box>

          <HStack justify="center">
            <Button
              size="xs"
              variant="outline"
              onClick={handleSwap}
              isDisabled={!branchA || !branchB}
            >
              ⇅ Swap
            </Button>
          </HStack>

          <Box>
            <Text fontSize="sm" fontWeight="medium" mb={2}>
              Compare Branch
            </Text>
            <Select
              value={branchB}
              onChange={(e) => setBranchB(e.target.value)}
              placeholder="Select compare branch"
              size="sm"
            >
              {branches.map((branch) => (
                <option key={branch.tag} value={branch.tag}>
                  {branch.tag} ({branch.versionsCount} versions)
                </option>
              ))}
            </Select>
          </Box>
        </VStack>

        {/* View Mode Selector */}
        <Box>
          <Text fontSize="sm" fontWeight="medium" mb={2}>
            View Mode
          </Text>
          <ButtonGroup size="sm" isAttached variant="outline" w="100%">
            <Button
              flex="1"
              leftIcon={<FiColumns />}
              onClick={() => setViewMode('side-by-side')}
              colorScheme={viewMode === 'side-by-side' ? 'blue' : 'gray'}
              variant={viewMode === 'side-by-side' ? 'solid' : 'outline'}
            >
              Side-by-Side
            </Button>
            <Button
              flex="1"
              leftIcon={<FiLayers />}
              onClick={() => setViewMode('overlay')}
              colorScheme={viewMode === 'overlay' ? 'blue' : 'gray'}
              variant={viewMode === 'overlay' ? 'solid' : 'outline'}
            >
              Overlay
            </Button>
          </ButtonGroup>
          <Text fontSize="xs" color="gray.500" mt={1}>
            {viewMode === 'overlay'
              ? 'Compare branch shown with dashed lines, semi-transparent'
              : 'View branches side-by-side for comparison'}
          </Text>
        </Box>

        {/* Compare Button */}
        <Button
          colorScheme="blue"
          onClick={handleCompare}
          isLoading={comparing}
          isDisabled={!canCompare}
          size="md"
        >
          Compare Branches
        </Button>

        {/* Validation Messages */}
        {branchA && branchB && branchA === branchB && (
          <Alert status="warning" size="sm">
            <AlertIcon />
            <AlertDescription fontSize="sm">
              Please select two different branches to compare
            </AlertDescription>
          </Alert>
        )}

        {branches.length < 2 && (
          <Alert status="info" size="sm">
            <AlertIcon />
            <AlertDescription fontSize="sm">
              At least 2 branches are required for comparison
            </AlertDescription>
          </Alert>
        )}

        {/* Diff Summary */}
        {diff && (
          <>
            <Divider />

            <Box>
              <Text fontSize="sm" fontWeight="medium" mb={3}>
                Comparison Summary
              </Text>

              <VStack align="stretch" spacing={2}>
                {/* Branch Names */}
                <HStack spacing={2} fontSize="sm">
                  <Badge colorScheme="blue" variant="subtle">
                    {branchA}
                  </Badge>
                  <Text color="gray.500">vs</Text>
                  <Badge colorScheme="purple" variant="subtle">
                    {branchB}
                  </Badge>
                </HStack>

                {/* Summary Badges */}
                <HStack spacing={2} flexWrap="wrap">
                  {diff.summary.nodes.added > 0 && (
                    <Badge colorScheme="green" fontSize="xs">
                      +{diff.summary.nodes.added} Node{diff.summary.nodes.added > 1 ? 's' : ''}
                    </Badge>
                  )}
                  {diff.summary.nodes.removed > 0 && (
                    <Badge colorScheme="red" fontSize="xs">
                      -{diff.summary.nodes.removed} Node{diff.summary.nodes.removed > 1 ? 's' : ''}
                    </Badge>
                  )}
                  {diff.summary.nodes.modified > 0 && (
                    <Badge colorScheme="yellow" fontSize="xs">
                      ~{diff.summary.nodes.modified} Modified
                    </Badge>
                  )}
                  {diff.summary.edges.added > 0 && (
                    <Badge colorScheme="green" fontSize="xs" variant="outline">
                      +{diff.summary.edges.added} Edge{diff.summary.edges.added > 1 ? 's' : ''}
                    </Badge>
                  )}
                  {diff.summary.edges.removed > 0 && (
                    <Badge colorScheme="red" fontSize="xs" variant="outline">
                      -{diff.summary.edges.removed} Edge{diff.summary.edges.removed > 1 ? 's' : ''}
                    </Badge>
                  )}
                </HStack>

                {/* Text Summary */}
                <Text fontSize="sm" color="gray.600">
                  {getDiffSummaryText(diff)}
                </Text>

                {/* No Differences Message */}
                {diff.summary.nodes.added === 0 &&
                  diff.summary.nodes.removed === 0 &&
                  diff.summary.nodes.modified === 0 &&
                  diff.summary.edges.added === 0 &&
                  diff.summary.edges.removed === 0 && (
                    <Alert status="success" size="sm">
                      <AlertIcon />
                      <AlertTitle fontSize="sm">No differences found!</AlertTitle>
                      <AlertDescription fontSize="xs">
                        The two branches are identical.
                      </AlertDescription>
                    </Alert>
                  )}
              </VStack>
            </Box>
          </>
        )}
      </VStack>
    </Box>
  );
}
