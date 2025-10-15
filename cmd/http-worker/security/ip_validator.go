package security

import (
	"fmt"
	"net"
)

// IPValidator validates IP addresses for security
type IPValidator struct{}

// NewIPValidator creates a new IP validator
func NewIPValidator() *IPValidator {
	return &IPValidator{}
}

// Validate checks if an IP address is safe to connect to
// Blocks: loopback, private networks, link-local, multicast, unspecified
func (v *IPValidator) Validate(ip net.IP) error {
	if ip == nil {
		return fmt.Errorf("IP address is nil")
	}

	// Block loopback addresses (127.0.0.0/8, ::1)
	if ip.IsLoopback() {
		return fmt.Errorf("IP %s is blocked (SSRF protection: loopback address)", ip.String())
	}

	// Block private IP ranges
	// IPv4: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
	// IPv6: fc00::/7
	if ip.IsPrivate() {
		return fmt.Errorf("IP %s is blocked (SSRF protection: private network)", ip.String())
	}

	// Block link-local addresses
	// IPv4: 169.254.0.0/16 (used for AWS metadata service, etc.)
	// IPv6: fe80::/10
	if ip.IsLinkLocalUnicast() {
		return fmt.Errorf("IP %s is blocked (SSRF protection: link-local address)", ip.String())
	}

	// Block multicast addresses
	// IPv4: 224.0.0.0/4
	// IPv6: ff00::/8
	if ip.IsMulticast() {
		return fmt.Errorf("IP %s is blocked (SSRF protection: multicast address)", ip.String())
	}

	// Block unspecified addresses (0.0.0.0, ::)
	if ip.IsUnspecified() {
		return fmt.Errorf("IP %s is blocked (SSRF protection: unspecified address)", ip.String())
	}

	return nil
}

// ValidateAll checks all IPs in a list
func (v *IPValidator) ValidateAll(ips []net.IP) error {
	if len(ips) == 0 {
		return fmt.Errorf("no IP addresses to validate")
	}

	for _, ip := range ips {
		if err := v.Validate(ip); err != nil {
			return err
		}
	}

	return nil
}

// GetBlockedCategories returns examples of blocked IP types
func (v *IPValidator) GetBlockedCategories() map[string][]string {
	return map[string][]string{
		"Loopback": {
			"127.0.0.1 (IPv4 loopback)",
			"::1 (IPv6 loopback)",
		},
		"Private Networks": {
			"10.0.0.1 (Class A private)",
			"172.16.0.1 (Class B private)",
			"192.168.1.1 (Class C private)",
			"fd00::1 (IPv6 ULA)",
		},
		"Link-Local": {
			"169.254.169.254 (IPv4 link-local, AWS metadata)",
			"fe80::1 (IPv6 link-local)",
		},
		"Special": {
			"0.0.0.0 (unspecified IPv4)",
			":: (unspecified IPv6)",
			"224.0.0.1 (multicast)",
		},
	}
}
