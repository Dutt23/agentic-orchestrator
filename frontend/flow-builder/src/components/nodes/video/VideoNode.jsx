import React from 'react';
import { FaVideo } from 'react-icons/fa';
import { NodeBase } from '../common/NodeBase';

export const VideoNode = ({ data, selected }) => {
  const { label = 'Video', url } = data;
  
  return (
    <NodeBase 
      selected={selected}
      type="video"
      label={label}
      icon={FaVideo}
      data={data}
    >
      {url ? (
        <div style={{ textAlign: 'center' }}>
          <div style={{ 
            background: '#f3f4f6', 
            height: '100px', 
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            borderRadius: '4px',
            marginBottom: '8px'
          }}>
            <FaVideo size={32} color="#9ca3af" />
          </div>
          <div style={{ fontSize: '12px', color: '#6b7280', wordBreak: 'break-all' }}>
            {url}
          </div>
        </div>
      ) : (
        <div style={{ color: '#6b7280', fontStyle: 'italic' }}>No video URL provided</div>
      )}
    </NodeBase>
  );
};

export default VideoNode;
