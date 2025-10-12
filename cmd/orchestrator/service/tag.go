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
func (s *TagService) CreateTag(ctx context.Context, username, tagName string, targetKind models.ArtifactKind, targetID uuid.UUID, targetHash, createdBy string) error {
	// Check if tag already exists
	exists, err := s.repo.Exists(ctx, username, tagName)
	if err != nil {
		return fmt.Errorf("failed to check tag existence: %w", err)
	}

	if exists {
		return fmt.Errorf("tag already exists: %s/%s", username, tagName)
	}

	tag := &models.Tag{
		Username:   username,
		TagName:    tagName,
		TargetKind: targetKind,
		TargetID:   targetID,
		TargetHash: &targetHash,
		Version:    1,
		CreatedBy:  &createdBy,
		MovedBy:    &createdBy,
		MovedAt:    time.Now(),
	}

	if err := s.repo.Create(ctx, tag); err != nil {
		return fmt.Errorf("failed to create tag: %w", err)
	}

	s.log.Info("created tag",
		"username", username,
		"tag", tagName,
		"target_id", targetID,
		"target_kind", targetKind,
	)

	return nil
}

// MoveTag moves a tag to a new target
func (s *TagService) MoveTag(ctx context.Context, username, tagName string, targetKind models.ArtifactKind, targetID uuid.UUID, targetHash, movedBy string) error {
	tag := &models.Tag{
		Username:   username,
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
		"username", username,
		"tag", tagName,
		"target_id", targetID,
		"target_kind", targetKind,
		"version", tag.Version,
	)

	return nil
}

// CreateOrMoveTag creates a new tag or moves an existing one
func (s *TagService) CreateOrMoveTag(ctx context.Context, username, tagName string, targetKind models.ArtifactKind, targetID uuid.UUID, targetHash, userIdentity string) error {
	exists, err := s.repo.Exists(ctx, username, tagName)
	if err != nil {
		return fmt.Errorf("failed to check tag existence: %w", err)
	}

	if exists {
		return s.MoveTag(ctx, username, tagName, targetKind, targetID, targetHash, userIdentity)
	}

	return s.CreateTag(ctx, username, tagName, targetKind, targetID, targetHash, userIdentity)
}

// GetTag retrieves a tag by username and name
func (s *TagService) GetTag(ctx context.Context, username, tagName string) (*models.Tag, error) {
	tag, err := s.repo.GetByName(ctx, username, tagName)
	if err != nil {
		return nil, fmt.Errorf("tag not found: %w", err)
	}

	return tag, nil
}

// ListUserTags returns tags belonging to a specific user
// Uses exact username match (secure - no LIKE query!)
func (s *TagService) ListUserTags(ctx context.Context, username string) ([]*models.Tag, error) {
	userTags, err := s.repo.ListByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to list user tags: %w", err)
	}

	s.log.Info("listed user tags", "username", username, "count", len(userTags))
	return userTags, nil
}

// ListGlobalTags returns system-wide shared tags
func (s *TagService) ListGlobalTags(ctx context.Context) ([]*models.Tag, error) {
	globalTags, err := s.repo.ListByUsername(ctx, GlobalUsername)
	if err != nil {
		return nil, fmt.Errorf("failed to list global tags: %w", err)
	}

	s.log.Info("listed global tags", "count", len(globalTags))
	return globalTags, nil
}

// ListAllAccessibleTags returns all tags the user can access (their own + global)
// Uses two exact match queries (secure!)
func (s *TagService) ListAllAccessibleTags(ctx context.Context, username string) ([]*models.Tag, error) {
	// Get user's tags with exact username match
	userTags, err := s.repo.ListByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to list user tags: %w", err)
	}

	// Get global tags with exact username match
	globalTags, err := s.repo.ListByUsername(ctx, GlobalUsername)
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

// DeleteTag deletes a tag
func (s *TagService) DeleteTag(ctx context.Context, username, tagName string) error {
	if err := s.repo.Delete(ctx, username, tagName); err != nil {
		return fmt.Errorf("failed to delete tag: %w", err)
	}

	s.log.Info("deleted tag", "username", username, "tag", tagName)
	return nil
}

// GetHistory retrieves the tag move history
func (s *TagService) GetHistory(ctx context.Context, username, tagName string, limit int) ([]*models.TagMove, error) {
	history, err := s.repo.GetHistory(ctx, username, tagName, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get tag history: %w", err)
	}

	return history, nil
}

// CompareAndSwap performs an optimistic lock update
func (s *TagService) CompareAndSwap(ctx context.Context, username, tagName string, expectedVersion int64, newTarget uuid.UUID, newTargetKind models.ArtifactKind, newTargetHash, movedBy string) (bool, error) {
	success, err := s.repo.CompareAndSwap(ctx, username, tagName, expectedVersion, newTarget, string(newTargetKind), newTargetHash, movedBy)
	if err != nil {
		return false, fmt.Errorf("CAS operation failed: %w", err)
	}

	if success {
		s.log.Info("CAS operation succeeded",
			"username", username,
			"tag", tagName,
			"expected_version", expectedVersion,
			"new_target", newTarget,
		)
	} else {
		s.log.Warn("CAS operation failed - version mismatch",
			"username", username,
			"tag", tagName,
			"expected_version", expectedVersion,
		)
	}

	return success, nil
}
