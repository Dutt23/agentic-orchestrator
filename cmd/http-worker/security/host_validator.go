package security

import (
	"fmt"
	"net"
	"strings"
)

// HostValidator validates hostnames and IPs for SSRF protection
type HostValidator struct {
	blockedHostnames []string
	ipValidator      *IPValidator
}

// NewHostValidator creates a new host validator with default blocked hosts
func NewHostValidator() *HostValidator {
	return &HostValidator{
		blockedHostnames: []string{
			"localhost",
			"127.0.0.1",
			"::1",
			"0.0.0.0",
			"::",
			"::ffff:127.0.0.1",
			"[::1]",
			"[::ffff:127.0.0.1]",
		},
		ipValidator: NewIPValidator(),
	}
}

// Validate checks if the hostname is safe (SSRF protection)
func (v *HostValidator) Validate(hostname string) error {
	if hostname == "" {
		return fmt.Errorf("hostname is required")
	}

	// Normalize hostname
	normalizedHost := strings.ToLower(strings.TrimSpace(hostname))

	// Check against blocked hostnames
	for _, blocked := range v.blockedHostnames {
		if normalizedHost == blocked {
			return fmt.Errorf("hostname '%s' is blocked (SSRF protection: localhost access)", hostname)
		}
	}

	// Resolve hostname to IPs and validate each one
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// If DNS lookup fails, allow it (might be network issue)
		// The actual HTTP request will fail anyway
		return nil
	}

	// Validate all resolved IPs using IPValidator
	if err := v.ipValidator.ValidateAll(ips); err != nil {
		return err
	}

	return nil
}

// GetBlockedExamples returns examples of blocked hostnames/IPs
func (v *HostValidator) GetBlockedExamples() []string {
	return []string{
		"localhost (loopback)",
		"127.0.0.1 (loopback IPv4)",
		"::1 (loopback IPv6)",
		"0.0.0.0 (unspecified)",
		"10.0.0.1 (private network)",
		"172.16.0.1 (private network)",
		"192.168.1.1 (private network)",
		"169.254.169.254 (link-local, AWS metadata service)",
		"fd00::1 (private IPv6)",
	}
}
