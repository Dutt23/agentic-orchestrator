import { Textarea, VStack, Text } from '@chakra-ui/react';
import { useState, useEffect, useCallback } from 'react';
import debounce from 'lodash.debounce';

export default function TextNodeEditor({ node, onUpdate }) {
  const [text, setText] = useState(node?.data?.text || '');

  // Update local state when node prop changes
  useEffect(() => {
    setText(node?.data?.text || '');
  }, [node?.data?.text]);

  // Create debounced update function
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const debouncedUpdate = useCallback(
    debounce((newText) => {
      if (onUpdate) {
        onUpdate({
          ...node,
          data: {
            ...node.data,
            text: newText
          }
        });
      }
    }, 300), // 300ms debounce delay
    [node, onUpdate]
  );

  const handleTextChange = (e) => {
    const newText = e.target.value;
    setText(newText);
    debouncedUpdate(newText);
  };

  return (
    <VStack spacing={4} align="stretch">
      <Text fontSize="sm" fontWeight="medium">Message</Text>
      <Textarea
        value={text}
        onChange={handleTextChange}
        placeholder="Enter your message..."
        minH="120px"
        size="sm"
        resize="vertical"
      />
    </VStack>
  );
}
