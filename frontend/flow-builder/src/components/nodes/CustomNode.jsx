import { TextNode, ImageNode, VideoNode, ButtonNode } from '../nodes';

export default function CustomNode({ selected, ...props }) {
  const { type = 'text' } = props.data || {};
  
  const nodeComponents = {
    text: TextNode,
    image: ImageNode,
    video: VideoNode,
    button: ButtonNode,
  };
  
  const NodeComponent = nodeComponents[type] || TextNode;
  
  return <NodeComponent selected={selected} {...props} />;
}
