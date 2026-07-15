// Package server implements the Network Manager gRPC/ConnectRPC service handlers.
// It translates RPC calls into store operations, applying validation and
// business logic (e.g., aggregating desired state per DPU).
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/mara-baysn/technicolor-haze-network-manager/internal/ipam"
	"github.com/mara-baysn/technicolor-haze-network-manager/internal/store"
	"github.com/mara-baysn/technicolor-haze-network-manager/internal/tenant"
)

// Server implements the NetworkManagerService handlers.
type Server struct {
	store  store.Store
	ipPool *ipam.Pool
	logger *slog.Logger
}

// New creates a new Server with the given store and IP pool.
func New(s store.Store, pool *ipam.Pool, logger *slog.Logger) *Server {
	return &Server{
		store:  s,
		ipPool: pool,
		logger: logger,
	}
}

// Handler returns an http.Handler with all routes registered.
// Routes follow the ConnectRPC URL pattern: /package.Service/Method
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Tenant network lifecycle
	mux.HandleFunc("POST /networkmanager.v1.NetworkManagerService/CreateTenantNetwork", s.handleCreateTenantNetwork)
	mux.HandleFunc("POST /networkmanager.v1.NetworkManagerService/DeleteTenantNetwork", s.handleDeleteTenantNetwork)
	mux.HandleFunc("POST /networkmanager.v1.NetworkManagerService/GetTenantNetwork", s.handleGetTenantNetwork)
	mux.HandleFunc("POST /networkmanager.v1.NetworkManagerService/ListTenantNetworks", s.handleListTenantNetworks)

	// Firewall rules
	mux.HandleFunc("POST /networkmanager.v1.NetworkManagerService/AddFirewallRule", s.handleAddFirewallRule)
	mux.HandleFunc("POST /networkmanager.v1.NetworkManagerService/DeleteFirewallRule", s.handleDeleteFirewallRule)
	mux.HandleFunc("POST /networkmanager.v1.NetworkManagerService/ListFirewallRules", s.handleListFirewallRules)

	// NAT rules
	mux.HandleFunc("POST /networkmanager.v1.NetworkManagerService/AddNATRule", s.handleAddNATRule)
	mux.HandleFunc("POST /networkmanager.v1.NetworkManagerService/DeleteNATRule", s.handleDeleteNATRule)
	mux.HandleFunc("POST /networkmanager.v1.NetworkManagerService/ListNATRules", s.handleListNATRules)

	// Rate limits
	mux.HandleFunc("POST /networkmanager.v1.NetworkManagerService/SetRateLimit", s.handleSetRateLimit)
	mux.HandleFunc("POST /networkmanager.v1.NetworkManagerService/GetRateLimit", s.handleGetRateLimit)

	// IP allocation
	mux.HandleFunc("POST /networkmanager.v1.NetworkManagerService/AllocatePublicIP", s.handleAllocatePublicIP)
	mux.HandleFunc("POST /networkmanager.v1.NetworkManagerService/ReleasePublicIP", s.handleReleasePublicIP)
	mux.HandleFunc("POST /networkmanager.v1.NetworkManagerService/ListPublicIPs", s.handleListPublicIPs)

	// DPU desired state (pull model)
	mux.HandleFunc("POST /networkmanager.v1.NetworkManagerService/GetDPUDesiredState", s.handleGetDPUDesiredState)

	// Agent status
	mux.HandleFunc("POST /networkmanager.v1.NetworkManagerService/ReportAgentStatus", s.handleReportAgentStatus)

	// DPU assignments
	mux.HandleFunc("POST /networkmanager.v1.NetworkManagerService/AssignTenantToDPU", s.handleAssignTenantToDPU)
	mux.HandleFunc("POST /networkmanager.v1.NetworkManagerService/UnassignTenantFromDPU", s.handleUnassignTenantFromDPU)
	mux.HandleFunc("POST /networkmanager.v1.NetworkManagerService/GetDPUAssignments", s.handleGetDPUAssignments)

	// Health check
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	return mux
}

// --- Request/Response types (matching proto messages as JSON) ---

// Tenant network types
type CreateTenantNetworkRequest struct {
	TenantID   string `json:"tenant_id"`
	Name       string `json:"name"`
	SubnetCIDR string `json:"subnet_cidr"`
	GatewayIP  string `json:"gateway_ip"`
}

type TenantNetworkResponse struct {
	Network *TenantNetworkJSON `json:"network"`
}

type TenantNetworkJSON struct {
	TenantID   string   `json:"tenant_id"`
	Name       string   `json:"name"`
	SubnetCIDR string   `json:"subnet_cidr"`
	GatewayIP  string   `json:"gateway_ip"`
	PublicIPs  []string `json:"public_ips"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
}

type DeleteTenantNetworkRequest struct {
	TenantID string `json:"tenant_id"`
}

type GetTenantNetworkRequest struct {
	TenantID string `json:"tenant_id"`
}

type ListTenantNetworksRequest struct {
	PageSize  int    `json:"page_size"`
	PageToken string `json:"page_token"`
}

type ListTenantNetworksResponse struct {
	Networks      []*TenantNetworkJSON `json:"networks"`
	NextPageToken string               `json:"next_page_token"`
}

// Firewall rule types
type AddFirewallRuleRequest struct {
	TenantID string  `json:"tenant_id"`
	SrcCIDR  string  `json:"src_cidr"`
	DstCIDR  string  `json:"dst_cidr"`
	SrcPort  *uint32 `json:"src_port,omitempty"`
	DstPort  *uint32 `json:"dst_port,omitempty"`
	Protocol int     `json:"protocol"`
	Action   int     `json:"action"`
	Priority uint32  `json:"priority"`
}

type FirewallRuleJSON struct {
	ID        string  `json:"id"`
	TenantID  string  `json:"tenant_id"`
	SrcCIDR   string  `json:"src_cidr"`
	DstCIDR   string  `json:"dst_cidr"`
	SrcPort   *uint32 `json:"src_port,omitempty"`
	DstPort   *uint32 `json:"dst_port,omitempty"`
	Protocol  int     `json:"protocol"`
	Action    int     `json:"action"`
	Priority  uint32  `json:"priority"`
	IPVersion int     `json:"ip_version"`
	CreatedAt string  `json:"created_at"`
}

type DeleteFirewallRuleRequest struct {
	TenantID string `json:"tenant_id"`
	RuleID   string `json:"rule_id"`
}

type ListFirewallRulesRequest struct {
	TenantID  string `json:"tenant_id"`
	PageSize  int    `json:"page_size"`
	PageToken string `json:"page_token"`
}

type ListFirewallRulesResponse struct {
	Rules         []*FirewallRuleJSON `json:"rules"`
	NextPageToken string              `json:"next_page_token"`
}

// NAT rule types
type AddNATRuleRequest struct {
	TenantID       string  `json:"tenant_id"`
	Type           int     `json:"type"`
	PublicIP       string  `json:"public_ip"`
	PublicPort     *uint32 `json:"public_port,omitempty"`
	PrivateIP      string  `json:"private_ip"`
	PrivatePort    *uint32 `json:"private_port,omitempty"`
	ExternalPrefix string  `json:"external_prefix"`
	InternalPrefix string  `json:"internal_prefix"`
	PrefixLength   uint32  `json:"prefix_length"`
	Protocol       int     `json:"protocol"`
	Mode           int     `json:"mode"`
}

type NATRuleJSON struct {
	ID             string  `json:"id"`
	TenantID       string  `json:"tenant_id"`
	Type           int     `json:"type"`
	PublicIP       string  `json:"public_ip"`
	PublicPort     *uint32 `json:"public_port,omitempty"`
	PrivateIP      string  `json:"private_ip"`
	PrivatePort    *uint32 `json:"private_port,omitempty"`
	ExternalPrefix string  `json:"external_prefix"`
	InternalPrefix string  `json:"internal_prefix"`
	PrefixLength   uint32  `json:"prefix_length"`
	Protocol       int     `json:"protocol"`
	Mode           int     `json:"mode"`
	IPVersion      int     `json:"ip_version"`
	CreatedAt      string  `json:"created_at"`
}

type DeleteNATRuleRequest struct {
	TenantID string `json:"tenant_id"`
	RuleID   string `json:"rule_id"`
}

type ListNATRulesRequest struct {
	TenantID  string `json:"tenant_id"`
	PageSize  int    `json:"page_size"`
	PageToken string `json:"page_token"`
}

type ListNATRulesResponse struct {
	Rules         []*NATRuleJSON `json:"rules"`
	NextPageToken string         `json:"next_page_token"`
}

// Rate limit types
type SetRateLimitRequest struct {
	TenantID           string `json:"tenant_id"`
	IngressBytesPerSec uint64 `json:"ingress_bytes_per_second"`
	EgressBytesPerSec  uint64 `json:"egress_bytes_per_second"`
	BurstBytes         uint64 `json:"burst_bytes"`
}

type RateLimitJSON struct {
	TenantID           string `json:"tenant_id"`
	IngressBytesPerSec uint64 `json:"ingress_bytes_per_second"`
	EgressBytesPerSec  uint64 `json:"egress_bytes_per_second"`
	BurstBytes         uint64 `json:"burst_bytes"`
}

type GetRateLimitRequest struct {
	TenantID string `json:"tenant_id"`
}

// IP allocation types
type AllocatePublicIPRequest struct {
	TenantID  string `json:"tenant_id"`
	IPVersion int    `json:"ip_version"`
}

type PublicIPAllocationJSON struct {
	IP          string `json:"ip"`
	TenantID    string `json:"tenant_id"`
	IPVersion   int    `json:"ip_version"`
	AllocatedAt string `json:"allocated_at"`
}

type ReleasePublicIPRequest struct {
	IP string `json:"ip"`
}

type ListPublicIPsRequest struct {
	TenantID  string `json:"tenant_id"`
	PageSize  int    `json:"page_size"`
	PageToken string `json:"page_token"`
}

type ListPublicIPsResponse struct {
	Allocations   []*PublicIPAllocationJSON `json:"allocations"`
	NextPageToken string                    `json:"next_page_token"`
}

// DPU desired state types
type GetDPUDesiredStateRequest struct {
	DPUID            string `json:"dpu_id"`
	LastKnownVersion uint64 `json:"last_known_version"`
}

type GetDPUDesiredStateResponse struct {
	DesiredState *DPUDesiredStateJSON `json:"desired_state"`
	Unchanged   bool                 `json:"unchanged"`
}

type DPUDesiredStateJSON struct {
	DPUID   string                    `json:"dpu_id"`
	Tenants []*TenantDesiredStateJSON `json:"tenants"`
	Version uint64                    `json:"version"`
}

type TenantDesiredStateJSON struct {
	TenantID      string             `json:"tenant_id"`
	InPort        string             `json:"in_port"`
	OutPort       string             `json:"out_port"`
	PublicIPs     []string           `json:"public_ips"`
	FirewallRules []*FirewallRuleJSON `json:"firewall_rules"`
	NATRules      []*NATRuleJSON     `json:"nat_rules"`
	RateLimit     *RateLimitJSON     `json:"rate_limit,omitempty"`
}

// Agent status types
type ReportAgentStatusRequest struct {
	DPUID            string `json:"dpu_id"`
	AgentType        int    `json:"agent_type"`
	TotalRules       uint32 `json:"total_rules"`
	HWOffloaded      uint32 `json:"hw_offloaded"`
	PacketsForwarded uint64 `json:"packets_forwarded"`
	PacketsDropped   uint64 `json:"packets_dropped"`
	BytesForwarded   uint64 `json:"bytes_forwarded"`
	BytesDropped     uint64 `json:"bytes_dropped"`
	UptimeSeconds    uint64 `json:"uptime_seconds"`
	LastSyncError    string `json:"last_sync_error"`
}

type ReportAgentStatusResponse struct {
	AcknowledgedAt string `json:"acknowledged_at"`
}

// DPU assignment types
type AssignTenantToDPURequest struct {
	TenantID string `json:"tenant_id"`
	DPUID    string `json:"dpu_id"`
	InPort   string `json:"in_port"`
	OutPort  string `json:"out_port"`
}

type DPUAssignmentJSON struct {
	DPUID      string `json:"dpu_id"`
	TenantID   string `json:"tenant_id"`
	InPort     string `json:"in_port"`
	OutPort    string `json:"out_port"`
	AssignedAt string `json:"assigned_at"`
}

type UnassignTenantFromDPURequest struct {
	TenantID string `json:"tenant_id"`
	DPUID    string `json:"dpu_id"`
}

type GetDPUAssignmentsRequest struct {
	DPUID string `json:"dpu_id"`
}

type GetDPUAssignmentsResponse struct {
	Assignments []*DPUAssignmentJSON `json:"assignments"`
}

// --- Handlers ---

func (s *Server) handleCreateTenantNetwork(w http.ResponseWriter, r *http.Request) {
	var req CreateTenantNetworkRequest
	if !s.decode(w, r, &req) {
		return
	}

	if err := tenant.ValidateCreateNetwork(req.TenantID, req.Name, req.SubnetCIDR, req.GatewayIP); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	net := &store.TenantNetwork{
		TenantID:   req.TenantID,
		Name:       req.Name,
		SubnetCIDR: req.SubnetCIDR,
		GatewayIP:  req.GatewayIP,
	}

	if err := s.store.CreateTenantNetwork(r.Context(), net); err != nil {
		if errors.Is(err, store.ErrAlreadyExists) {
			s.writeError(w, http.StatusConflict, err)
			return
		}
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.logger.Info("created tenant network", "tenant_id", req.TenantID, "subnet", req.SubnetCIDR)
	s.writeJSON(w, http.StatusOK, TenantNetworkResponse{Network: toTenantNetworkJSON(net)})
}

func (s *Server) handleDeleteTenantNetwork(w http.ResponseWriter, r *http.Request) {
	var req DeleteTenantNetworkRequest
	if !s.decode(w, r, &req) {
		return
	}

	if err := s.store.DeleteTenantNetwork(r.Context(), req.TenantID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.writeError(w, http.StatusNotFound, err)
			return
		}
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.logger.Info("deleted tenant network", "tenant_id", req.TenantID)
	s.writeJSON(w, http.StatusOK, struct{}{})
}

func (s *Server) handleGetTenantNetwork(w http.ResponseWriter, r *http.Request) {
	var req GetTenantNetworkRequest
	if !s.decode(w, r, &req) {
		return
	}

	net, err := s.store.GetTenantNetwork(r.Context(), req.TenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.writeError(w, http.StatusNotFound, err)
			return
		}
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.writeJSON(w, http.StatusOK, TenantNetworkResponse{Network: toTenantNetworkJSON(net)})
}

func (s *Server) handleListTenantNetworks(w http.ResponseWriter, r *http.Request) {
	var req ListTenantNetworksRequest
	if !s.decode(w, r, &req) {
		return
	}

	nets, nextToken, err := s.store.ListTenantNetworks(r.Context(), req.PageSize, req.PageToken)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	resp := ListTenantNetworksResponse{NextPageToken: nextToken}
	for _, n := range nets {
		resp.Networks = append(resp.Networks, toTenantNetworkJSON(n))
	}
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAddFirewallRule(w http.ResponseWriter, r *http.Request) {
	var req AddFirewallRuleRequest
	if !s.decode(w, r, &req) {
		return
	}

	if err := tenant.ValidateFirewallRule(req.TenantID, req.SrcCIDR, req.DstCIDR, req.SrcPort, req.DstPort, req.Priority); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	// Auto-detect IP version
	ipVer := store.IPVersionUnspecified
	if v := tenant.DetectIPVersion(req.SrcCIDR); v == 6 {
		ipVer = store.IPVersion6
	} else if v == 4 {
		ipVer = store.IPVersion4
	} else if v := tenant.DetectIPVersion(req.DstCIDR); v == 6 {
		ipVer = store.IPVersion6
	} else if v == 4 {
		ipVer = store.IPVersion4
	}

	rule := &store.FirewallRule{
		ID:        uuid.New().String(),
		TenantID:  req.TenantID,
		SrcCIDR:   req.SrcCIDR,
		DstCIDR:   req.DstCIDR,
		SrcPort:   req.SrcPort,
		DstPort:   req.DstPort,
		Protocol:  store.Protocol(req.Protocol),
		Action:    store.Action(req.Action),
		Priority:  req.Priority,
		IPVersion: ipVer,
	}

	if err := s.store.AddFirewallRule(r.Context(), rule); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.writeError(w, http.StatusNotFound, err)
			return
		}
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.logger.Info("added firewall rule", "tenant_id", req.TenantID, "rule_id", rule.ID)
	s.writeJSON(w, http.StatusOK, struct {
		Rule *FirewallRuleJSON `json:"rule"`
	}{Rule: toFirewallRuleJSON(rule)})
}

func (s *Server) handleDeleteFirewallRule(w http.ResponseWriter, r *http.Request) {
	var req DeleteFirewallRuleRequest
	if !s.decode(w, r, &req) {
		return
	}

	if err := s.store.DeleteFirewallRule(r.Context(), req.TenantID, req.RuleID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.writeError(w, http.StatusNotFound, err)
			return
		}
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.writeJSON(w, http.StatusOK, struct{}{})
}

func (s *Server) handleListFirewallRules(w http.ResponseWriter, r *http.Request) {
	var req ListFirewallRulesRequest
	if !s.decode(w, r, &req) {
		return
	}

	rules, nextToken, err := s.store.ListFirewallRules(r.Context(), req.TenantID, req.PageSize, req.PageToken)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	resp := ListFirewallRulesResponse{NextPageToken: nextToken}
	for _, rule := range rules {
		resp.Rules = append(resp.Rules, toFirewallRuleJSON(rule))
	}
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAddNATRule(w http.ResponseWriter, r *http.Request) {
	var req AddNATRuleRequest
	if !s.decode(w, r, &req) {
		return
	}

	if err := tenant.ValidateNATRule(req.TenantID, req.PublicIP, req.PrivateIP); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	// Determine IP version
	ipVer := store.IPVersionUnspecified
	natType := store.NATType(req.Type)
	if natType == store.NATTypeNPTv6 {
		ipVer = store.IPVersion6
	} else if req.PublicIP != "" {
		if tenant.DetectIPVersion(req.PublicIP) == 6 {
			ipVer = store.IPVersion6
		} else {
			ipVer = store.IPVersion4
		}
	}

	rule := &store.NATRule{
		ID:             uuid.New().String(),
		TenantID:       req.TenantID,
		Type:           natType,
		PublicIP:       req.PublicIP,
		PublicPort:     req.PublicPort,
		PrivateIP:      req.PrivateIP,
		PrivatePort:    req.PrivatePort,
		ExternalPrefix: req.ExternalPrefix,
		InternalPrefix: req.InternalPrefix,
		PrefixLength:   req.PrefixLength,
		Protocol:       store.Protocol(req.Protocol),
		Mode:           store.NATMode(req.Mode),
		IPVersion:      ipVer,
	}

	if err := s.store.AddNATRule(r.Context(), rule); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.writeError(w, http.StatusNotFound, err)
			return
		}
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.logger.Info("added NAT rule", "tenant_id", req.TenantID, "rule_id", rule.ID, "type", req.Type)
	s.writeJSON(w, http.StatusOK, struct {
		Rule *NATRuleJSON `json:"rule"`
	}{Rule: toNATRuleJSON(rule)})
}

func (s *Server) handleDeleteNATRule(w http.ResponseWriter, r *http.Request) {
	var req DeleteNATRuleRequest
	if !s.decode(w, r, &req) {
		return
	}

	if err := s.store.DeleteNATRule(r.Context(), req.TenantID, req.RuleID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.writeError(w, http.StatusNotFound, err)
			return
		}
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.writeJSON(w, http.StatusOK, struct{}{})
}

func (s *Server) handleListNATRules(w http.ResponseWriter, r *http.Request) {
	var req ListNATRulesRequest
	if !s.decode(w, r, &req) {
		return
	}

	rules, nextToken, err := s.store.ListNATRules(r.Context(), req.TenantID, req.PageSize, req.PageToken)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	resp := ListNATRulesResponse{NextPageToken: nextToken}
	for _, rule := range rules {
		resp.Rules = append(resp.Rules, toNATRuleJSON(rule))
	}
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSetRateLimit(w http.ResponseWriter, r *http.Request) {
	var req SetRateLimitRequest
	if !s.decode(w, r, &req) {
		return
	}

	if err := tenant.ValidateRateLimit(req.TenantID, req.IngressBytesPerSec, req.EgressBytesPerSec, req.BurstBytes); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	limit := &store.RateLimit{
		TenantID:           req.TenantID,
		IngressBytesPerSec: req.IngressBytesPerSec,
		EgressBytesPerSec:  req.EgressBytesPerSec,
		BurstBytes:         req.BurstBytes,
	}

	if err := s.store.SetRateLimit(r.Context(), limit); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.writeError(w, http.StatusNotFound, err)
			return
		}
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.writeJSON(w, http.StatusOK, struct {
		RateLimit *RateLimitJSON `json:"rate_limit"`
	}{RateLimit: toRateLimitJSON(limit)})
}

func (s *Server) handleGetRateLimit(w http.ResponseWriter, r *http.Request) {
	var req GetRateLimitRequest
	if !s.decode(w, r, &req) {
		return
	}

	limit, err := s.store.GetRateLimit(r.Context(), req.TenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.writeError(w, http.StatusNotFound, err)
			return
		}
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.writeJSON(w, http.StatusOK, struct {
		RateLimit *RateLimitJSON `json:"rate_limit"`
	}{RateLimit: toRateLimitJSON(limit)})
}

func (s *Server) handleAllocatePublicIP(w http.ResponseWriter, r *http.Request) {
	var req AllocatePublicIPRequest
	if !s.decode(w, r, &req) {
		return
	}

	if req.TenantID == "" {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("tenant_id is required"))
		return
	}

	// Map proto IP version to ipam version
	preferVer := ipam.IPv4
	if req.IPVersion == 2 { // IP_VERSION_6
		preferVer = ipam.IPv6
	}

	alloc, err := s.ipPool.Allocate(req.TenantID, preferVer)
	if err != nil {
		if errors.Is(err, ipam.ErrNoAvailableIPs) {
			s.writeError(w, http.StatusServiceUnavailable, err)
			return
		}
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	// Also update the tenant network's public IPs list
	ctx := r.Context()
	net, err := s.store.GetTenantNetwork(ctx, req.TenantID)
	if err == nil {
		net.PublicIPs = append(net.PublicIPs, alloc.IP)
		net.UpdatedAt = time.Now()
	}

	s.logger.Info("allocated public IP", "tenant_id", req.TenantID, "ip", alloc.IP)
	s.writeJSON(w, http.StatusOK, struct {
		Allocation *PublicIPAllocationJSON `json:"allocation"`
	}{Allocation: toPublicIPAllocationJSON(alloc)})
}

func (s *Server) handleReleasePublicIP(w http.ResponseWriter, r *http.Request) {
	var req ReleasePublicIPRequest
	if !s.decode(w, r, &req) {
		return
	}

	if req.IP == "" {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("ip is required"))
		return
	}

	if err := s.ipPool.Release(req.IP); err != nil {
		if errors.Is(err, ipam.ErrIPNotAllocated) {
			s.writeError(w, http.StatusNotFound, err)
			return
		}
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.logger.Info("released public IP", "ip", req.IP)
	s.writeJSON(w, http.StatusOK, struct{}{})
}

func (s *Server) handleListPublicIPs(w http.ResponseWriter, r *http.Request) {
	var req ListPublicIPsRequest
	if !s.decode(w, r, &req) {
		return
	}

	allocs := s.ipPool.ListAllocations(req.TenantID)

	resp := ListPublicIPsResponse{}
	for _, a := range allocs {
		resp.Allocations = append(resp.Allocations, toPublicIPAllocationJSON(a))
	}
	s.writeJSON(w, http.StatusOK, resp)
}

// handleGetDPUDesiredState aggregates all rules for all tenants assigned to a DPU.
// This is the critical pull endpoint that DPU agents call.
func (s *Server) handleGetDPUDesiredState(w http.ResponseWriter, r *http.Request) {
	var req GetDPUDesiredStateRequest
	if !s.decode(w, r, &req) {
		return
	}

	if req.DPUID == "" {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("dpu_id is required"))
		return
	}

	ctx := r.Context()

	// Check version for conditional fetch
	currentVersion, err := s.store.GetDPUStateVersion(ctx, req.DPUID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	if req.LastKnownVersion > 0 && req.LastKnownVersion >= currentVersion {
		s.writeJSON(w, http.StatusOK, GetDPUDesiredStateResponse{Unchanged: true})
		return
	}

	// Get all tenant assignments for this DPU
	assignments, err := s.store.GetDPUAssignments(ctx, req.DPUID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	desiredState := &DPUDesiredStateJSON{
		DPUID:   req.DPUID,
		Tenants: make([]*TenantDesiredStateJSON, 0, len(assignments)),
		Version: currentVersion,
	}

	// For each assigned tenant, aggregate their rules
	for _, assignment := range assignments {
		tenantState := &TenantDesiredStateJSON{
			TenantID: assignment.TenantID,
			InPort:   assignment.InPort,
			OutPort:  assignment.OutPort,
		}

		// Get tenant network for public IPs
		if net, err := s.store.GetTenantNetwork(ctx, assignment.TenantID); err == nil {
			tenantState.PublicIPs = net.PublicIPs
		}

		// Get firewall rules
		fwRules, _, err := s.store.ListFirewallRules(ctx, assignment.TenantID, 0, "")
		if err == nil {
			for _, r := range fwRules {
				tenantState.FirewallRules = append(tenantState.FirewallRules, toFirewallRuleJSON(r))
			}
		}

		// Get NAT rules
		natRules, _, err := s.store.ListNATRules(ctx, assignment.TenantID, 0, "")
		if err == nil {
			for _, r := range natRules {
				tenantState.NATRules = append(tenantState.NATRules, toNATRuleJSON(r))
			}
		}

		// Get rate limit
		if limit, err := s.store.GetRateLimit(ctx, assignment.TenantID); err == nil {
			tenantState.RateLimit = toRateLimitJSON(limit)
		}

		desiredState.Tenants = append(desiredState.Tenants, tenantState)
	}

	s.writeJSON(w, http.StatusOK, GetDPUDesiredStateResponse{DesiredState: desiredState})
}

func (s *Server) handleReportAgentStatus(w http.ResponseWriter, r *http.Request) {
	var req ReportAgentStatusRequest
	if !s.decode(w, r, &req) {
		return
	}

	if req.DPUID == "" {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("dpu_id is required"))
		return
	}

	status := &store.AgentStatus{
		DPUID:            req.DPUID,
		AgentType:        store.AgentType(req.AgentType),
		TotalRules:       req.TotalRules,
		HWOffloaded:      req.HWOffloaded,
		PacketsForwarded: req.PacketsForwarded,
		PacketsDropped:   req.PacketsDropped,
		BytesForwarded:   req.BytesForwarded,
		BytesDropped:     req.BytesDropped,
		UptimeSeconds:    req.UptimeSeconds,
		LastSyncError:    req.LastSyncError,
		LastSyncTime:     time.Now(),
	}

	if err := s.store.UpdateAgentStatus(r.Context(), status); err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.writeJSON(w, http.StatusOK, ReportAgentStatusResponse{
		AcknowledgedAt: time.Now().Format(time.RFC3339),
	})
}

func (s *Server) handleAssignTenantToDPU(w http.ResponseWriter, r *http.Request) {
	var req AssignTenantToDPURequest
	if !s.decode(w, r, &req) {
		return
	}

	if req.TenantID == "" || req.DPUID == "" {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("tenant_id and dpu_id are required"))
		return
	}

	assignment := &store.DPUAssignment{
		DPUID:    req.DPUID,
		TenantID: req.TenantID,
		InPort:   req.InPort,
		OutPort:  req.OutPort,
	}

	if err := s.store.AssignTenantToDPU(r.Context(), assignment); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.writeError(w, http.StatusNotFound, err)
			return
		}
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.logger.Info("assigned tenant to DPU", "tenant_id", req.TenantID, "dpu_id", req.DPUID)
	s.writeJSON(w, http.StatusOK, struct {
		Assignment *DPUAssignmentJSON `json:"assignment"`
	}{Assignment: toDPUAssignmentJSON(assignment)})
}

func (s *Server) handleUnassignTenantFromDPU(w http.ResponseWriter, r *http.Request) {
	var req UnassignTenantFromDPURequest
	if !s.decode(w, r, &req) {
		return
	}

	if err := s.store.UnassignTenantFromDPU(r.Context(), req.TenantID, req.DPUID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.writeError(w, http.StatusNotFound, err)
			return
		}
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.logger.Info("unassigned tenant from DPU", "tenant_id", req.TenantID, "dpu_id", req.DPUID)
	s.writeJSON(w, http.StatusOK, struct{}{})
}

func (s *Server) handleGetDPUAssignments(w http.ResponseWriter, r *http.Request) {
	var req GetDPUAssignmentsRequest
	if !s.decode(w, r, &req) {
		return
	}

	assignments, err := s.store.GetDPUAssignments(r.Context(), req.DPUID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	resp := GetDPUAssignmentsResponse{}
	for _, a := range assignments {
		resp.Assignments = append(resp.Assignments, toDPUAssignmentJSON(a))
	}
	s.writeJSON(w, http.StatusOK, resp)
}

// --- Helpers ---

func (s *Server) decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return false
	}
	return true
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.logger.Error("failed to encode response", "error", err)
	}
}

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (s *Server) writeError(w http.ResponseWriter, status int, err error) {
	code := "internal"
	switch status {
	case http.StatusBadRequest:
		code = "invalid_argument"
	case http.StatusNotFound:
		code = "not_found"
	case http.StatusConflict:
		code = "already_exists"
	case http.StatusServiceUnavailable:
		code = "unavailable"
	}

	s.logger.Warn("request error", "code", code, "error", err)
	s.writeJSON(w, status, errorResponse{Code: code, Message: err.Error()})
}

// --- Converters ---

func toTenantNetworkJSON(n *store.TenantNetwork) *TenantNetworkJSON {
	return &TenantNetworkJSON{
		TenantID:   n.TenantID,
		Name:       n.Name,
		SubnetCIDR: n.SubnetCIDR,
		GatewayIP:  n.GatewayIP,
		PublicIPs:  n.PublicIPs,
		CreatedAt:  n.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  n.UpdatedAt.Format(time.RFC3339),
	}
}

func toFirewallRuleJSON(r *store.FirewallRule) *FirewallRuleJSON {
	return &FirewallRuleJSON{
		ID:        r.ID,
		TenantID:  r.TenantID,
		SrcCIDR:   r.SrcCIDR,
		DstCIDR:   r.DstCIDR,
		SrcPort:   r.SrcPort,
		DstPort:   r.DstPort,
		Protocol:  int(r.Protocol),
		Action:    int(r.Action),
		Priority:  r.Priority,
		IPVersion: int(r.IPVersion),
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
	}
}

func toNATRuleJSON(r *store.NATRule) *NATRuleJSON {
	return &NATRuleJSON{
		ID:             r.ID,
		TenantID:       r.TenantID,
		Type:           int(r.Type),
		PublicIP:       r.PublicIP,
		PublicPort:     r.PublicPort,
		PrivateIP:      r.PrivateIP,
		PrivatePort:    r.PrivatePort,
		ExternalPrefix: r.ExternalPrefix,
		InternalPrefix: r.InternalPrefix,
		PrefixLength:   r.PrefixLength,
		Protocol:       int(r.Protocol),
		Mode:           int(r.Mode),
		IPVersion:      int(r.IPVersion),
		CreatedAt:      r.CreatedAt.Format(time.RFC3339),
	}
}

func toRateLimitJSON(l *store.RateLimit) *RateLimitJSON {
	return &RateLimitJSON{
		TenantID:           l.TenantID,
		IngressBytesPerSec: l.IngressBytesPerSec,
		EgressBytesPerSec:  l.EgressBytesPerSec,
		BurstBytes:         l.BurstBytes,
	}
}

func toPublicIPAllocationJSON(a *ipam.Allocation) *PublicIPAllocationJSON {
	ipVer := 1 // IPv4
	if a.Version == ipam.IPv6 {
		ipVer = 2
	}
	return &PublicIPAllocationJSON{
		IP:          a.IP,
		TenantID:    a.TenantID,
		IPVersion:   ipVer,
		AllocatedAt: a.AllocatedAt.Format(time.RFC3339),
	}
}

func toDPUAssignmentJSON(a *store.DPUAssignment) *DPUAssignmentJSON {
	return &DPUAssignmentJSON{
		DPUID:      a.DPUID,
		TenantID:   a.TenantID,
		InPort:     a.InPort,
		OutPort:    a.OutPort,
		AssignedAt: a.AssignedAt.Format(time.RFC3339),
	}
}

