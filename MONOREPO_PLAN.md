# Monorepo Plan: `technicolor-haze-trail`

## Decision

Full monorepo. One repo for ALL network components (control plane + DPU agents).
Rename `technicolor-haze-network-manager` → `technicolor-haze-trail`.

Pattern: Cilium/OVN model — northd + controller in one repo, separate binaries.
Reference: follows `technicolor-haze-hypervisor` (herd) conventions exactly.

---

## Naming Convention: Trail

Texas/pastoral theme. The trail boss leads the cattle drive; trail hands patrol sections.

| Component | Binary | Meaning | Deploys To | Responsibilities |
|-----------|--------|---------|------------|-----------------|
| Network Manager | `trail-boss` | The trail boss leads the cattle drive | Server (amd64) | Control plane: tenant state, IPAM client, scheduling, HA coordination, public API (ConnectRPC) |
| DPU Master Agent | `trail-hand` | Ranch hand — patrols one section | DPU (arm64) | Session to trail-boss, capability discovery, sub-agent lifecycle (systemd), IPC broker |
| Interface Manager | `trail-corral` | Corral — where animals are penned for work | DPU (arm64) | VF/SF creation and teardown, link up/down, representor management, MTU/VLAN config |
| Firewall sub-agent | `trail-gate` | Gate — controls what passes through | DPU (arm64) | L3/L4 tc-flower rules, SNAT/DNAT, NPTv6, conntrack offload |
| Security Groups sub-agent | `trail-pen` | Pen — per-animal enclosure | DPU (arm64) | Per-VM stateful ACLs (security group rules → tc-flower) |
| Overlay/routing sub-agent | `trail-path` | Path — routes connecting pastures | DPU (arm64) | VPC overlay (VXLAN/Geneve), route programming, VNI management |
| DHCP sub-agent | `trail-brand` | Brand — marks identity | DPU (arm64) | DHCP server responses on VF representors, lease management |
| Load Balancer sub-agent | `trail-trough` | Trough — shared resource, animals gather | DPU (arm64) — L4 only | L4 DNAT/ECMP rule programming on eSwitch. Health checks + backend selection driven by trail-boss. L7 (HTTP/TLS) requires dedicated VM — out of scope for Phase 1 |
| Secure Access sub-agent | `trail-latch` | Latch — secure entry point | DPU (arm64) + Server (amd64) | DPU: WireGuard tunnel data-plane, packet filtering. Server: ZTNA auth (OIDC/cert validation), session management, policy decisions. Split binary — DPU agent + server-side controller |

---

## Target Structure

```
technicolor-haze-trail/
├── cmd/
│   ├── trail-boss/                ← Control plane binary (amd64)
│   │   └── main.go
│   ├── trail-hand/                ← DPU master agent (arm64)
│   │   └── main.go
│   ├── trail-corral/              ← Sub-agent: interface lifecycle (arm64)
│   │   └── main.go
│   ├── trail-gate/                ← Sub-agent: firewall + NAT (arm64)
│   │   └── main.go
│   ├── trail-pen/                 ← Sub-agent: security groups + ACL (arm64)
│   │   └── main.go
│   ├── trail-path/                ← Sub-agent: VPC overlay routing (arm64)
│   │   └── main.go
│   ├── trail-brand/               ← Sub-agent: DHCP responses (arm64)
│   │   └── main.go
│   ├── trail-trough/              ← Sub-agent: L4/L7 load balancer (arm64)
│   │   └── main.go
│   └── trail-latch/               ← Sub-agent: VPN/ZTNA (arm64)
│       └── main.go
│
├── internal/
│   ├── config/                    ← TOML + env-var config loading per binary
│   │   ├── boss.go               ← trail-boss config struct + defaults
│   │   ├── hand.go               ← trail-hand config struct + defaults
│   │   └── common.go             ← shared config helpers (Load, env override)
│   │
│   ├── domain/                    ← Pure domain types (ZERO external imports)
│   │   ├── vpc.go
│   │   ├── firewall.go
│   │   ├── secgroup.go
│   │   ├── nat.go
│   │   ├── ratelimit.go
│   │   ├── iface.go              ← Interface lifecycle types (VF/SF spec, state)
│   │   ├── eip.go
│   │   ├── dhcp.go
│   │   └── doc.go
│   │
│   ├── obs/                       ← Observability (slog JSON, prometheus, OTLP)
│   │   ├── logger.go             ← NewLogger() → slog.Logger (JSON)
│   │   ├── metrics.go            ← NewMetrics(reg) — injected registry, never global
│   │   ├── push.go               ← OTLP metrics push (to observability platform)
│   │   ├── tracing.go            ← OTLP tracer setup
│   │   ├── health.go             ← HealthHandler() /healthz /readyz /metrics (pull)
│   │   └── doc.go
│   │
│   ├── boss/                      ← trail-boss server logic
│   │   ├── server.go             ← ConnectRPC service implementation (public API)
│   │   ├── scheduler.go          ← DPU selection, tenant placement
│   │   ├── operations.go         ← Operation tracking
│   │   └── doc.go
│   │
│   ├── boss/store/                ← trail-boss storage backend
│   │   ├── store.go              ← Store interface
│   │   ├── memory.go             ← MemStore (dev/test)
│   │   ├── postgres.go           ← PGStore (production, durable state)
│   │   └── doc.go
│   │
│   ├── boss/bus/                  ← Cross-instance HA (NATS embedded)
│   │   ├── bus.go                ← MessageBus interface
│   │   ├── inprocess.go          ← InProcessBus (single-instance dev)
│   │   ├── nats.go               ← NATSBus (production HA, embedded server)
│   │   └── doc.go
│   │
│   ├── ipamclient/                ← IPAM client (talks to external Inventory/IPAM service)
│   │   ├── client.go             ← IPAMClient interface
│   │   ├── stub.go               ← In-memory stub (Phase 1, dev/test)
│   │   ├── grpc.go               ← Real implementation (Phase 2, talks to Inventory)
│   │   ├── errors.go             ← ErrPoolExhausted, ErrLeaseNotFound, ErrServiceDown
│   │   └── doc.go
│   │
│   ├── hand/                      ← trail-hand (DPU master agent) logic
│   │   ├── agent.go              ← Main agent loop, session management
│   │   ├── capabilities.go      ← Capability discovery + sub-agent spawn
│   │   ├── lifecycle.go          ← Systemd management of sub-agents
│   │   ├── registry.go           ← Running sub-agent registry
│   │   └── doc.go
│   │
│   ├── subagent/                  ← Shared sub-agent framework
│   │   ├── ipc/                  ← Unix socket + protobuf IPC
│   │   │   ├── server.go        ← Sub-agent side (listen on socket)
│   │   │   ├── client.go        ← trail-hand side (connect to socket)
│   │   │   ├── protocol.go      ← Wire format (4-byte len + proto)
│   │   │   └── doc.go
│   │   ├── reconciler/          ← Pull → diff → apply loop (shared)
│   │   │   ├── reconciler.go
│   │   │   └── doc.go
│   │   └── doc.go
│   │
│   ├── netlink/                   ← Shared netlink/tc-flower primitives
│   │   ├── flower.go            ← tc-flower rule CRUD
│   │   ├── police.go            ← tc police rate limiting
│   │   ├── pedit.go             ← pedit NAT actions
│   │   ├── iface.go             ← VF/SF creation, link up/down, representors
│   │   ├── types.go             ← FilterSpec, Action, etc.
│   │   └── doc.go
│   │
│   ├── nmclient/                  ← Shared trail-boss ConnectRPC client
│   │   ├── client.go
│   │   └── doc.go
│   │
│   └── transport/                 ← Transport helpers (h2c, TLS)
│       └── doc.go
│
├── gen/                            ← Generated Go code from proto (committed)
│   └── trail/v1/
│       ├── trail.pb.go
│       └── trailv1connect/
│           └── trail.connect.go
│
├── proto/                          ← Local proto definitions (trail-internal RPCs)
│   └── trail/v1/
│       ├── agent.proto            ← trail-hand ↔ trail-boss bidi session
│       └── ipc.proto              ← trail-hand ↔ sub-agents (local Unix socket)
│
├── third_party/
│   └── technicolor-haze-proto/    ← Git submodule: shared platform protos
│                                     External services communicate to trail-boss
│                                     via protos defined HERE (firewall.proto, etc.)
│                                     This is the PUBLIC API contract.
│
├── packaging/
│   ├── config/
│   │   ├── trail-boss.toml       ← Default config shipped in package
│   │   ├── trail-hand.toml
│   │   └── trail-gate.toml
│   ├── systemd/
│   │   ├── trail-boss.service
│   │   ├── trail-hand.service
│   │   ├── trail-corral.service
│   │   ├── trail-gate.service
│   │   ├── trail-pen.service
│   │   ├── trail-path.service
│   │   ├── trail-brand.service
│   │   ├── trail-trough.service
│   │   └── trail-latch.service
│   ├── scripts/
│   │   ├── preinstall.sh         ← Create trail user/group
│   │   └── postinstall.sh        ← Create dirs, chown, enable (no start)
│   ├── nfpm-boss.yaml            ← RPM/DEB for trail-boss (amd64)
│   └── nfpm-dpu.yaml             ← RPM/DEB for DPU bundle (arm64)
│
├── deploy/
│   ├── deploy-to-lab.sh          ← rsync + SSH + systemd restart
│   └── monitoring/
│       ├── prometheus.yml        ← Scrape config
│       └── dashboards/           ← Grafana JSON
│
├── test/
│   ├── integration/              ← //go:build integration
│   ├── e2e/                      ← //go:build e2e (ALL components, mock netlink)
│   │   ├── suite_test.go        ← TestMain: start all binaries, inject mock netlink
│   │   ├── lifecycle_test.go    ← Full tenant lifecycle (create→rules→NAT→teardown)
│   │   ├── failure_test.go      ← Sub-agent crash, trail-hand restart, boss failover
│   │   ├── retry_test.go        ← NATS disconnect, reconcile retry, eventual consistency
│   │   └── rollback_test.go     ← Partial apply failure → rollback to last-known-good
│   └── recon/                    ← //go:build recon (convergence)
│
├── scripts/
│   ├── coverage.sh              ← Coverage gate (85% threshold)
│   └── buf-generate.sh          ← Proto codegen (uses buf.infra.voxel.fyi)
│
├── docs/
│   ├── DESIGN.md                 ← Architecture design
│   ├── DEPLOYMENT.md             ← Where to deploy what
│   └── API.md                    ← Proto API reference
│
├── go.mod                         ← module github.com/mara-baysn/technicolor-haze-trail
├── go.sum
├── Makefile
├── buf.yaml                       ← Buf v2 workspace for proto/
├── buf.gen.yaml                   ← Codegen via buf.infra.voxel.fyi plugins
├── tools.go                       ← Pin protoc-gen-go, protoc-gen-connect-go
├── .github/
│   └── workflows/
│       └── ci.yml
├── .gitmodules
├── .gitignore
└── README.md
```

---

## Key Patterns (from technicolor-haze-hypervisor)

### ConnectRPC (public API) + gRPC (internal)

ConnectRPC server natively speaks BOTH Connect protocol AND gRPC on the same port.
No need to split — one server implementation, two protocol families.

```go
// trail-boss public API: ConnectRPC (human-debuggable via curl/buf curl)
// trail-hand internal bidi: same server, gRPC clients connect directly
mux := http.NewServeMux()
mux.Handle(trailv1connect.NewBossServiceHandler(srv))
server := &http.Server{Handler: h2c.NewHandler(mux, &http2.Server{})}
```

**Decision**: ConnectRPC for trail-boss public interface (orchestrator, tools, potential web UI).
Internal trail-hand ↔ trail-boss uses the same endpoint — Connect handlers accept native gRPC
clients transparently. No performance penalty at control-plane message rates (<10K msg/sec/core).

### Config Pattern (TOML)

```go
// internal/config/boss.go
type BossConfig struct {
    Listen       string `toml:"listen"`
    StoreBackend string `toml:"store_backend"`
    PostgresDSN  string `toml:"postgres_dsn"`
    NATS         NATSConfig `toml:"nats"`
    IPAM         IPAMConfig `toml:"ipam"`
}

func DefaultBoss() BossConfig {
    return BossConfig{Listen: ":9090", StoreBackend: "memory"}
}

// Env override: TRAIL_BOSS_LISTEN, TRAIL_BOSS_STORE_BACKEND
```

```toml
# /etc/trail-boss/config.toml
listen = ":9090"
store_backend = "postgres"
postgres_dsn = "postgres://trail:...@pg:5432/trail"

[nats]
enabled = true
port = 4222
cluster_port = 6222
seeds = ["boss-b:6222", "boss-c:6222"]

[ipam]
backend = "stub"  # "stub" for Phase 1, "grpc" for production

[obs]
otlp_endpoint = "otel-collector:4317"
metrics_push_interval = "15s"
```

### Systemd Units

```ini
[Unit]
Description=trail-hand - DPU network agent
After=network.target

[Service]
Type=simple
User=trail
Group=trail
ExecStart=/usr/bin/trail-hand --config /etc/trail-hand/config.toml
Restart=always
RestartSec=2s
StartLimitIntervalSec=0
WorkingDirectory=/var/lib/trail-hand
StateDirectory=trail-hand
ConfigurationDirectory=trail-hand
AmbientCapabilities=CAP_NET_ADMIN CAP_SYS_ADMIN

[Install]
WantedBy=multi-user.target
```

### Observability (internal/obs/) — Push + Pull

```go
// Push: OTLP metrics to observability platform (always-on)
pusher := obs.NewOTLPPusher(cfg.OTLPEndpoint, cfg.PushInterval)
defer pusher.Shutdown(ctx)

// Pull: /metrics endpoint for Prometheus scraping (optional, for debugging)
mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

// Both coexist: push is the primary path, pull is for ad-hoc debugging
```

### Domain Purity

`internal/domain/` contains ONLY:
- Type definitions (structs, enums)
- Pure functions (validation, FSM transitions)
- ZERO imports from other `internal/` packages
- Can be imported by any package without circular deps

### Testing Strategy

```makefile
test:
	go test ./... -race -timeout 90s

test-integration:
	go test -tags=integration ./test/integration/ -timeout 60s

test-e2e:
	go test -tags=e2e ./test/e2e/ -timeout 300s

coverage:
	./scripts/coverage.sh  # 85% gate per package, skips gen/ and cmd/
```

**E2E tests** (`test/e2e/`):
- Start ALL components in-process (trail-boss, trail-hand, trail-gate, trail-corral)
- Mock `internal/netlink/` via interface (no real DPU needed)
- Validate full lifecycle: tenant create → interface provision → rules apply → NAT → teardown
- Failure scenarios: sub-agent crash → trail-hand restarts it → reconcile recovers state
- Retry scenarios: NATS disconnect → reconnect → pending ops delivered
- Rollback scenarios: partial tc-flower apply fails → rollback to last-known-good state

---

## Proto Strategy (aligned with platform)

**Local protos** (trail-internal, NOT consumed by other teams):
- `proto/trail/v1/agent.proto` — trail-hand ↔ trail-boss bidi session
- `proto/trail/v1/ipc.proto` — trail-hand ↔ sub-agents (Unix socket)
- Generated into `gen/trail/v1/` and `gen/trail/v1/trailv1connect/`

**Shared protos** (PUBLIC API — external services talk to trail-boss via these):
- Live in `technicolor-haze-proto` repo under `definitions/api/firewall/v1/`, `definitions/api/network/v1/`, etc.
- Consumed via git submodule at `third_party/technicolor-haze-proto`
- This is the **contract**: orchestrator, tenant API, herd-manager all call trail-boss through these protos
- Go `replace` directive in go.mod:
  ```
  replace github.com/technicolor-haze/technicolor-haze-proto/packages/proto-codegen/go => ./third_party/technicolor-haze-proto/packages/proto-codegen/go
  ```

**Buf config** (buf.yaml v2):
```yaml
version: v2
modules:
  - path: proto
lint:
  use: [STANDARD]
  except: [SERVICE_SUFFIX, RPC_REQUEST_STANDARD_NAME, RPC_RESPONSE_STANDARD_NAME,
           RPC_REQUEST_RESPONSE_UNIQUE, PACKAGE_DIRECTORY_MATCH]
breaking:
  use: [FILE]
```

**Buf codegen** (buf.gen.yaml v2) — uses internal BSR:
```yaml
version: v2
plugins:
  - remote: buf.infra.voxel.fyi/plugins/protocolbuffers/go:v1.36.11
    out: gen
    opt: [paths=source_relative]
  - remote: buf.infra.voxel.fyi/plugins/connectrpc/go:v1.19.1
    out: gen
    opt: [paths=source_relative]
```

---

## trail-corral: Interface Lifecycle Manager

Dedicated sub-agent for VF/SF creation and teardown. Upstream services (herd-manager creating a VM)
ask trail-boss to provision interfaces; trail-boss delegates to the trail-hand on the target DPU,
which delegates to trail-corral.

**Flow**: herd-manager → trail-boss (CreateInterface RPC) → trail-hand → trail-corral → netlink

**Responsibilities**:
- Create/destroy VFs and SFs on BF3
- Link up/down, set MTU, VLAN tag
- Assign representors to tenants
- Report interface state back to trail-boss (which updates inventory stock)
- Handle ordering: interface must exist before trail-gate/trail-pen can attach rules

**Why separate from trail-hand?**
- Clear ownership boundary: trail-corral owns the physical interface lifecycle
- Other sub-agents (trail-gate, trail-pen) depend on interfaces existing — trail-corral is a prerequisite
- Failure isolation: a bug in interface teardown doesn't crash the firewall agent

---

## IPAM Strategy (external, stub for Phase 1)

IPAM lives in the external Inventory/Capacity service. trail-boss is a **lease consumer only** — it never mints IPs or VNIs directly.

```go
// internal/ipamclient/client.go
type IPAMClient interface {
    AllocateIPs(ctx context.Context, req AllocateIPRequest) ([]Allocation, error)
    ReleaseIPs(ctx context.Context, leaseIDs []string) error
    AllocateVNI(ctx context.Context, tenantID, vpcID string) (*VNILease, error)
    ReleaseVNI(ctx context.Context, leaseID string) error
    ListAllocations(ctx context.Context, tenantID string) ([]Allocation, error)
}
```

**Phase 1**: `StubIPAMClient` — in-memory bump allocator with pre-seeded pools.
**Phase 2**: `GRPCIPAMClient` — talks to the real Inventory service.

**Inventory stock updates**: when trail-corral creates/destroys interfaces, trail-boss reports capacity changes back to Inventory. For Phase 1 this is a no-op stub; the interface is ready for when Inventory API is available.

**Error handling**:
- IPAM down + new allocation → **fail-closed** (no double-allocation risk)
- IPAM down + read existing → **serve from local cache** (existing leases are stable)
- IPAM down + release → **queue locally, retry with backoff** (idempotent)

---

## trail-boss HA (NATS embedded)

NATS replaces Valkey entirely. Each trail-boss instance embeds a NATS server; they cluster automatically via route gossip. Zero external dependencies beyond PostgreSQL.

**Why NATS over Valkey?**
- Embedded: single binary deploy, no sidecar
- Subject-based routing: `trail.session.{dpu_id}` → only the owning instance subscribes
- Built-in request/reply with timeouts (no DIY correlation)
- JetStream KV: ephemeral state with Watch + CAS (replaces Redis keyspace notifications)
- Auto-mesh clustering: point at seed, mesh forms via gossip
- Operationally simpler: no hash slot management, no Sentinel

**Architecture**:
```
┌─────────────────────────────────────────────────────────────┐
│                       Load Balancer                           │
└────────┬───────────────────┬───────────────────┬────────────┘
         │                   │                   │
┌────────▼─────────┐ ┌──────▼──────────┐ ┌──────▼──────────┐
│  trail-boss-1    │ │  trail-boss-2   │ │  trail-boss-3   │
│  ┌────────────┐  │ │  ┌────────────┐ │ │  ┌────────────┐ │
│  │ Embedded   │◄─┼─┼──┤ Embedded   │◄┼─┼──┤ Embedded   │ │
│  │ NATS Node  │──┼─┼──► NATS Node  │─┼─┼──► NATS Node  │ │
│  └────────────┘  │ │  └────────────┘ │ │  └────────────┘ │
│  ┌────────────┐  │ │  ┌────────────┐ │ │  ┌────────────┐ │
│  │ JetStream  │  │ │  │ JetStream  │ │ │  │ JetStream  │ │
│  │ KV (R=3)   │  │ │  │ KV (R=3)   │ │ │  │ KV (R=3)   │ │
│  └────────────┘  │ │  └────────────┘ │ │  └────────────┘ │
│                  │ │                  │ │                  │
│  Sessions:       │ │  Sessions:       │ │  Sessions:       │
│  dpu-01, dpu-02  │ │  dpu-03, dpu-04  │ │  dpu-05, dpu-06  │
└──────────────────┘ └──────────────────┘ └──────────────────┘
        ▲                    ▲                    ▲
   trail-hand           trail-hand           trail-hand
```

**Subject design**:
- `trail.cmd.{dpu_id}` — routed to owning instance (only it subscribes)
- `trail.health.{dpu_id}` — published by owning instance
- `trail.broadcast.>` — all instances subscribe
- `trail.events.>` — JetStream stream for audit (optional)

**JetStream KV buckets**:
| Bucket | Replicas | TTL | Storage | Purpose |
|--------|----------|-----|---------|---------|
| `sessions` | 3 | 5min | memory | Active trail-hand sessions |
| `health` | 1 | 30s | memory | DPU health reports |
| `metrics-cache` | 1 | 60s | memory | Latest metrics per DPU |
| `instances` | 3 | 15s | memory | trail-boss instance registry |

**Epoch-based fencing**: session registration bumps per-DPU epoch in the `sessions` KV bucket (CAS); stale commands from old owners are rejected by comparing epochs.

**Config**:
```toml
# /etc/trail-boss/config.toml
[nats]
enabled = true
port = 4222
cluster_name = "trail-boss"
cluster_port = 6222
seeds = ["boss-b:6222", "boss-c:6222"]
store_dir = "/var/lib/trail-boss/nats"  # JetStream data (memory-backed, this is just WAL)

[ha]
instance_id = "boss-a"  # unique per instance, auto-generated if empty
```

---

## Build System (Makefile)

```makefile
VERSION ?= 0.1.0
LDFLAGS := -X main.version=$(VERSION)

.PHONY: build-all build-boss build-dpu gen test lint clean

build-all: build-boss build-dpu

build-boss:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS) -s -w" \
		-o dist/trail-boss ./cmd/trail-boss/

build-dpu: build-hand build-corral build-gate build-pen build-path build-brand build-trough build-latch

build-hand:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS) -s -w" \
		-o dist/arm64/trail-hand ./cmd/trail-hand/

build-corral:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS) -s -w" \
		-o dist/arm64/trail-corral ./cmd/trail-corral/

build-gate:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS) -s -w" \
		-o dist/arm64/trail-gate ./cmd/trail-gate/

# ... etc for each sub-agent

gen:
	./scripts/buf-generate.sh  # uses buf.infra.voxel.fyi

test:
	go test ./... -race -timeout 90s

test-integration:
	go test -tags=integration ./test/integration/ -timeout 60s

test-e2e:
	go test -tags=e2e ./test/e2e/ -timeout 300s

lint:
	golangci-lint run ./...

proto-lint:
	buf lint

proto-breaking:
	buf breaking --against '.git#branch=main'

package:
	nfpm package --config packaging/nfpm-boss.yaml --target dist/ --packager deb
	nfpm package --config packaging/nfpm-boss.yaml --target dist/ --packager rpm
	nfpm package --config packaging/nfpm-dpu.yaml --target dist/ --packager deb
	nfpm package --config packaging/nfpm-dpu.yaml --target dist/ --packager rpm

clean:
	rm -rf dist/
```

---

## Packaging (nfpm, NOT Docker)

```yaml
# packaging/nfpm-dpu.yaml
name: trail-dpu
arch: arm64
version: "${VERSION}"
maintainer: "Mara Baysn <infra@mara-baysn.com>"
description: "Trail DPU network agents (trail-hand + sub-agents)"
contents:
  - src: dist/arm64/trail-hand
    dst: /usr/bin/trail-hand
  - src: dist/arm64/trail-corral
    dst: /usr/bin/trail-corral
  - src: dist/arm64/trail-gate
    dst: /usr/bin/trail-gate
  - src: dist/arm64/trail-pen
    dst: /usr/bin/trail-pen
  - src: dist/arm64/trail-path
    dst: /usr/bin/trail-path
  - src: dist/arm64/trail-brand
    dst: /usr/bin/trail-brand
  - src: dist/arm64/trail-trough
    dst: /usr/bin/trail-trough
  - src: dist/arm64/trail-latch
    dst: /usr/bin/trail-latch
  - src: packaging/config/trail-hand.toml
    dst: /etc/trail-hand/config.toml
    type: config|noreplace
  - src: packaging/systemd/trail-hand.service
    dst: /usr/lib/systemd/system/trail-hand.service
  # ... all sub-agent configs + units
scripts:
  preinstall: packaging/scripts/preinstall.sh
  postinstall: packaging/scripts/postinstall.sh
```

---

## CI Strategy (.github/workflows/ci.yml)

```yaml
name: CI
on:
  push:
    branches: [main, develop, "feat/**"]
  pull_request:
    branches: [main, develop]

jobs:
  ci:
    runs-on: voxel-builder-amd64
    steps:
      - uses: actions/checkout@v4
        with:
          submodules: recursive
      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"

      # Proto checks (internal BSR)
      - run: buf lint
      - run: ./scripts/buf-generate.sh  # uses buf.infra.voxel.fyi
      - run: git diff --exit-code gen/  # drift detection

      # Build
      - run: make build-all

      # Quality
      - run: go vet ./...
      - uses: golangci/golangci-lint-action@v6
      - run: go test ./... -race -timeout 90s
      - run: go test -tags=integration ./test/integration/ -timeout 60s
      - run: go test -tags=e2e ./test/e2e/ -timeout 300s
```

---

## What Comes From Where

| Source | Destination | Action |
|--------|-------------|--------|
| `technicolor-haze-network-manager/internal/store/` | `internal/boss/store/` | Move |
| `technicolor-haze-network-manager/internal/server/` | `internal/boss/server.go` | Refactor to ConnectRPC |
| `technicolor-haze-network-manager/internal/tenant/` | `internal/domain/` | Extract types |
| `technicolor-haze-network-manager/cmd/network-manager/` | `cmd/trail-boss/` | Move |
| `technicolor-haze-firewall/internal/tcflower/` | `internal/netlink/` | Move + rename |
| `technicolor-haze-firewall/internal/nat/` | sub-agent internal | Move |
| `technicolor-haze-firewall/internal/ratelimit/` | `internal/netlink/police.go` | Merge |
| `technicolor-haze-firewall/internal/reconciler/` | `internal/subagent/reconciler/` | Generalize |
| `technicolor-haze-firewall/internal/manager/client.go` | `internal/nmclient/` | Refactor to ConnectRPC |
| `technicolor-haze-firewall/internal/health/` | `internal/obs/` | Merge into obs package |
| `technicolor-haze-firewall/cmd/firewall/` | `cmd/trail-gate/` | Move |
| `technicolor-haze-proto` (firewall.proto, network.proto) | `third_party/` submodule | Submodule |

---

## Shared Code (why monorepo wins)

| Package | Used By |
|---------|---------|
| `internal/netlink/` | trail-corral, trail-gate, trail-pen, trail-path, trail-brand, trail-trough |
| `internal/subagent/ipc/` | ALL sub-agents + trail-hand |
| `internal/subagent/reconciler/` | ALL sub-agents |
| `internal/obs/` | ALL binaries |
| `internal/config/` | ALL binaries |
| `internal/domain/` | ALL binaries |
| `internal/nmclient/` | trail-hand + all sub-agents |
| `gen/` (proto types) | ALL binaries |

In multi-repo, each of these would be a separate Go module with version coordination hell.
In monorepo: one `go.mod`, one version, one PR to change shared code.

---

## Release Strategy

- No goreleaser or semantic-release — manual `make package` for now
- One version number for ALL binaries (they deploy together)
- `VERSION ?= 0.1.0` in Makefile, injected via ldflags
- Tag: `v1.2.3` → all binaries are v1.2.3
- Packages: `trail-boss-0.1.0.amd64.deb` + `trail-dpu-0.1.0.arm64.deb`
- Published to internal package registry
- Deploy: `deploy/deploy-to-lab.sh` (rsync + SSH + systemd restart)

---

## go.mod

```go
module github.com/mara-baysn/technicolor-haze-trail

go 1.25

replace (
    github.com/mara-baysn/technicolor-haze-proto => ./third_party/technicolor-haze-proto
    github.com/technicolor-haze/technicolor-haze-proto/packages/proto-codegen/go => ./third_party/technicolor-haze-proto/packages/proto-codegen/go
)

require (
    connectrpc.com/connect v1.19.1
    github.com/BurntSushi/toml v1.4.0
    github.com/jackc/pgx/v5 v5.7.0
    github.com/nats-io/nats-server/v2 v2.11.0
    github.com/nats-io/nats.go v1.38.0
    github.com/vishvananda/netlink v1.3.0
    github.com/prometheus/client_golang v1.20.0
    go.opentelemetry.io/otel v1.32.0
    go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.32.0
    golang.org/x/net v0.30.0
    google.golang.org/protobuf v1.36.0
)
```

---

## Migration Steps

1. Create new repo: `technicolor-haze-trail`
2. Initialize go.mod, Makefile, buf.yaml, buf.gen.yaml, tools.go
3. Add `third_party/technicolor-haze-proto` as git submodule
4. Create `internal/domain/` with pure types
5. Create `internal/obs/` (logger, metrics push+pull, health)
6. Create `internal/config/` (TOML + env)
7. Move NM code → `internal/boss/` + `cmd/trail-boss/`
8. Implement `internal/boss/bus/` (NATS embedded)
9. Create `internal/ipamclient/` (interface + stub)
10. Move firewall agent → `cmd/trail-gate/` + `internal/netlink/`
11. Create `cmd/trail-corral/` (interface lifecycle)
12. Create `internal/subagent/` framework (IPC, reconciler)
13. Create `internal/hand/` (trail-hand agent logic)
14. Create stub `cmd/` entries for other sub-agents
15. Create `packaging/` (nfpm, systemd, scripts)
16. Create `test/e2e/` (full lifecycle with mock netlink)
17. Create CI workflow
18. Create `deploy/deploy-to-lab.sh`
19. Update `technicolor-haze-firewall` README: "DEPRECATED — moved to technicolor-haze-trail"
20. Archive `technicolor-haze-firewall` repo

---

## What to Implement in Phase 1

| Binary | Status | Phase 1 |
|--------|--------|---------|
| trail-boss | Exists (v1) | Refactor: ConnectRPC, TOML config, NATS HA, IPAM stub |
| trail-hand | New | Implement: capability discovery, sub-agent lifecycle, bidi session |
| trail-corral | New | Implement: VF create/destroy, link up/down (required by all others) |
| trail-gate | Exists | Move from technicolor-haze-firewall, wire to sub-agent framework |
| trail-pen | New | Stub (reconciler loop + placeholder) |
| trail-path | New | Stub |
| trail-brand | New | Stub |
| trail-trough | New | Stub |
| trail-latch | New | Stub |

Phase 1 delivers: trail-boss + trail-hand + trail-corral + trail-gate working end-to-end.
Other sub-agents are stubs that can be implemented incrementally.

---

## Differences from Herd (hypervisor)

| Aspect | Herd | Trail | Why |
|--------|------|-------|-----|
| Agent topology | Single binary per host | Master + sub-agents per DPU | Network needs isolation per domain |
| IPC | Bidi Connect stream (host↔manager) | Unix socket (hand↔sub-agents) + Connect (hand↔boss) | Sub-agents are local to DPU, not remote |
| Target arch | amd64 only | amd64 (boss) + arm64 (DPU) | BF3 is ARM |
| HA bus | Valkey Pub/Sub | Embedded NATS | Zero external deps, subject routing, JetStream KV |
| Config format | YAML | TOML | Cleaner syntax for nested config, unambiguous types |
| IPAM | Internal to manager | External (Inventory service) | Platform IPAM is centralized, Trail is a consumer |
| Capabilities | CAP_SYS_ADMIN | CAP_NET_ADMIN CAP_SYS_ADMIN | tc-flower needs NET_ADMIN |
| Interface lifecycle | herd-handler owns VF attach | trail-corral (dedicated sub-agent) | Shared dependency, needs ordering guarantees |
| Metrics | Pull only (Prometheus scrape) | Push (OTLP) + Pull (/metrics) | Push is primary for DPU (no inbound scrape path) |
| Proto generation | Local protoc-gen-* | buf.infra.voxel.fyi (internal BSR) | Platform standard, centralized plugin versions |
