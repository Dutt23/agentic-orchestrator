package security

import (
	"fmt"
	"strings"
)

// ProtocolValidator validates URL protocols
type ProtocolValidator struct {
	allowedProtocols map[string]bool
}

// NewProtocolValidator creates a new protocol validator
func NewProtocolValidator() *ProtocolValidator {
	return &ProtocolValidator{
		allowedProtocols: map[string]bool{
			"http":  true,
			"https": true,
		},
	}
}

// Validate checks if the protocol is allowed
func (v *ProtocolValidator) Validate(scheme string) error {
	normalizedScheme := strings.ToLower(strings.TrimSpace(scheme))

	if normalizedScheme == "" {
		return fmt.Errorf("protocol scheme is required")
	}

	if !v.allowedProtocols[normalizedScheme] {
		return fmt.Errorf("protocol '%s' is not allowed (only http/https permitted)", scheme)
	}

	return nil
}

// GetBlockedProtocols returns examples of blocked protocols
func (v *ProtocolValidator) GetBlockedProtocols() []string {
	return []string{
		"file://",   // Local file access
		"ftp://",    // FTP protocol
		"jdbc://",   // Database connections
		"mysql://",  // MySQL
		"postgres://", // PostgreSQL
		"mongodb://", // MongoDB
		"redis://",  // Redis
		"ssh://",    // SSH
		"telnet://", // Telnet
		"ldap://",   // LDAP
		"dict://",   // Dictionary protocol (SSRF vector)
		"gopher://", // Gopher protocol (SSRF vector)
	}
}
