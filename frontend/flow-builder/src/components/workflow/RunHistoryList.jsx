import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Box,
  Heading,
  VStack,
  HStack,
  Text,
  Badge,
  Spinner,
  Alert,
  AlertIcon,
} from '@chakra-ui/react';
import { listWorkflowRuns } from '../../services/api';

/**
 * RunHistoryList displays recent runs for a workflow
 * Shows status badges (green/red/blue) and allows navigation to run details
 */
export default function RunHistoryList({ workflowTag }) {
  const [runs, setRuns] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const navigate = useNavigate();

  useEffect(() => {
    if (!workflowTag) return;

    const fetchRuns = async () => {
      try {
        setLoading(true);
        const data = await listWorkflowRuns(workflowTag, 10); // Get last 10 runs
        setRuns(data);
      } catch (err) {
        console.error('Failed to fetch run history:', err);
        setError(err.message);
      } finally {
        setLoading(false);
      }
    };

    fetchRuns();

    // Auto-refresh every 5 seconds to show live status updates
    const interval = setInterval(fetchRuns, 5000);
    return () => clearInterval(interval);
  }, [workflowTag]);

  const getStatusBadge = (status) => {
    const statusLower = status?.toLowerCase() || '';

    if (statusLower === 'completed') {
      return <Badge colorScheme="green">Success</Badge>;
    } else if (statusLower === 'failed') {
      return <Badge colorScheme="red">Failed</Badge>;
    } else if (statusLower === 'running') {
      return (
        <HStack spacing={2}>
          <Spinner size="xs" />
          <Badge colorScheme="blue">Running</Badge>
        </HStack>
      );
    } else if (statusLower === 'queued') {
      return <Badge colorScheme="gray">Queued</Badge>;
    }
    return <Badge>{status}</Badge>;
  };

  const formatDate = (dateString) => {
    if (!dateString) return 'N/A';
    const date = new Date(dateString);
    return date.toLocaleString();
  };

  const handleRunClick = (runId) => {
    navigate(`/runs/${runId}`);
  };

  if (loading && runs.length === 0) {
    return (
      <Box>
        <Heading size="sm" mb={4}>
          Run History
        </Heading>
        <Spinner size="sm" />
      </Box>
    );
  }

  if (error) {
    return (
      <Box>
        <Heading size="sm" mb={4}>
          Run History
        </Heading>
        <Alert status="error" size="sm">
          <AlertIcon />
          Failed to load runs: {error}
        </Alert>
      </Box>
    );
  }

  if (runs.length === 0) {
    return (
      <Box>
        <Heading size="sm" mb={4}>
          Run History
        </Heading>
        <Text color="gray.500" fontSize="sm">
          No runs yet. Execute this workflow to see run history.
        </Text>
      </Box>
    );
  }

  return (
    <Box>
      <Heading size="sm" mb={4}>
        Run History
      </Heading>
      <VStack spacing={2} align="stretch">
        {runs.map((run) => (
          <Box
            key={run.run_id}
            p={3}
            border="1px solid"
            borderColor="gray.200"
            borderRadius="md"
            cursor="pointer"
            _hover={{ bg: 'gray.50' }}
            onClick={() => handleRunClick(run.run_id)}
          >
            <HStack justify="space-between" mb={1}>
              <Text fontSize="sm" fontWeight="medium" isTruncated maxW="200px">
                {run.run_id}
              </Text>
              {getStatusBadge(run.status)}
            </HStack>
            <HStack spacing={4} fontSize="xs" color="gray.600">
              <Text>
                {run.submitted_by && `By: ${run.submitted_by}`}
              </Text>
              <Text>{formatDate(run.submitted_at)}</Text>
            </HStack>
          </Box>
        ))}
      </VStack>
    </Box>
  );
}
