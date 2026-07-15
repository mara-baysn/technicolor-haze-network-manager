package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryStore_TenantNetwork(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	// Create
	net := &TenantNetwork{
		TenantID:   "tenant-1",
		Name:       "Test Network",
		SubnetCIDR: "10.100.0.0/24",
		GatewayIP:  "10.100.0.1",
	}
	require.NoError(t, s.CreateTenantNetwork(ctx, net))

	// Duplicate
	err := s.CreateTenantNetwork(ctx, net)
	assert.ErrorIs(t, err, ErrAlreadyExists)

	// Get
	got, err := s.GetTenantNetwork(ctx, "tenant-1")
	require.NoError(t, err)
	assert.Equal(t, "Test Network", got.Name)
	assert.Equal(t, "10.100.0.0/24", got.SubnetCIDR)
	assert.False(t, got.CreatedAt.IsZero())

	// Get not found
	_, err = s.GetTenantNetwork(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrNotFound)

	// List
	nets, token, err := s.ListTenantNetworks(ctx, 10, "")
	require.NoError(t, err)
	assert.Len(t, nets, 1)
	assert.Empty(t, token)

	// Delete
	require.NoError(t, s.DeleteTenantNetwork(ctx, "tenant-1"))
	_, err = s.GetTenantNetwork(ctx, "tenant-1")
	assert.ErrorIs(t, err, ErrNotFound)

	// Delete not found
	err = s.DeleteTenantNetwork(ctx, "tenant-1")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryStore_FirewallRules(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	// Must create tenant first
	require.NoError(t, s.CreateTenantNetwork(ctx, &TenantNetwork{
		TenantID:   "tenant-1",
		Name:       "test",
		SubnetCIDR: "10.0.0.0/24",
		GatewayIP:  "10.0.0.1",
	}))

	// Add rule without tenant fails
	err := s.AddFirewallRule(ctx, &FirewallRule{
		ID:       "rule-1",
		TenantID: "nonexistent",
		SrcCIDR:  "0.0.0.0/0",
		Action:   ActionDeny,
	})
	assert.ErrorIs(t, err, ErrNotFound)

	// Add rule
	rule := &FirewallRule{
		ID:       "rule-1",
		TenantID: "tenant-1",
		SrcCIDR:  "10.0.0.0/8",
		DstCIDR:  "192.168.1.0/24",
		Protocol: ProtocolTCP,
		Action:   ActionAllow,
		Priority: 100,
	}
	require.NoError(t, s.AddFirewallRule(ctx, rule))
	assert.False(t, rule.CreatedAt.IsZero())

	// List
	rules, token, err := s.ListFirewallRules(ctx, "tenant-1", 10, "")
	require.NoError(t, err)
	assert.Len(t, rules, 1)
	assert.Empty(t, token)
	assert.Equal(t, "rule-1", rules[0].ID)

	// Delete
	require.NoError(t, s.DeleteFirewallRule(ctx, "tenant-1", "rule-1"))
	rules, _, err = s.ListFirewallRules(ctx, "tenant-1", 10, "")
	require.NoError(t, err)
	assert.Len(t, rules, 0)

	// Delete not found
	err = s.DeleteFirewallRule(ctx, "tenant-1", "rule-1")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryStore_NATRules(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	require.NoError(t, s.CreateTenantNetwork(ctx, &TenantNetwork{
		TenantID:   "tenant-1",
		Name:       "test",
		SubnetCIDR: "10.0.0.0/24",
		GatewayIP:  "10.0.0.1",
	}))

	rule := &NATRule{
		ID:        "nat-1",
		TenantID:  "tenant-1",
		Type:      NATTypeSNAT,
		PublicIP:  "203.0.113.1",
		PrivateIP: "10.0.0.5",
		Protocol:  ProtocolTCP,
		Mode:      NATModeStatic,
		IPVersion: IPVersion4,
	}
	require.NoError(t, s.AddNATRule(ctx, rule))

	rules, token, err := s.ListNATRules(ctx, "tenant-1", 10, "")
	require.NoError(t, err)
	assert.Len(t, rules, 1)
	assert.Empty(t, token)
	assert.Equal(t, "nat-1", rules[0].ID)

	require.NoError(t, s.DeleteNATRule(ctx, "tenant-1", "nat-1"))
	rules, _, err = s.ListNATRules(ctx, "tenant-1", 10, "")
	require.NoError(t, err)
	assert.Len(t, rules, 0)
}

func TestMemoryStore_RateLimits(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	require.NoError(t, s.CreateTenantNetwork(ctx, &TenantNetwork{
		TenantID:   "tenant-1",
		Name:       "test",
		SubnetCIDR: "10.0.0.0/24",
		GatewayIP:  "10.0.0.1",
	}))

	// Set rate limit for nonexistent tenant
	err := s.SetRateLimit(ctx, &RateLimit{TenantID: "nonexistent"})
	assert.ErrorIs(t, err, ErrNotFound)

	// Set
	limit := &RateLimit{
		TenantID:           "tenant-1",
		IngressBytesPerSec: 1_000_000_000, // 1 Gbps
		EgressBytesPerSec:  500_000_000,   // 500 Mbps
		BurstBytes:         10_000_000,
	}
	require.NoError(t, s.SetRateLimit(ctx, limit))

	// Get
	got, err := s.GetRateLimit(ctx, "tenant-1")
	require.NoError(t, err)
	assert.Equal(t, uint64(1_000_000_000), got.IngressBytesPerSec)
	assert.Equal(t, uint64(500_000_000), got.EgressBytesPerSec)

	// Get not found
	_, err = s.GetRateLimit(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryStore_DPUAssignments(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	require.NoError(t, s.CreateTenantNetwork(ctx, &TenantNetwork{
		TenantID:   "tenant-1",
		Name:       "test",
		SubnetCIDR: "10.0.0.0/24",
		GatewayIP:  "10.0.0.1",
	}))
	require.NoError(t, s.CreateTenantNetwork(ctx, &TenantNetwork{
		TenantID:   "tenant-2",
		Name:       "test2",
		SubnetCIDR: "10.0.1.0/24",
		GatewayIP:  "10.0.1.1",
	}))

	// Assign nonexistent tenant
	err := s.AssignTenantToDPU(ctx, &DPUAssignment{
		DPUID:    "dpu-1",
		TenantID: "nonexistent",
		InPort:   "pf0vf0",
		OutPort:  "pf0",
	})
	assert.ErrorIs(t, err, ErrNotFound)

	// Assign tenant-1 to dpu-1
	require.NoError(t, s.AssignTenantToDPU(ctx, &DPUAssignment{
		DPUID:    "dpu-1",
		TenantID: "tenant-1",
		InPort:   "pf0vf0",
		OutPort:  "pf0",
	}))

	// Assign tenant-2 to dpu-1
	require.NoError(t, s.AssignTenantToDPU(ctx, &DPUAssignment{
		DPUID:    "dpu-1",
		TenantID: "tenant-2",
		InPort:   "pf0vf1",
		OutPort:  "pf0",
	}))

	// Get assignments for dpu-1
	assignments, err := s.GetDPUAssignments(ctx, "dpu-1")
	require.NoError(t, err)
	assert.Len(t, assignments, 2)

	// Get tenant assignments
	tAssign, err := s.GetTenantDPUAssignment(ctx, "tenant-1")
	require.NoError(t, err)
	assert.Len(t, tAssign, 1)
	assert.Equal(t, "dpu-1", tAssign[0].DPUID)

	// Unassign
	require.NoError(t, s.UnassignTenantFromDPU(ctx, "tenant-1", "dpu-1"))
	assignments, err = s.GetDPUAssignments(ctx, "dpu-1")
	require.NoError(t, err)
	assert.Len(t, assignments, 1)

	// Unassign not found
	err = s.UnassignTenantFromDPU(ctx, "tenant-1", "dpu-1")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryStore_DPUDesiredState(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	// Setup: tenant + rules + DPU assignment
	require.NoError(t, s.CreateTenantNetwork(ctx, &TenantNetwork{
		TenantID:   "tenant-1",
		Name:       "test",
		SubnetCIDR: "10.0.0.0/24",
		GatewayIP:  "10.0.0.1",
		PublicIPs:  []string{"203.0.113.10"},
	}))

	require.NoError(t, s.AssignTenantToDPU(ctx, &DPUAssignment{
		DPUID:    "dpu-1",
		TenantID: "tenant-1",
		InPort:   "pf0vf0",
		OutPort:  "pf0",
	}))

	require.NoError(t, s.AddFirewallRule(ctx, &FirewallRule{
		ID:       "rule-1",
		TenantID: "tenant-1",
		SrcCIDR:  "0.0.0.0/0",
		DstCIDR:  "10.0.0.0/24",
		Protocol: ProtocolTCP,
		Action:   ActionAllow,
		Priority: 100,
	}))

	require.NoError(t, s.AddNATRule(ctx, &NATRule{
		ID:        "nat-1",
		TenantID:  "tenant-1",
		Type:      NATTypeDNAT,
		PublicIP:  "203.0.113.10",
		PrivateIP: "10.0.0.5",
		Protocol:  ProtocolTCP,
	}))

	require.NoError(t, s.SetRateLimit(ctx, &RateLimit{
		TenantID:           "tenant-1",
		IngressBytesPerSec: 1_000_000_000,
		EgressBytesPerSec:  500_000_000,
		BurstBytes:         10_000_000,
	}))

	// Query by DPU
	rules, err := s.ListFirewallRulesByDPU(ctx, "dpu-1")
	require.NoError(t, err)
	assert.Len(t, rules, 1)

	natRules, err := s.ListNATRulesByDPU(ctx, "dpu-1")
	require.NoError(t, err)
	assert.Len(t, natRules, 1)

	rateLimits, err := s.GetRateLimitsByDPU(ctx, "dpu-1")
	require.NoError(t, err)
	assert.Len(t, rateLimits, 1)

	// Version tracking
	ver, err := s.GetDPUStateVersion(ctx, "dpu-1")
	require.NoError(t, err)
	assert.True(t, ver > 0)
}

func TestMemoryStore_AgentStatus(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	status := &AgentStatus{
		DPUID:            "dpu-1",
		AgentType:        AgentTypeFirewall,
		TotalRules:       42,
		HWOffloaded:      40,
		PacketsForwarded: 1000000,
		UptimeSeconds:    3600,
	}
	require.NoError(t, s.UpdateAgentStatus(ctx, status))

	got, err := s.GetAgentStatus(ctx, "dpu-1", AgentTypeFirewall)
	require.NoError(t, err)
	assert.Equal(t, uint32(42), got.TotalRules)
	assert.Equal(t, uint32(40), got.HWOffloaded)
	assert.False(t, got.ReportedAt.IsZero())

	// Not found
	_, err = s.GetAgentStatus(ctx, "dpu-1", AgentTypeRouting)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryStore_Pagination(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	// Create 5 tenants
	for i := 0; i < 5; i++ {
		require.NoError(t, s.CreateTenantNetwork(ctx, &TenantNetwork{
			TenantID:   fmt.Sprintf("tenant-%02d", i),
			Name:       fmt.Sprintf("Network %d", i),
			SubnetCIDR: fmt.Sprintf("10.%d.0.0/24", i),
			GatewayIP:  fmt.Sprintf("10.%d.0.1", i),
		}))
	}

	// Page 1
	nets, token, err := s.ListTenantNetworks(ctx, 2, "")
	require.NoError(t, err)
	assert.Len(t, nets, 2)
	assert.NotEmpty(t, token)

	// Page 2
	nets2, token2, err := s.ListTenantNetworks(ctx, 2, token)
	require.NoError(t, err)
	assert.Len(t, nets2, 2)
	assert.NotEmpty(t, token2)

	// Page 3
	nets3, token3, err := s.ListTenantNetworks(ctx, 2, token2)
	require.NoError(t, err)
	assert.Len(t, nets3, 1)
	assert.Empty(t, token3)

	// Ensure no duplicates
	seen := make(map[string]bool)
	for _, n := range append(append(nets, nets2...), nets3...) {
		assert.False(t, seen[n.TenantID], "duplicate tenant: %s", n.TenantID)
		seen[n.TenantID] = true
	}
}

func TestMemoryStore_DeleteTenantCascade(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	require.NoError(t, s.CreateTenantNetwork(ctx, &TenantNetwork{
		TenantID:   "tenant-1",
		Name:       "test",
		SubnetCIDR: "10.0.0.0/24",
		GatewayIP:  "10.0.0.1",
	}))

	// Add some rules and assignments
	require.NoError(t, s.AddFirewallRule(ctx, &FirewallRule{
		ID: "rule-1", TenantID: "tenant-1", SrcCIDR: "0.0.0.0/0",
	}))
	require.NoError(t, s.AddNATRule(ctx, &NATRule{
		ID: "nat-1", TenantID: "tenant-1", Type: NATTypeSNAT,
	}))
	require.NoError(t, s.SetRateLimit(ctx, &RateLimit{
		TenantID: "tenant-1", IngressBytesPerSec: 1000,
	}))
	require.NoError(t, s.AssignTenantToDPU(ctx, &DPUAssignment{
		DPUID: "dpu-1", TenantID: "tenant-1", InPort: "pf0vf0", OutPort: "pf0",
	}))

	// Delete tenant - should cascade
	require.NoError(t, s.DeleteTenantNetwork(ctx, "tenant-1"))

	// All related data should be gone
	rules, _, _ := s.ListFirewallRules(ctx, "tenant-1", 10, "")
	assert.Len(t, rules, 0)

	natRules, _, _ := s.ListNATRules(ctx, "tenant-1", 10, "")
	assert.Len(t, natRules, 0)

	_, err := s.GetRateLimit(ctx, "tenant-1")
	assert.ErrorIs(t, err, ErrNotFound)

	assignments, _ := s.GetDPUAssignments(ctx, "dpu-1")
	assert.Len(t, assignments, 0)
}
