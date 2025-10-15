package ratelimit

// WorkflowTier represents the rate limit tier based on workflow complexity
type WorkflowTier string

const (
	TierSimple   WorkflowTier = "simple"   // No agent nodes
	TierStandard WorkflowTier = "standard" // 1-2 agent nodes
	TierHeavy    WorkflowTier = "heavy"    // 3+ agent nodes
)

// WorkflowProfile contains analysis of a workflow's complexity
type WorkflowProfile struct {
	Tier          WorkflowTier // Determined tier
	AgentCount    int          // Number of agent nodes
	HasAgentNodes bool         // Whether workflow has any agents
	TotalNodes    int          // Total node count
}

// InspectWorkflow analyzes a workflow and determines its complexity tier
func InspectWorkflow(workflow map[string]interface{}) WorkflowProfile {
	profile := WorkflowProfile{
		Tier:          TierSimple,
		AgentCount:    0,
		HasAgentNodes: false,
		TotalNodes:    0,
	}

	// Get nodes (handles both array and map formats)
	nodes := workflow["nodes"]

	if nodesList, ok := nodes.([]interface{}); ok {
		// Workflow schema format: nodes is an array
		profile.TotalNodes = len(nodesList)

		for _, nodeInterface := range nodesList {
			node, ok := nodeInterface.(map[string]interface{})
			if !ok {
				continue
			}

			nodeType, _ := node["type"].(string)
			if nodeType == "agent" {
				profile.AgentCount++
				profile.HasAgentNodes = true
			}
		}
	} else if nodesMap, ok := nodes.(map[string]interface{}); ok {
		// IR format: nodes is a map[nodeID]Node
		profile.TotalNodes = len(nodesMap)

		for _, nodeInterface := range nodesMap {
			node, ok := nodeInterface.(map[string]interface{})
			if !ok {
				continue
			}

			nodeType, _ := node["type"].(string)
			if nodeType == "agent" {
				profile.AgentCount++
				profile.HasAgentNodes = true
			}
		}
	}

	// Determine tier based on agent count
	profile.Tier = determineTier(profile.AgentCount)

	return profile
}

// determineTier returns the appropriate tier based on agent count
func determineTier(agentCount int) WorkflowTier {
	switch {
	case agentCount == 0:
		return TierSimple
	case agentCount <= 2:
		return TierStandard
	default: // 3+
		return TierHeavy
	}
}

// String returns a human-readable description of the tier
func (t WorkflowTier) String() string {
	switch t {
	case TierSimple:
		return "simple"
	case TierStandard:
		return "standard"
	case TierHeavy:
		return "heavy"
	default:
		return "unknown"
	}
}
