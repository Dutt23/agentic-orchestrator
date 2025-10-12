package service

import (
	"strings"
)

// Tag namespacing constants
const (
	// TagSeparator separates username from tag name
	TagSeparator = "/"

	// GlobalPrefix is used for system-wide shared tags
	// Example: "_global_/prod"
	GlobalPrefix = "_global_"
)

// BuildInternalTagName creates the internal tag name with user prefix
// This allows multiple users to have tags with the same name.
//
// Examples:
//   - BuildInternalTagName("alice", "main") → "alice/main"
//   - BuildInternalTagName("", "prod") → "_global_/prod"
//   - BuildInternalTagName("bob", "feature") → "bob/feature"
func BuildInternalTagName(username, userTag string) string {
	if username == "" {
		return GlobalPrefix + TagSeparator + userTag
	}
	return username + TagSeparator + userTag
}

// ExtractUserTagName removes the username prefix for display to users
// This gives users a clean view without seeing internal namespacing.
//
// Examples:
//   - ExtractUserTagName("alice/main") → "main"
//   - ExtractUserTagName("_global_/prod") → "prod"
//   - ExtractUserTagName("main") → "main" (no prefix)
func ExtractUserTagName(internalTag string) string {
	parts := strings.SplitN(internalTag, TagSeparator, 2)
	if len(parts) != 2 {
		return internalTag // No prefix, return as-is
	}
	return parts[1]
}

// ExtractUsername gets the username from an internal tag name
// Returns empty string for global tags or tags without prefixes.
//
// Examples:
//   - ExtractUsername("alice/main") → "alice"
//   - ExtractUsername("_global_/prod") → ""
//   - ExtractUsername("main") → "" (no prefix)
func ExtractUsername(internalTag string) string {
	parts := strings.SplitN(internalTag, TagSeparator, 2)
	if len(parts) != 2 {
		return "" // No prefix
	}
	if parts[0] == GlobalPrefix {
		return "" // Global tag has no user owner
	}
	return parts[0]
}

// ListUserTagPrefix returns the prefix for filtering a user's tags
// Use this with SQL LIKE queries to find all tags belonging to a user.
//
// Examples:
//   - ListUserTagPrefix("alice") → "alice/"
//   - ListUserTagPrefix("") → "_global_/"
//
// Usage:
//   prefix := ListUserTagPrefix("alice")
//   SELECT * FROM tag WHERE tag_name LIKE 'alice/%'
func ListUserTagPrefix(username string) string {
	if username == "" {
		return GlobalPrefix + TagSeparator
	}
	return username + TagSeparator
}

// IsGlobalTag checks if a tag is a system-wide shared tag
// Global tags are accessible to all users (with proper permissions).
//
// Examples:
//   - IsGlobalTag("_global_/prod") → true
//   - IsGlobalTag("alice/main") → false
//   - IsGlobalTag("main") → false
func IsGlobalTag(internalTag string) bool {
	return strings.HasPrefix(internalTag, GlobalPrefix+TagSeparator)
}

// IsUserTag checks if a tag belongs to a specific user
//
// Examples:
//   - IsUserTag("alice/main", "alice") → true
//   - IsUserTag("bob/main", "alice") → false
//   - IsUserTag("_global_/prod", "alice") → false
func IsUserTag(internalTag, username string) bool {
	return strings.HasPrefix(internalTag, username+TagSeparator)
}

// CanAccessTag checks if a user can access a specific tag
// Rules:
//   - User can access their own tags (user/*)
//   - All users can read global tags (_global_/*)
//   - Tags without prefix are treated as owned by that user for backwards compatibility
//
// Note: Write permissions should be checked separately at the service/handler layer
func CanAccessTag(internalTag, username string) bool {
	// Global tags are readable by everyone
	if IsGlobalTag(internalTag) {
		return true
	}

	// User's own tags
	if IsUserTag(internalTag, username) {
		return true
	}

	// Legacy tags without prefix (backwards compatibility)
	if !strings.Contains(internalTag, TagSeparator) {
		return true
	}

	return false
}

// ValidateUserTagName validates a user-provided tag name
// User tags should not contain the separator or global prefix.
//
// Returns error message if invalid, empty string if valid.
func ValidateUserTagName(userTag string) string {
	if userTag == "" {
		return "tag name cannot be empty"
	}

	if strings.Contains(userTag, TagSeparator) {
		return "tag name cannot contain '/'"
	}

	if strings.HasPrefix(userTag, GlobalPrefix) {
		return "tag name cannot start with '_global_'"
	}

	if len(userTag) > 100 {
		return "tag name too long (max 100 characters)"
	}

	return "" // Valid
}
