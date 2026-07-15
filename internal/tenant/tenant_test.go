package tenant

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateCreateNetwork(t *testing.T) {
	tests := []struct {
		name       string
		tenantID   string
		netName    string
		subnetCIDR string
		gatewayIP  string
		wantErr    error
	}{
		{
			name:       "valid",
			tenantID:   "tenant-1",
			netName:    "Production",
			subnetCIDR: "10.100.0.0/24",
			gatewayIP:  "10.100.0.1",
		},
		{
			name:       "valid IPv6",
			tenantID:   "tenant-2",
			netName:    "IPv6 Net",
			subnetCIDR: "fd00:1::/64",
			gatewayIP:  "fd00:1::1",
		},
		{
			name:       "empty tenant ID",
			tenantID:   "",
			netName:    "test",
			subnetCIDR: "10.0.0.0/24",
			gatewayIP:  "10.0.0.1",
			wantErr:    ErrEmptyTenantID,
		},
		{
			name:       "empty name",
			tenantID:   "t1",
			netName:    "",
			subnetCIDR: "10.0.0.0/24",
			gatewayIP:  "10.0.0.1",
			wantErr:    ErrEmptyName,
		},
		{
			name:       "invalid subnet",
			tenantID:   "t1",
			netName:    "test",
			subnetCIDR: "not-a-cidr",
			gatewayIP:  "10.0.0.1",
			wantErr:    ErrInvalidSubnet,
		},
		{
			name:       "invalid gateway",
			tenantID:   "t1",
			netName:    "test",
			subnetCIDR: "10.0.0.0/24",
			gatewayIP:  "not-an-ip",
			wantErr:    ErrInvalidGateway,
		},
		{
			name:       "gateway not in subnet",
			tenantID:   "t1",
			netName:    "test",
			subnetCIDR: "10.0.0.0/24",
			gatewayIP:  "192.168.1.1",
			wantErr:    ErrGatewayNotInSubnet,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreateNetwork(tt.tenantID, tt.netName, tt.subnetCIDR, tt.gatewayIP)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateFirewallRule(t *testing.T) {
	p80 := uint32(80)
	p0 := uint32(0)
	p99999 := uint32(99999)

	tests := []struct {
		name     string
		tenantID string
		srcCIDR  string
		dstCIDR  string
		srcPort  *uint32
		dstPort  *uint32
		priority uint32
		wantErr  error
	}{
		{
			name:     "valid",
			tenantID: "tenant-1",
			srcCIDR:  "10.0.0.0/8",
			dstCIDR:  "192.168.0.0/16",
			dstPort:  &p80,
			priority: 100,
		},
		{
			name:     "valid with empty CIDRs",
			tenantID: "tenant-1",
			priority: 50,
		},
		{
			name:     "empty tenant",
			tenantID: "",
			priority: 100,
			wantErr:  ErrEmptyTenantID,
		},
		{
			name:     "invalid src CIDR",
			tenantID: "t1",
			srcCIDR:  "invalid",
			priority: 100,
			wantErr:  ErrInvalidCIDR,
		},
		{
			name:     "invalid dst CIDR",
			tenantID: "t1",
			dstCIDR:  "invalid",
			priority: 100,
			wantErr:  ErrInvalidCIDR,
		},
		{
			name:     "port zero",
			tenantID: "t1",
			srcPort:  &p0,
			priority: 100,
			wantErr:  ErrInvalidPort,
		},
		{
			name:     "port too large",
			tenantID: "t1",
			dstPort:  &p99999,
			priority: 100,
			wantErr:  ErrInvalidPort,
		},
		{
			name:     "zero priority",
			tenantID: "t1",
			priority: 0,
			wantErr:  ErrInvalidPriority,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFirewallRule(tt.tenantID, tt.srcCIDR, tt.dstCIDR, tt.srcPort, tt.dstPort, tt.priority)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateNATRule(t *testing.T) {
	tests := []struct {
		name      string
		tenantID  string
		publicIP  string
		privateIP string
		wantErr   error
	}{
		{name: "valid", tenantID: "t1", publicIP: "203.0.113.1", privateIP: "10.0.0.5"},
		{name: "valid IPv6", tenantID: "t1", publicIP: "2001:db8::1", privateIP: "fd00::5"},
		{name: "empty IPs ok", tenantID: "t1"},
		{name: "empty tenant", tenantID: "", wantErr: ErrEmptyTenantID},
		{name: "invalid public IP", tenantID: "t1", publicIP: "not-ip", wantErr: ErrInvalidIP},
		{name: "invalid private IP", tenantID: "t1", privateIP: "not-ip", wantErr: ErrInvalidIP},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNATRule(tt.tenantID, tt.publicIP, tt.privateIP)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateRateLimit(t *testing.T) {
	err := ValidateRateLimit("tenant-1", 1000, 500, 100)
	assert.NoError(t, err)

	err = ValidateRateLimit("", 1000, 500, 100)
	assert.ErrorIs(t, err, ErrEmptyTenantID)

	err = ValidateRateLimit("tenant-1", 0, 0, 100)
	assert.Error(t, err)
}

func TestDetectIPVersion(t *testing.T) {
	assert.Equal(t, 4, DetectIPVersion("10.0.0.0/24"))
	assert.Equal(t, 4, DetectIPVersion("192.168.1.1"))
	assert.Equal(t, 6, DetectIPVersion("2001:db8::/32"))
	assert.Equal(t, 6, DetectIPVersion("fe80::1"))
	assert.Equal(t, 0, DetectIPVersion(""))
	assert.Equal(t, 0, DetectIPVersion("invalid"))
}
