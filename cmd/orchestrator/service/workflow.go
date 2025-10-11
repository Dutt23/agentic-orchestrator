package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lyzr/orchestrator/common/logger"
)

// WorkflowServiceV2 is a lightweight orchestrator for workflow operations
// It composes CAS, Artifact, and Tag services
type WorkflowServiceV2 struct {
	casService      *CASService
	artifactService *ArtifactService
	tagService      *TagService
	log             *logger.Logger
}

// NewWorkflowServiceV2 creates a new workflow service
func NewWorkflowServiceV2(
	casService *CASService,
	artifactService *ArtifactService,
	tagService *TagService,
	log *logger.Logger,
) *WorkflowServiceV2 {
	return &WorkflowServiceV2{
		casService:      casService,
		artifactService: artifactService,
		tagService:      tagService,
		log:             log,
	}
}

// CreateWorkflowRequest represents the input for creating a workflow
type CreateWorkflowRequest struct {
	TagName   string                 `json:"tag_name" validate:"required"`
	Workflow  map[string]interface{} `json:"workflow" validate:"required"`
	CreatedBy string                 `json:"created_by"`
}

// CreateWorkflowResponse represents the output after creating a workflow
type CreateWorkflowResponse struct {
	ArtifactID  uuid.UUID `json:"artifact_id"`
	CASID       string    `json:"cas_id"`
	VersionHash string    `json:"version_hash"`
	TagName     string    `json:"tag_name"`
	NodesCount  int       `json:"nodes_count"`
	EdgesCount  int       `json:"edges_count"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateWorkflow orchestrates workflow creation across services
func (s *WorkflowServiceV2) CreateWorkflow(ctx context.Context, req *CreateWorkflowRequest) (*CreateWorkflowResponse, error) {
	s.log.Info("creating workflow", "tag", req.TagName, "created_by", req.CreatedBy)

	// 1. Validate and serialize workflow
	workflowJSON, err := json.Marshal(req.Workflow)
	if err != nil {
		return nil, fmt.Errorf("invalid workflow JSON: %w", err)
	}

	// 2. Store in CAS (handles deduplication)
	casID, err := s.casService.StoreContent(ctx, workflowJSON, "application/json;type=dag")
	if err != nil {
		return nil, fmt.Errorf("failed to store workflow content: %w", err)
	}

	versionHash := casID // For DAG versions, version_hash = cas_id

	// 3. Check if artifact already exists for this version
	var artifactID uuid.UUID
	existingArtifact, err := s.artifactService.GetByVersionHash(ctx, versionHash)
	if err == nil {
		// Artifact exists, reuse it
		s.log.Info("artifact already exists", "artifact_id", existingArtifact.ArtifactID)
		artifactID = existingArtifact.ArtifactID
	} else {
		// Create new artifact
		nodesCount, edgesCount := CountWorkflowElements(req.Workflow)
		artifactID, err = s.artifactService.CreateDAGVersion(
			ctx,
			casID,
			versionHash,
			req.TagName,
			req.CreatedBy,
			nodesCount,
			edgesCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create artifact: %w", err)
		}
	}

	// 4. Create or move tag
	if err := s.tagService.CreateOrMoveTag(ctx, req.TagName, "dag_version", artifactID, versionHash, req.CreatedBy); err != nil {
		return nil, fmt.Errorf("failed to create/move tag: %w", err)
	}

	s.log.Info("workflow created successfully",
		"artifact_id", artifactID,
		"cas_id", casID,
		"tag", req.TagName,
	)

	nodesCount, edgesCount := CountWorkflowElements(req.Workflow)

	return &CreateWorkflowResponse{
		ArtifactID:  artifactID,
		CASID:       casID,
		VersionHash: versionHash,
		TagName:     req.TagName,
		NodesCount:  nodesCount,
		EdgesCount:  edgesCount,
		CreatedAt:   time.Now(),
	}, nil
}

// GetWorkflowByTag retrieves a workflow by tag name
func (s *WorkflowServiceV2) GetWorkflowByTag(ctx context.Context, tagName string) (map[string]interface{}, error) {
	// 1. Resolve tag
	tag, err := s.tagService.GetTag(ctx, tagName)
	if err != nil {
		return nil, err
	}

	// 2. Get artifact
	artifact, err := s.artifactService.GetByID(ctx, tag.TargetID)
	if err != nil {
		return nil, err
	}

	// 3. Get CAS content
	content, err := s.casService.GetContent(ctx, artifact.CasID)
	if err != nil {
		return nil, err
	}

	// 4. Unmarshal workflow
	var workflow map[string]interface{}
	if err := json.Unmarshal(content, &workflow); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow: %w", err)
	}

	return workflow, nil
}

// countWorkflowElements counts nodes and edges in a workflow
func CountWorkflowElements(workflow map[string]interface{}) (int, int) {
	nodesCount := 0
	edgesCount := 0

	// Count nodes
	if nodes, ok := workflow["nodes"].([]interface{}); ok {
		nodesCount = len(nodes)
	} else if nodes, ok := workflow["nodes"].(map[string]interface{}); ok {
		nodesCount = len(nodes)
	}

	// Count edges/dependencies
	if edges, ok := workflow["edges"].([]interface{}); ok {
		edgesCount = len(edges)
	} else if deps, ok := workflow["dependencies"].([]interface{}); ok {
		edgesCount = len(deps)
	}

	// Count dependencies within nodes
	if nodesMap, ok := workflow["nodes"].(map[string]interface{}); ok {
		for _, node := range nodesMap {
			if nodeMap, ok := node.(map[string]interface{}); ok {
				if deps, ok := nodeMap["dependencies"].([]interface{}); ok {
					edgesCount += len(deps)
				}
			}
		}
	}

	return nodesCount, edgesCount
}
