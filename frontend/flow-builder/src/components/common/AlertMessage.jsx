import { Alert, AlertIcon, AlertTitle, AlertDescription, Box } from '@chakra-ui/react';

/**
 * Reusable alert component with consistent styling
 * Supports all Chakra alert statuses with optional title
 *
 * @example
 * <AlertMessage status="error" message="Failed to load data" />
 *
 * @example
 * <AlertMessage
 *   status="warning"
 *   title="Warning"
 *   message="This action cannot be undone"
 * />
 *
 * @example
 * <AlertMessage status="info">
 *   Custom content here
 * </AlertMessage>
 */
export default function AlertMessage({
  status = 'info',
  title,
  message,
  children,
  ...chakraProps
}) {
  // If children provided, use custom content
  if (children) {
    return (
      <Alert status={status} borderRadius="md" {...chakraProps}>
        <AlertIcon />
        {children}
      </Alert>
    );
  }

  // If title provided, use title + description layout
  if (title) {
    return (
      <Alert status={status} borderRadius="md" {...chakraProps}>
        <AlertIcon />
        <Box>
          <AlertTitle>{title}</AlertTitle>
          {message && <AlertDescription>{message}</AlertDescription>}
        </Box>
      </Alert>
    );
  }

  // Simple message-only layout
  return (
    <Alert status={status} borderRadius="md" {...chakraProps}>
      <AlertIcon />
      {message}
    </Alert>
  );
}
