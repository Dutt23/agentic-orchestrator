package container

import (
	"fmt"
	"os"

	"github.com/lyzr/orchestrator/cmd/orchestrator/repository"
	"github.com/lyzr/orchestrator/cmd/orchestrator/service"
	"github.com/lyzr/orchestrator/common/bootstrap"
	"github.com/redis/go-redis/v9"
)

// Container holds all initialized services and repositories (singleton pattern)
type Container struct {
	// Components
	Components *bootstrap.Components
	Redis      *redis.Client

	// Repositories
	RunRepo      *repository.RunRepository
	ArtifactRepo *repository.ArtifactRepository
	CASBlobRepo  *repository.CASBlobRepository
	TagRepo      *repository.TagRepository

	// Services
	CASService          *service.CASService
	ArtifactService     *service.ArtifactService
	TagService          *service.TagService
	MaterializerService *service.MaterializerService
	WorkflowService     *service.WorkflowServiceV2
	RunService          *service.RunService
}

// NewContainer initializes all services and repositories once
func NewContainer(components *bootstrap.Components) (*Container, error) {
	// Create Redis client
	redisClient, err := createRedisClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create redis client: %w", err)
	}

	// Initialize repositories
	runRepo := repository.NewRunRepository(components.DB)
	artifactRepo := repository.NewArtifactRepository(components.DB)
	casBlobRepo := repository.NewCASBlobRepository(components.DB)
	tagRepo := repository.NewTagRepository(components.DB)

	// Initialize services (bottom-up: dependencies first)
	casService := service.NewCASService(casBlobRepo, components.Logger)
	artifactService := service.NewArtifactService(artifactRepo, components.Logger)
	tagService := service.NewTagService(tagRepo, components.Logger)
	materializerService := service.NewMaterializerService(components.Logger)
	workflowService := service.NewWorkflowServiceV2(
		casService,
		artifactService,
		tagService,
		components.Logger,
	)
	runService := service.NewRunService(
		runRepo,
		artifactRepo,
		casService,
		workflowService,
		materializerService,
		components,
		redisClient,
	)

	return &Container{
		Components:          components,
		Redis:               redisClient,
		RunRepo:             runRepo,
		ArtifactRepo:        artifactRepo,
		CASBlobRepo:         casBlobRepo,
		TagRepo:             tagRepo,
		CASService:          casService,
		ArtifactService:     artifactService,
		TagService:          tagService,
		MaterializerService: materializerService,
		WorkflowService:     workflowService,
		RunService:          runService,
	}, nil
}

// createRedisClient creates a Redis client from environment variables
func createRedisClient() (*redis.Client, error) {
	redisHost := getEnv("REDIS_HOST", "localhost")
	redisPort := getEnv("REDIS_PORT", "6379")
	redisPassword := getEnv("REDIS_PASSWORD", "")

	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", redisHost, redisPort),
		Password: redisPassword,
		DB:       0,
	})

	return client, nil
}

// getEnv gets an environment variable or returns a default
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
