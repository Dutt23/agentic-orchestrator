export const initialNodes = [
  {
    id: '1',
    type: 'customNode',
    position: { x: 100, y: 100 },
    data: {
      type: 'text',
      label: 'Text Node',
      text: 'This is a text node.'
    },
  }
];

export const initialEdges = [];

// Mock API function to simulate data fetching
const fetchInitialFlowData = () => {
  return new Promise((resolve) => {
    // Simulate network delay
    setTimeout(() => {
      resolve({
        nodes: initialNodes,
        edges: initialEdges
      });
    }, 500);
  });
};

export default fetchInitialFlowData;
