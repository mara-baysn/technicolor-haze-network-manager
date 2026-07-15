# Network Manager

Fleet Orchestration component that manages all network configuration for the Technicolor Haze platform.

## Architecture

Network Manager is the central network state store. It:

- Receives tenant network requirements from Shepherd (Master Manager)
- Stores all network state: VPCs, subnets, firewall rules, NAT, routing, rate limits
- Serves desired state to DPU agents (firewall agent, routing agent) via a pull model
- Manages public IP allocation (IPAM)
- Tracks DPU-to-tenant assignments

## Quick Start

```bash
# Run locally
make run

# Run tests
make test

# Build binary
make build

# Docker
make docker-build
docker run -p 8080:8080 network-manager:latest
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP listen port |
| `LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `IP_POOL_CIDR` | `203.0.113.0/24` | Public IP pool CIDR for allocation |

## API

All RPCs follow ConnectRPC conventions (POST, JSON body, path = `/package.Service/Method`).

### Management (called by Shepherd)

- `CreateTenantNetwork` / `DeleteTenantNetwork` / `GetTenantNetwork` / `ListTenantNetworks`
- `AddFirewallRule` / `DeleteFirewallRule` / `ListFirewallRules`
- `AddNATRule` / `DeleteNATRule` / `ListNATRules`
- `SetRateLimit` / `GetRateLimit`
- `AllocatePublicIP` / `ReleasePublicIP` / `ListPublicIPs`

### DPU Agent (pull model)

- `GetDPUDesiredState` — returns aggregated rules for all tenants on a DPU
- `ReportAgentStatus` — agent reports applied state + metrics

### Placement (called by DPU Orchestrator)

- `AssignTenantToDPU` / `UnassignTenantFromDPU` / `GetDPUAssignments`

## Development

```bash
# Proto definition
proto/networkmanager/v1/network_manager.proto

# Run with debug logging
LOG_LEVEL=debug make run
```
