package ratelimit

// TierConfig defines rate limits for each workflow tier
type TierConfig struct {
	Tier          WorkflowTier
	Limit         int64  // Requests allowed per window
	WindowSeconds int    // Time window in seconds
	Description   string // Human-readable description
}

// Default tier configurations
var DefaultTierConfigs = map[WorkflowTier]TierConfig{
	TierSimple: {
		Tier:          TierSimple,
		Limit:         100,
		WindowSeconds: 60,
		Description:   "Simple workflows (no agent nodes) - 100 runs/minute",
	},
	TierStandard: {
		Tier:          TierStandard,
		Limit:         20,
		WindowSeconds: 60,
		Description:   "Standard workflows (1-2 agent nodes) - 20 runs/minute",
	},
	TierHeavy: {
		Tier:          TierHeavy,
		Limit:         5,
		WindowSeconds: 60,
		Description:   "Heavy workflows (3+ agent nodes) - 5 runs/minute",
	},
}

// GlobalConfig contains global service-wide limits
type GlobalConfig struct {
	Limit         int64 // Total requests per window (all users)
	WindowSeconds int   // Time window
}

// Default global configuration
var DefaultGlobalConfig = GlobalConfig{
	Limit:         100, // 100 total requests per minute across all users
	WindowSeconds: 60,
}

// GetLimitForTier returns the rate limit for a given tier
func GetLimitForTier(tier WorkflowTier) int64 {
	if config, exists := DefaultTierConfigs[tier]; exists {
		return config.Limit
	}
	// Fallback to most restrictive tier
	return DefaultTierConfigs[TierHeavy].Limit
}

// GetWindowForTier returns the time window for a given tier
func GetWindowForTier(tier WorkflowTier) int {
	if config, exists := DefaultTierConfigs[tier]; exists {
		return config.WindowSeconds
	}
	return DefaultTierConfigs[TierHeavy].WindowSeconds
}

// GetDescription returns a human-readable description of the tier
func GetDescription(tier WorkflowTier) string {
	if config, exists := DefaultTierConfigs[tier]; exists {
		return config.Description
	}
	return "Unknown tier"
}

// GetAllTiers returns all configured tiers for documentation/API responses
func GetAllTiers() []TierConfig {
	return []TierConfig{
		DefaultTierConfigs[TierSimple],
		DefaultTierConfigs[TierStandard],
		DefaultTierConfigs[TierHeavy],
	}
}
