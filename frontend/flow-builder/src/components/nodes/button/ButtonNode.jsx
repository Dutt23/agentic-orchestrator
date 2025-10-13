import React from 'react';
import { FaMousePointer } from 'react-icons/fa';
import { NodeBase } from '../common/NodeBase';

export const ButtonNode = ({ data }) => {
  const { label = 'Button', text = 'Button', action } = data;
  
  return (
    <NodeBase 
      type="button"
      label={label}
      icon={FaMousePointer}
      data={data}
    >
      <div style={{ textAlign: 'center' }}>
        <button 
          style={{
            background: '#4f46e5',
            color: 'white',
            border: 'none',
            padding: '8px 16px',
            borderRadius: '4px',
            cursor: 'pointer',
            width: '100%',
            fontSize: '14px'
          }}
        >
          {text}
        </button>
        {action && (
          <div style={{ fontSize: '12px', color: '#6b7280', marginTop: '8px' }}>
            Action: {action}
          </div>
        )}
      </div>
    </NodeBase>
  );
};

export default ButtonNode;
