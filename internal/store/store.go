// Package store defines the storage interface for Network Manager state.
// The interface is designed to be backend-agnostic: v1 uses an in-memory
// implementation, with PostgreSQL or etcd planned for production.
package store

import (
	"context"
	"errors"
	"time"
)

// Common errors returned by Store implementations.
var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrNoCapacity    = errors.New("no capacity available")
)

// Store defines the persistence interface for all network manager state.
type Store interface {
	// Tenant networks
	CreateTenantNetwork(ctx context.Context, network *TenantNetwork) error
	GetTenantNetwork(ctx context.Context, tenantID string) (*TenantNetwork, error)
	DeleteTenantNetwork(ctx context.Context, tenantID string) error
	ListTenantNetworks(ctx context.Context, pageSize int, pageToken string) ([]*TenantNetwork, string, error)

	// Firewall rules
	AddFirewallRule(ctx context.Context, rule *FirewallRule) error
	DeleteFirewallRule(ctx context.Context, tenantID, ruleID string) error
	ListFirewallRules(ctx context.Context, tenantID string, pageSize int, pageToken string) ([]*FirewallRule, string, error)
	ListFirewallRulesByDPU(ctx context.Context, dpuID string) ([]*FirewallRule, error)

	// NAT rules
	AddNATRule(ctx context.Context, rule *NATRule) error
	DeleteNATRule(ctx context.Context, tenantID, ruleID string) error
	ListNATRules(ctx context.Context, tenantID string, pageSize int, pageToken string) ([]*NATRule, string, error)
	ListNATRulesByDPU(ctx context.Context, dpuID string) ([]*NATRule, error)

	// Rate limits
	SetRateLimit(ctx context.Context, limit *RateLimit) error
	GetRateLimit(ctx context.Context, tenantID string) (*RateLimit, error)
	GetRateLimitsByDPU(ctx context.Context, dpuID string) ([]*RateLimit, error)

	// DPU assignments
	AssignTenantToDPU(ctx context.Context, assignment *DPUAssignment) error
	UnassignTenantFromDPU(ctx context.Context, tenantID, dpuID string) error
	GetDPUAssignments(ctx context.Context, dpuID string) ([]*DPUAssignment, error)
	GetTenantDPUAssignment(ctx context.Context, tenantID string) ([]*DPUAssignment, error)

	// Agent status
	UpdateAgentStatus(ctx context.Context, status *AgentStatus) error
	GetAgentStatus(ctx context.Context, dpuID string, agentType AgentType) (*AgentStatus, error)

	// Version tracking for conditional fetch
	GetDPUStateVersion(ctx context.Context, dpuID string) (uint64, error)
}

// TenantNetwork represents a tenant's network configuration.
type TenantNetwork struct {
	TenantID   string
	Name       string
	SubnetCIDR string
	GatewayIP  string
	PublicIPs  []string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// FirewallRule represents a firewall rule for a tenant.
type FirewallRule struct {
	ID        string
	TenantID  string
	SrcCIDR   string
	DstCIDR   string
	SrcPort   *uint32
	DstPort   *uint32
	Protocol  Protocol
	Action    Action
	Priority  uint32
	IPVersion IPVersion
	CreatedAt time.Time
}

// NATRule represents a NAT rule for a tenant.
type NATRule struct {
	ID             string
	TenantID       string
	Type           NATType
	PublicIP       string
	PublicPort     *uint32
	PrivateIP      string
	PrivatePort    *uint32
	ExternalPrefix string
	InternalPrefix string
	PrefixLength   uint32
	Protocol       Protocol
	Mode           NATMode
	IPVersion      IPVersion
	CreatedAt      time.Time
}

// RateLimit represents rate limiting configuration for a tenant.
type RateLimit struct {
	TenantID            string
	IngressBytesPerSec  uint64
	EgressBytesPerSec   uint64
	BurstBytes          uint64
}

// DPUAssignment represents a tenant-to-DPU mapping.
type DPUAssignment struct {
	DPUID      string
	TenantID   string
	InPort     string
	OutPort    string
	AssignedAt time.Time
}

// AgentStatus represents the last reported status from a DPU agent.
type AgentStatus struct {
	DPUID            string
	AgentType        AgentType
	TotalRules       uint32
	HWOffloaded      uint32
	PacketsForwarded uint64
	PacketsDropped   uint64
	BytesForwarded   uint64
	BytesDropped     uint64
	UptimeSeconds    uint64
	LastSyncError    string
	LastSyncTime     time.Time
	ReportedAt       time.Time
}

// Enums matching the proto definitions.

type Action int

const (
	ActionUnspecified Action = iota
	ActionAllow
	ActionDeny
)

type Protocol int

const (
	ProtocolUnspecified Protocol = iota
	ProtocolTCP
	ProtocolUDP
	ProtocolICMP
	ProtocolAny
)

type NATType int

const (
	NATTypeUnspecified NATType = iota
	NATTypeSNAT
	NATTypeDNAT
	NATTypeForward
	NATTypeNPTv6
)

type NATMode int

const (
	NATModeUnspecified NATMode = iota
	NATModeStatic
	NATModeMasquerade
)

type IPVersion int

const (
	IPVersionUnspecified IPVersion = iota
	IPVersion4
	IPVersion6
)

type AgentType int

const (
	AgentTypeUnspecified AgentType = iota
	AgentTypeFirewall
	AgentTypeRouting
	AgentTypeMonitoring
)
