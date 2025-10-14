import { Box, Text, Code, Collapse, IconButton, HStack } from '@chakra-ui/react';
import { ChevronDownIcon, ChevronUpIcon } from '@chakra-ui/icons';
import { useState } from 'react';

/**
 * Displays JSON data in a formatted code block with optional label and collapsible support
 *
 * @example
 * <JsonDisplay label="Input" data={inputData} />
 *
 * @example
 * <JsonDisplay
 *   label="Large Output"
 *   data={outputData}
 *   maxHeight="400px"
 *   collapsible
 *   defaultCollapsed
 * />
 */
export default function JsonDisplay({
  label,
  data,
  maxHeight,
  collapsible = false,
  defaultCollapsed = false,
  fontSize = 'sm',
  ...chakraProps
}) {
  const [isCollapsed, setIsCollapsed] = useState(defaultCollapsed);

  if (!data) {
    return null;
  }

  const jsonString = typeof data === 'string' ? data : JSON.stringify(data, null, 2);

  const codeBlock = (
    <Code
      display="block"
      whiteSpace="pre"
      p={4}
      borderRadius="md"
      overflowX="auto"
      maxH={maxHeight}
      overflowY={maxHeight ? 'auto' : 'visible'}
      fontSize={fontSize}
    >
      {jsonString}
    </Code>
  );

  if (!label && !collapsible) {
    return <Box {...chakraProps}>{codeBlock}</Box>;
  }

  if (collapsible) {
    return (
      <Box {...chakraProps}>
        <HStack justify="space-between" mb={2}>
          {label && <Text fontWeight="bold">{label}</Text>}
          <IconButton
            icon={isCollapsed ? <ChevronDownIcon /> : <ChevronUpIcon />}
            size="xs"
            variant="ghost"
            onClick={() => setIsCollapsed(!isCollapsed)}
            aria-label={isCollapsed ? `Expand ${label}` : `Collapse ${label}`}
          />
        </HStack>
        <Collapse in={!isCollapsed} animateOpacity>
          {codeBlock}
        </Collapse>
      </Box>
    );
  }

  return (
    <Box {...chakraProps}>
      {label && (
        <Text fontWeight="bold" mb={2}>
          {label}
        </Text>
      )}
      {codeBlock}
    </Box>
  );
}
