import React from 'react';
import { FaImage } from 'react-icons/fa';
import { NodeBase } from '../common/NodeBase';

export const ImageNode = ({ data, selected }) => {
  const { label = 'Image', url, alt = '' } = data;
  
  return (
    <NodeBase 
      selected={selected}
      type="image"
      label={label}
      icon={FaImage}
      data={data}
    >
      <div style={{ 
        background: '#f9fafb', 
        padding: '12px', 
        borderRadius: '4px',
        borderLeft: '3px solid #93c5fd',
        textAlign: 'center'
      }}>
        {url ? (
          <div style={{ 
            maxHeight: '200px',
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            overflow: 'hidden',
            marginBottom: '8px'
          }}>
            <img 
              src={url} 
              alt={alt} 
              style={{ 
                maxWidth: '100%', 
                maxHeight: '100%', 
                objectFit: 'contain' 
              }} 
            />
          </div>
        ) : (
          <div style={{ 
            color: '#6b7280', 
            fontStyle: 'italic',
            padding: '20px 0'
          }}>
            No image selected
          </div>
        )}
        {alt && (
          <div style={{ 
            fontSize: '12px', 
            color: '#6b7280',
            marginTop: '8px',
            fontStyle: 'italic'
          }}>
            {alt}
          </div>
        )}
      </div>
    </NodeBase>
  );
};

export default ImageNode;
