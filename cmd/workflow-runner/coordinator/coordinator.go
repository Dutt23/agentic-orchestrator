package coordinator

import (
	"context"
	"encoding/json"
	"time"

	"github.com/lyzr/orchestrator/cmd/workflow-runner/condition"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/operators"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/resolver"
	"github.com/lyzr/orchestrator/common/sdk"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/workflow_lifecycle"
	"github.com/lyzr/orchestrator/common/clients"
	"github.com/lyzr/orchestrator/common/ratelimit"
	redisWrapper "github.com/lyzr/orchestrator/common/redis"
	"github.com/redis/go-redis/v9"
)

// CompletionSignal represents a worker's completion notification
type CompletionSignal struct {
	Version    string                 `json:"version"`              // Protocol version (1.0)
	JobID      string                 `json:"job_id"`               // Unique job ID
	RunID      string                 `json:"run_id"`               // Workflow run ID
	NodeID     string                 `json:"node_id"`              // Node that completed
	Status     string                 `json:"status"`               // completed|failed
	ResultData map[string]interface{} `json:"result_data,omitempty"` // Actual result data (coordinator stores in CAS)
	ResultRef  string                 `json:"result_ref,omitempty"` // CAS reference (deprecated, for backward compat)
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// Coordinator handles choreography for workflow execution
type Coordinator struct {
	redis               *redis.Client // Raw client for BLPOP and other blocking ops
	redisWrapper        *redisWrapper.Client // Wrapped client for common ops
	sdk                 *sdk.SDK
	logger              Logger
	router              *StreamRouter
	evaluator           *condition.Evaluator
	resolver            *resolver.Resolver
	orchestratorClient  *clients.OrchestratorClient
	orchestratorBaseURL string
	casClient           clients.CASClient // CAS client for compiler
	rateLimiter         *ratelimit.RateLimiter // Rate limiter for dynamic checks

	// Extracted modules for clean separation of concerns
	operators *OperatorOpts
	lifecycle *LifecycleHandlerOpts
}

// OperatorOpts contains all control flow operators
type OperatorOpts struct {
	ControlFlowRouter *operators.ControlFlowRouter
}

// LifecycleHandlerOpts contains all workflow lifecycle handlers
type LifecycleHandlerOpts struct {
	CompletionChecker *workflow_lifecycle.CompletionChecker
	EventPublisher    *workflow_lifecycle.EventPublisher
	StatusManager     *workflow_lifecycle.StatusManager
}

// Logger interface for logging
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Debug(msg string, keysAndValues ...interface{})
}

// CoordinatorOpts contains options for creating a coordinator
type CoordinatorOpts struct {
	Redis               *redis.Client
	SDK                 *sdk.SDK
	Logger              Logger
	OrchestratorBaseURL string
	CASClient           clients.CASClient
	RateLimiter         *ratelimit.RateLimiter
}

// NewCoordinator creates a new coordinator instance
func NewCoordinator(opts *CoordinatorOpts) *Coordinator {
	orchestratorClient := clients.NewOrchestratorClient(opts.OrchestratorBaseURL, opts.Logger)
	evaluator := condition.NewEvaluator()

	// Wrap Redis client for better abstractions and instrumentation
	redisClient := redisWrapper.NewClient(opts.Redis, opts.Logger)

	// Create workflow lifecycle modules with wrapped Redis client
	eventPublisher := workflow_lifecycle.NewEventPublisher(redisClient, opts.Logger)
	statusManager := workflow_lifecycle.NewStatusManager(redisClient, opts.Logger)
	completionChecker := workflow_lifecycle.NewCompletionChecker(redisClient, opts.SDK, opts.Logger, eventPublisher, statusManager)

	// Create control flow router (still uses raw Redis for complex operations like XREADGROUP)
	controlFlowRouter := operators.NewControlFlowRouter(opts.Redis, opts.SDK, evaluator, opts.Logger)

	return &Coordinator{
		redis:               opts.Redis, // Keep raw for BLPOP
		redisWrapper:        redisClient, // Use wrapper for common ops
		sdk:                 opts.SDK,
		logger:              opts.Logger,
		router:              NewStreamRouter(),
		evaluator:           evaluator,
		resolver:            resolver.NewResolver(opts.SDK, opts.Logger),
		orchestratorClient:  orchestratorClient,
		orchestratorBaseURL: opts.OrchestratorBaseURL,
		casClient:           opts.CASClient,
		rateLimiter:         opts.RateLimiter,
		operators: &OperatorOpts{
			ControlFlowRouter: controlFlowRouter,
		},
		lifecycle: &LifecycleHandlerOpts{
			CompletionChecker: completionChecker,
			EventPublisher:    eventPublisher,
			StatusManager:     statusManager,
		},
	}
}

// Start begins the coordinator main loop
func (c *Coordinator) Start(ctx context.Context) error {
	c.logger.Info("coordinator starting", "queue", "completion_signals")

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("coordinator shutting down")
			return ctx.Err()
		default:
			// Block waiting for completion signals (5 second timeout)
			result := c.redis.BLPop(ctx, 5*time.Second, "completion_signals")
			if result.Err() == redis.Nil {
				// Timeout, continue loop
				continue
			}
			if result.Err() != nil {
				c.logger.Error("failed to read completion signal", "error", result.Err())
				continue
			}

			// Parse signal (result.Val()[1] contains the JSON payload)
			if len(result.Val()) < 2 {
				c.logger.Error("invalid completion signal format")
				continue
			}

			var signal CompletionSignal
			if err := json.Unmarshal([]byte(result.Val()[1]), &signal); err != nil {
				c.logger.Error("failed to parse completion signal", "error", err)
				continue
			}

			// Handle completion in goroutine for parallel processing
			go c.handleCompletion(ctx, &signal)
		}
	}
}
