# Technicolor Haze Network Manager - Design Document

## 1. Overview & Goals

The Network Manager (NM) is the centralized control plane for all virtual networking
in the Technicolor Haze platform. It owns tenant network state and pushes desired
configuration to DPU agents that enforce it in hardware via tc-flower and netlink.

**Goals:**

- Single source of truth for all network configuration (VPCs, firewalls, NAT, security
  groups, overlays, DHCP, load balancers, VPN/ZTNA)
- Serve desired state to DPU agents via gRPC pull model with conditional fetch
- Domain-driven monolith internally: each network service is an isolated package with
  its own proto, store interface, and business logic
- Sub-agent model on DPUs: master agent manages specialized sub-agents as separate
  systemd-managed binaries, communicating via Unix socket + protobuf IPC
- Stateless recovery: kernel tc-flower rules persist across restarts; sub-agents
  reconcile from NM on startup rather than maintaining local state files

**Non-goals:**

- NM does not run on DPUs. It runs in the control plane (Kubernetes pod or bare metal).
- NM does not directly program hardware. That is the sub-agent's job.
- NM does not manage physical network topology (that is Shepherd/Herd Manager).


## 2. Architecture Diagram

```
                         Control Plane
    +----------------------------------------------------------+
    |                                                          |
    |   Shepherd (Fleet Orchestrator)                          |
    |       |                                                  |
    |       | tenant lifecycle, DPU placement                  |
    |       v                                                  |
    |   +-------------------+         +------------------+     |
    |   | Network Manager   |         | technicolor-haze |     |
    |   | (this repo)       |         | -proto (schemas) |     |
    |   |                   |         +------------------+     |
    |   | domain/vpc        |                                  |
    |   | domain/firewall   |                                  |
    |   | domain/secgroup   |                                  |
    |   | domain/acl        |                                  |
    |   | domain/eip        |                                  |
    |   | domain/lb         |                                  |
    |   | domain/dns        |                                  |
    |   | domain/dhcp       |                                  |
    |   | domain/vpn        |                                  |
    |   +--------+----------+                                  |
    |            |                                             |
    +------------|---------------------------------------------+
                 | gRPC (GetDesiredState, ReportStatus)
                 | per-domain streaming updates
                 |
    =============|================================================
                 |            DPU (BlueField-3 / etc.)
    +------------|---------------------------------------------+
    |            v                                             |
    |   +-------------------+                                  |
    |   | Master Agent      |  (haze-agent-master)             |
    |   | - capability reg  |                                  |
    |   | - sub-agent mgmt  |                                  |
    |   | - systemd control |                                  |
    |   +--+---+---+---+---+                                  |
    |      |   |   |   |                                       |
    |      |   |   |   +--- Unix socket IPC                    |
    |      v   v   v   v                                       |
    |   +----+ +----+ +-------+ +------+ +----+ +----------+  |
    |   | FW | | SG | |Overlay| | DHCP | | LB | |SecAccess |  |
    |   +--+-+ +--+-+ +---+---+ +--+---+ +--+-+ +----+-----+  |
    |      |      |       |        |        |         |        |
    |      v      v       v        v        v         v        |
    |   +------------------------------------------------------+
    |   |  Linux Kernel: tc-flower / netlink / OVS / eBPF      |
    |   +------------------------------------------------------+
    |                                                          |
    +----------------------------------------------------------+

    Legend:
    FW       = haze-firewall (Firewall + NAT rules)
    SG       = haze-secgroup (Security Groups + Network ACL)
    Overlay  = haze-overlay  (VXLAN/Geneve tunnels + routing)
    DHCP     = haze-dhcp     (DHCP responses for VMs)
    LB       = haze-loadbalancer (L4/L7 load balancing)
    SecAccess = haze-secureaccess (VPN/ZTNA via Nexus)
```


## 3. DPU Master Agent Design

The master agent (`haze-agent-master`) is the single process on each DPU that
communicates with the Network Manager and orchestrates local sub-agents.

### Lifecycle

1. **Boot**: systemd starts `haze-agent-master.service` (After=network-online.target)
2. **Registration**: Master calls NM `RegisterDPU(dpu_id, tier, host, hw_capabilities)`
3. **Capability assignment**: NM responds with the list of capabilities this DPU should run
4. **Sub-agent spawn**: Master writes systemd unit files and starts sub-agents matching capabilities
5. **Steady state**: Master polls NM for desired state updates, fans out to sub-agents via IPC
6. **Shutdown**: On SIGTERM, master sends graceful shutdown to sub-agents, waits, exits

### Capability Discovery

On registration, the master reports:
- DPU hardware ID and firmware version
- Host server ID (which physical machine it sits in)
- Available hardware offload features (e.g., ct-flower support, connection tracking HW)
- Current tier assignment (set by Shepherd during provisioning)

NM responds with the definitive capability set. The master does not decide its own
capabilities -- NM is authoritative.

### Sub-Agent Management

The master agent is a **systemd orchestrator**, not a process supervisor:

```
# Master writes unit file:
/run/systemd/transient/haze-firewall.service

[Unit]
Description=Haze Firewall Sub-Agent
After=haze-agent-master.service

[Service]
ExecStart=/usr/local/bin/haze-firewall --socket /run/haze/agents/firewall.sock
Restart=on-failure
RestartSec=2s
```

- Sub-agents survive master restarts (systemd keeps them alive)
- Master discovers running sub-agents by scanning `/run/haze/agents/*.sock`
- If a sub-agent is running but not in the current capability set, master stops it
- If a capability is assigned but no sub-agent is running, master starts it

### Reconnection After Master Restart

1. Master starts, reads capability set from NM
2. Scans `/run/haze/agents/` for existing sockets
3. Connects to each discovered socket, sends `HealthCheck`
4. Any socket that responds is adopted; unresponsive sockets trigger a restart of that sub-agent
5. Resumes normal operation (no disruption to running sub-agents)


## 4. Sub-Agent Model

### Binary Naming Convention

All sub-agent binaries follow the pattern `haze-{domain}`:

| Binary | Purpose |
|--------|---------|
| `haze-firewall` | Stateful firewall rules + NAT (SNAT, DNAT, NPTv6) |
| `haze-secgroup` | Per-VF security group rules + per-subnet Network ACLs |
| `haze-overlay` | VPC overlay encapsulation (VXLAN/Geneve) + routing tables |
| `haze-dhcp` | DHCP server for VM IP assignment |
| `haze-loadbalancer` | L4/L7 load balancer (IPVS / eBPF) |
| `haze-secureaccess` | VPN/ZTNA tunnel termination (Nexus integration) |

### Socket Paths

Each sub-agent listens on a well-known Unix domain socket:

```
/run/haze/agents/firewall.sock
/run/haze/agents/secgroup.sock
/run/haze/agents/overlay.sock
/run/haze/agents/dhcp.sock
/run/haze/agents/loadbalancer.sock
/run/haze/agents/secureaccess.sock
```

The master connects to these sockets. Sub-agents are servers; master is the client.

### Systemd Integration

Each sub-agent runs as an independent systemd service:

- `Type=notify` (sub-agent sends sd_notify READY after socket is listening)
- `Restart=on-failure` with `RestartSec=2s`
- `WatchdogSec=30s` (sub-agent must ping systemd watchdog)
- Sandboxed: `ProtectSystem=strict`, `PrivateNetwork=no` (needs netlink access)
- `CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW` (minimum kernel capabilities)

The master writes transient units via `systemd-run` or the D-Bus API. On DPU
reimage/upgrade, permanent units are installed by the provisioning system.


## 5. IPC Protocol Specification

### Wire Format

```
+----------------+------------------+
| Length (4 bytes| Protobuf message |
| big-endian)    | (variable)       |
+----------------+------------------+
```

- Transport: Abstract Unix domain socket (SOCK_STREAM)
- Framing: 4-byte big-endian length prefix, followed by serialized protobuf
- Max message size: 4 MB (configurable)
- No TLS (local-only, same DPU)
- Latency: ~2-5 microseconds per message (measured on BF-3)

### Message Types

Defined in `agent/v1/agent.proto`:

```protobuf
// Master -> Sub-agent
message ConfigUpdate {
  string agent_name = 1;
  uint64 version = 2;
  google.protobuf.Any config = 3;  // domain-specific config
  bool full_sync = 4;              // true = replace all, false = incremental
}

// Sub-agent -> Master
message StatusReport {
  string agent_name = 1;
  AgentHealth health = 2;
  uint32 rules_installed = 3;
  uint32 rules_hw_offloaded = 4;
  string last_error = 5;
  google.protobuf.Timestamp last_sync = 6;
  map<string, uint64> counters = 7;  // packets_fwd, bytes_fwd, etc.
}

// Bidirectional
message HealthCheck {
  bool request = 1;   // true = asking, false = responding
  uint64 uptime_ms = 2;
}

enum AgentHealth {
  AGENT_HEALTH_UNSPECIFIED = 0;
  AGENT_HEALTH_HEALTHY = 1;
  AGENT_HEALTH_DEGRADED = 2;  // some rules failed to install
  AGENT_HEALTH_UNHEALTHY = 3; // unable to program datapath
}
```

### Connection Lifecycle

1. Sub-agent starts, creates socket at `/run/haze/agents/{name}.sock`
2. Sub-agent calls `listen()` then sends `sd_notify(READY=1)`
3. Master connects (or reconnects after restart)
4. Master sends initial `ConfigUpdate` with `full_sync=true`
5. Sub-agent applies config, sends `StatusReport`
6. Steady state: master sends incremental `ConfigUpdate` on changes
7. Master sends `HealthCheck` every 10 seconds; expects response within 5 seconds
8. If 3 consecutive health checks fail, master restarts the sub-agent via systemd

### Reconnection

If the connection drops (sub-agent crash, socket error):
- Master detects via read/write error on the socket
- Waits for systemd to restart the sub-agent (Restart=on-failure)
- Re-scans socket directory, reconnects when socket reappears
- Sends `full_sync=true` ConfigUpdate after reconnection


## 6. Network Manager Domains

### VPC (`internal/domain/vpc`)
- VPC CRUD (create, read, update, delete)
- Subnet management within VPCs
- Route table management (per-VPC routing)
- VPC peering configuration
- Key operations: `CreateVPC`, `CreateSubnet`, `AddRoute`, `PeerVPCs`

### Firewall (`internal/domain/firewall`)
- Stateful firewall rules (L3/L4 match + action)
- NAT rules (SNAT, DNAT, static, masquerade, NPTv6)
- Rule ordering by priority
- Per-tenant rule isolation
- Key operations: `AddFirewallRule`, `AddNATRule`, `GetDesiredFirewallState`

### Security Groups (`internal/domain/secgroup`)
- Per-VF (virtual function) ingress/egress rules
- Stateful connection tracking (allow return traffic)
- Default-deny with explicit allow rules
- Key operations: `CreateSecurityGroup`, `AddRule`, `AttachToVF`

### Network ACL (`internal/domain/acl`)
- Per-subnet stateless ACLs
- Ordered rules with explicit deny
- Applied at subnet boundary (before security groups)
- Key operations: `CreateNetworkACL`, `AddACLEntry`, `AssociateSubnet`

### Elastic IP (`internal/domain/eip`)
- Public IP allocation from pool (IPv4 and IPv6)
- Association to VMs/interfaces
- BGP announcement coordination (NM tells edge routers)
- Key operations: `AllocateIP`, `AssociateIP`, `ReleaseIP`
- Note: No DPU sub-agent. NM coordinates with edge routers directly.

### Load Balancer (`internal/domain/lb`)
- L4 load balancer (TCP/UDP, IPVS-backed)
- L7 load balancer (HTTP/HTTPS, eBPF-backed)
- Health checks and backend management
- Key operations: `CreateLoadBalancer`, `AddBackend`, `UpdateHealthCheck`

### Private DNS (`internal/domain/dns`)
- Per-VPC DNS zones
- A/AAAA/CNAME/SRV record management
- DNS forwarding rules
- Key operations: `CreateZone`, `AddRecord`, `SetForwarder`
- Note: Served by a separate DNS service, not a DPU sub-agent.

### DHCP (`internal/domain/dhcp`)
- IP assignment for VMs within subnets
- Lease management
- Custom DHCP options (DNS servers, routes, MTU)
- Key operations: `CreateLease`, `SetDHCPOptions`, `GetLeaseState`

### Secure Access / VPN (`internal/domain/vpn`)
- Site-to-site IPsec tunnels
- ZTNA (Zero Trust Network Access) via Nexus
- WireGuard tunnels for remote access
- Key operations: `CreateTunnel`, `AddPeer`, `SetAccessPolicy`


## 7. Service-to-SubAgent Mapping Table

| Sub-Agent Binary | NM Domain(s) | DPU Tier | Kernel Mechanism |
|-----------------|--------------|----------|------------------|
| `haze-firewall` | firewall (rules + NAT) | Tier 3 only | tc-flower + ct (connection tracking) |
| `haze-secgroup` | secgroup + acl | All (Tier 1, 2, 3) | tc-flower per-VF, netlink |
| `haze-overlay` | vpc (overlay + routing) | All (Tier 1, 2, 3) | VXLAN/Geneve tunnel + FDB + routing |
| `haze-dhcp` | dhcp | All (Tier 1, 2, 3) | userspace (raw socket, responds to DHCP) |
| `haze-loadbalancer` | lb | Tier 3 only | IPVS (L4) + eBPF XDP (L7) |
| `haze-secureaccess` | vpn (Nexus) | Tier 3 only | WireGuard / xfrm (IPsec) |

**Not sub-agents (control-plane only):**

| Service | Why No Sub-Agent |
|---------|-----------------|
| Elastic IP | NM coordinates BGP announcements with edge routers. No DPU datapath logic. |
| Private DNS | Separate DNS service (CoreDNS/PowerDNS). NM manages records, DNS service serves them. |


## 8. Capabilities & Registration

### Registration Flow

```
DPU Boot
   |
   v
Master Agent starts
   |
   v
RegisterDPU(dpu_id, tier, host, hw_info)  ---->  Network Manager
   |                                                    |
   |     <---- RegisterDPUResponse(capabilities)  <-----+
   v
Parse capabilities list
   |
   v
For each capability:
   - If sub-agent not running: write systemd unit, start it
   - If sub-agent running: verify health, keep it
   |
   v
For each running sub-agent NOT in capabilities:
   - Stop via systemd
   - Remove socket
```

### Tier-Based Capability Assignment

| DPU Tier | Role | Assigned Capabilities |
|----------|------|----------------------|
| Tier 1 | Tenant-dedicated DPU | `secgroup`, `overlay`, `dhcp` |
| Tier 2 | Shared compute DPU | `secgroup`, `overlay`, `dhcp` |
| Tier 3 | Shared services / gateway DPU | `firewall`, `secgroup`, `overlay`, `dhcp`, `loadbalancer`, `secureaccess` |

### Dynamic Capability Updates

Capabilities can change at runtime (e.g., DPU promoted from Tier 1 to Tier 3):

1. NM pushes `CapabilityUpdate` message to master agent via gRPC stream
2. Master agent diffs current vs new capability set
3. New capabilities: spawn sub-agents
4. Removed capabilities: gracefully stop sub-agents (drain connections first for LB)
5. Master acknowledges update to NM

### Hardware Capability Reporting

The master reports hardware features that influence NM decisions:

```protobuf
message HardwareCapabilities {
  uint32 max_tc_flower_rules = 1;    // HW offload limit
  bool connection_tracking_hw = 2;    // CT offload available
  uint32 max_vxlan_tunnels = 3;
  bool ipsec_offload = 4;
  uint32 vf_count = 5;               // number of virtual functions
  string firmware_version = 6;
}
```

NM uses this to:
- Decide whether to split firewall rules across multiple DPUs
- Choose between HW-offloaded and software-fallback paths
- Set appropriate rule limits per sub-agent


## 9. Data Flow

### Example: Tenant Creates Firewall Rule

```
1. User/API -> Shepherd: "Add firewall rule: allow TCP/443 from 10.0.0.0/8"
       |
2.     +-> Shepherd -> NM: CreateFirewallRule(tenant_id, rule)
              |
3.            +-> NM: validates, stores in DB, increments DPU state version
              |
4.            +-> NM: identifies which DPU(s) serve this tenant
              |
5.            +-> NM -> Master Agent (on DPU): push notification via gRPC stream
                     |                         OR master polls and sees version bump
6.                   +-> Master Agent: calls GetDesiredState (conditional fetch)
                     |
7.                   +-> Master Agent -> haze-firewall: ConfigUpdate (incremental)
                            |              via Unix socket IPC
8.                          +-> haze-firewall: translates rule to tc-flower
                            |
9.                          +-> haze-firewall: `tc filter add dev <rep> ...`
                            |                  netlink call to kernel
10.                         +-> haze-firewall -> Master: StatusReport (success)
                                   |
11.                                +-> Master -> NM: ReportAgentStatus
```

### Pull vs Push

The system uses a **hybrid pull + push-notification** model:

- **Pull (primary)**: Master agent polls `GetDPUDesiredState` with `last_known_version`.
  NM returns `unchanged=true` if version matches, avoiding serialization cost.
- **Push (optimization)**: NM sends a lightweight "version bumped" notification on the
  gRPC stream. Master then pulls the full state. This reduces polling latency from
  seconds to milliseconds.
- **Fallback**: If push notification is missed (network blip), polling catches it on
  the next interval (default: 5 seconds).


## 10. Persistence & Recovery

### Key Principle: No Local State Files

The system has two sources of truth:
1. **Network Manager database** -- the desired state
2. **Linux kernel** -- the actual state (tc-flower rules, netlink routes, tunnel config)

Sub-agents do NOT maintain local state files. This eliminates an entire class of
state-corruption and split-brain bugs.

### Recovery Scenarios

#### Sub-Agent Crash (process dies, systemd restarts it)

1. systemd restarts sub-agent within 2 seconds
2. Sub-agent opens socket at `/run/haze/agents/{name}.sock`
3. Master reconnects, sends `ConfigUpdate` with `full_sync=true`
4. Sub-agent reads desired state, reads kernel state via netlink
5. Diffs desired vs actual, applies only the delta
6. Reports status to master

Kernel rules are NOT lost -- they persist across process restarts.

#### Master Agent Crash

1. Sub-agents continue running (systemd manages them independently)
2. Datapath is unaffected (kernel rules still enforced)
3. systemd restarts master agent
4. Master scans `/run/haze/agents/`, reconnects to all sub-agents
5. Master pulls latest desired state from NM
6. Sends `full_sync=true` to each sub-agent
7. Normal operation resumes

#### DPU Reboot

1. All kernel state is lost (tc-flower rules cleared)
2. systemd starts `haze-agent-master` (enabled service)
3. Master registers with NM, gets capabilities
4. Master starts sub-agents via systemd
5. Each sub-agent receives full desired state from master
6. Sub-agents program all rules from scratch (empty kernel -> full install)
7. Traffic resumes once rules are installed (typically < 3 seconds)

#### Network Manager Restart

1. DPU master agents detect gRPC disconnect
2. Master agents retry connection with exponential backoff
3. Sub-agents continue enforcing existing rules (no disruption)
4. When NM comes back, masters reconnect and pull latest state
5. Any changes made during NM downtime are queued and applied

### Reconciliation Algorithm (per sub-agent)

```
func reconcile(desired []Rule, kernel []Rule) {
    toAdd    := desired - kernel   // in desired but not in kernel
    toRemove := kernel - desired   // in kernel but not in desired
    toUpdate := intersect where params differ

    for _, r := range toRemove { netlinkDelete(r) }
    for _, r := range toAdd    { netlinkAdd(r) }
    for _, r := range toUpdate { netlinkReplace(r) }
}
```


## 11. Proto Files Reference

All proto definitions live in the `technicolor-haze-proto` repository.
NM imports generated Go code from these protos.

### Domain Protos

| Proto File | Package | Used By |
|-----------|---------|---------|
| `vpc/v1/vpc.proto` | `vpc.v1` | NM (VPC domain), haze-overlay |
| `securitygroup/v1/security_group.proto` | `securitygroup.v1` | NM (secgroup domain), haze-secgroup |
| `networkacl/v1/network_acl.proto` | `networkacl.v1` | NM (ACL domain), haze-secgroup |
| `firewall/v1/firewall.proto` | `firewall.v1` | NM (firewall domain), haze-firewall |
| `elasticip/v1/elastic_ip.proto` | `elasticip.v1` | NM (EIP domain) |
| `loadbalancer/v1/load_balancer.proto` | `loadbalancer.v1` | NM (LB domain), haze-loadbalancer |
| `dns/v1/dns.proto` | `dns.v1` | NM (DNS domain) |
| `dhcp/v1/dhcp.proto` | `dhcp.v1` | NM (DHCP domain), haze-dhcp |
| `secureaccess/v1/secure_access.proto` | `secureaccess.v1` | NM (VPN domain), haze-secureaccess |

### Agent Proto

| Proto File | Package | Used By |
|-----------|---------|---------|
| `agent/v1/agent.proto` | `agent.v1` | Master agent <-> NM communication |

This proto defines:
- `RegisterDPU` / `RegisterDPUResponse`
- `GetDesiredState` / `DesiredStateResponse` (per-domain sections)
- `ReportStatus` / `StatusAck`
- `CapabilityUpdate` (NM -> master, streaming)
- `HardwareCapabilities` message

### IPC Proto (internal to DPU)

The IPC messages between master and sub-agents use the same domain protos for
config payloads, wrapped in a generic envelope:

```protobuf
// In agent/v1/ipc.proto (or inline in agent.proto)
message IPCEnvelope {
  oneof payload {
    ConfigUpdate config_update = 1;
    StatusReport status_report = 2;
    HealthCheck health_check = 3;
  }
}
```


## 12. Implementation Phases

### Phase 1: Foundation (Weeks 1-3)

**Goal**: Master agent boots, registers with NM, spawns one sub-agent.

- [ ] Define `agent/v1/agent.proto` (RegisterDPU, GetDesiredState, ReportStatus)
- [ ] Implement NM `internal/agent/` package (registration, capability assignment)
- [ ] Implement NM `internal/domain/vpc/` (basic VPC + subnet CRUD)
- [ ] Build `haze-agent-master` binary (registration, capability parsing, systemd unit writing)
- [ ] Build `haze-firewall` binary (socket listener, IPC protocol, stub tc-flower calls)
- [ ] Integration test: master registers -> gets capabilities -> starts firewall sub-agent

### Phase 2: Firewall Data Path (Weeks 4-6)

**Goal**: End-to-end firewall rule from API to kernel.

- [ ] Complete `firewall/v1/firewall.proto` with all rule types
- [ ] Implement NM `internal/domain/firewall/` (rules + NAT, replaces current flat store)
- [ ] Implement `haze-firewall` tc-flower programming (netlink Go library)
- [ ] Implement reconciliation loop (desired vs kernel diff)
- [ ] Implement conditional fetch (version-based polling)
- [ ] Integration test: create rule via API -> appears as tc-flower rule on DPU

### Phase 3: Security Groups + Overlay (Weeks 7-10)

**Goal**: Multi-tenant isolation working end-to-end.

- [ ] Define `securitygroup/v1/security_group.proto`
- [ ] Define `vpc/v1/vpc.proto` overlay sections (VXLAN tunnel spec)
- [ ] Implement NM `internal/domain/secgroup/` (per-VF rules, connection tracking)
- [ ] Implement NM `internal/domain/vpc/` overlay logic
- [ ] Build `haze-secgroup` sub-agent (per-VF tc-flower rules)
- [ ] Build `haze-overlay` sub-agent (VXLAN/Geneve tunnel setup, FDB, routing)
- [ ] Build `haze-dhcp` sub-agent (raw socket DHCP responder)
- [ ] Integration test: two VMs on different hosts communicate via overlay

### Phase 4: Shared Services (Weeks 11-14)

**Goal**: Tier 3 DPU services operational.

- [ ] Define `loadbalancer/v1/load_balancer.proto`
- [ ] Define `secureaccess/v1/secure_access.proto`
- [ ] Implement NM `internal/domain/lb/` and `internal/domain/vpn/`
- [ ] Build `haze-loadbalancer` sub-agent (IPVS for L4, eBPF for L7)
- [ ] Build `haze-secureaccess` sub-agent (WireGuard/xfrm)
- [ ] Dynamic capability updates (promote DPU tier at runtime)
- [ ] Integration test: L4 LB distributes traffic across backends

### Phase 5: Production Hardening (Weeks 15-18)

**Goal**: Ready for production deployment.

- [ ] NM store migration: memory -> PostgreSQL (with migration framework)
- [ ] Implement NM `internal/domain/eip/` (BGP announcement via edge router API)
- [ ] Implement NM `internal/domain/dns/` (zone management, CoreDNS integration)
- [ ] Metrics export (Prometheus) from master agent and all sub-agents
- [ ] Distributed tracing (OpenTelemetry) across NM -> master -> sub-agent
- [ ] Chaos testing (kill sub-agents, kill master, kill NM, partition network)
- [ ] Performance benchmarks (rule install latency, max rules per DPU, reconciliation time)
- [ ] Security audit (socket permissions, capability restrictions, input validation)

### Phase 6: Scale & Multi-Region (Weeks 19+)

**Goal**: Handle 1000+ DPUs, multi-region deployment.

- [ ] NM horizontal scaling (read replicas for GetDesiredState)
- [ ] Event-driven push notifications (replace polling for large fleets)
- [ ] Rule batching and transaction support in sub-agents
- [ ] Multi-region NM federation (each region has its own NM, cross-region state sync)
- [ ] Canary deployments for sub-agent binary upgrades
- [ ] Automated DPU firmware compatibility matrix


## Appendix A: NM Internal Package Layout (Target State)

```
technicolor-haze-network-manager/
├── cmd/
│   └── network-manager/
│       └── main.go                 # Entry point, wiring
├── internal/
│   ├── domain/
│   │   ├── vpc/
│   │   │   ├── service.go          # VPC CRUD, subnet, routing logic
│   │   │   ├── store.go            # VPC-specific store interface
│   │   │   └── service_test.go
│   │   ├── firewall/
│   │   │   ├── service.go          # Firewall + NAT rule logic
│   │   │   ├── store.go
│   │   │   └── service_test.go
│   │   ├── secgroup/
│   │   │   ├── service.go          # Security group + per-VF rules
│   │   │   ├── store.go
│   │   │   └── service_test.go
│   │   ├── acl/
│   │   │   ├── service.go          # Network ACL (per-subnet)
│   │   │   ├── store.go
│   │   │   └── service_test.go
│   │   ├── eip/
│   │   │   ├── service.go          # Elastic IP allocation + BGP
│   │   │   ├── store.go
│   │   │   └── service_test.go
│   │   ├── lb/
│   │   │   ├── service.go          # Load balancer config
│   │   │   ├── store.go
│   │   │   └── service_test.go
│   │   ├── dns/
│   │   │   ├── service.go          # Private DNS zones + records
│   │   │   ├── store.go
│   │   │   └── service_test.go
│   │   ├── dhcp/
│   │   │   ├── service.go          # DHCP lease + options
│   │   │   ├── store.go
│   │   │   └── service_test.go
│   │   └── vpn/
│   │       ├── service.go          # VPN/ZTNA tunnel config
│   │       ├── store.go
│   │       └── service_test.go
│   ├── store/
│   │   ├── store.go                # Aggregate store interface
│   │   ├── memory.go               # In-memory (dev/test)
│   │   ├── postgres.go             # PostgreSQL (production)
│   │   └── migrations/             # SQL migrations
│   ├── ipam/
│   │   ├── ipam.go                 # IP address management
│   │   └── ipam_test.go
│   ├── agent/
│   │   ├── registry.go             # DPU registration + capability assignment
│   │   ├── desired_state.go        # Aggregate desired state per DPU
│   │   ├── push.go                 # Push notifications to agents
│   │   └── registry_test.go
│   └── server/
│       ├── server.go               # gRPC service wiring
│       ├── vpc_handlers.go         # VPC RPC handlers
│       ├── firewall_handlers.go    # Firewall RPC handlers
│       ├── secgroup_handlers.go    # Security group RPC handlers
│       ├── agent_handlers.go       # Agent registration/status handlers
│       └── server_test.go
├── proto/                          # Local proto copies (buf.gen.yaml)
├── docs/
│   └── DESIGN.md                   # This file
├── Makefile
├── Dockerfile
├── go.mod
└── go.sum
```


## Appendix B: Key Invariants

1. **NM is always authoritative.** If NM says "these are the rules," that is the truth.
   Sub-agents reconcile toward NM state, never away from it.

2. **Kernel is the runtime state.** There are no state files on disk. If you want to
   know what is enforced, query the kernel (tc-flower dump, ip route show, etc.).

3. **Sub-agents are idempotent.** Sending the same ConfigUpdate twice produces no change.
   Reconciliation is a pure function of (desired_state, kernel_state) -> actions.

4. **Master agent is stateless.** It holds no persistent state. Everything it needs comes
   from NM (desired state) and the filesystem (socket discovery). Kill it anytime.

5. **No cross-sub-agent dependencies at runtime.** Each sub-agent operates on its own
   domain independently. Ordering (e.g., overlay must exist before secgroup rules
   reference it) is handled by NM sending configs in the correct sequence.

6. **Version monotonically increases.** DPU state versions in NM only go up. Agents use
   this for conditional fetch. A version bump means "something changed for this DPU."
