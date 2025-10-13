package sdk

import (
	"time"

	"github.com/google/uuid"
)

// Token represents a workflow token with execution metadata
type Token struct {
	// Unique token ID
	ID string `json:"id"`

	// Run reference
	RunID string `json:"run_id"`

	// Path tracking: who sent this token
	FromNode string `json:"from_node"`

	// Destination: who should receive this token
	ToNode string `json:"to_node"`

	// Payload reference (CAS)
	PayloadRef string `json:"payload_ref"`

	// Hop count (for tracking traversal depth)
	Hop int `json:"hop"`

	// Timestamp
	CreatedAt time.Time `json:"created_at"`

	// Metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// NodeContext holds execution context for a node
type NodeContext struct {
	// Run metadata
	RunID          string
	RunSubmittedAt time.Time

	// Current node
	NodeID string
	Node   *Node

	// Token that triggered this execution
	Token *Token

	// Configuration (loaded from CAS)
	Config interface{}

	// Payload (loaded from CAS)
	Payload interface{}

	// Execution context (previous node outputs)
	Context map[string]interface{}
}

// Node represents a workflow node in the IR
type Node struct {
	ID           string        `json:"id"`
	Type         string        `json:"type"`
	ConfigRef    string        `json:"config_ref"`
	Dependencies []string      `json:"dependencies"`
	Dependents   []string      `json:"dependents"`
	WaitForAll   bool          `json:"wait_for_all"` // Join pattern
	IsTerminal   bool          `json:"is_terminal"`  // Pre-computed terminal flag
	Loop         *LoopConfig   `json:"loop,omitempty"`
	Branch       *BranchConfig `json:"branch,omitempty"`
}

// LoopConfig defines loop behavior for a node
type LoopConfig struct {
	Enabled       bool       `json:"enabled"`
	Condition     *Condition `json:"condition"`
	MaxIterations int        `json:"max_iterations"`
	LoopBackTo    string     `json:"loop_back_to"`
	BreakPath     []string   `json:"break_path"`
	TimeoutPath   []string   `json:"timeout_path"`
}

// BranchConfig defines branching behavior
type BranchConfig struct {
	Enabled            bool         `json:"enabled"`
	Type               string       `json:"type"` // "conditional" or "agent_driven"
	Rules              []BranchRule `json:"rules,omitempty"`
	Default            []string     `json:"default"`
	AvailableNextNodes []string     `json:"available_next_nodes,omitempty"` // For agent-driven
}

// BranchRule represents a conditional branch rule
type BranchRule struct {
	Condition *Condition `json:"condition"`
	NextNodes []string   `json:"next_nodes"`
}

// Condition represents a condition for loops or branches
type Condition struct {
	Type       string                 `json:"type"` // "cel", "schema_validation", "jsonpath", etc.
	Expression string                 `json:"expression,omitempty"`
	Schema     map[string]interface{} `json:"schema,omitempty"`
	SchemaRef  string                 `json:"schema_ref,omitempty"`
	Invert     bool                   `json:"invert,omitempty"`
	// Add more fields as needed for other condition types
}

// IR represents the intermediate representation of a workflow
type IR struct {
	Version string           `json:"version"`
	Nodes   map[string]*Node `json:"nodes"`
}

// ApplyDeltaResult holds the result from the Lua script
type ApplyDeltaResult struct {
	CounterValue int
	Changed      bool
	HitZero      bool
}

// CASClient interface for content-addressable storage
type CASClient interface {
	Get(ref string) (interface{}, error)
	Put(data []byte, mediaType string) (string, error)
	Store(data interface{}) (string, error)
}

// EventType represents different evenyest types in the system
type EventType string

const (
	EventTokenEmitted  EventType = "token.emitted"
	EventTokenConsumed EventType = "token.consumed"
	EventNodeExecuted  EventType = "node.executed"
	EventNodeFailed    EventType = "node.failed"
	EventLoopStarted   EventType = "loop.started"
	EventLoopCompleted EventType = "loop.completed"
	EventBranchTaken   EventType = "branch.taken"
)

// Event represents a workflow execution event
type Event struct {
	EventID       uuid.UUID              `json:"event_id"`
	RunID         uuid.UUID              `json:"run_id"`
	EventType     EventType              `json:"event_type"`
	SequenceNum   int64                  `json:"sequence_num"`
	Timestamp     time.Time              `json:"timestamp"`
	EventData     map[string]interface{} `json:"event_data"`
	ParentEventID *uuid.UUID             `json:"parent_event_id,omitempty"`
	CorrelationID *uuid.UUID             `json:"correlation_id,omitempty"`
}
