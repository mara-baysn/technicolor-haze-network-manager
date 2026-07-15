package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mara-baysn/technicolor-haze-network-manager/internal/ipam"
	"github.com/mara-baysn/technicolor-haze-network-manager/internal/store"
)

func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	memStore := store.NewMemoryStore()
	pool := ipam.NewPool()
	_ = pool.AddIP("203.0.113.10")
	_ = pool.AddIP("203.0.113.11")
	_ = pool.AddIP("2001:db8::1")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(memStore, pool, logger)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return srv, ts
}

func postJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	require.NoError(t, err)
	return resp
}

func decodeJSON[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()
	var v T
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&v))
	return v
}

func TestCreateAndGetTenantNetwork(t *testing.T) {
	_, ts := newTestServer(t)

	// Create
	resp := postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/CreateTenantNetwork", CreateTenantNetworkRequest{
		TenantID:   "tenant-1",
		Name:       "Production",
		SubnetCIDR: "10.100.0.0/24",
		GatewayIP:  "10.100.0.1",
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	result := decodeJSON[TenantNetworkResponse](t, resp)
	assert.Equal(t, "tenant-1", result.Network.TenantID)
	assert.Equal(t, "10.100.0.0/24", result.Network.SubnetCIDR)

	// Get
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/GetTenantNetwork", GetTenantNetworkRequest{
		TenantID: "tenant-1",
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	result = decodeJSON[TenantNetworkResponse](t, resp)
	assert.Equal(t, "Production", result.Network.Name)

	// Duplicate
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/CreateTenantNetwork", CreateTenantNetworkRequest{
		TenantID:   "tenant-1",
		Name:       "Dup",
		SubnetCIDR: "10.101.0.0/24",
		GatewayIP:  "10.101.0.1",
	})
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	resp.Body.Close()

	// Not found
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/GetTenantNetwork", GetTenantNetworkRequest{
		TenantID: "nonexistent",
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

func TestCreateTenantNetwork_Validation(t *testing.T) {
	_, ts := newTestServer(t)

	// Empty tenant ID
	resp := postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/CreateTenantNetwork", CreateTenantNetworkRequest{
		Name:       "test",
		SubnetCIDR: "10.0.0.0/24",
		GatewayIP:  "10.0.0.1",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()

	// Invalid CIDR
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/CreateTenantNetwork", CreateTenantNetworkRequest{
		TenantID:   "t1",
		Name:       "test",
		SubnetCIDR: "not-a-cidr",
		GatewayIP:  "10.0.0.1",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}

func TestFirewallRules(t *testing.T) {
	_, ts := newTestServer(t)

	// Create tenant first
	resp := postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/CreateTenantNetwork", CreateTenantNetworkRequest{
		TenantID:   "tenant-1",
		Name:       "test",
		SubnetCIDR: "10.0.0.0/24",
		GatewayIP:  "10.0.0.1",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Add rule
	port80 := uint32(80)
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/AddFirewallRule", AddFirewallRuleRequest{
		TenantID: "tenant-1",
		SrcCIDR:  "0.0.0.0/0",
		DstCIDR:  "10.0.0.0/24",
		DstPort:  &port80,
		Protocol: 1, // TCP
		Action:   1, // ALLOW
		Priority: 100,
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var ruleResp struct {
		Rule *FirewallRuleJSON `json:"rule"`
	}
	json.NewDecoder(resp.Body).Decode(&ruleResp)
	resp.Body.Close()
	assert.NotEmpty(t, ruleResp.Rule.ID)
	assert.Equal(t, "0.0.0.0/0", ruleResp.Rule.SrcCIDR)

	// List rules
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/ListFirewallRules", ListFirewallRulesRequest{
		TenantID: "tenant-1",
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	listResp := decodeJSON[ListFirewallRulesResponse](t, resp)
	assert.Len(t, listResp.Rules, 1)

	// Delete rule
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/DeleteFirewallRule", DeleteFirewallRuleRequest{
		TenantID: "tenant-1",
		RuleID:   ruleResp.Rule.ID,
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

func TestNATRules(t *testing.T) {
	_, ts := newTestServer(t)

	// Create tenant
	resp := postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/CreateTenantNetwork", CreateTenantNetworkRequest{
		TenantID:   "tenant-1",
		Name:       "test",
		SubnetCIDR: "10.0.0.0/24",
		GatewayIP:  "10.0.0.1",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Add NAT rule
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/AddNATRule", AddNATRuleRequest{
		TenantID:  "tenant-1",
		Type:      2, // DNAT
		PublicIP:  "203.0.113.10",
		PrivateIP: "10.0.0.5",
		Protocol:  1, // TCP
		Mode:      1, // Static
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var natResp struct {
		Rule *NATRuleJSON `json:"rule"`
	}
	json.NewDecoder(resp.Body).Decode(&natResp)
	resp.Body.Close()
	assert.NotEmpty(t, natResp.Rule.ID)

	// List
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/ListNATRules", ListNATRulesRequest{
		TenantID: "tenant-1",
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	listResp := decodeJSON[ListNATRulesResponse](t, resp)
	assert.Len(t, listResp.Rules, 1)
}

func TestRateLimits(t *testing.T) {
	_, ts := newTestServer(t)

	// Create tenant
	resp := postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/CreateTenantNetwork", CreateTenantNetworkRequest{
		TenantID:   "tenant-1",
		Name:       "test",
		SubnetCIDR: "10.0.0.0/24",
		GatewayIP:  "10.0.0.1",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Set rate limit
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/SetRateLimit", SetRateLimitRequest{
		TenantID:           "tenant-1",
		IngressBytesPerSec: 1_000_000_000,
		EgressBytesPerSec:  500_000_000,
		BurstBytes:         10_000_000,
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Get rate limit
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/GetRateLimit", GetRateLimitRequest{
		TenantID: "tenant-1",
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var rlResp struct {
		RateLimit *RateLimitJSON `json:"rate_limit"`
	}
	json.NewDecoder(resp.Body).Decode(&rlResp)
	resp.Body.Close()
	assert.Equal(t, uint64(1_000_000_000), rlResp.RateLimit.IngressBytesPerSec)
}

func TestIPAllocation(t *testing.T) {
	_, ts := newTestServer(t)

	// Create tenant
	resp := postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/CreateTenantNetwork", CreateTenantNetworkRequest{
		TenantID:   "tenant-1",
		Name:       "test",
		SubnetCIDR: "10.0.0.0/24",
		GatewayIP:  "10.0.0.1",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Allocate
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/AllocatePublicIP", AllocatePublicIPRequest{
		TenantID:  "tenant-1",
		IPVersion: 1,
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var allocResp struct {
		Allocation *PublicIPAllocationJSON `json:"allocation"`
	}
	json.NewDecoder(resp.Body).Decode(&allocResp)
	resp.Body.Close()
	assert.NotEmpty(t, allocResp.Allocation.IP)
	assert.Equal(t, "tenant-1", allocResp.Allocation.TenantID)

	// List
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/ListPublicIPs", ListPublicIPsRequest{
		TenantID: "tenant-1",
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	listResp := decodeJSON[ListPublicIPsResponse](t, resp)
	assert.Len(t, listResp.Allocations, 1)

	// Release
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/ReleasePublicIP", ReleasePublicIPRequest{
		IP: allocResp.Allocation.IP,
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

func TestGetDPUDesiredState(t *testing.T) {
	_, ts := newTestServer(t)

	// Setup: create tenant, add rules, assign to DPU
	resp := postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/CreateTenantNetwork", CreateTenantNetworkRequest{
		TenantID:   "tenant-1",
		Name:       "Production",
		SubnetCIDR: "10.100.0.0/24",
		GatewayIP:  "10.100.0.1",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Assign to DPU
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/AssignTenantToDPU", AssignTenantToDPURequest{
		TenantID: "tenant-1",
		DPUID:    "dpu-bf3-001",
		InPort:   "pf0vf0",
		OutPort:  "pf0",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Add firewall rule
	port443 := uint32(443)
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/AddFirewallRule", AddFirewallRuleRequest{
		TenantID: "tenant-1",
		SrcCIDR:  "0.0.0.0/0",
		DstCIDR:  "10.100.0.0/24",
		DstPort:  &port443,
		Protocol: 1,
		Action:   1,
		Priority: 100,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Set rate limit
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/SetRateLimit", SetRateLimitRequest{
		TenantID:           "tenant-1",
		IngressBytesPerSec: 1_000_000_000,
		EgressBytesPerSec:  1_000_000_000,
		BurstBytes:         10_000_000,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Get desired state
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/GetDPUDesiredState", GetDPUDesiredStateRequest{
		DPUID: "dpu-bf3-001",
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	dsResp := decodeJSON[GetDPUDesiredStateResponse](t, resp)

	assert.False(t, dsResp.Unchanged)
	require.NotNil(t, dsResp.DesiredState)
	assert.Equal(t, "dpu-bf3-001", dsResp.DesiredState.DPUID)
	require.Len(t, dsResp.DesiredState.Tenants, 1)

	tenantState := dsResp.DesiredState.Tenants[0]
	assert.Equal(t, "tenant-1", tenantState.TenantID)
	assert.Equal(t, "pf0vf0", tenantState.InPort)
	assert.Equal(t, "pf0", tenantState.OutPort)
	assert.Len(t, tenantState.FirewallRules, 1)
	assert.NotNil(t, tenantState.RateLimit)
	assert.Equal(t, uint64(1_000_000_000), tenantState.RateLimit.IngressBytesPerSec)
}

func TestGetDPUDesiredState_ConditionalFetch(t *testing.T) {
	_, ts := newTestServer(t)

	// Setup
	resp := postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/CreateTenantNetwork", CreateTenantNetworkRequest{
		TenantID: "tenant-1", Name: "test", SubnetCIDR: "10.0.0.0/24", GatewayIP: "10.0.0.1",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/AssignTenantToDPU", AssignTenantToDPURequest{
		TenantID: "tenant-1", DPUID: "dpu-1", InPort: "pf0vf0", OutPort: "pf0",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// First fetch - get version
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/GetDPUDesiredState", GetDPUDesiredStateRequest{
		DPUID: "dpu-1",
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	dsResp := decodeJSON[GetDPUDesiredStateResponse](t, resp)
	version := dsResp.DesiredState.Version

	// Second fetch with same version - should return unchanged
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/GetDPUDesiredState", GetDPUDesiredStateRequest{
		DPUID:            "dpu-1",
		LastKnownVersion: version,
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	dsResp2 := decodeJSON[GetDPUDesiredStateResponse](t, resp)
	assert.True(t, dsResp2.Unchanged)
}

func TestReportAgentStatus(t *testing.T) {
	_, ts := newTestServer(t)

	resp := postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/ReportAgentStatus", ReportAgentStatusRequest{
		DPUID:            "dpu-bf3-001",
		AgentType:        1, // FIREWALL
		TotalRules:       42,
		HWOffloaded:      40,
		PacketsForwarded: 1000000,
		PacketsDropped:   500,
		UptimeSeconds:    3600,
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	statusResp := decodeJSON[ReportAgentStatusResponse](t, resp)
	assert.NotEmpty(t, statusResp.AcknowledgedAt)
}

func TestDPUAssignments(t *testing.T) {
	_, ts := newTestServer(t)

	// Create tenant
	resp := postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/CreateTenantNetwork", CreateTenantNetworkRequest{
		TenantID: "tenant-1", Name: "test", SubnetCIDR: "10.0.0.0/24", GatewayIP: "10.0.0.1",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Assign
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/AssignTenantToDPU", AssignTenantToDPURequest{
		TenantID: "tenant-1", DPUID: "dpu-1", InPort: "pf0vf0", OutPort: "pf0",
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Get assignments
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/GetDPUAssignments", GetDPUAssignmentsRequest{
		DPUID: "dpu-1",
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assignResp := decodeJSON[GetDPUAssignmentsResponse](t, resp)
	assert.Len(t, assignResp.Assignments, 1)
	assert.Equal(t, "tenant-1", assignResp.Assignments[0].TenantID)

	// Unassign
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/UnassignTenantFromDPU", UnassignTenantFromDPURequest{
		TenantID: "tenant-1", DPUID: "dpu-1",
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Verify empty
	resp = postJSON(t, ts.URL+"/networkmanager.v1.NetworkManagerService/GetDPUAssignments", GetDPUAssignmentsRequest{
		DPUID: "dpu-1",
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assignResp = decodeJSON[GetDPUAssignmentsResponse](t, resp)
	assert.Len(t, assignResp.Assignments, 0)
}

func TestHealthCheck(t *testing.T) {
	_, ts := newTestServer(t)

	resp, err := http.Get(ts.URL + "/healthz")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, "ok", string(body))
}
