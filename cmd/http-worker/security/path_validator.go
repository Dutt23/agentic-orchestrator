package security

import (
	"fmt"
	"strings"
)

// PathValidator validates URL paths for security
type PathValidator struct {
	blockedPatterns []string
}

// NewPathValidator creates a new path validator
func NewPathValidator() *PathValidator {
	return &PathValidator{
		blockedPatterns: []string{
			"file://",        // Direct file access
			"../",            // Path traversal
			"..\\",           // Path traversal (Windows)
			"/etc/",          // System files (Unix)
			"/proc/",         // Process info (Linux)
			"/sys/",          // System info (Linux)
			"c:/",            // Windows drive
			"c:\\",           // Windows drive
			"\\\\.\\pipe\\",  // Windows named pipes
		},
	}
}

// Validate checks if the URL path contains dangerous patterns
func (v *PathValidator) Validate(urlPath string) error {
	if urlPath == "" {
		// Empty path is OK (root path)
		return nil
	}

	normalizedPath := strings.ToLower(urlPath)

	// Check for blocked patterns
	for _, pattern := range v.blockedPatterns {
		if strings.Contains(normalizedPath, pattern) {
			return fmt.Errorf("path contains blocked pattern '%s' (security risk: file access attempt)", pattern)
		}
	}

	// Check for URL-encoded attacks
	if v.containsEncodedAttack(normalizedPath) {
		return fmt.Errorf("path contains encoded attack patterns (security risk)")
	}

	return nil
}

// containsEncodedAttack detects URL-encoded path traversal attempts
func (v *PathValidator) containsEncodedAttack(path string) bool {
	encodedPatterns := []string{
		"%2e%2e/",     // ../
		"%2e%2e%2f",   // ../
		"..%2f",       // ../
		"%2e%2e\\",    // ..\
		"%2e%2e%5c",   // ..\
		"..%5c",       // ..\
	}

	for _, pattern := range encodedPatterns {
		if strings.Contains(path, pattern) {
			return true
		}
	}

	return false
}

// GetBlockedExamples returns examples of blocked path patterns
func (v *PathValidator) GetBlockedExamples() []string {
	return []string{
		"file:///etc/passwd (local file access)",
		"../../../etc/passwd (path traversal)",
		"/etc/shadow (system file access)",
		"/proc/self/environ (process info)",
		"c:/windows/system32 (Windows system)",
		"\\\\.\\pipe\\named_pipe (Windows pipes)",
	}
}
