import React from 'react';
import { FaWhatsapp } from 'react-icons/fa';
import { NodeBase } from '../common/NodeBase';

export const TextNode = ({ data, selected }) => {
  const { label = 'Text', text = '' } = data;
  
  return (
    <NodeBase 
      selected={selected}
      type="text"
      label={label}
      icon={FaWhatsapp}
      data={data}
    >
      <div style={{ 
        background: '#f9fafb', 
        padding: '12px', 
        borderRadius: '4px',
        borderLeft: '3px solid #86efac'
      }}>
        {text || 'No content'}
      </div>
    </NodeBase>
  );
};

export default TextNode;
