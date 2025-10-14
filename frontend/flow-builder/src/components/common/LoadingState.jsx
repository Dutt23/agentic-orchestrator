import { Flex, Center, Container, Spinner, Text, VStack } from '@chakra-ui/react';

/**
 * Reusable loading state component with flexible layouts
 *
 * @example
 * <LoadingState text="Loading run details..." />
 *
 * @example
 * <LoadingState size="xl" fullScreen />
 *
 * @example
 * <LoadingState
 *   text="Processing..."
 *   centered
 *   size="lg"
 *   color="blue.500"
 * />
 */
export default function LoadingState({
  size = 'lg',
  text,
  fullScreen = false,
  centered = false,
  color = 'blue.500',
  thickness = '4px',
  containerProps = {},
}) {
  const spinner = <Spinner size={size} color={color} thickness={thickness} />;

  const content = text ? (
    <VStack spacing={4}>
      {spinner}
      <Text>{text}</Text>
    </VStack>
  ) : (
    spinner
  );

  // Full screen layout
  if (fullScreen) {
    return (
      <Flex justify="center" align="center" height="100vh">
        {content}
      </Flex>
    );
  }

  // Centered layout
  if (centered) {
    return (
      <Center height="100vh">
        {content}
      </Center>
    );
  }

  // Container layout (default)
  return (
    <Container maxW="container.2xl" py={8} px={8} {...containerProps}>
      {content}
    </Container>
  );
}

/**
 * Inline loading spinner (for use within other components)
 *
 * @example
 * <LoadingSpinner size="xs" />
 */
export function LoadingSpinner({ size = 'sm', color = 'blue.500' }) {
  return <Spinner size={size} color={color} />;
}
