package security

import (
	"fmt"
	"net/url"
)

// URLValidator orchestrates all security validations for URLs
type URLValidator struct {
	protocolValidator *ProtocolValidator
	hostValidator     *HostValidator
	pathValidator     *PathValidator
}

// NewURLValidator creates a new URL validator with all security checks
func NewURLValidator() *URLValidator {
	return &URLValidator{
		protocolValidator: NewProtocolValidator(),
		hostValidator:     NewHostValidator(),
		pathValidator:     NewPathValidator(),
	}
}

// Validate performs comprehensive security validation on a URL
// Checks: protocol, hostname/IP (SSRF), and path (file access)
func (v *URLValidator) Validate(urlStr string) error {
	// Parse URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	// 1. Validate protocol (http/https only, blocks file://, jdbc://, etc.)
	if err := v.protocolValidator.Validate(parsedURL.Scheme); err != nil {
		return fmt.Errorf("protocol validation failed: %w", err)
	}

	// 2. Validate hostname (SSRF protection: blocks localhost, private IPs)
	hostname := parsedURL.Hostname()
	if err := v.hostValidator.Validate(hostname); err != nil {
		return fmt.Errorf("host validation failed: %w", err)
	}

	// 3. Validate path (blocks file access attempts, path traversal)
	if err := v.pathValidator.Validate(parsedURL.Path); err != nil {
		return fmt.Errorf("path validation failed: %w", err)
	}

	// 4. Validate query parameters for injection attempts
	if err := v.validateQueryParams(parsedURL.Query()); err != nil {
		return fmt.Errorf("query parameter validation failed: %w", err)
	}

	return nil
}

// validateQueryParams checks for dangerous patterns in query parameters
func (v *URLValidator) validateQueryParams(params url.Values) error {
	for key, values := range params {
		for _, value := range values {
			// Check for file access patterns in query params
			if err := v.pathValidator.Validate(value); err != nil {
				return fmt.Errorf("query parameter '%s' contains dangerous pattern: %w", key, err)
			}
		}
	}
	return nil
}

// ValidationReport provides a summary of all validation rules
type ValidationReport struct {
	AllowedProtocols  []string `json:"allowed_protocols"`
	BlockedProtocols  []string `json:"blocked_protocols"`
	BlockedHosts      []string `json:"blocked_hosts"`
	BlockedPathPatterns []string `json:"blocked_path_patterns"`
}

// GetValidationReport returns a summary of all security rules
func (v *URLValidator) GetValidationReport() ValidationReport {
	return ValidationReport{
		AllowedProtocols:  []string{"http", "https"},
		BlockedProtocols:  v.protocolValidator.GetBlockedProtocols(),
		BlockedHosts:      v.hostValidator.GetBlockedExamples(),
		BlockedPathPatterns: v.pathValidator.GetBlockedExamples(),
	}
}
