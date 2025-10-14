import { VStack } from '@chakra-ui/react';
import KeyValuePair from './KeyValuePair';

/**
 * Displays a list of key-value pairs in a consistent vertical layout
 *
 * @example
 * <KeyValueList
 *   items={[
 *     { label: 'Run ID', value: runId, code: true },
 *     { label: 'Status', value: status, badge: true, colorScheme: 'green' },
 *     { label: 'Submitted By', value: user }
 *   ]}
 * />
 *
 * @example
 * <KeyValueList
 *   items={metrics}
 *   spacing={3}
 *   align="stretch"
 * />
 */
export default function KeyValueList({
  items = [],
  spacing = 2,
  align = 'stretch',
  ...vstackProps
}) {
  if (!items || items.length === 0) {
    return null;
  }

  return (
    <VStack align={align} spacing={spacing} {...vstackProps}>
      {items.map((item, index) => (
        <KeyValuePair
          key={item.label || index}
          label={item.label}
          value={item.value}
          code={item.code}
          badge={item.badge}
          colorScheme={item.colorScheme}
          justify={item.justify || 'flex-start'}
        />
      ))}
    </VStack>
  );
}
