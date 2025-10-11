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
