package coordinator

// StreamRouter handles routing tokens to appropriate Redis streams based on node type
type StreamRouter struct {
	// Stream mapping can be configured here if needed
	streamMap map[string]string
}

// NewStreamRouter creates a new stream router
func NewStreamRouter() *StreamRouter {
	return &StreamRouter{
		streamMap: make(map[string]string),
	}
}

// GetStreamForNodeType returns the Redis stream name for a given node type
func (r *StreamRouter) GetStreamForNodeType(nodeType string) string {
	// Check custom mapping first
	if stream, exists := r.streamMap[nodeType]; exists {
		return stream
	}

	// Default mapping based on node type
	switch nodeType {
	case "agent":
		return "wf.tasks.agent"
	case "classifier":
		return "wf.tasks.classifier"
	case "search":
		return "wf.tasks.search"
	case "function":
		return "wf.tasks.function"
	case "http":
		return "wf.tasks.http"
	case "hitl":
		return "wf.tasks.hitl"
	case "transform":
		return "wf.tasks.transform"
	case "aggregate":
		return "wf.tasks.aggregate"
	case "filter":
		return "wf.tasks.filter"
	case "task":
		// Generic task type
		return "wf.tasks.default"
	default:
		// Unknown type, route to default stream
		return "wf.tasks.default"
	}
}

// RegisterCustomMapping allows registering custom stream mappings for node types
func (r *StreamRouter) RegisterCustomMapping(nodeType, stream string) {
	r.streamMap[nodeType] = stream
}

// GetAllStreams returns all registered stream names
func (r *StreamRouter) GetAllStreams() []string {
	streams := make(map[string]bool)

	// Add custom streams
	for _, stream := range r.streamMap {
		streams[stream] = true
	}

	// Add default streams
	defaultStreams := []string{
		"wf.tasks.agent",
		"wf.tasks.classifier",
		"wf.tasks.search",
		"wf.tasks.function",
		"wf.tasks.http",
		"wf.tasks.hitl",
		"wf.tasks.transform",
		"wf.tasks.aggregate",
		"wf.tasks.filter",
		"wf.tasks.default",
	}

	for _, stream := range defaultStreams {
		streams[stream] = true
	}

	// Convert to slice
	result := make([]string, 0, len(streams))
	for stream := range streams {
		result = append(result, stream)
	}

	return result
}
