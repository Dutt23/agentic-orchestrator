# Router Setup for RunDetail Page

## Add Route to Your Router Configuration

The RunDetail page has been created at `src/pages/RunDetail.jsx`. You need to add the following route to your React Router configuration:

```jsx
import RunDetail from './pages/RunDetail';

// In your router configuration (main.jsx, App.jsx, or routes.jsx):
<Route path="/runs/:runId" element={<RunDetail />} />
```

## Complete Example

If you're using React Router v6 with BrowserRouter:

```jsx
import { BrowserRouter, Routes, Route } from 'react-router-dom';
import Flow from './pages/Flow';
import RunDetail from './pages/RunDetail';

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<HomePage />} />
        <Route path="/flow/:tag" element={<Flow />} />
        <Route path="/runs/:runId" element={<RunDetail />} />  {/* ADD THIS */}
      </Routes>
    </BrowserRouter>
  );
}

export default App;
```

## Navigation

The RunHistoryList component already includes navigation to this route:

```jsx
const handleRunClick = (runId) => {
  navigate(`/runs/${runId}`);
};
```

When a user clicks on a run in the history list, they will be taken to `/runs/{run-id}`.

## Required Dependencies

Make sure you have these dependencies installed:

```bash
npm install react-router-dom reactflow
```

## Testing

1. Start your backend: `make start` or `./start.sh`
2. Start your frontend: `npm run dev`
3. Navigate to a workflow and click "Run"
4. After execution starts, click on a run in the history list
5. You should be taken to the RunDetail page showing:
   - Run metadata (ID, status, submitter, timestamp)
   - Execution graph with color-coded nodes
   - Node details tab with input/output for each node
   - Patches tab (if any patches were applied)
