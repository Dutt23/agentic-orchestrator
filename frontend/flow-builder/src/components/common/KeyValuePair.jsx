import { HStack, Text, Code, Badge } from '@chakra-ui/react';

/**
 * Displays a single key-value pair with consistent styling
 *
 * @example
 * <KeyValuePair label="Run ID" value={runId} code />
 *
 * @example
 * <KeyValuePair
 *   label="Status"
 *   value="completed"
 *   badge
 *   colorScheme="green"
 * />
 */
export default function KeyValuePair({
  label,
  value,
  code = false,
  badge = false,
  colorScheme = 'gray',
  justify = 'flex-start',
  labelProps = {},
  valueProps = {},
  ...hstackProps
}) {
  if (value === undefined || value === null) {
    return null;
  }

  const displayValue = value || 'N/A';

  return (
    <HStack justify={justify} {...hstackProps}>
      <Text fontWeight="bold" {...labelProps}>
        {label}:
      </Text>
      {code ? (
        <Code fontSize="sm" {...valueProps}>
          {displayValue}
        </Code>
      ) : badge ? (
        <Badge colorScheme={colorScheme} {...valueProps}>
          {displayValue}
        </Badge>
      ) : (
        <Text {...valueProps}>{displayValue}</Text>
      )}
    </HStack>
  );
}
