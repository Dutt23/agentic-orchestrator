import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { AuthProvider } from './contexts/AuthContext';
import WorkflowList from './pages/WorkflowList';
import Flow from './pages/Flow';
import RunDetail from './pages/RunDetail';

function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          {/* Landing page - list of workflows */}
          <Route path="/" element={<WorkflowList />} />

          {/* Workflow detail page - owner comes from X-User-ID header */}
          <Route path="/workflow/:tag" element={<Flow />} />

          {/* Run detail page - shows execution details for a specific run */}
          <Route path="/runs/:runId" element={<RunDetail />} />

          {/* Redirect any unknown routes to home */}
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  );
}

export default App;
