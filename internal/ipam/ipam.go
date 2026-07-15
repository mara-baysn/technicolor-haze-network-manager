// Package ipam provides simple IP Address Management for public IP allocation.
// It manages a pool of available public IPv4 and IPv6 addresses that can be
// allocated to tenants and released back to the pool.
package ipam

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// Common errors.
var (
	ErrNoAvailableIPs = errors.New("no available IPs in pool")
	ErrIPNotAllocated = errors.New("IP not allocated")
	ErrIPNotInPool    = errors.New("IP not in pool")
	ErrIPAlreadyInUse = errors.New("IP already allocated")
	ErrInvalidCIDR    = errors.New("invalid CIDR notation")
)

// IPVersion indicates the IP protocol version.
type IPVersion int

const (
	IPv4 IPVersion = 4
	IPv6 IPVersion = 6
)

// Allocation represents an allocated IP address.
type Allocation struct {
	IP          string
	TenantID    string
	Version     IPVersion
	AllocatedAt time.Time
}

// Pool manages a pool of IP addresses for allocation.
type Pool struct {
	mu          sync.RWMutex
	available   map[string]IPVersion // ip -> version (available for allocation)
	allocated   map[string]*Allocation // ip -> allocation
}

// NewPool creates a new empty IP pool.
func NewPool() *Pool {
	return &Pool{
		available: make(map[string]IPVersion),
		allocated: make(map[string]*Allocation),
	}
}

// AddCIDR adds all usable host addresses from a CIDR range to the available pool.
// For IPv4, excludes network and broadcast addresses.
func (p *Pool) AddCIDR(cidr string) (int, error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return 0, fmt.Errorf("%w: %s", ErrInvalidCIDR, cidr)
	}

	version := IPv4
	if ip.To4() == nil {
		version = IPv6
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	count := 0
	for ip := ip.Mask(ipNet.Mask); ipNet.Contains(ip); incIP(ip) {
		addr := ip.String()
		if version == IPv4 {
			// Skip network and broadcast for IPv4
			if isNetworkOrBroadcast(ip, ipNet) {
				continue
			}
		}
		if _, inUse := p.allocated[addr]; !inUse {
			if _, avail := p.available[addr]; !avail {
				p.available[addr] = version
				count++
			}
		}
	}

	return count, nil
}

// AddIP adds a single IP address to the available pool.
func (p *Pool) AddIP(ipStr string) error {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return fmt.Errorf("%w: %s", ErrInvalidCIDR, ipStr)
	}

	version := IPv4
	if ip.To4() == nil {
		version = IPv6
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	normalized := ip.String()
	if _, inUse := p.allocated[normalized]; inUse {
		return fmt.Errorf("%s: %w", normalized, ErrIPAlreadyInUse)
	}
	p.available[normalized] = version
	return nil
}

// Allocate assigns an available IP to a tenant. If preferVersion is specified,
// it tries to allocate from that version pool first.
func (p *Pool) Allocate(tenantID string, preferVersion IPVersion) (*Allocation, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// First pass: try preferred version
	for ip, ver := range p.available {
		if ver == preferVersion {
			return p.allocateIP(ip, tenantID, ver), nil
		}
	}

	// Second pass: any available IP
	for ip, ver := range p.available {
		_ = ver
		return p.allocateIP(ip, tenantID, ver), nil
	}

	return nil, ErrNoAvailableIPs
}

// allocateIP moves an IP from available to allocated. Must hold lock.
func (p *Pool) allocateIP(ip string, tenantID string, version IPVersion) *Allocation {
	delete(p.available, ip)
	alloc := &Allocation{
		IP:          ip,
		TenantID:    tenantID,
		Version:     version,
		AllocatedAt: time.Now(),
	}
	p.allocated[ip] = alloc
	return alloc
}

// Release returns an allocated IP back to the available pool.
func (p *Pool) Release(ip string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	alloc, ok := p.allocated[ip]
	if !ok {
		return fmt.Errorf("%s: %w", ip, ErrIPNotAllocated)
	}

	delete(p.allocated, ip)
	p.available[ip] = alloc.Version
	return nil
}

// GetAllocation returns the allocation for a specific IP.
func (p *Pool) GetAllocation(ip string) (*Allocation, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	alloc, ok := p.allocated[ip]
	if !ok {
		return nil, fmt.Errorf("%s: %w", ip, ErrIPNotAllocated)
	}
	return alloc, nil
}

// ListAllocations returns all current allocations, optionally filtered by tenant.
func (p *Pool) ListAllocations(tenantID string) []*Allocation {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var result []*Allocation
	for _, alloc := range p.allocated {
		if tenantID == "" || alloc.TenantID == tenantID {
			result = append(result, alloc)
		}
	}
	return result
}

// Stats returns pool statistics.
func (p *Pool) Stats() (available, allocated int) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.available), len(p.allocated)
}

// incIP increments an IP address in place.
func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// isNetworkOrBroadcast checks if an IP is the network or broadcast address for a subnet.
func isNetworkOrBroadcast(ip net.IP, network *net.IPNet) bool {
	// Network address: all host bits are 0
	// Broadcast address: all host bits are 1
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}

	mask := network.Mask
	// Check network address
	isNetwork := true
	isBroadcast := true
	for i := range ip4 {
		hostBits := ip4[i] & ^mask[i]
		if hostBits != 0 {
			isNetwork = false
		}
		if hostBits != ^mask[i] {
			isBroadcast = false
		}
	}

	return isNetwork || isBroadcast
}
