import React from 'react';
import { DefaultNodeEditor } from '.';

// Map of node types to their editor components
const editorRegistry = {
  text: React.lazy(() => import('../nodes/text/TextEditor')),
  image: React.lazy(() => import('../nodes/image/ImageEditor')),
  // Add more editors here as needed
};


export default function NodeEditor ({ node, onUpdate }) {
  if (!node?.data) return null;

  // Get the appropriate editor component or fallback to DefaultNodeEditor
  const EditorComponent = editorRegistry[node.data.type] || DefaultNodeEditor;
  
  return (
    <React.Suspense fallback={<div>Loading editor...</div>}>
      <EditorComponent node={node} onUpdate={onUpdate} />
    </React.Suspense>
  );
};
