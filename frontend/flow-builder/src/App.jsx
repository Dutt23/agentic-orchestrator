import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { AuthProvider } from './contexts/AuthContext';
import WorkflowList from './pages/WorkflowList';
import Flow from './pages/Flow';

function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          {/* Landing page - list of workflows */}
          <Route path="/" element={<WorkflowList />} />

          {/* Workflow detail page */}
          <Route path="/workflow/:owner/:tag" element={<Flow />} />

          {/* Redirect any unknown routes to home */}
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  );
}

export default App;
