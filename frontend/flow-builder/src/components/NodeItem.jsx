import { Box, Center, Text, Badge, VStack } from '@chakra-ui/react';
import {
  MdMessage,
  MdImage,
  MdVideocam,
  MdSmartButton
} from 'react-icons/md';

const iconComponents = {
  MdMessage,
  MdImage,
  MdVideocam,
  MdSmartButton
};

export default function NodeItem({ nodeType, onDragStart }) {
  const IconComponent = iconComponents[nodeType.icon] || MdMessage;
  const isComingSoon = nodeType.status === 'coming_soon';
  const isDisabled = nodeType.disabled || isComingSoon;

  return (
    <Center width="100%" mb={4}>
      <Box
        as="div"
        draggable={!isDisabled}
        onDragStart={(e) => !isDisabled && onDragStart(e, nodeType)}
        border="2px solid"
        borderColor={isDisabled ? "gray.300" : "blue.400"}
        borderRadius="lg"
        p={4}
        width="90%"
        textAlign="center"
        cursor={isDisabled ? "not-allowed" : "grab"}
        color={isDisabled ? "gray.500" : "blue.600"}
        bg={isDisabled ? "gray.50" : "white"}
        opacity={isDisabled ? 0.5 : 1}
        _hover={!isDisabled ? {
          bg: 'blue.50',
          transform: 'translateY(-2px)',
          boxShadow: 'md',
        } : {}}
        transition="all 0.2s"
        boxShadow="sm"
        userSelect="none"
      >
        <VStack spacing={2}>
          <Center>
            <IconComponent size={18} />
          </Center>
          <Text fontWeight="semibold" fontSize="sm">
            {nodeType.label}
          </Text>
          {isComingSoon && (
            <Badge colorScheme="yellow" fontSize="10px" variant="solid">
              Coming Soon
            </Badge>
          )}
          <Text fontSize="xs" color={isDisabled ? "gray.400" : "gray.500"} mt={1}>
            {nodeType.description}
          </Text>
        </VStack>
      </Box>
    </Center>
  );
}
