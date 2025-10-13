import { useState, useCallback } from 'react';
import { Box, Input, Button, VStack, Text, Image, FormControl, FormLabel } from '@chakra-ui/react';

export default function ImageNodeEditor({ node, onUpdate }) {
  const [imageUrl, setImageUrl] = useState(node?.data?.url || '');
  const [altText, setAltText] = useState(node?.data?.alt || '');
  const [label, setLabel] = useState(node?.data?.label || 'Image Node');

  const handleImageUpload = useCallback((event) => {
    const file = event.target.files[0];
    if (!file) return;

    const reader = new FileReader();
    reader.onloadend = () => {
      const base64String = reader.result;
      setImageUrl(base64String);
      updateNode({ url: base64String });
    };
    reader.readAsDataURL(file);
  }, []);

  const updateNode = useCallback((updates) => {
    if (!onUpdate) return;
    
    onUpdate({
      ...node,
      data: {
        ...node.data,
        ...updates,
        alt: altText || updates.alt || 'Image',
        label: label || 'Image Node'
      }
    });
  }, [node, onUpdate, altText, label]);

  const handleAltTextChange = (e) => {
    const newAlt = e.target.value;
    setAltText(newAlt);
    updateNode({ alt: newAlt });
  };

  const handleLabelChange = (e) => {
    const newLabel = e.target.value;
    setLabel(newLabel);
    updateNode({ label: newLabel });
  };

  return (
    <VStack spacing={4} align="stretch">
      <FormControl>
        <FormLabel>Label</FormLabel>
        <Input
          value={label}
          onChange={handleLabelChange}
          placeholder="Node label"
          size="sm"
          mb={4}
        />
      </FormControl>

      <FormControl>
        <FormLabel>Image</FormLabel>
        {imageUrl ? (
          <Box mb={4}>
            <Image 
              src={imageUrl} 
              alt={altText || 'Preview'} 
              maxH="200px" 
              objectFit="contain"
              borderRadius="md"
              mb={2}
            />
            <Button 
              size="sm" 
              colorScheme="blue" 
              variant="outline"
              onClick={() => document.getElementById('image-upload').click()}
              width="100%"
            >
              Change Image
            </Button>
          </Box>
        ) : (
          <Box 
            border="2px dashed" 
            borderColor="gray.300" 
            p={6} 
            textAlign="center"
            borderRadius="md"
            mb={4}
          >
            <Text mb={2}>No image selected</Text>
            <Button 
              as="label" 
              htmlFor="image-upload" 
              size="sm" 
              colorScheme="blue"
              cursor="pointer"
            >
              Upload Image
            </Button>
          </Box>
        )}
        <Input
          type="file"
          id="image-upload"
          accept="image/*"
          onChange={handleImageUpload}
          display="none"
        />
      </FormControl>

      <FormControl>
        <FormLabel>Alt Text</FormLabel>
        <Input
          value={altText}
          onChange={handleAltTextChange}
          placeholder="Description of the image"
          size="sm"
        />
      </FormControl>
    </VStack>
  );
}
