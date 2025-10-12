package service

import (
	"strings"
)

// Tag namespacing constants
const (
	// GlobalUsername is used for system-wide shared tags
	// Example: username='_global_', tag_name='prod'
	GlobalUsername = "_global_"
)

// ValidateUserTagName validates a user-provided tag name
// User tags should not contain invalid characters or reserved prefixes.
//
// Returns error message if invalid, empty string if valid.
func ValidateUserTagName(userTag string) string {
	if userTag == "" {
		return "tag name cannot be empty"
	}

	// Tag names can now contain "/" for hierarchical organization
	// Example: "release/v1.0", "exp/quality"
	// This is safe because username is stored separately

	if strings.HasPrefix(userTag, GlobalUsername) {
		return "tag name cannot start with '_global_'"
	}

	if len(userTag) > 200 {
		return "tag name too long (max 200 characters)"
	}

	return "" // Valid
}

// ValidateUsername validates a username
// Ensures username doesn't conflict with reserved values
//
// Returns error message if invalid, empty string if valid.
func ValidateUsername(username string) string {
	if username == "" {
		return "username cannot be empty"
	}

	if username == GlobalUsername {
		return "username '_global_' is reserved for system tags"
	}

	if len(username) > 100 {
		return "username too long (max 100 characters)"
	}

	// Usernames should be simple alphanumeric + hyphens/underscores
	for _, ch := range username {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return "username can only contain letters, numbers, hyphens, and underscores"
		}
	}

	return "" // Valid
}
