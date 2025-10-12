package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lyzr/orchestrator/cmd/orchestrator/models"
	"github.com/lyzr/orchestrator/cmd/orchestrator/repository"
	"github.com/lyzr/orchestrator/common/logger"
)

// TagService handles tag operations
type TagService struct {
	repo *repository.TagRepository
	log  *logger.Logger
}

// NewTagService creates a new tag service
func NewTagService(repo *repository.TagRepository, log *logger.Logger) *TagService {
	return &TagService{
		repo: repo,
		log:  log,
	}
}

// CreateTag creates a new tag pointing to an artifact
func (s *TagService) CreateTag(ctx context.Context, tagName string, targetKind models.ArtifactKind, targetID uuid.UUID, targetHash, movedBy string) error {
	// Check if tag already exists
	exists, err := s.repo.Exists(ctx, tagName)
	if err != nil {
		return fmt.Errorf("failed to check tag existence: %w", err)
	}

	if exists {
		return fmt.Errorf("tag already exists: %s", tagName)
	}

	tag := &models.Tag{
		TagName:    tagName,
		TargetKind: targetKind,
		TargetID:   targetID,
		TargetHash: &targetHash,
		Version:    1,
		MovedBy:    &movedBy,
		MovedAt:    time.Now(),
	}

	if err := s.repo.Create(ctx, tag); err != nil {
		return fmt.Errorf("failed to create tag: %w", err)
	}

	s.log.Info("created tag",
		"tag", tagName,
		"target_id", targetID,
		"target_kind", targetKind,
	)

	return nil
}

// MoveTag moves a tag to a new target
func (s *TagService) MoveTag(ctx context.Context, tagName string, targetKind models.ArtifactKind, targetID uuid.UUID, targetHash, movedBy string) error {
	tag := &models.Tag{
		TagName:    tagName,
		TargetKind: targetKind,
		TargetID:   targetID,
		TargetHash: &targetHash,
		MovedBy:    &movedBy,
		MovedAt:    time.Now(),
	}

	if err := s.repo.Update(ctx, tag); err != nil {
		return fmt.Errorf("failed to move tag: %w", err)
	}

	s.log.Info("moved tag",
		"tag", tagName,
		"target_id", targetID,
		"target_kind", targetKind,
		"version", tag.Version,
	)

	return nil
}

// CreateOrMoveTag creates a new tag or moves an existing one
func (s *TagService) CreateOrMoveTag(ctx context.Context, tagName string, targetKind models.ArtifactKind, targetID uuid.UUID, targetHash, movedBy string) error {
	exists, err := s.repo.Exists(ctx, tagName)
	if err != nil {
		return fmt.Errorf("failed to check tag existence: %w", err)
	}

	if exists {
		return s.MoveTag(ctx, tagName, targetKind, targetID, targetHash, movedBy)
	}

	return s.CreateTag(ctx, tagName, targetKind, targetID, targetHash, movedBy)
}

// GetTag retrieves a tag by name
func (s *TagService) GetTag(ctx context.Context, tagName string) (*models.Tag, error) {
	tag, err := s.repo.GetByName(ctx, tagName)
	if err != nil {
		return nil, fmt.Errorf("tag not found: %w", err)
	}

	return tag, nil
}

// ListTags lists all tags
func (s *TagService) ListTags(ctx context.Context) ([]*models.Tag, error) {
	tags, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}

	return tags, nil
}

// DeleteTag deletes a tag
func (s *TagService) DeleteTag(ctx context.Context, tagName string) error {
	if err := s.repo.Delete(ctx, tagName); err != nil {
		return fmt.Errorf("failed to delete tag: %w", err)
	}

	s.log.Info("deleted tag", "tag", tagName)
	return nil
}

// GetHistory retrieves the tag move history
func (s *TagService) GetHistory(ctx context.Context, tagName string, limit int) ([]*models.TagMove, error) {
	history, err := s.repo.GetHistory(ctx, tagName, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get tag history: %w", err)
	}

	return history, nil
}

// CompareAndSwap performs an optimistic lock update
func (s *TagService) CompareAndSwap(ctx context.Context, tagName string, expectedVersion int64, newTarget uuid.UUID, newTargetKind models.ArtifactKind, newTargetHash, movedBy string) (bool, error) {
	success, err := s.repo.CompareAndSwap(ctx, tagName, expectedVersion, newTarget, string(newTargetKind), newTargetHash, movedBy)
	if err != nil {
		return false, fmt.Errorf("CAS operation failed: %w", err)
	}

	if success {
		s.log.Info("CAS operation succeeded",
			"tag", tagName,
			"expected_version", expectedVersion,
			"new_target", newTarget,
		)
	} else {
		s.log.Warn("CAS operation failed - version mismatch",
			"tag", tagName,
			"expected_version", expectedVersion,
		)
	}

	return success, nil
}

// ============================================================================
// Namespace-Aware Methods (User-Facing API)
// ============================================================================
// These methods handle automatic username prefixing and validation.
// Use these methods in API handlers for user-facing operations.

// CreateTagWithNamespace creates a tag with automatic username prefixing
// User provides: "main" → Internally stored as: "username/main"
func (s *TagService) CreateTagWithNamespace(ctx context.Context, userTag, username string, targetKind models.ArtifactKind, targetID uuid.UUID, targetHash string) error {
	// Validate user-provided tag name
	if errMsg := ValidateUserTagName(userTag); errMsg != "" {
		return fmt.Errorf("invalid tag name: %s", errMsg)
	}

	// Build internal tag name with namespace
	internalTag := BuildInternalTagName(username, userTag)

	// Use existing CreateTag logic
	return s.CreateTag(ctx, internalTag, targetKind, targetID, targetHash, username)
}

// GetTagWithNamespace retrieves a tag with automatic namespace lookup
// User requests: "main" → Internally looks for: "username/main"
func (s *TagService) GetTagWithNamespace(ctx context.Context, userTag, username string) (*models.Tag, error) {
	// Validate user-provided tag name
	if errMsg := ValidateUserTagName(userTag); errMsg != "" {
		return nil, fmt.Errorf("invalid tag name: %s", errMsg)
	}

	// Build internal tag name with namespace
	internalTag := BuildInternalTagName(username, userTag)

	// Use existing GetTag logic
	tag, err := s.GetTag(ctx, internalTag)
	if err != nil {
		return nil, err
	}

	// Verify user has access
	if !CanAccessTag(tag.TagName, username) {
		return nil, fmt.Errorf("access denied: tag belongs to another user")
	}

	return tag, nil
}

// MoveTagWithNamespace moves a tag with automatic namespace lookup
func (s *TagService) MoveTagWithNamespace(ctx context.Context, userTag, username string, targetKind models.ArtifactKind, targetID uuid.UUID, targetHash string) error {
	// Validate user-provided tag name
	if errMsg := ValidateUserTagName(userTag); errMsg != "" {
		return fmt.Errorf("invalid tag name: %s", errMsg)
	}

	// Build internal tag name with namespace
	internalTag := BuildInternalTagName(username, userTag)

	// Verify user owns this tag before moving
	existingTag, err := s.GetTag(ctx, internalTag)
	if err != nil {
		return fmt.Errorf("tag not found: %w", err)
	}

	if !IsUserTag(existingTag.TagName, username) && !IsGlobalTag(existingTag.TagName) {
		return fmt.Errorf("access denied: cannot move tag owned by another user")
	}

	// Use existing MoveTag logic
	return s.MoveTag(ctx, internalTag, targetKind, targetID, targetHash, username)
}

// CreateOrMoveTagWithNamespace creates or moves a tag with automatic namespace
func (s *TagService) CreateOrMoveTagWithNamespace(ctx context.Context, userTag, username string, targetKind models.ArtifactKind, targetID uuid.UUID, targetHash string) error {
	// Validate user-provided tag name
	if errMsg := ValidateUserTagName(userTag); errMsg != "" {
		return fmt.Errorf("invalid tag name: %s", errMsg)
	}

	// Build internal tag name with namespace
	internalTag := BuildInternalTagName(username, userTag)

	// Use existing CreateOrMoveTag logic
	return s.CreateOrMoveTag(ctx, internalTag, targetKind, targetID, targetHash, username)
}

// DeleteTagWithNamespace deletes a tag with automatic namespace lookup
func (s *TagService) DeleteTagWithNamespace(ctx context.Context, userTag, username string) error {
	// Validate user-provided tag name
	if errMsg := ValidateUserTagName(userTag); errMsg != "" {
		return fmt.Errorf("invalid tag name: %s", errMsg)
	}

	// Build internal tag name with namespace
	internalTag := BuildInternalTagName(username, userTag)

	// Verify user owns this tag before deleting
	existingTag, err := s.GetTag(ctx, internalTag)
	if err != nil {
		return fmt.Errorf("tag not found: %w", err)
	}

	if !IsUserTag(existingTag.TagName, username) {
		return fmt.Errorf("access denied: cannot delete tag owned by another user")
	}

	// Use existing DeleteTag logic
	return s.DeleteTag(ctx, internalTag)
}

// ListUserTags returns tags belonging to a specific user
// Returns only the user's tags (uses indexed LIKE query for efficiency)
func (s *TagService) ListUserTags(ctx context.Context, username string) ([]*models.Tag, error) {
	// Use indexed prefix query instead of fetching all and filtering
	prefix := ListUserTagPrefix(username)
	userTags, err := s.repo.ListByPrefix(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list user tags: %w", err)
	}

	s.log.Info("listed user tags", "username", username, "count", len(userTags))
	return userTags, nil
}

// ListGlobalTags returns system-wide shared tags
func (s *TagService) ListGlobalTags(ctx context.Context) ([]*models.Tag, error) {
	// Use indexed prefix query instead of fetching all and filtering
	globalPrefix := ListUserTagPrefix("") // Returns "_global_/"
	globalTags, err := s.repo.ListByPrefix(ctx, globalPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list global tags: %w", err)
	}

	s.log.Info("listed global tags", "count", len(globalTags))
	return globalTags, nil
}

// ListAllAccessibleTags returns all tags the user can access (their own + global)
// Uses two indexed queries instead of fetching all and filtering
func (s *TagService) ListAllAccessibleTags(ctx context.Context, username string) ([]*models.Tag, error) {
	// Get user's tags with indexed query
	userPrefix := ListUserTagPrefix(username)
	userTags, err := s.repo.ListByPrefix(ctx, userPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list user tags: %w", err)
	}

	// Get global tags with indexed query
	globalPrefix := ListUserTagPrefix("") // "_global_/"
	globalTags, err := s.repo.ListByPrefix(ctx, globalPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list global tags: %w", err)
	}

	// Combine results
	accessibleTags := make([]*models.Tag, 0, len(userTags)+len(globalTags))
	accessibleTags = append(accessibleTags, userTags...)
	accessibleTags = append(accessibleTags, globalTags...)

	s.log.Info("listed accessible tags", "username", username, "count", len(accessibleTags))
	return accessibleTags, nil
}

// GetHistoryWithNamespace retrieves tag history with namespace lookup
func (s *TagService) GetHistoryWithNamespace(ctx context.Context, userTag, username string, limit int) ([]*models.TagMove, error) {
	// Validate user-provided tag name
	if errMsg := ValidateUserTagName(userTag); errMsg != "" {
		return nil, fmt.Errorf("invalid tag name: %s", errMsg)
	}

	// Build internal tag name with namespace
	internalTag := BuildInternalTagName(username, userTag)

	// Use existing GetHistory logic
	return s.GetHistory(ctx, internalTag, limit)
}
