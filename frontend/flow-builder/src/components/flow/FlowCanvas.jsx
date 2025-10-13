import { useCallback, useRef, useState, useEffect } from 'react';
import { useToast } from '@chakra-ui/react';
import {
  ReactFlow,
  ReactFlowProvider,
  useNodesState,
  useEdgesState,
  Controls,
  Background,
  MiniMap,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import CustomNode from '../nodes/CustomNode';

const nodeTypes = { 
  customNode: (props) => <CustomNode {...props} />,
};

function FlowCanvasInner({ 
  nodes: propNodes = [], 
  edges: propEdges = [], 
  selectedNode,
  setSelectedNode,
  onNodesChange: onNodesChangeProp,
  onEdgesChange: onEdgesChangeProp,
  onConnect: onConnectProp
}) {
  const [reactFlowInstance, setReactFlowInstance] = useState(null);
  const [nodes, setNodes, onNodesChangeLocal] = useNodesState(propNodes);
  const [edges, setEdges, onEdgesChangeLocal] = useEdgesState(propEdges);
  const reactFlowWrapper = useRef(null);
  const toast = useToast();

  // Sync local state with props
  useEffect(() => {
    setNodes(propNodes);
  }, [propNodes, setNodes]);

  useEffect(() => {
    setEdges(propEdges);
  }, [propEdges, setEdges]);

  // Combine local and parent handlers
  const onNodesChange = useCallback((changes) => {
    onNodesChangeLocal(changes);
    if (onNodesChangeProp) onNodesChangeProp(changes);
  }, [onNodesChangeProp, onNodesChangeLocal]);

  const onEdgesChange = useCallback((changes) => {
    onEdgesChangeLocal(changes);
    if (onEdgesChangeProp) onEdgesChangeProp(changes);
  }, [onEdgesChangeProp, onEdgesChangeLocal]);

  const onConnect = useCallback((params) => {
    // Check if source handle already has an edge
    const hasExistingEdge = edges.some(edge => 
      edge.source === params.source && 
      edge.sourceHandle === params.sourceHandle
    );

    if (hasExistingEdge) {
      toast({
        title: 'Connection Error',
        description: 'Source handle already has an edge',
        status: 'error',
        duration: 2000,
        isClosable: true,
        position: 'bottom-right'
      });
      return;
    }

    if (onConnectProp) onConnectProp(params);
  }, [edges, onConnectProp]);

  // Handle node click
  const onNodeClick = useCallback((event, node) => {
    event.stopPropagation();
    setSelectedNode(node);
  }, [setSelectedNode]);

  // Handle pane click to clear selection
  const onPaneClick = useCallback(() => {
    setSelectedNode(null);
  }, [setSelectedNode]);

  const onDrop = useCallback((event) => {
    event.preventDefault();
    const reactFlowBounds = reactFlowWrapper.current.getBoundingClientRect();
    const nodeData = event.dataTransfer.getData('application/reactflow');
    if (!nodeData) return;

    const { type, data } = JSON.parse(nodeData);
    
    // Get the drop position relative to the React Flow container
    const position = reactFlowInstance.screenToFlowPosition({
      x: event.clientX - reactFlowBounds.left,
      y: event.clientY - reactFlowBounds.top,
    });
    
    const newNode = {
      id: `${type}-${Date.now()}`,
      type: 'customNode',
      position,
      data: {
        ...data,
        type,
        label: data?.label || type.charAt(0).toUpperCase() + type.slice(1)
      }
    };

    // Update local state
    setNodes(nds => [...nds, newNode]);
    
    // Notify parent
    if (onNodesChangeProp) {
      onNodesChangeProp([{ type: 'add', item: newNode }]);
    }

    // Select the new node
    setSelectedNode(newNode);
  }, [reactFlowInstance, setNodes, setSelectedNode, onNodesChangeProp]);

  const onDragOver = useCallback((event) => {
    event.preventDefault();
    event.dataTransfer.dropEffect = 'move';
  }, []);

  // Update nodes with selected state
  const nodesWithSelection = nodes.map(node => ({
    ...node,
    selected: selectedNode ? node.id === selectedNode.id : false,
    data: {
      ...node.data,
      selected: selectedNode ? node.id === selectedNode.id : false
    }
  }));

  return (
    <div className="reactflow-wrapper" ref={reactFlowWrapper} style={{ height: '100%', width: '100%' }}>
      <ReactFlow
        nodes={nodesWithSelection}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onConnect={onConnect}
        onInit={setReactFlowInstance}
        onDrop={onDrop}
        onDragOver={onDragOver}
        onNodeClick={onNodeClick}
        onPaneClick={onPaneClick}
        nodeTypes={nodeTypes}
        fitView
        nodesDraggable={true}
        nodesConnectable={true}
        elementsSelectable={true}
      >
        <MiniMap />
        <Controls />
        <Background />
      </ReactFlow>
    </div>
  );
}

export default function FlowCanvas({ selectedNode, ...props }) {
  return (
    <ReactFlowProvider>
      <FlowCanvasInner selectedNode={selectedNode} {...props} />
    </ReactFlowProvider>
  );
}
