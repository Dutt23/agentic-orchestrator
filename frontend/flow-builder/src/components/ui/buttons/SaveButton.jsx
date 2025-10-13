import { Button } from '@chakra-ui/react';

export default function SaveButton({ size = 'md', onClick }) {
  return (
    <Button
      colorScheme="blue"
      variant="outline"
      size={size}
      fontSize={size === 'sm' ? 'xs' : 'sm'}
      px={size === 'sm' ? 3 : 4}
      fontWeight="medium"
      onClick={onClick}
    >
      Save Changes
    </Button>
  );
}
