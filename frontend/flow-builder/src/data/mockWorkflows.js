// Mock workflow data for agent-based orchestration
// Simulates workflows with multiple branches and patch versions

export const mockWorkflows = {
  "document-analysis": {
    id: "document-analysis",
    name: "Document Analysis Workflow",
    description: "Analyze documents using AI agents",
    branches: {
      "main": {
        tag: "main",
        versions: [
          {
            version: "v1",
            timestamp: "2025-10-10T10:00:00Z",
            metadata: {
              name: "Document Analysis Workflow",
              description: "Analyze documents using AI agents",
              version: "v1",
              tags: ["main"]
            },
            nodes: [
              {
                id: "search-docs",
                type: "file-search",
                config: {
                  name: "Search Documents",
                  query: "*.pdf",
                  filters: { type: "document" }
                }
              },
              {
                id: "check-count",
                type: "conditional",
                config: {
                  name: "Check Document Count",
                  condition: "results.length > 0"
                }
              },
              {
                id: "analyze-agent",
                type: "agent",
                config: {
                  name: "Analyze Documents",
                  agent_id: "document-analyzer",
                  parameters: { mode: "detailed" }
                }
              },
              {
                id: "extract-insights",
                type: "transform",
                config: {
                  name: "Extract Key Insights",
                  operation: "map",
                  field: "insights"
                }
              }
            ],
            edges: [
              { from: "search-docs", to: "check-count" },
              { from: "check-count", to: "analyze-agent", condition: "has_documents" },
              { from: "analyze-agent", to: "extract-insights" }
            ]
          },
          {
            version: "v2",
            timestamp: "2025-10-11T14:30:00Z",
            metadata: {
              name: "Document Analysis Workflow",
              description: "Enhanced with human review",
              version: "v2",
              tags: ["main"]
            },
            nodes: [
              {
                id: "search-docs",
                type: "file-search",
                config: {
                  name: "Search Documents",
                  query: "*.pdf",
                  filters: { type: "document" }
                }
              },
              {
                id: "check-count",
                type: "conditional",
                config: {
                  name: "Check Document Count",
                  condition: "results.length > 0"
                }
              },
              {
                id: "analyze-agent",
                type: "agent",
                config: {
                  name: "Analyze Documents",
                  agent_id: "document-analyzer",
                  parameters: { mode: "detailed" }
                }
              },
              {
                id: "human-review",
                type: "hitl",
                config: {
                  name: "Review Analysis",
                  message: "Please review the document analysis",
                  timeout_ms: 86400000
                }
              },
              {
                id: "extract-insights",
                type: "transform",
                config: {
                  name: "Extract Key Insights",
                  operation: "map",
                  field: "insights"
                }
              }
            ],
            edges: [
              { from: "search-docs", to: "check-count" },
              { from: "check-count", to: "analyze-agent", condition: "has_documents" },
              { from: "analyze-agent", to: "human-review" },
              { from: "human-review", to: "extract-insights" }
            ]
          }
        ]
      },
      "staging": {
        tag: "staging",
        versions: [
          {
            version: "v1",
            timestamp: "2025-10-13T10:00:00Z",
            metadata: {
              name: "Document Analysis Workflow",
              description: "Staging - with filtering",
              version: "v1",
              tags: ["staging"]
            },
            nodes: [
              {
                id: "search-docs",
                type: "file-search",
                config: {
                  name: "Search Documents",
                  query: "*.pdf",
                  filters: { type: "document" }
                }
              },
              {
                id: "filter-docs",
                type: "filter",
                config: {
                  name: "Filter Large Documents",
                  condition: "size > 1000000"
                }
              },
              {
                id: "analyze-agent",
                type: "agent",
                config: {
                  name: "Analyze Documents",
                  agent_id: "document-analyzer",
                  parameters: { mode: "quick" }
                }
              },
              {
                id: "aggregate-results",
                type: "aggregate",
                config: {
                  name: "Aggregate Analysis",
                  operation: "merge"
                }
              }
            ],
            edges: [
              { from: "search-docs", to: "filter-docs" },
              { from: "filter-docs", to: "analyze-agent" },
              { from: "analyze-agent", to: "aggregate-results" }
            ]
          }
        ]
      }
    }
  },
  "code-review": {
    id: "code-review",
    name: "Code Review Workflow",
    description: "AI-powered code review process",
    branches: {
      "main": {
        tag: "main",
        versions: [
          {
            version: "v1",
            timestamp: "2025-10-09T12:00:00Z",
            metadata: {
              name: "Code Review Workflow",
              description: "AI-powered code review process",
              version: "v1",
              tags: ["main"]
            },
            nodes: [
              {
                id: "search-files",
                type: "file-search",
                config: {
                  name: "Find Changed Files",
                  query: "*.{js,ts,jsx,tsx}",
                  filters: { status: "modified" }
                }
              },
              {
                id: "review-agent",
                type: "agent",
                config: {
                  name: "AI Code Reviewer",
                  agent_id: "code-reviewer",
                  parameters: { strict: true }
                }
              },
              {
                id: "check-issues",
                type: "conditional",
                config: {
                  name: "Check for Issues",
                  condition: "issues.length > 0"
                }
              },
              {
                id: "human-approval",
                type: "hitl",
                config: {
                  name: "Senior Dev Review",
                  message: "Issues found - please review",
                  timeout_ms: 3600000
                }
              }
            ],
            edges: [
              { from: "search-files", to: "review-agent" },
              { from: "review-agent", to: "check-issues" },
              { from: "check-issues", to: "human-approval", condition: "has_issues" }
            ]
          }
        ]
      }
    }
  },
  "data-pipeline": {
    id: "data-pipeline",
    name: "Data Processing Pipeline",
    description: "Transform and aggregate data",
    branches: {
      "main": {
        tag: "main",
        versions: [
          {
            version: "v1",
            timestamp: "2025-10-08T09:00:00Z",
            metadata: {
              name: "Data Processing Pipeline",
              description: "Transform and aggregate data",
              version: "v1",
              tags: ["main"]
            },
            nodes: [
              {
                id: "search-data",
                type: "file-search",
                config: {
                  name: "Find Data Files",
                  query: "*.json",
                  filters: {}
                }
              },
              {
                id: "process-loop",
                type: "loop",
                config: {
                  name: "Process Each File",
                  iterations: 0,
                  condition: "has_more_files"
                }
              },
              {
                id: "transform-data",
                type: "transform",
                config: {
                  name: "Transform Records",
                  operation: "map",
                  field: "records"
                }
              },
              {
                id: "filter-valid",
                type: "filter",
                config: {
                  name: "Filter Valid Records",
                  condition: "valid === true"
                }
              },
              {
                id: "aggregate-results",
                type: "aggregate",
                config: {
                  name: "Combine Results",
                  operation: "merge"
                }
              }
            ],
            edges: [
              { from: "search-data", to: "process-loop" },
              { from: "process-loop", to: "transform-data" },
              { from: "transform-data", to: "filter-valid" },
              { from: "filter-valid", to: "aggregate-results" }
            ]
          }
        ]
      }
    }
  }
};

// Helper function to get workflow by ID
export const getWorkflowById = (workflowId) => {
  return mockWorkflows[workflowId] || null;
};

// Helper function to get all workflow IDs and names
export const getAllWorkflows = () => {
  return Object.entries(mockWorkflows).map(([id, workflow]) => ({
    id,
    name: workflow.name,
    description: workflow.description
  }));
};

// Helper function to get branches for a workflow
export const getBranches = (workflowId) => {
  const workflow = mockWorkflows[workflowId];
  if (!workflow) return [];
  return Object.keys(workflow.branches).map(tag => ({
    tag,
    versionsCount: workflow.branches[tag].versions.length
  }));
};

// Helper function to get specific version
export const getVersion = (workflowId, branch, versionIndex) => {
  const workflow = mockWorkflows[workflowId];
  if (!workflow || !workflow.branches[branch]) return null;
  return workflow.branches[branch].versions[versionIndex] || null;
};

// Helper function to get latest version for a branch
export const getLatestVersion = (workflowId, branch) => {
  const workflow = mockWorkflows[workflowId];
  if (!workflow || !workflow.branches[branch]) return null;
  const versions = workflow.branches[branch].versions;
  return versions[versions.length - 1];
};
