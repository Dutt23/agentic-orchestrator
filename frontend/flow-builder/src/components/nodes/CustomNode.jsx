import { TextNode, ImageNode, VideoNode, ButtonNode } from '../nodes';
import WorkflowNode from './WorkflowNode';

export default function CustomNode({ selected, ...props }) {
  const { type = 'text' } = props.data || {};

  // Workflow orchestrator node types
  const workflowTypes = ['agent', 'http', 'file-search', 'transform', 'aggregate', 'filter', 'conditional', 'loop', 'hitl'];

  // If it's a workflow node type, use WorkflowNode
  if (workflowTypes.includes(type)) {
    return <WorkflowNode selected={selected} {...props} />;
  }

  // Otherwise use legacy node types
  const nodeComponents = {
    text: TextNode,
    image: ImageNode,
    video: VideoNode,
    button: ButtonNode,
  };

  const NodeComponent = nodeComponents[type] || TextNode;

  return <NodeComponent selected={selected} {...props} />;
}
