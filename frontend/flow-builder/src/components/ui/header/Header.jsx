import { Box, Flex, Text } from '@chakra-ui/react';
import SaveButton from '../buttons/SaveButton';

export default function Header({ onSave }) {
  return (
    <Box
      width="100%"
      bg="gray.50"
      borderBottom="1px solid"
      borderColor="gray.200"
      px={4}
      h="40px"
      position="fixed"
      top={0}
      left={0}
      right={0}
      zIndex={1000}
      boxShadow="sm"
    >
      <Flex 
        justify="space-between" 
        align="center" 
        maxW="container.xl" 
        mx="auto"
        h="100%"
      >
        <Text fontSize="sm" fontWeight="medium" color="gray.700">
          Flow Builder
        </Text>
        <Box display="flex" alignItems="center" height="100%">
          <SaveButton size="sm" onClick={onSave} />
        </Box>
      </Flex>
    </Box>
  );
}
