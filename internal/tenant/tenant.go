// Package tenant provides validation and business logic for tenant network operations.
package tenant

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

// Validation errors.
var (
	ErrEmptyTenantID  = errors.New("tenant_id is required")
	ErrEmptyName      = errors.New("name is required")
	ErrInvalidSubnet  = errors.New("invalid subnet CIDR")
	ErrInvalidGateway = errors.New("invalid gateway IP")
	ErrGatewayNotInSubnet = errors.New("gateway IP not within subnet")
	ErrInvalidCIDR    = errors.New("invalid CIDR in rule")
	ErrInvalidPort    = errors.New("port must be 1-65535")
	ErrInvalidPriority = errors.New("priority must be > 0")
	ErrInvalidIP      = errors.New("invalid IP address")
)

// ValidateCreateNetwork validates parameters for creating a tenant network.
func ValidateCreateNetwork(tenantID, name, subnetCIDR, gatewayIP string) error {
	if strings.TrimSpace(tenantID) == "" {
		return ErrEmptyTenantID
	}
	if strings.TrimSpace(name) == "" {
		return ErrEmptyName
	}

	_, ipNet, err := net.ParseCIDR(subnetCIDR)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidSubnet, subnetCIDR)
	}

	gw := net.ParseIP(gatewayIP)
	if gw == nil {
		return fmt.Errorf("%w: %s", ErrInvalidGateway, gatewayIP)
	}

	if !ipNet.Contains(gw) {
		return fmt.Errorf("%w: %s not in %s", ErrGatewayNotInSubnet, gatewayIP, subnetCIDR)
	}

	return nil
}

// ValidateFirewallRule validates parameters for a firewall rule.
func ValidateFirewallRule(tenantID, srcCIDR, dstCIDR string, srcPort, dstPort *uint32, priority uint32) error {
	if strings.TrimSpace(tenantID) == "" {
		return ErrEmptyTenantID
	}

	if srcCIDR != "" {
		if _, _, err := net.ParseCIDR(srcCIDR); err != nil {
			return fmt.Errorf("%w: src_cidr %s", ErrInvalidCIDR, srcCIDR)
		}
	}

	if dstCIDR != "" {
		if _, _, err := net.ParseCIDR(dstCIDR); err != nil {
			return fmt.Errorf("%w: dst_cidr %s", ErrInvalidCIDR, dstCIDR)
		}
	}

	if srcPort != nil && (*srcPort == 0 || *srcPort > 65535) {
		return fmt.Errorf("%w: src_port %d", ErrInvalidPort, *srcPort)
	}
	if dstPort != nil && (*dstPort == 0 || *dstPort > 65535) {
		return fmt.Errorf("%w: dst_port %d", ErrInvalidPort, *dstPort)
	}

	if priority == 0 {
		return ErrInvalidPriority
	}

	return nil
}

// ValidateNATRule validates parameters for a NAT rule.
func ValidateNATRule(tenantID, publicIP, privateIP string) error {
	if strings.TrimSpace(tenantID) == "" {
		return ErrEmptyTenantID
	}

	if publicIP != "" {
		if ip := net.ParseIP(publicIP); ip == nil {
			return fmt.Errorf("%w: public_ip %s", ErrInvalidIP, publicIP)
		}
	}

	if privateIP != "" {
		if ip := net.ParseIP(privateIP); ip == nil {
			return fmt.Errorf("%w: private_ip %s", ErrInvalidIP, privateIP)
		}
	}

	return nil
}

// ValidateRateLimit validates rate limit parameters.
func ValidateRateLimit(tenantID string, ingress, egress, burst uint64) error {
	if strings.TrimSpace(tenantID) == "" {
		return ErrEmptyTenantID
	}
	// At least one rate must be specified
	if ingress == 0 && egress == 0 {
		return errors.New("at least one of ingress or egress rate must be specified")
	}
	return nil
}

// DetectIPVersion returns the IP version based on CIDR or IP string.
func DetectIPVersion(addr string) int {
	if addr == "" {
		return 0
	}

	// Try as CIDR
	ip, _, err := net.ParseCIDR(addr)
	if err == nil {
		if ip.To4() != nil {
			return 4
		}
		return 6
	}

	// Try as plain IP
	ip = net.ParseIP(addr)
	if ip == nil {
		return 0
	}
	if ip.To4() != nil {
		return 4
	}
	return 6
}
