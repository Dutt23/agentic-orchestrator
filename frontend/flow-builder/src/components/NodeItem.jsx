import { Box, Center, Text } from '@chakra-ui/react';
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

  return (
    <Center width="100%" mb={4}>
      <Box
        as="div"
        draggable
        onDragStart={(e) => onDragStart(e, nodeType)}
        border="2px solid"
        borderColor="blue.400"
        borderRadius="lg"
        p={4}
        width="90%"
        textAlign="center"
        cursor="grab"
        color="blue.600"
        bg="white"
        _hover={{
          bg: 'blue.50',
          transform: 'translateY(-2px)',
          boxShadow: 'md',
        }}
        transition="all 0.2s"
        boxShadow="sm"
        userSelect="none"
      >
        <Center mb={2}>
          <IconComponent size={24} />
        </Center>
        <Text fontWeight="semibold" fontSize="sm">
          {nodeType.label}
        </Text>
        <Text fontSize="xs" color="gray.500" mt={1}>
          {nodeType.description}
        </Text>
      </Box>
    </Center>
  );
}
