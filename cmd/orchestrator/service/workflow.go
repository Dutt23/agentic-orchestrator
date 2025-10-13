package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lyzr/orchestrator/cmd/orchestrator/models"
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
	Username  string                 `json:"username" validate:"required"`
	TagName   string                 `json:"tag_name" validate:"required"`
	Workflow  map[string]interface{} `json:"workflow" validate:"required"`
	CreatedBy string                 `json:"created_by"`
}

// CreateWorkflowResponse represents the output after creating a workflow
type CreateWorkflowResponse struct {
	ArtifactID  uuid.UUID `json:"artifact_id"`
	CASID       string    `json:"cas_id"`
	VersionHash string    `json:"version_hash"`
	Username    string    `json:"username"`
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
	if err := s.tagService.CreateOrMoveTag(ctx, req.Username, req.TagName, "dag_version", artifactID, versionHash, req.CreatedBy); err != nil {
		return nil, fmt.Errorf("failed to create/move tag: %w", err)
	}

	s.log.Info("workflow created successfully",
		"artifact_id", artifactID,
		"cas_id", casID,
		"username", req.Username,
		"tag", req.TagName,
	)

	nodesCount, edgesCount := CountWorkflowElements(req.Workflow)

	return &CreateWorkflowResponse{
		ArtifactID:  artifactID,
		CASID:       casID,
		VersionHash: versionHash,
		Username:    req.Username,
		TagName:     req.TagName,
		NodesCount:  nodesCount,
		EdgesCount:  edgesCount,
		CreatedAt:   time.Now(),
	}, nil
}

// CreatePatchRequest represents the input for creating a patch
type CreatePatchRequest struct {
	Username    string                   `json:"username" validate:"required"`
	TagName     string                   `json:"tag_name" validate:"required"`
	Operations  []map[string]interface{} `json:"operations" validate:"required"`
	Description string                   `json:"description"`
	CreatedBy   string                   `json:"created_by"`
}

// CreatePatchResponse represents the output after creating a patch
type CreatePatchResponse struct {
	ArtifactID  uuid.UUID `json:"artifact_id"`
	CASID       string    `json:"cas_id"`
	Depth       int       `json:"depth"`
	OpCount     int       `json:"op_count"`
	Username    string    `json:"username"`
	TagName     string    `json:"tag_name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreatePatch creates a new patch artifact and updates the tag
func (s *WorkflowServiceV2) CreatePatch(ctx context.Context, req *CreatePatchRequest) (*CreatePatchResponse, error) {
	s.log.Info("creating patch", "tag", req.TagName, "op_count", len(req.Operations), "created_by", req.CreatedBy)

	// 1. Resolve current tag to get current artifact
	currentArtifact, err := s.resolveTagToArtifact(ctx, req.Username, req.TagName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve tag: %w", err)
	}

	// 2. Determine base version and previous patch set
	var baseVersionID uuid.UUID
	var previousPatchSetID *uuid.UUID
	var newDepth int

	if currentArtifact.IsDAGVersion() {
		// Current is a DAG version, it becomes the base
		baseVersionID = currentArtifact.ArtifactID
		previousPatchSetID = nil
		newDepth = 1
	} else if currentArtifact.IsPatchSet() {
		// Current is a patch set, use its base version
		if currentArtifact.BaseVersion == nil {
			return nil, fmt.Errorf("patch_set missing base_version")
		}
		baseVersionID = *currentArtifact.BaseVersion
		previousPatchSetID = &currentArtifact.ArtifactID
		if currentArtifact.Depth != nil {
			newDepth = *currentArtifact.Depth + 1
		} else {
			newDepth = 1
		}
	} else {
		return nil, fmt.Errorf("unsupported artifact kind: %s", currentArtifact.Kind)
	}

	// 3. Serialize patch operations
	patchJSON, err := json.Marshal(req.Operations)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize patch operations: %w", err)
	}

	// 4. Store patch in CAS
	casID, err := s.casService.StoreContent(ctx, patchJSON, "application/json;type=patch")
	if err != nil {
		return nil, fmt.Errorf("failed to store patch content: %w", err)
	}

	// 5. Create patch artifact
	opCount := len(req.Operations)
	patchArtifactID, err := s.artifactService.CreatePatch(
		ctx,
		casID,
		baseVersionID,
		previousPatchSetID,
		newDepth,
		opCount,
		req.CreatedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create patch artifact: %w", err)
	}

	// 6. Move tag to new patch artifact
	// For patches, version_hash is the patch's cas_id
	if err := s.tagService.CreateOrMoveTag(ctx, req.Username, req.TagName, "patch_set", patchArtifactID, casID, req.CreatedBy); err != nil {
		return nil, fmt.Errorf("failed to move tag: %w", err)
	}

	s.log.Info("patch created successfully",
		"artifact_id", patchArtifactID,
		"cas_id", casID,
		"depth", newDepth,
		"username", req.Username,
		"tag", req.TagName,
	)

	return &CreatePatchResponse{
		ArtifactID:  patchArtifactID,
		CASID:       casID,
		Depth:       newDepth,
		OpCount:     opCount,
		Username:    req.Username,
		TagName:     req.TagName,
		Description: req.Description,
		CreatedAt:   time.Now(),
	}, nil
}

// GetWorkflowByTag retrieves a workflow by tag name
// NOTE: This function is incomplete and not currently used
// TODO: Update to accept username parameter
func (s *WorkflowServiceV2) GetWorkflowByTag(ctx context.Context, username, tagName string) (map[string]interface{}, error) {
	// 1. Resolve tag
	tag, err := s.tagService.GetTag(ctx, username, tagName)
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

// GetWorkflowComponents fetches all components needed to reconstruct a workflow
// Implements the 4-query pattern:
// 1. Resolve tag → artifact
// 2. Get patch chain (if patch_set)
// 3. Load base DAG from CAS
// 4. Load all patches from CAS
func (s *WorkflowServiceV2) GetWorkflowComponents(ctx context.Context, username, tagName string) (*models.WorkflowComponents, error) {
	s.log.Info("fetching workflow components", "username", username, "tag", tagName)

	// Query 1: Resolve tag to artifact
	artifact, err := s.resolveTagToArtifact(ctx, username, tagName)
	if err != nil {
		return nil, err
	}

	components := s.initializeComponents(username, tagName, artifact)

	// Handle based on artifact kind
	if artifact.IsDAGVersion() {
		if err := s.loadDAGVersionComponents(ctx, artifact, components); err != nil {
			return nil, err
		}
	} else if artifact.IsPatchSet() {
		if err := s.loadPatchSetComponents(ctx, artifact, components); err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("unsupported artifact kind: %s", artifact.Kind)
	}

	s.log.Info("workflow components fetched successfully",
		"kind", components.Kind,
		"depth", components.Depth,
		"patch_count", components.PatchCount,
	)

	return components, nil
}

// resolveTagToArtifact resolves a tag name to its artifact (Query 1)
func (s *WorkflowServiceV2) resolveTagToArtifact(ctx context.Context, username, tagName string) (*models.Artifact, error) {
	tag, err := s.tagService.GetTag(ctx, username, tagName)
	if err != nil {
		return nil, fmt.Errorf("tag not found: %w", err)
	}

	artifact, err := s.artifactService.GetByID(ctx, tag.TargetID)
	if err != nil {
		return nil, fmt.Errorf("artifact not found: %w", err)
	}

	return artifact, nil
}

// initializeComponents creates the base components structure
func (s *WorkflowServiceV2) initializeComponents(username, tagName string, artifact *models.Artifact) *models.WorkflowComponents {
	return &models.WorkflowComponents{
		Username:   username,
		TagName:    tagName,
		ArtifactID: artifact.ArtifactID,
		Kind:       artifact.Kind,
		CreatedAt:  artifact.CreatedAt,
		CreatedBy:  &artifact.CreatedBy,
	}
}

// loadDAGVersionComponents loads components for a simple DAG version (no patches)
func (s *WorkflowServiceV2) loadDAGVersionComponents(ctx context.Context, artifact *models.Artifact, components *models.WorkflowComponents) error {
	s.log.Info("workflow is a dag_version", "artifact_id", artifact.ArtifactID)

	components.BaseCASID = artifact.CasID
	components.Depth = 0
	components.PatchCount = 0

	if artifact.VersionHash != nil {
		components.BaseVersionHash = *artifact.VersionHash
	}

	// Query 3: Load base DAG content
	content, err := s.casService.GetContent(ctx, artifact.CasID)
	if err != nil {
		return fmt.Errorf("failed to load base DAG content: %w", err)
	}
	components.BaseContent = content

	return nil
}

// loadPatchSetComponents loads components for a patch set with chain
func (s *WorkflowServiceV2) loadPatchSetComponents(ctx context.Context, artifact *models.Artifact, components *models.WorkflowComponents) error {
	s.log.Info("workflow is a patch_set", "artifact_id", artifact.ArtifactID, "depth", artifact.Depth)

	if artifact.BaseVersion == nil {
		return fmt.Errorf("patch_set artifact missing base_version")
	}

	components.BaseVersion = artifact.BaseVersion
	if artifact.Depth != nil {
		components.Depth = *artifact.Depth
	}

	// Query 2: Get patch chain
	patchArtifacts, err := s.artifactService.GetPatchChain(ctx, artifact.ArtifactID)
	if err != nil {
		return fmt.Errorf("failed to get patch chain: %w", err)
	}

	components.PatchCount = len(patchArtifacts)
	s.log.Info("loaded patch chain", "patch_count", components.PatchCount)

	// Load base DAG
	if err := s.loadBaseDAG(ctx, artifact, components); err != nil {
		return err
	}

	// Query 4: Load all patches from CAS
	if err := s.loadPatchChain(ctx, patchArtifacts, components); err != nil {
		return err
	}

	return nil
}

// loadBaseDAG loads the base DAG artifact and content for a patch set
func (s *WorkflowServiceV2) loadBaseDAG(ctx context.Context, artifact *models.Artifact, components *models.WorkflowComponents) error {
	// Get base DAG artifact
	baseArtifact, err := s.artifactService.GetByID(ctx, *artifact.BaseVersion)
	if err != nil {
		return fmt.Errorf("failed to get base artifact: %w", err)
	}

	components.BaseCASID = baseArtifact.CasID
	if baseArtifact.VersionHash != nil {
		components.BaseVersionHash = *baseArtifact.VersionHash
	}

	// Load base DAG content
	baseContent, err := s.casService.GetContent(ctx, baseArtifact.CasID)
	if err != nil {
		return fmt.Errorf("failed to load base DAG content: %w", err)
	}
	components.BaseContent = baseContent

	return nil
}

// loadPatchChain loads all patch contents from CAS using a single bulk query
func (s *WorkflowServiceV2) loadPatchChain(ctx context.Context, patchArtifacts []*models.Artifact, components *models.WorkflowComponents) error {
	if len(patchArtifacts) == 0 {
		components.PatchChain = []models.PatchInfo{}
		return nil
	}

	// Collect all CAS IDs for bulk fetch
	casIDs := make([]string, 0, len(patchArtifacts))
	for _, patchArt := range patchArtifacts {
		casIDs = append(casIDs, patchArt.CasID)
	}

	// Bulk fetch all patch contents in a single query
	s.log.Info("bulk loading patch chain", "patch_count", len(casIDs))
	contentsMap, err := s.casService.GetContentBulk(ctx, casIDs)
	if err != nil {
		return fmt.Errorf("failed to bulk load patch contents: %w", err)
	}

	// Build patch chain with fetched contents
	components.PatchChain = make([]models.PatchInfo, 0, len(patchArtifacts))
	for i, patchArt := range patchArtifacts {
		content, found := contentsMap[patchArt.CasID]
		if !found {
			return fmt.Errorf("patch %d content not found (cas_id: %s)", i+1, patchArt.CasID)
		}

		patchInfo := models.PatchInfo{
			Seq:        i + 1, // 1-indexed
			ArtifactID: patchArt.ArtifactID,
			CASID:      patchArt.CasID,
			OpCount:    patchArt.OpCount,
			Content:    content,
			CreatedAt:  patchArt.CreatedAt,
			CreatedBy:  &patchArt.CreatedBy,
		}

		if patchArt.Depth != nil {
			patchInfo.Depth = *patchArt.Depth
		}

		components.PatchChain = append(components.PatchChain, patchInfo)
	}

	s.log.Info("patch chain loaded successfully", "patch_count", len(components.PatchChain))
	return nil
}

// GetWorkflowComponentsAtVersion fetches workflow components up to a specific version
// seq indexing:
//   - seq=0: returns base DAG version only (no patches)
//   - seq=1: returns base + patch 1
//   - seq=2: returns base + patches 1,2
//   - etc.
func (s *WorkflowServiceV2) GetWorkflowComponentsAtVersion(ctx context.Context, username, tagName string, seq int) (*models.WorkflowComponents, error) {
	s.log.Info("fetching workflow components at version", "username", username, "tag", tagName, "seq", seq)

	if seq < 0 {
		return nil, fmt.Errorf("invalid seq: must be >= 0")
	}

	// Query 1: Resolve tag to artifact
	artifact, err := s.resolveTagToArtifact(ctx, username, tagName)
	if err != nil {
		return nil, err
	}

	components := s.initializeComponents(username, tagName, artifact)

	// Handle based on artifact kind
	if artifact.IsDAGVersion() {
		// DAG version has no patches, seq must be 0 or 1 (both return same thing)
		if seq > 1 {
			return nil, fmt.Errorf("dag_version only supports seq=0 or seq=1, requested seq=%d", seq)
		}
		if err := s.loadDAGVersionComponents(ctx, artifact, components); err != nil {
			return nil, err
		}
	} else if artifact.IsPatchSet() {
		// Special case: seq=0 means return just the base version
		if seq == 0 {
			if err := s.loadBaseVersionOnly(ctx, artifact, components); err != nil {
				return nil, err
			}
		} else {
			// seq >= 1: apply patches up to seq
			if err := s.loadPatchSetComponentsAtVersion(ctx, artifact, components, seq); err != nil {
				return nil, err
			}
		}
	} else {
		return nil, fmt.Errorf("unsupported artifact kind: %s", artifact.Kind)
	}

	s.log.Info("workflow components at version fetched successfully",
		"kind", components.Kind,
		"depth", components.Depth,
		"patch_count", components.PatchCount,
		"requested_seq", seq,
	)

	return components, nil
}

// loadBaseVersionOnly loads only the base DAG version without any patches (for seq=0)
func (s *WorkflowServiceV2) loadBaseVersionOnly(ctx context.Context, artifact *models.Artifact, components *models.WorkflowComponents) error {
	s.log.Info("loading base version only (seq=0)", "artifact_id", artifact.ArtifactID)

	if artifact.BaseVersion == nil {
		return fmt.Errorf("patch_set artifact missing base_version")
	}

	components.BaseVersion = artifact.BaseVersion
	components.Depth = 0        // Base version has depth 0
	components.PatchCount = 0   // No patches for version 0

	// Load base DAG
	if err := s.loadBaseDAG(ctx, artifact, components); err != nil {
		return err
	}

	// No patches to load
	components.PatchChain = []models.PatchInfo{}

	return nil
}

// loadPatchSetComponentsAtVersion loads components for a patch set up to a specific version
func (s *WorkflowServiceV2) loadPatchSetComponentsAtVersion(ctx context.Context, artifact *models.Artifact, components *models.WorkflowComponents, seq int) error {
	s.log.Info("workflow is a patch_set, loading up to version", "artifact_id", artifact.ArtifactID, "seq", seq)

	if artifact.BaseVersion == nil {
		return fmt.Errorf("patch_set artifact missing base_version")
	}

	components.BaseVersion = artifact.BaseVersion
	if artifact.Depth != nil {
		components.Depth = *artifact.Depth
	}

	// Query 2: Get full patch chain
	patchArtifacts, err := s.artifactService.GetPatchChain(ctx, artifact.ArtifactID)
	if err != nil {
		return fmt.Errorf("failed to get patch chain: %w", err)
	}

	// Validate seq is within bounds
	if seq > len(patchArtifacts) {
		return fmt.Errorf("requested seq %d exceeds patch chain length %d", seq, len(patchArtifacts))
	}

	// Take only patches up to seq
	patchArtifacts = patchArtifacts[:seq]
	components.PatchCount = len(patchArtifacts)
	s.log.Info("loaded partial patch chain", "requested_seq", seq, "patch_count", components.PatchCount)

	// Load base DAG
	if err := s.loadBaseDAG(ctx, artifact, components); err != nil {
		return err
	}

	// Query 4: Load patches up to seq from CAS
	if err := s.loadPatchChain(ctx, patchArtifacts, components); err != nil {
		return err
	}

	return nil
}
