package store

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// MemoryStore is an in-memory implementation of Store for development and testing.
// All data is lost on restart. Thread-safe via sync.RWMutex.
type MemoryStore struct {
	mu sync.RWMutex

	tenants       map[string]*TenantNetwork            // tenantID -> network
	firewallRules map[string]map[string]*FirewallRule   // tenantID -> ruleID -> rule
	natRules      map[string]map[string]*NATRule        // tenantID -> ruleID -> rule
	rateLimits    map[string]*RateLimit                 // tenantID -> limit
	assignments   map[string]map[string]*DPUAssignment  // dpuID -> tenantID -> assignment
	agentStatuses map[string]map[AgentType]*AgentStatus // dpuID -> agentType -> status

	// dpuVersions tracks state version per DPU for conditional fetch.
	dpuVersions map[string]*atomic.Uint64
	globalVer   atomic.Uint64
}

// NewMemoryStore creates a new in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		tenants:       make(map[string]*TenantNetwork),
		firewallRules: make(map[string]map[string]*FirewallRule),
		natRules:      make(map[string]map[string]*NATRule),
		rateLimits:    make(map[string]*RateLimit),
		assignments:   make(map[string]map[string]*DPUAssignment),
		agentStatuses: make(map[string]map[AgentType]*AgentStatus),
		dpuVersions:   make(map[string]*atomic.Uint64),
	}
}

// bumpDPUVersion increments the version for all DPUs that have this tenant assigned.
// Must be called with mu held (at least read lock for assignments, write for versions).
func (m *MemoryStore) bumpDPUVersionsForTenant(tenantID string) {
	for dpuID, tenants := range m.assignments {
		if _, ok := tenants[tenantID]; ok {
			m.getDPUVersion(dpuID).Add(1)
		}
	}
	m.globalVer.Add(1)
}

func (m *MemoryStore) getDPUVersion(dpuID string) *atomic.Uint64 {
	v, ok := m.dpuVersions[dpuID]
	if !ok {
		v = &atomic.Uint64{}
		m.dpuVersions[dpuID] = v
	}
	return v
}

// --- Tenant Networks ---

func (m *MemoryStore) CreateTenantNetwork(_ context.Context, network *TenantNetwork) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.tenants[network.TenantID]; exists {
		return fmt.Errorf("tenant %s: %w", network.TenantID, ErrAlreadyExists)
	}

	now := time.Now()
	network.CreatedAt = now
	network.UpdatedAt = now
	m.tenants[network.TenantID] = network
	return nil
}

func (m *MemoryStore) GetTenantNetwork(_ context.Context, tenantID string) (*TenantNetwork, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	net, ok := m.tenants[tenantID]
	if !ok {
		return nil, fmt.Errorf("tenant %s: %w", tenantID, ErrNotFound)
	}
	return net, nil
}

func (m *MemoryStore) DeleteTenantNetwork(_ context.Context, tenantID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.tenants[tenantID]; !ok {
		return fmt.Errorf("tenant %s: %w", tenantID, ErrNotFound)
	}

	delete(m.tenants, tenantID)
	delete(m.firewallRules, tenantID)
	delete(m.natRules, tenantID)
	delete(m.rateLimits, tenantID)

	// Clean up DPU assignments for this tenant
	for dpuID, tenants := range m.assignments {
		delete(tenants, tenantID)
		if len(tenants) == 0 {
			delete(m.assignments, dpuID)
		}
	}

	m.globalVer.Add(1)
	return nil
}

func (m *MemoryStore) ListTenantNetworks(_ context.Context, pageSize int, pageToken string) ([]*TenantNetwork, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if pageSize <= 0 {
		pageSize = 50
	}

	// Collect and sort by tenant ID for stable pagination
	all := make([]*TenantNetwork, 0, len(m.tenants))
	for _, n := range m.tenants {
		all = append(all, n)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].TenantID < all[j].TenantID })

	// Find start position
	start := 0
	if pageToken != "" {
		for i, n := range all {
			if n.TenantID > pageToken {
				start = i
				break
			}
		}
	}

	end := start + pageSize
	if end > len(all) {
		end = len(all)
	}

	result := all[start:end]
	nextToken := ""
	if end < len(all) {
		nextToken = result[len(result)-1].TenantID
	}

	return result, nextToken, nil
}

// --- Firewall Rules ---

func (m *MemoryStore) AddFirewallRule(_ context.Context, rule *FirewallRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.tenants[rule.TenantID]; !ok {
		return fmt.Errorf("tenant %s: %w", rule.TenantID, ErrNotFound)
	}

	if m.firewallRules[rule.TenantID] == nil {
		m.firewallRules[rule.TenantID] = make(map[string]*FirewallRule)
	}

	rule.CreatedAt = time.Now()
	m.firewallRules[rule.TenantID][rule.ID] = rule
	m.bumpDPUVersionsForTenant(rule.TenantID)
	return nil
}

func (m *MemoryStore) DeleteFirewallRule(_ context.Context, tenantID, ruleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rules, ok := m.firewallRules[tenantID]
	if !ok {
		return fmt.Errorf("rule %s for tenant %s: %w", ruleID, tenantID, ErrNotFound)
	}
	if _, ok := rules[ruleID]; !ok {
		return fmt.Errorf("rule %s for tenant %s: %w", ruleID, tenantID, ErrNotFound)
	}

	delete(rules, ruleID)
	m.bumpDPUVersionsForTenant(tenantID)
	return nil
}

func (m *MemoryStore) ListFirewallRules(_ context.Context, tenantID string, pageSize int, pageToken string) ([]*FirewallRule, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if pageSize <= 0 {
		pageSize = 100
	}

	rules := m.firewallRules[tenantID]
	all := make([]*FirewallRule, 0, len(rules))
	for _, r := range rules {
		all = append(all, r)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })

	start := 0
	if pageToken != "" {
		for i, r := range all {
			if r.ID > pageToken {
				start = i
				break
			}
		}
	}

	end := start + pageSize
	if end > len(all) {
		end = len(all)
	}

	result := all[start:end]
	nextToken := ""
	if end < len(all) {
		nextToken = result[len(result)-1].ID
	}

	return result, nextToken, nil
}

func (m *MemoryStore) ListFirewallRulesByDPU(_ context.Context, dpuID string) ([]*FirewallRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tenants := m.assignments[dpuID]
	var result []*FirewallRule
	for tenantID := range tenants {
		for _, rule := range m.firewallRules[tenantID] {
			result = append(result, rule)
		}
	}
	return result, nil
}

// --- NAT Rules ---

func (m *MemoryStore) AddNATRule(_ context.Context, rule *NATRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.tenants[rule.TenantID]; !ok {
		return fmt.Errorf("tenant %s: %w", rule.TenantID, ErrNotFound)
	}

	if m.natRules[rule.TenantID] == nil {
		m.natRules[rule.TenantID] = make(map[string]*NATRule)
	}

	rule.CreatedAt = time.Now()
	m.natRules[rule.TenantID][rule.ID] = rule
	m.bumpDPUVersionsForTenant(rule.TenantID)
	return nil
}

func (m *MemoryStore) DeleteNATRule(_ context.Context, tenantID, ruleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rules, ok := m.natRules[tenantID]
	if !ok {
		return fmt.Errorf("NAT rule %s for tenant %s: %w", ruleID, tenantID, ErrNotFound)
	}
	if _, ok := rules[ruleID]; !ok {
		return fmt.Errorf("NAT rule %s for tenant %s: %w", ruleID, tenantID, ErrNotFound)
	}

	delete(rules, ruleID)
	m.bumpDPUVersionsForTenant(tenantID)
	return nil
}

func (m *MemoryStore) ListNATRules(_ context.Context, tenantID string, pageSize int, pageToken string) ([]*NATRule, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if pageSize <= 0 {
		pageSize = 100
	}

	rules := m.natRules[tenantID]
	all := make([]*NATRule, 0, len(rules))
	for _, r := range rules {
		all = append(all, r)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })

	start := 0
	if pageToken != "" {
		for i, r := range all {
			if r.ID > pageToken {
				start = i
				break
			}
		}
	}

	end := start + pageSize
	if end > len(all) {
		end = len(all)
	}

	result := all[start:end]
	nextToken := ""
	if end < len(all) {
		nextToken = result[len(result)-1].ID
	}

	return result, nextToken, nil
}

func (m *MemoryStore) ListNATRulesByDPU(_ context.Context, dpuID string) ([]*NATRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tenants := m.assignments[dpuID]
	var result []*NATRule
	for tenantID := range tenants {
		for _, rule := range m.natRules[tenantID] {
			result = append(result, rule)
		}
	}
	return result, nil
}

// --- Rate Limits ---

func (m *MemoryStore) SetRateLimit(_ context.Context, limit *RateLimit) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.tenants[limit.TenantID]; !ok {
		return fmt.Errorf("tenant %s: %w", limit.TenantID, ErrNotFound)
	}

	m.rateLimits[limit.TenantID] = limit
	m.bumpDPUVersionsForTenant(limit.TenantID)
	return nil
}

func (m *MemoryStore) GetRateLimit(_ context.Context, tenantID string) (*RateLimit, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	limit, ok := m.rateLimits[tenantID]
	if !ok {
		return nil, fmt.Errorf("rate limit for tenant %s: %w", tenantID, ErrNotFound)
	}
	return limit, nil
}

func (m *MemoryStore) GetRateLimitsByDPU(_ context.Context, dpuID string) ([]*RateLimit, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tenants := m.assignments[dpuID]
	var result []*RateLimit
	for tenantID := range tenants {
		if limit, ok := m.rateLimits[tenantID]; ok {
			result = append(result, limit)
		}
	}
	return result, nil
}

// --- DPU Assignments ---

func (m *MemoryStore) AssignTenantToDPU(_ context.Context, assignment *DPUAssignment) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.tenants[assignment.TenantID]; !ok {
		return fmt.Errorf("tenant %s: %w", assignment.TenantID, ErrNotFound)
	}

	if m.assignments[assignment.DPUID] == nil {
		m.assignments[assignment.DPUID] = make(map[string]*DPUAssignment)
	}

	assignment.AssignedAt = time.Now()
	m.assignments[assignment.DPUID][assignment.TenantID] = assignment
	m.getDPUVersion(assignment.DPUID).Add(1)
	return nil
}

func (m *MemoryStore) UnassignTenantFromDPU(_ context.Context, tenantID, dpuID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tenants, ok := m.assignments[dpuID]
	if !ok {
		return fmt.Errorf("assignment for tenant %s on DPU %s: %w", tenantID, dpuID, ErrNotFound)
	}
	if _, ok := tenants[tenantID]; !ok {
		return fmt.Errorf("assignment for tenant %s on DPU %s: %w", tenantID, dpuID, ErrNotFound)
	}

	delete(tenants, tenantID)
	if len(tenants) == 0 {
		delete(m.assignments, dpuID)
	}
	m.getDPUVersion(dpuID).Add(1)
	return nil
}

func (m *MemoryStore) GetDPUAssignments(_ context.Context, dpuID string) ([]*DPUAssignment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tenants := m.assignments[dpuID]
	result := make([]*DPUAssignment, 0, len(tenants))
	for _, a := range tenants {
		result = append(result, a)
	}
	return result, nil
}

func (m *MemoryStore) GetTenantDPUAssignment(_ context.Context, tenantID string) ([]*DPUAssignment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*DPUAssignment
	for _, tenants := range m.assignments {
		if a, ok := tenants[tenantID]; ok {
			result = append(result, a)
		}
	}
	return result, nil
}

// --- Agent Status ---

func (m *MemoryStore) UpdateAgentStatus(_ context.Context, status *AgentStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.agentStatuses[status.DPUID] == nil {
		m.agentStatuses[status.DPUID] = make(map[AgentType]*AgentStatus)
	}

	status.ReportedAt = time.Now()
	m.agentStatuses[status.DPUID][status.AgentType] = status
	return nil
}

func (m *MemoryStore) GetAgentStatus(_ context.Context, dpuID string, agentType AgentType) (*AgentStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents, ok := m.agentStatuses[dpuID]
	if !ok {
		return nil, fmt.Errorf("agent status for DPU %s: %w", dpuID, ErrNotFound)
	}
	status, ok := agents[agentType]
	if !ok {
		return nil, fmt.Errorf("agent type %d status for DPU %s: %w", agentType, dpuID, ErrNotFound)
	}
	return status, nil
}

// --- Version Tracking ---

func (m *MemoryStore) GetDPUStateVersion(_ context.Context, dpuID string) (uint64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	v, ok := m.dpuVersions[dpuID]
	if !ok {
		return 0, nil
	}
	return v.Load(), nil
}
