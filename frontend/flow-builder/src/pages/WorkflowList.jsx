import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Box,
  Container,
  Heading,
  Text,
  Table,
  Thead,
  Tbody,
  Tr,
  Th,
  Td,
  Button,
  Center,
  HStack,
  Badge,
  IconButton,
  useToast,
} from '@chakra-ui/react';
import { FiPlus, FiRefreshCw } from 'react-icons/fi';
import { listWorkflows } from '../services/api';
import { useAuth } from '../contexts/AuthContext';
import { AlertMessage, LoadingState } from '../components/common';

export default function WorkflowList() {
  const [workflows, setWorkflows] = useState([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(null);
  const navigate = useNavigate();
  const { username } = useAuth();
  const toast = useToast();

  const fetchWorkflows = async () => {
    setIsLoading(true);
    setError(null);

    try {
      const data = await listWorkflows('user');
      setWorkflows(data);
    } catch (err) {
      console.error('Failed to fetch workflows:', err);
      setError(err.message || 'Failed to load workflows');

      // For development: if API fails, show mock data
      if (process.env.NODE_ENV === 'development') {
        console.warn('API failed, using mock data');
        setWorkflows([]);
      }
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchWorkflows();
  }, []);

  const handleWorkflowClick = (workflow) => {
    // Navigate to detailed workflow view
    // Owner is passed via X-User-ID header, so we only need tag in URL
    // URL-encode the tag to handle slashes (e.g., "test-user/main")
    navigate(`/workflow/${encodeURIComponent(workflow.tag)}`);
  };

  const handleRefresh = () => {
    fetchWorkflows();
    toast({
      title: 'Refreshing workflows',
      status: 'info',
      duration: 2000,
      isClosable: true,
    });
  };

  const handleCreateNew = () => {
    // For now, just show a toast. Later can navigate to create page
    toast({
      title: 'Create workflow',
      description: 'This feature is coming soon',
      status: 'info',
      duration: 3000,
      isClosable: true,
    });
  };

  // Format date for display
  const formatDate = (dateString) => {
    if (!dateString) return '-';
    const date = new Date(dateString);
    return date.toLocaleDateString() + ' ' + date.toLocaleTimeString();
  };

  if (isLoading) {
    return <LoadingState centered size="xl" />;
  }

  return (
    <Box minH="100vh" bg="gray.50">
      {/* Header */}
      <Box bg="white" borderBottom="1px solid" borderColor="gray.200" py={4}>
        <Container maxW="container.xl">
          <HStack justify="space-between">
            <Box>
              <Heading size="lg" mb={1}>
                Workflows
              </Heading>
              <Text fontSize="sm" color="gray.600">
                Logged in as: <Badge colorScheme="blue">{username}</Badge>
              </Text>
            </Box>
            <HStack spacing={3}>
              <IconButton
                icon={<FiRefreshCw />}
                onClick={handleRefresh}
                aria-label="Refresh workflows"
                variant="outline"
              />
              <Button
                leftIcon={<FiPlus />}
                colorScheme="blue"
                onClick={handleCreateNew}
              >
                Create Workflow
              </Button>
            </HStack>
          </HStack>
        </Container>
      </Box>

      {/* Content */}
      <Container maxW="container.xl" py={8}>
        {error && (
          <AlertMessage
            status="error"
            title="Error loading workflows"
            message={error}
            mb={6}
          />
        )}

        {!error && workflows.length === 0 ? (
          <Center py={16}>
            <Box textAlign="center">
              <Heading size="md" color="gray.600" mb={2}>
                No workflows found
              </Heading>
              <Text color="gray.500" mb={6}>
                Create your first workflow to get started
              </Text>
              <Button
                leftIcon={<FiPlus />}
                colorScheme="blue"
                onClick={handleCreateNew}
              >
                Create Workflow
              </Button>
            </Box>
          </Center>
        ) : (
          <Box bg="white" borderRadius="lg" boxShadow="sm" overflow="hidden">
            <Table variant="simple">
              <Thead bg="gray.50">
                <Tr>
                  <Th>Workflow Name</Th>
                  <Th>Branch/Tag</Th>
                  <Th>Owner</Th>
                  <Th>Last Modified</Th>
                  <Th>Version</Th>
                </Tr>
              </Thead>
              <Tbody>
                {workflows.map((workflow) => (
                  <Tr
                    key={`${workflow.owner}-${workflow.tag}`}
                    _hover={{ bg: 'blue.50', cursor: 'pointer' }}
                    onClick={() => handleWorkflowClick(workflow)}
                  >
                    <Td>
                      <Text fontWeight="medium" color="blue.600">
                        {workflow.tag}
                      </Text>
                    </Td>
                    <Td>
                      <Badge colorScheme="purple" fontSize="xs">
                        {workflow.tag}
                      </Badge>
                    </Td>
                    <Td>
                      <Badge colorScheme="gray" fontSize="xs">
                        {workflow.owner}
                      </Badge>
                    </Td>
                    <Td>
                      <Text fontSize="sm" color="gray.600">
                        {formatDate(workflow.moved_at)}
                      </Text>
                    </Td>
                    <Td>
                      <Text fontSize="sm" color="gray.600">
                        v{workflow.version || '1'}
                      </Text>
                    </Td>
                  </Tr>
                ))}
              </Tbody>
            </Table>
          </Box>
        )}
      </Container>
    </Box>
  );
}
