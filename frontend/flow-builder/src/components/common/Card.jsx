import { Box } from '@chakra-ui/react';

/**
 * Reusable card component with consistent border, padding, and color variants
 *
 * @example
 * <Card>Simple card content</Card>
 *
 * @example
 * <Card variant="info" p={6}>
 *   Info card with custom padding
 * </Card>
 *
 * @example
 * <Card clickable onClick={() => console.log('clicked')}>
 *   Clickable card
 * </Card>
 */
export default function Card({
  variant = 'default',
  clickable = false,
  onClick,
  children,
  ...chakraProps
}) {
  // Variant configurations
  const variants = {
    default: {
      bg: 'white',
      borderColor: 'gray.200',
    },
    info: {
      bg: 'blue.50',
      borderColor: 'blue.200',
    },
    success: {
      bg: 'green.50',
      borderColor: 'green.200',
    },
    warning: {
      bg: 'orange.50',
      borderColor: 'orange.200',
    },
    error: {
      bg: 'red.50',
      borderColor: 'red.200',
    },
    purple: {
      bg: 'purple.50',
      borderColor: 'purple.200',
    },
    gray: {
      bg: 'gray.50',
      borderColor: 'gray.200',
    },
  };

  const variantStyle = variants[variant] || variants.default;

  // Clickable card styles
  const clickableStyles = clickable
    ? {
        cursor: 'pointer',
        _hover: { bg: `${variantStyle.bg}`, opacity: 0.8 },
        transition: 'all 0.2s',
      }
    : {};

  return (
    <Box
      p={4}
      border="1px solid"
      borderRadius="md"
      bg={variantStyle.bg}
      borderColor={variantStyle.borderColor}
      onClick={onClick}
      {...clickableStyles}
      {...chakraProps}
    >
      {children}
    </Box>
  );
}
