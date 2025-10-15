import React from 'react';
import { Box, Container, Heading, Text, Button, VStack, Code } from '@chakra-ui/react';
import { AlertMessage } from './common';

class ErrorBoundary extends React.Component {
  constructor(props) {
    super(props);
    this.state = { hasError: false, error: null, errorInfo: null };
  }

  static getDerivedStateFromError(error) {
    return { hasError: true };
  }

  componentDidCatch(error, errorInfo) {
    console.error('ErrorBoundary caught:', error, errorInfo);
    this.state = {
      hasError: true,
      error,
      errorInfo
    };
  }

  render() {
    if (this.state.hasError) {
      return (
        <Container maxW="container.md" py={8}>
          <VStack spacing={4} align="stretch">
            <AlertMessage
              status="error"
              message="Something went wrong"
            />

            <Box>
              <Heading size="md" mb={2}>Error Details</Heading>
              <Text fontSize="sm" color="gray.600" mb={4}>
                {this.state.error?.toString() || 'Unknown error'}
              </Text>

              {this.state.errorInfo && (
                <Code p={4} borderRadius="md" fontSize="xs" display="block" whiteSpace="pre-wrap">
                  {this.state.errorInfo.componentStack}
                </Code>
              )}
            </Box>

            <Button
              colorScheme="blue"
              onClick={() => {
                this.setState({ hasError: false, error: null, errorInfo: null });
                window.location.reload();
              }}
            >
              Reload Page
            </Button>
          </VStack>
        </Container>
      );
    }

    return this.props.children;
  }
}

export default ErrorBoundary;
