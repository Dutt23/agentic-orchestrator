package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/lyzr/orchestrator/cmd/orchestrator/models"
	"github.com/lyzr/orchestrator/cmd/orchestrator/repository"
	"github.com/lyzr/orchestrator/common/logger"
)

// CASService handles content-addressed storage operations
type CASService struct {
	repo *repository.CASBlobRepository
	log  *logger.Logger
}

// NewCASService creates a new CAS service
func NewCASService(repo *repository.CASBlobRepository, log *logger.Logger) *CASService {
	return &CASService{
		repo: repo,
		log:  log,
	}
}

// StoreContent stores content and returns its CAS ID (hash)
func (s *CASService) StoreContent(ctx context.Context, content []byte, mediaType string) (string, error) {
	// Compute SHA256 hash
	hash := sha256.Sum256(content)
	casID := fmt.Sprintf("sha256:%x", hash)

	// Check if content already exists (deduplication)
	exists, err := s.repo.Exists(ctx, casID)
	if err != nil {
		return "", fmt.Errorf("failed to check existence: %w", err)
	}

	if exists {
		s.log.Info("content already exists in CAS", "cas_id", casID)
		return casID, nil
	}

	// Store new content
	blob := &models.CASBlob{
		CasID:      casID,
		MediaType:  mediaType,
		SizeBytes:  int64(len(content)),
		Content:    content,
		StorageURL: nil, // Inline storage for MVP
		CreatedAt:  time.Now(),
	}

	if err := s.repo.Create(ctx, blob); err != nil {
		return "", fmt.Errorf("failed to store content: %w", err)
	}

	s.log.Info("stored content in CAS", "cas_id", casID, "size_bytes", len(content))
	return casID, nil
}

// GetContent retrieves content by CAS ID
func (s *CASService) GetContent(ctx context.Context, casID string) ([]byte, error) {
	content, err := s.repo.GetContentByID(ctx, casID)
	if err != nil {
		return nil, fmt.Errorf("failed to get content: %w", err)
	}

	return content, nil
}

// GetBlob retrieves full CAS blob metadata
func (s *CASService) GetBlob(ctx context.Context, casID string) (*models.CASBlob, error) {
	blob, err := s.repo.GetByID(ctx, casID)
	if err != nil {
		return nil, fmt.Errorf("failed to get blob: %w", err)
	}

	return blob, nil
}

// Exists checks if content exists
func (s *CASService) Exists(ctx context.Context, casID string) (bool, error) {
	return s.repo.Exists(ctx, casID)
}

// ComputeHash computes SHA256 hash without storing
func (s *CASService) ComputeHash(content []byte) string {
	hash := sha256.Sum256(content)
	return fmt.Sprintf("sha256:%x", hash)
}
