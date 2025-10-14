import { useState } from 'react';
import {
  Box,
  VStack,
  HStack,
  Text,
  Heading,
  Badge,
  Divider,
  IconButton,
  Collapse,
  Stat,
  StatLabel,
  StatNumber,
  StatHelpText,
  SimpleGrid,
  Progress,
  Tooltip,
} from '@chakra-ui/react';
import {
  ChevronLeftIcon,
  ChevronRightIcon,
  ChevronDownIcon,
  ChevronUpIcon,
  TimeIcon,
  InfoIcon,
} from '@chakra-ui/icons';

/**
 * Helper function to count absorbed nodes (branch/conditional/loop nodes)
 */
const countAbsorbedNodes = (nodeExecutions) => {
  if (!nodeExecutions) {
    return 0;
  }

  let count = 0;
  Object.values(nodeExecutions).forEach((exec) => {
    // Check metrics to identify absorbed nodes (they have ~1ms execution time and 0 resources)
    if (exec.metrics && exec.metrics.execution_time_ms === 1 && exec.metrics.memory_peak_mb === 0) {
      count++;
    }
  });

  return count;
};

/**
 * MetricsSidebar displays performance metrics for all executed nodes
 */
export default function MetricsSidebar({ nodeExecutions, workflowIR }) {
  const [isOpen, setIsOpen] = useState(true);
  const [selectedNode, setSelectedNode] = useState(null);
  const [expandedSummary, setExpandedSummary] = useState(true);

  if (!nodeExecutions || Object.keys(nodeExecutions).length === 0) {
    return null;
  }

  // Calculate aggregate metrics
  const executedNodes = Object.entries(nodeExecutions).filter(
    ([_, exec]) => exec.status === 'completed' || exec.status === 'failed'
  );

  const completedNodes = executedNodes.filter(
    ([_, exec]) => exec.status === 'completed'
  );

  const failedNodes = executedNodes.filter(
    ([_, exec]) => exec.status === 'failed'
  );

  const totalExecutionTime = executedNodes.reduce((sum, [_, exec]) => {
    return sum + (exec.metrics?.execution_time_ms || 0);
  }, 0);

  const totalQueueTime = executedNodes.reduce((sum, [_, exec]) => {
    return sum + (exec.metrics?.queue_time_ms || 0);
  }, 0);

  const totalDuration = executedNodes.reduce((sum, [_, exec]) => {
    return sum + (exec.metrics?.total_duration_ms || 0);
  }, 0);

  // Filter nodes that have metrics with memory data
  const nodesWithMetrics = executedNodes.filter(
    ([_, exec]) => exec.metrics && exec.metrics.memory_peak_mb !== undefined
  );

  const avgMemoryUsage = nodesWithMetrics.length > 0
    ? nodesWithMetrics.reduce((sum, [_, exec]) => {
        return sum + (exec.metrics?.memory_peak_mb || 0);
      }, 0) / nodesWithMetrics.length
    : 0;

  const maxMemoryUsage = nodesWithMetrics.length > 0
    ? Math.max(...nodesWithMetrics.map(([_, exec]) => exec.metrics?.memory_peak_mb || 0))
    : 0;

  const successRate =
    executedNodes.length > 0
      ? (completedNodes.length / executedNodes.length) * 100
      : 0;

  return (
    <Box
      position="fixed"
      right={isOpen ? 0 : -400}
      top={0}
      h="100vh"
      w="400px"
      bg="white"
      borderLeft="1px solid"
      borderColor="gray.200"
      boxShadow="lg"
      transition="right 0.3s ease"
      zIndex={1000}
      overflowY="auto"
    >
      {/* Toggle Button */}
      <IconButton
        icon={isOpen ? <ChevronRightIcon /> : <ChevronLeftIcon />}
        onClick={() => setIsOpen(!isOpen)}
        position="absolute"
        left="-40px"
        top="50%"
        transform="translateY(-50%)"
        size="sm"
        colorScheme="blue"
        aria-label={isOpen ? 'Close metrics' : 'Open metrics'}
        borderRadius="md 0 0 md"
      />

      {/* Header */}
      <Box p={4} borderBottom="1px solid" borderColor="gray.200" bg="blue.50">
        <HStack justify="space-between">
          <Heading size="md">Metrics</Heading>
          <Badge colorScheme="blue" fontSize="sm">
            {executedNodes.length} executed
          </Badge>
        </HStack>
      </Box>

      {/* Summary Section */}
      <Box p={4} borderBottom="1px solid" borderColor="gray.200" bg="gray.50">
        <HStack
          justify="space-between"
          cursor="pointer"
          onClick={() => setExpandedSummary(!expandedSummary)}
          mb={expandedSummary ? 3 : 0}
        >
          <Heading size="sm">Summary</Heading>
          <IconButton
            icon={expandedSummary ? <ChevronUpIcon /> : <ChevronDownIcon />}
            size="xs"
            variant="ghost"
            aria-label={expandedSummary ? 'Collapse' : 'Expand'}
          />
        </HStack>

        <Collapse in={expandedSummary} animateOpacity>
          <VStack align="stretch" spacing={3}>
            {/* Success Rate */}
            <Box>
              <HStack justify="space-between" mb={1}>
                <Text fontSize="xs" fontWeight="medium" color="gray.600">
                  Success Rate
                </Text>
                <Text fontSize="xs" fontWeight="bold">
                  {successRate.toFixed(1)}%
                </Text>
              </HStack>
              <Progress
                value={successRate}
                colorScheme={successRate === 100 ? 'green' : successRate > 50 ? 'yellow' : 'red'}
                size="sm"
                borderRadius="md"
              />
            </Box>

            {/* Node Status Breakdown */}
            <SimpleGrid columns={3} spacing={2}>
              <Stat size="sm" bg="white" p={2} borderRadius="md" boxShadow="sm">
                <StatLabel fontSize="10px">Completed</StatLabel>
                <StatNumber fontSize="lg" color="green.500">
                  {completedNodes.length}
                </StatNumber>
              </Stat>
              <Stat size="sm" bg="white" p={2} borderRadius="md" boxShadow="sm">
                <StatLabel fontSize="10px">Failed</StatLabel>
                <StatNumber fontSize="lg" color="red.500">
                  {failedNodes.length}
                </StatNumber>
              </Stat>
              <Stat size="sm" bg="white" p={2} borderRadius="md" boxShadow="sm">
                <StatLabel fontSize="10px">Total</StatLabel>
                <StatNumber fontSize="lg" color="blue.500">
                  {executedNodes.length}
                </StatNumber>
              </Stat>
            </SimpleGrid>

            {/* Timing Metrics */}
            <VStack align="stretch" spacing={2} bg="white" p={3} borderRadius="md" boxShadow="sm">
              <HStack justify="space-between">
                <HStack spacing={1}>
                  <TimeIcon boxSize={3} color="gray.500" />
                  <Text fontSize="xs" color="gray.600">
                    Total Duration
                  </Text>
                </HStack>
                <Text fontSize="sm" fontWeight="bold">
                  {formatDuration(totalDuration)}
                </Text>
              </HStack>
              <HStack justify="space-between">
                <Text fontSize="xs" color="gray.600" pl={4}>
                  Execution Time
                </Text>
                <Text fontSize="sm" fontWeight="medium">
                  {formatDuration(totalExecutionTime)}
                </Text>
              </HStack>
              <HStack justify="space-between">
                <Text fontSize="xs" color="gray.600" pl={4}>
                  Queue Time
                </Text>
                <Text fontSize="sm" fontWeight="medium">
                  {formatDuration(totalQueueTime)}
                </Text>
              </HStack>
            </VStack>

            {/* Memory Metrics */}
            <VStack align="stretch" spacing={2} bg="white" p={3} borderRadius="md" boxShadow="sm">
              <Text fontSize="xs" fontWeight="bold" color="gray.700">
                Memory Usage
              </Text>
              <HStack justify="space-between">
                <Text fontSize="xs" color="gray.600">
                  Peak
                </Text>
                <Text fontSize="sm" fontWeight="bold">
                  {maxMemoryUsage.toFixed(1)} MB
                </Text>
              </HStack>
              <HStack justify="space-between">
                <Text fontSize="xs" color="gray.600">
                  Average
                </Text>
                <Text fontSize="sm" fontWeight="medium">
                  {avgMemoryUsage.toFixed(1)} MB
                </Text>
              </HStack>
            </VStack>

            {/* Optimizations Applied */}
            <VStack align="stretch" spacing={2} bg="green.50" p={3} borderRadius="md" boxShadow="sm" border="1px solid" borderColor="green.200">
              <HStack justify="space-between">
                <Text fontSize="xs" fontWeight="bold" color="green.700">
                  Optimizations Applied
                </Text>
                <Tooltip
                  label="Branch/conditional/loop nodes handled inline without worker overhead"
                  placement="left"
                  hasArrow
                >
                  <InfoIcon boxSize={3} color="green.500" />
                </Tooltip>
              </HStack>
              <HStack justify="space-between">
                <Text fontSize="xs" color="green.600">
                  Nodes Absorbed
                </Text>
                <Text fontSize="sm" fontWeight="bold" color="green.700">
                  {countAbsorbedNodes(nodeExecutions)}
                </Text>
              </HStack>
            </VStack>
          </VStack>
        </Collapse>
      </Box>

      {/* Node List */}
      <Box p={4}>
        <Heading size="sm" mb={3}>
          Node Performance
        </Heading>
        <VStack align="stretch" spacing={2}>
          {executedNodes.map(([nodeId, execution]) => (
            <NodeMetricCard
              key={nodeId}
              nodeId={nodeId}
              execution={execution}
              isSelected={selectedNode === nodeId}
              onClick={() => setSelectedNode(selectedNode === nodeId ? null : nodeId)}
            />
          ))}
        </VStack>
      </Box>
    </Box>
  );
}

/**
 * NodeMetricCard displays metrics for a single node
 */
function NodeMetricCard({ nodeId, execution, isSelected, onClick }) {
  const metrics = execution.metrics;

  if (!metrics) {
    return (
      <Box
        p={3}
        bg="gray.50"
        borderRadius="md"
        border="1px solid"
        borderColor="gray.200"
        cursor="pointer"
        onClick={onClick}
      >
        <HStack justify="space-between">
          <Text fontSize="sm" fontWeight="medium" isTruncated>
            {nodeId}
          </Text>
          <Badge colorScheme={execution.status === 'completed' ? 'green' : 'red'} fontSize="xs">
            {execution.status}
          </Badge>
        </HStack>
        <Text fontSize="xs" color="gray.500" mt={1}>
          No metrics available
        </Text>
      </Box>
    );
  }

  const executionTime = metrics.execution_time_ms || 0;
  const queueTime = metrics.queue_time_ms || 0;
  const totalTime = metrics.total_duration_ms || 0;
  const memoryPeak = metrics.memory_peak_mb || 0;
  const cpuPercent = metrics.cpu_percent || 0;

  return (
    <Box
      p={3}
      bg={isSelected ? 'blue.50' : 'white'}
      borderRadius="md"
      border="1px solid"
      borderColor={isSelected ? 'blue.300' : 'gray.200'}
      cursor="pointer"
      onClick={onClick}
      transition="all 0.2s"
      _hover={{ bg: isSelected ? 'blue.50' : 'gray.50', borderColor: 'blue.200' }}
    >
      {/* Node Header */}
      <HStack justify="space-between" mb={2}>
        <Tooltip label={nodeId} placement="top" hasArrow>
          <Text fontSize="sm" fontWeight="medium" isTruncated maxW="250px">
            {nodeId}
          </Text>
        </Tooltip>
        <Badge colorScheme={execution.status === 'completed' ? 'green' : 'red'} fontSize="xs">
          {execution.status}
        </Badge>
      </HStack>

      {/* Quick Metrics */}
      <SimpleGrid columns={2} spacing={2} mb={2}>
        <Box>
          <Text fontSize="10px" color="gray.500">
            Execution
          </Text>
          <Text fontSize="xs" fontWeight="bold">
            {formatDuration(executionTime)}
          </Text>
        </Box>
        <Box>
          <Text fontSize="10px" color="gray.500">
            Queue
          </Text>
          <Text fontSize="xs" fontWeight="bold">
            {formatDuration(queueTime)}
          </Text>
        </Box>
      </SimpleGrid>

      {/* Progress Bar for Timing Breakdown */}
      <Box mb={2}>
        <HStack spacing={0} h="4px" borderRadius="sm" overflow="hidden">
          <Box
            w={`${(queueTime / totalTime) * 100}%`}
            h="100%"
            bg="orange.400"
            title={`Queue: ${formatDuration(queueTime)}`}
          />
          <Box
            w={`${(executionTime / totalTime) * 100}%`}
            h="100%"
            bg="blue.400"
            title={`Execution: ${formatDuration(executionTime)}`}
          />
        </HStack>
        <HStack justify="space-between" mt={1}>
          <Text fontSize="9px" color="gray.400">
            {formatTimestamp(metrics.start_time)}
          </Text>
          <Text fontSize="9px" color="gray.400">
            {formatTimestamp(metrics.end_time)}
          </Text>
        </HStack>
      </Box>

      {/* Expanded Details */}
      <Collapse in={isSelected} animateOpacity>
        <Divider mb={2} />
        <VStack align="stretch" spacing={2} fontSize="xs">
          <HStack justify="space-between">
            <Text color="gray.600">Total Duration:</Text>
            <Text fontWeight="medium">{formatDuration(totalTime)}</Text>
          </HStack>
          <HStack justify="space-between">
            <Text color="gray.600">Memory Peak:</Text>
            <Text fontWeight="medium">{memoryPeak.toFixed(1)} MB</Text>
          </HStack>
          {metrics.memory_start_mb !== undefined && (
            <HStack justify="space-between">
              <Text color="gray.600">Memory Growth:</Text>
              <Text fontWeight="medium">
                +{(metrics.memory_peak_mb - metrics.memory_start_mb).toFixed(1)} MB
              </Text>
            </HStack>
          )}
          {cpuPercent > 0 && (
            <HStack justify="space-between">
              <Text color="gray.600">CPU Usage:</Text>
              <Text fontWeight="medium">{cpuPercent.toFixed(1)}%</Text>
            </HStack>
          )}
          {metrics.thread_count > 0 && (
            <HStack justify="space-between">
              <Text color="gray.600">Threads:</Text>
              <Text fontWeight="medium">{metrics.thread_count}</Text>
            </HStack>
          )}

          {/* Error Details */}
          {execution.error && (
            <>
              <Divider />
              <Box>
                <Text color="red.500" fontWeight="bold" mb={1}>
                  Error:
                </Text>
                <Text color="red.600" fontSize="xs" whiteSpace="pre-wrap">
                  {execution.error}
                </Text>
              </Box>
            </>
          )}
        </VStack>
      </Collapse>
    </Box>
  );
}

/**
 * Format duration in ms to human-readable format
 */
function formatDuration(ms) {
  if (ms < 1000) {
    return `${ms}ms`;
  } else if (ms < 60000) {
    return `${(ms / 1000).toFixed(2)}s`;
  } else {
    const minutes = Math.floor(ms / 60000);
    const seconds = ((ms % 60000) / 1000).toFixed(0);
    return `${minutes}m ${seconds}s`;
  }
}

/**
 * Format ISO timestamp to time only
 */
function formatTimestamp(isoString) {
  if (!isoString) return '';
  try {
    const date = new Date(isoString);
    return date.toLocaleTimeString('en-US', {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    });
  } catch {
    return '';
  }
}
