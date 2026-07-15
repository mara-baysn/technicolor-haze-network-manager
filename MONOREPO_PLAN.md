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
├── Dockerfile                     ← trail-boss K8s container image (multi-stage, distroless)
├── buf.yaml                       ← Buf v2 workspace for proto/
├── buf.gen.yaml                   ← Codegen via buf.infra.voxel.fyi plugins
├── tools.go                       ← Pin protoc-gen-go, protoc-gen-connect-go
├── .github/
│   └── workflows/
│       └── ci.yml
├── .releaserc.yaml                ← semantic-release root config
├── release/
│   ├── trail-boss.releaserc.yaml  ← Boss release config
│   └── trail-dpu.releaserc.yaml   ← DPU package release config
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

All DPU agents run under `trail.slice` with per-binary memory enforcement (see DPU Memory Budget section).

```ini
[Unit]
Description=trail-hand - DPU network master agent
After=network.target
Before=trail-corral.service trail-gate.service trail-pen.service trail-path.service trail-brand.service trail-trough.service trail-latch.service

[Service]
Type=notify
User=trail
Group=trail
ExecStart=/usr/bin/trail-hand --config /etc/trail-hand/config.toml
Slice=trail.slice
Restart=always
RestartSec=2s
StartLimitIntervalSec=0
WorkingDirectory=/var/lib/trail-hand
StateDirectory=trail-hand
ConfigurationDirectory=trail-hand
AmbientCapabilities=CAP_NET_ADMIN CAP_SYS_ADMIN

# Memory enforcement
MemoryMax=64M
MemoryHigh=51M
Environment=GOMEMLIMIT=64MiB
Environment=GOGC=50
Environment=GOMAXPROCS=4

# IPC watchdog
WatchdogSec=30s

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

**trail-boss capacity metrics** (critical for growth planning):

```
trail_boss_fd_used                    # current open file descriptors
trail_boss_fd_limit                   # ulimit -n (max available)
trail_boss_fd_utilization_ratio       # used/limit — alert at 80%
trail_boss_active_agent_streams       # persistent bidi streams to DPU agents
trail_boss_push_inflight_total        # PushStageConfig messages awaiting StageAck
trail_boss_push_fan_out_duration_seconds  # histogram: time to push to all affected DPUs
trail_boss_compilation_duration_seconds   # histogram: intent → per-DPU config
```

Alert thresholds: `fd_utilization_ratio > 0.8` fires a warning (approaching limit). `active_agent_streams` compared against fleet inventory detects DPU disconnections.

### Domain Purity

`internal/domain/` contains ONLY:
- Type definitions (structs, enums)
- Pure functions (validation, FSM transitions)
- ZERO imports from other `internal/` packages
- Can be imported by any package without circular deps

### Testing Strategy

Four test tiers enforce correctness from unit through hardware integration:

```makefile
# Tier 1: Fast feedback (per-commit, <60s)
test:            go test ./... -short -count=1
test-race:       go test ./... -short -race -count=1
lint:            golangci-lint run ./...
bench:           go test ./... -bench=. -benchmem -count=6 | tee bench-current.txt && \
                 benchstat bench-baseline.txt bench-current.txt

# Tier 2: Fault injection (nightly CI, ~10min)
test-fault:      go test ./... -tags=faultinject -timeout=10m -count=1
test-fault-race: go test ./... -tags=faultinject -race -timeout=15m -count=1

# Tier 3: Multi-process E2E (nightly CI, ~20min)
test-multiprocess: go test ./test/e2e/... -tags=multiprocess -timeout=20m -count=1 -v

# Tier 4: Hardware integration (gates releases, arm64 only)
test-hardware:   go test ./... -tags=hardware -race -timeout=30m -count=1

# Quality gates
coverage:        go test ./... -coverprofile=cover.out && go tool cover -func=cover.out
mutate:          go-mutesting ./internal/subagent/reconciler/... ./internal/hand/... ./internal/netlink/...
```

**Netlink Fault-Injection Layer**

The `internal/netlink/faultinject` package is NOT a mock — it is a programmable error injector wrapping real `netlink.Handle` types. Production code accepts a `netlink.Conn` interface; the fault layer implements the same interface but injects configurable failures:

| Fault Mode | Simulates | Use Case |
|---|---|---|
| `ENOSPC` | Hardware flow table full (4096 rules) | Validates table-full backpressure and eviction |
| `EBUSY` | Firmware operation in progress | Tests retry/backoff logic in reconciler |
| `ENOMEM` | Kernel memory pressure | Tests graceful degradation path |
| Partial batch failure | N of M rules in a batch fail | Validates atomic rollback of partial applies |

Configurable failure policies:
- `FailAfterN(n int)` — first N calls succeed, then inject error
- `RandomRate(pct float64)` — fail randomly at given percentage (e.g., 5%)
- `LatencySpike(d time.Duration)` — inject delay before returning (e.g., 50ms)
- `FailSequence(errs []error)` — return errors in order, then succeed

Tests using fault injection are gated behind `//go:build faultinject` to avoid polluting fast unit runs.

**Multi-Process E2E**

The `test/e2e/` package launches trail-boss, trail-hand, and sub-agents as separate OS processes using `exec.Command`, not in-process goroutines. This validates real IPC paths:

- **Real Unix sockets** — trail-hand listens on `/tmp/trail-test-<random>/hand.sock`; sub-agents connect as independent processes
- **Real NATS clustering** — spins up a 3-node embedded NATS cluster with actual TCP ports
- **Process isolation** — each binary runs with its own PID, memory space, and signal handling

Chaos injection scenarios:

| Scenario | Method | Assertion |
|---|---|---|
| Sub-agent crash | `SIGKILL` random sub-agent PID | trail-hand detects within 5s, respawns, state converges within 60s |
| NATS partition | `iptables DROP` between NATS nodes (netns) | Boss detects partition, fences stale agents, reconverges after heal |
| Boss-hand delay | `tc netem delay 5000ms` on loopback | Hand enters autonomous mode, queues updates, replays after reconnect |
| Cascading failure | Kill 3 of 5 sub-agents simultaneously | System recovers without operator intervention within 60s SLO |

Every chaos scenario asserts the **60-second convergence SLO**: after injection, the desired-state/actual-state delta must reach zero within 60 seconds.

**Hardware Integration Gate**

A self-hosted GitHub Actions runner on a lab BlueField-3 DPU (arm64) executes `test-hardware` nightly:

- `go test -race` on native arm64 (catches alignment bugs, ARM memory model issues)
- BF3-specific tc-flower validation: programs real eSwitch rules via netlink, verifies with `tc filter show`
- OVS representor port verification with actual VF-rep netdevs
- Hardware tests MUST pass before any deployment outside the lab environment

**Quality Gates (Tiered)**

| Gate | Threshold | Scope | Enforcement |
|---|---|---|---|
| Line coverage floor | 85% | All packages | Per-PR, merge-blocking |
| Mutation testing | No surviving mutants in critical paths | `internal/subagent/reconciler`, `internal/hand`, `internal/netlink` | Nightly, blocks release |
| Error-path coverage | Every function returning `error` has ≥ 1 error-path test | All packages | Per-PR via custom linter |
| FSM transition coverage | Every state transition exercised, including illegal transitions | `internal/subagent/reconciler`, `internal/hand` | Per-PR, merge-blocking |
| Benchmark regression | No metric regresses > 10% vs baseline | All `_test.go` benchmarks | Per-PR via `benchstat`, advisory warning at 5% |

The 85% line coverage floor is a necessary minimum, not a quality signal. Mutation testing and error-path coverage provide the actual correctness guarantee.

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

## Sub-Agent Failure Behavior

All sub-agents on a DPU are supervised by trail-hand via Unix socket IPC. Failure isolation ensures that a single component crash does not tear down active tenant traffic.

### trail-hand Death Behavior

When trail-hand terminates (crash, OOM-kill, upgrade), sub-agents detect the loss of their IPC connection and transition independently:

| Phase | Behavior |
|-------|----------|
| Detection | Sub-agent IPC heartbeat missed for configurable watchdog interval (default 30s) |
| Transition | Sub-agent enters `DEGRADED` state |
| Enforcement | Last-known rules remain programmed in hardware — existing flows continue forwarding |
| Restriction | All new flow programming requests are refused with `UNAVAILABLE` status |
| Recovery | On trail-hand reconnect, sub-agent sends a `FullActualStateReport` containing every rule/interface/flow it currently enforces |

The fail-safe posture preserves tenant connectivity: tc-flower rules already installed in the eSwitch persist regardless of userspace process state. Sub-agents only gate *mutations*, not the datapath itself.

**Reconnection protocol:**

1. trail-hand opens new Unix socket connection to each sub-agent.
2. Sub-agent replies with `FullActualStateReport` (all programmed flows, interface states, counters).
3. trail-hand performs reconciliation: diffs reported state against desired state from trail-boss.
4. Stale entries are removed; missing entries are programmed. Sub-agent transitions back to `READY`.

### trail-corral Failure Mid-Operation

trail-corral manages interface lifecycle through a per-interface state machine. Failures at any stage trigger compensating teardown rather than leaving partial state.

```
┌──────────────────────────────────────────────────────────────────┐
│              trail-corral Interface State Machine                 │
├──────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────┐    success    ┌─────────────┐    success           │
│  │ CREATING ├──────────────►│ CONFIGURING ├──────────────┐       │
│  └────┬─────┘              └──────┬──────┘              │       │
│       │                           │                      ▼       │
│       │ error                     │ error          ┌─────────┐   │
│       │                           │                │  READY  │   │
│       ▼                           ▼                └────┬────┘   │
│  ┌──────────┐              ┌──────────┐                 │        │
│  │  FAILED  │◄─────────────┤  FAILED  │                 │ delete │
│  └────┬─────┘              └────┬─────┘                 │        │
│       │                         │                        ▼        │
│       │ compensate              │ compensate      ┌────────────┐  │
│       ▼                         ▼                 │ DESTROYING │  │
│  ┌────────────┐           ┌────────────┐          └─────┬──────┘  │
│  │ DESTROYING │           │ DESTROYING │                │        │
│  └────────────┘           └────────────┘                ▼        │
│                                                    (removed)     │
└──────────────────────────────────────────────────────────────────┘
```

**State semantics:**

- **CREATING** — VF representor allocated, netdev created, not yet configured.
- **CONFIGURING** — VLAN, MAC, rate-limit, and QoS parameters being applied via netlink.
- **READY** — Interface fully operational. Only interfaces in this state are visible to downstream sub-agents (trail-gate, trail-pen).
- **FAILED** — Error during creation or configuration. Triggers compensating teardown before retry.
- **DESTROYING** — Teardown in progress. Resources released in reverse order.

**Gating behavior:** trail-hand will not dispatch interface references to trail-gate or trail-pen until trail-corral reports that interface has reached `READY`.

### trail-boss Detection of DPU Degradation

trail-boss monitors the gRPC heartbeat stream from every trail-hand instance:

- Heartbeat interval: 3s (configurable via `trail-boss.toml`)
- Miss threshold: 3 consecutive misses (~10s wall-clock)
- On threshold breach: DPU marked `DEGRADED` in PostgreSQL placement table
- Effect: scheduler excludes that DPU from new VM placement decisions
- On heartbeat recovery: DPU transitions to `RECOVERING`, then `READY` after full state reconciliation

### Sub-Agent Lifecycle State Machine

```
     start
       │
       ▼
  ┌──────────┐   IPC connected   ┌────────────┐  all checks pass  ┌───────┐
  │  INIT    ├───────────────────►│ REGISTERING├───────────────────►│ READY │
  └──────────┘                   └─────┬──────┘                   └───┬───┘
                                       │                               │
                                       │ register failed               │ heartbeat
                                       ▼                               │ lost
                                  ┌──────────┐                         │
                                  │  FATAL   │                         ▼
                                  └──────────┘                  ┌──────────┐
                                                                │ DEGRADED │
                                       ┌────────────────────────┴────┬─────┘
                                       │                             │
                                       │ reconnect +                 │ watchdog
                                       │ FullActualStateReport       │ expires
                                       ▼                             ▼
                                  ┌──────────┐                  ┌──────────┐
                                  │RECOVERING│                  │  FATAL   │
                                  └────┬─────┘                  └──────────┘
                                       │ reconciliation complete
                                       ▼
                                  ┌───────┐
                                  │ READY │
                                  └───────┘
```

### IPC Watchdog Configuration

```toml
[ipc.watchdog]
heartbeat_interval = "5s"
degraded_threshold = "30s"
fatal_threshold = "5m"
preserve_datapath = true  # fail-safe: existing flows continue forwarding

[ipc.reconnect]
initial_backoff = "100ms"
max_backoff = "5s"
multiplier = 2.0
reconciliation_timeout = "30s"
```

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

**Phase 1**: `ChaosIPAMStub` — in-memory bump allocator with pre-seeded pools and built-in fault injection that exercises error paths from day one.

The chaos stub ensures that code consuming IPAM never silently depends on happy-path assumptions that collapse under real allocator behavior. On every `Allocate` call the stub:

1. Injects random latency (10-100ms, uniform) on 20% of calls, forcing all callers through async-safe code paths.
2. Returns `POOL_EXHAUSTED` on 5% of allocations, exercising partial-allocation rollback.
3. Expires leases after a configurable interval (default 1h), triggering lease-renewal logic.
4. On process restart, loads persisted allocation state from a local JSON file rather than re-seeding — simulates existing-allocation reconciliation.

```toml
[ipam_stub]
chaos_mode = true          # default true in dev/test, false for demos
latency_pct = 20
latency_min_ms = 10
latency_max_ms = 100
exhaustion_pct = 5
lease_ttl = "1h"
state_file = "/var/lib/trail-boss/ipam-stub-state.json"
```

**Phase 2**: `GRPCIPAMClient` — talks to the real Inventory service.

**Inventory stock updates**: when trail-corral creates/destroys interfaces, trail-boss reports capacity changes back to Inventory. For Phase 1 this is a no-op stub; the interface is ready for when Inventory API is available.

**Error handling** — Because the chaos stub injects `POOL_EXHAUSTED`, timeout, and stale-lease errors from day one, all error paths are continuously validated in every CI run:
- IPAM down + new allocation → **fail-closed** (no double-allocation risk)
- IPAM down + read existing → **serve from local cache** (existing leases are stable)
- IPAM down + release → **queue locally, retry with backoff** (idempotent)

---

## Intent Compilation (trail-boss)

trail-boss is the **compilation engine**: it receives high-level intent from upstream controllers (Tenant/VPC Controller, Gateway Controller, Prism, Nexus, NAT, LB, DNS, DHCP) and compiles it into per-DPU desired state scoped to the services available on each DPU.

**Compilation flow:**

```
Upstream Intent (per-tenant)          trail-boss Compilation          Per-DPU Desired State
─────────────────────────────    →    ────────────────────────    →   ─────────────────────────
"SG rule: allow tcp/443 from       "DPU-17 has VF pf0vf3 for       "trail-gate@DPU-17:
 10.0.1.0/24 for tenant-A"          tenant-A in VNI 100042"          stage=NACL, vni=100042,
                                                                      match=10.0.1.0/24:tcp:443
                                                                      action=ALLOW"
```

**Per-DPU scoping:** trail-boss knows which DPU hosts which tenant VFs (from trail-corral capacity reports). It compiles intent only for DPUs that have relevant VFs. A Security Group change for tenant-A only produces desired-state for DPUs hosting tenant-A's VFs — not the entire fleet.

**Service availability awareness:** trail-hand reports its registered sub-agents (capabilities) to trail-boss during session establishment. trail-boss only emits desired state for pipeline stages that have a running sub-agent on that DPU. If trail-pen is not yet deployed, trail-boss does not emit Security Group entries for that DPU's pipeline stage 3.

**Generation tracking:** Each compilation produces a monotonically increasing `generation` per (dpu_id, vni, stage) tuple. trail-hand rejects any push with `generation <= current` (prevents rollback from stale/replayed messages). Full-replace semantics: each push contains the COMPLETE desired state for that tuple — no partial updates, no ordering dependencies between pushes.

**Fan-out model:** When upstream pushes a VPC-wide SG change affecting 50 DPUs, trail-boss compiles the per-DPU config and pushes to all 50 agent streams concurrently. Wall-clock convergence: ~17ms (5ms compile + 2ms push + 10ms eSwitch program). The 60-second SLO is trivially met.

---

## trail-boss HA (NATS embedded)

NATS replaces Valkey entirely. Each trail-boss instance embeds a NATS server; they cluster automatically via route gossip. Zero external dependencies beyond PostgreSQL.

**Deployment model:** trail-boss produces TWO artifacts:
1. **Bare-metal systemd** — `.deb`/`.rpm` via nfpm for lab and non-K8s environments
2. **Dockerfile** — for running in Kubernetes (Tier 2 management cluster) alongside other control-plane controllers

Both artifacts use the same binary with identical config. The K8s variant uses a Kubernetes Deployment with embedded NATS forming a cluster via headless Service DNS. The systemd variant uses static seed addresses. The HA mechanism (embedded NATS + epoch fencing) is deployment-model agnostic.

**Communication model:** trail-hand maintains a persistent gRPC bidi stream to trail-boss (agent dials out — no inbound connectivity to DPU). trail-boss pushes compiled config over this established stream. The data flows:

```
Upstream (8 services) ──push──► trail-boss ──push_over_bidi──► trail-hand ──push──► sub-agents
                                             ◄──ack/status──    ◄──status──
```

- **Downstream (boss → hand):** `PushStageConfig` (compiled per-DPU desired state), `FlushStage`, `RecoveryGateAdvance`, `HeartbeatAck`
- **Upstream (hand → boss):** `AgentHello`, `FullActualStateReport`, `StageAck`/`StageNack`, `Heartbeat`, `FlowTableAlert`

Agent-side safety controls:
- **Circuit breaker:** reject if >N config changes per time window (prevents fleet-wide poisoning from compromised boss)
- **Rate limiting:** max 5 config applications per 10-second window per pipeline stage
- **Validation:** verify epoch, generation monotonicity, and VNI ownership before applying any pushed config
- **gRPC flow control:** agent sizes receive window (256KB) to bound kernel buffer accumulation during GC pauses

NATS is used for inter-boss coordination (session ownership, broadcast) — NOT for boss-to-agent communication.

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

**Subject design** (inter-boss coordination only — NOT agent communication):
- `trail.session.{dpu_id}` — owning instance subscribes for session ownership events
- `trail.broadcast.>` — all instances subscribe (policy pushes, config changes)
- `trail.events.>` — JetStream stream for audit (optional)
- `trail.rebalance` — session redistribution on instance join/leave

**JetStream KV buckets**:
| Bucket | Replicas | TTL | Storage | Purpose |
|--------|----------|-----|---------|---------|
| `sessions` | 3 | 5min | memory | Active trail-hand sessions |
| `health` | 1 | 30s | memory | DPU health reports |
| `metrics-cache` | 1 | 60s | memory | Latest metrics per DPU |
| `instances` | 3 | 15s | memory | trail-boss instance registry |

**Epoch-based fencing**: session registration bumps per-DPU epoch in the `sessions` KV bucket (CAS); stale commands from old owners are rejected by comparing epochs.

### Epoch Fencing Protocol (trail-hand validation)

Every message from trail-boss to trail-hand on the bidi stream carries the session epoch in a `CommandEnvelope` protobuf wrapper. trail-hand enforces monotonic epoch ordering and rejects stale pushes.

**Rules:**

1. trail-hand stores `registered_epoch` — set when a boss successfully calls `RegisterOwnership(epoch)`
2. Any `CommandEnvelope` with `epoch < registered_epoch` is rejected immediately with `EPOCH_STALE`
3. trail-hand does NOT execute partial commands from a stale epoch; the entire envelope is discarded
4. Boss must send a `LeaseRenew` heartbeat every 10s. If trail-hand misses 3 consecutive heartbeats (30s timeout), it transitions to `UNOWNED` state and refuses all commands until a new boss registers

**Proto definition:**

```protobuf
// Wraps all boss→hand messages with epoch fencing
message CommandEnvelope {
  uint64 epoch = 1;
  uint64 boot_epoch = 2;       // must match agent's NV-stored boot_epoch
  google.protobuf.Timestamp issued_at = 3;
  oneof payload {
    PushStageConfig push_stage_config = 10;
    FlushStage flush_stage = 11;
    ProgramSessionBatch program_session_batch = 12;
    RecoveryGateAdvance recovery_gate_advance = 13;
    HeartbeatAck heartbeat_ack = 14;
    LeaseRenew lease_renew = 15;
  }
}

message PushStageConfig {
  string dpu_id = 1;
  string vni = 2;
  uint32 pipeline_stage = 3;
  uint64 generation = 4;       // monotonic per (dpu_id, vni, stage)
  bytes compiled_config = 5;   // stage-specific protobuf payload
}

message LeaseRenew {
  uint64 epoch = 1;
  google.protobuf.Duration interval = 2;
}

message CommandResponse {
  uint64 accepted_epoch = 1;
  enum Status {
    OK = 0;
    EPOCH_STALE = 1;
    BOOT_EPOCH_MISMATCH = 2;
    UNOWNED = 3;
    GENERATION_STALE = 4;
    RATE_LIMITED = 5;
    INTERNAL_ERROR = 6;
  }
  Status status = 2;
  string detail = 3;
}
```

**GC-pause fencing scenario:**

```
Boss-A          trail-hand           Boss-B (new leader)
  │                 │                      │
  │── LeaseRenew(epoch=7) ──►│             │
  │                 │ registered_epoch=7   │
  │                 │                      │
  │  [Boss-A GC pause / network partition] │
  │                 │                      │
  │                 │◄── 30s no heartbeat ─┤
  │                 │ → state = UNOWNED    │
  │                 │                      │
  │                 │◄── RegisterOwnership(epoch=8)
  │                 │ registered_epoch=8   │
  │                 │ → state = OWNED      │
  │                 │──── OK ─────────────►│
  │                 │                      │
  │                 │◄── PushStageConfig(epoch=8)
  │                 │──── OK ─────────────►│
  │                 │                      │
  │── PushStageConfig(epoch=7) ──►│        │
  │                 │ epoch 7 < 8 → REJECT │
  │◄── EPOCH_STALE ─┤                      │
  │                 │                      │
```

Boss-A's GC pause causes it to miss the heartbeat window. trail-hand revokes ownership after 30s, Boss-B wins leader election and registers epoch=8. When Boss-A recovers and sends stale epoch=7 command, trail-hand rejects it deterministically.

### trail-boss Capacity (persistent bidi streams)

Each DPU connection costs ~200KB on trail-boss (TCP socket + HTTP/2 state + goroutines + per-DPU state cache). Connections are cheap; burst fan-out is the bottleneck.

| DPUs | Memory | Goroutines | Burst Push (50KB to all, 4 cores) |
|------|--------|------------|-----------------------------------|
| 200 | 340 MB | 1,050 | 25ms |
| 500 | 400 MB | 2,550 | 62ms |
| 1,000 | 500 MB | 5,050 | 125ms |
| 2,000 | 700 MB | 10,050 | 250ms |
| 5,000 | 1,300 MB | 25,050 | 625ms |

**Practical ceilings:**
- 4 cores, 16GB pod: **2,000 DPUs** comfortable, 4,000 stretched
- 8 cores, 32GB pod: **5,000 DPUs** comfortable, 8,000 stretched
- Beyond 5,000: shard by tenant group or rack affinity (two 4c pods > one 8c pod due to halved blast radius)

**Bottleneck ordering:** CPU burst > Network burst > GC pressure > Memory. File descriptors never limit (set `ulimit -n 65536`).

**v1.0 target (200 DPUs):** 340MB RAM, 0.1 CPU core steady-state. 10x headroom before any scaling concern.

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

## DPU Memory Budget

The BlueField-3 DPU provides 16GB of RAM shared between the embedded ARM OS, DOCA
framework services, crypto offload buffers, and all Trail agent processes. The total
memory allocation for all 8 Trail agent binaries under peak load must not exceed
**1GB**, leaving at minimum 20% headroom (approximately 3GB on a 16GB SoC) for the
Linux kernel, DOCA runtime, OpenSSL/libcrypto page cache, OVS-related kernel
structures, and tc-flower netlink buffers.

### Per-Binary Memory Targets

| Binary | Role | GOMEMLIMIT | MemoryHigh (80%) | MemoryMax | Measured Idle RSS |
|--------|------|-----------|------------------|-----------|-------------------|
| trail-hand | Coordinator, session state for all sub-agents | 64MiB | 51MiB | 64MiB | ~28MiB |
| trail-corral | VF/SF lifecycle management | 48MiB | 38MiB | 48MiB | ~18MiB |
| trail-gate | Firewall rule compilation, tc-flower batch ops | 128MiB | 102MiB | 128MiB | ~42MiB |
| trail-pen | Security group evaluation and flow caching | 96MiB | 76MiB | 96MiB | ~34MiB |
| trail-path | Overlay routing, VNI lookup table | 64MiB | 51MiB | 64MiB | ~24MiB |
| trail-brand | DHCP lease state, minimal footprint | 32MiB | 25MiB | 32MiB | ~14MiB |
| trail-trough | Load-balancer rule sets and health state | 48MiB | 38MiB | 48MiB | ~20MiB |
| trail-latch | WireGuard tunnel state and key material | 48MiB | 38MiB | 48MiB | ~18MiB |
| **Total** | | **528MiB** | | **528MiB** | **~198MiB** |

The 528MiB total leaves 472MiB of the 1GB envelope unallocated, providing burst
capacity during rule-set compilation spikes and protecting against GC pressure
cascades across binaries.

### Cgroup Slice Architecture

All Trail agents run under a shared `trail.slice` with a combined hard ceiling:

```ini
# /etc/systemd/system/trail.slice
[Slice]
Description=Trail DPU Agent Slice
MemoryMax=1200M
MemoryHigh=960M
ManagedOOMMemoryPressure=kill
ManagedOOMMemoryPressureLimit=80%
```

The slice-level `MemoryMax=1200M` (rather than exactly 1024M) accounts for transient
kernel-side allocations (netlink socket buffers, cgroup metadata) attributed to the
slice but outside Go heap.

### Systemd Unit Example: trail-gate

```ini
[Unit]
Description=Trail Gate — DPU Firewall Rule Agent
After=trail-hand.service
BindsTo=trail-hand.service
PartOf=trail.slice

[Service]
Type=notify
ExecStart=/usr/bin/trail-gate --config /etc/trail-gate/config.toml
Slice=trail.slice

# Memory enforcement
MemoryMax=128M
MemoryHigh=102M

# Go runtime tuning
Environment=GOMEMLIMIT=128MiB
Environment=GOGC=50
Environment=GOMAXPROCS=4

# Security hardening
User=trail
Group=trail
AmbientCapabilities=CAP_NET_ADMIN CAP_SYS_ADMIN
ProtectSystem=strict
PrivateTmp=true
NoNewPrivileges=true
ReadWritePaths=/var/lib/trail-gate
StateDirectory=trail-gate
ConfigurationDirectory=trail-gate

# Restart policy
Restart=on-failure
RestartSec=2s
WatchdogSec=30s

[Install]
WantedBy=multi-user.target
```

Key settings:

- **`MemoryMax=128M`** — hard cgroup limit; OOM killer terminates if exceeded. Matches GOMEMLIMIT so the Go GC aggressively reclaims before the kernel intervenes.
- **`MemoryHigh=102M`** (80%) — kernel throttles allocations and emits `memory.high` event, giving time to shed load before hitting the hard wall.
- **`GOMEMLIMIT=128MiB`** — Go 1.19+ runtime targets this as heap ceiling. GC runs more frequently as RSS approaches.
- **`GOGC=50`** — reduces GC target growth ratio from 100% to 50%, producing more frequent but smaller GC pauses.
- **`GOMAXPROCS=4`** — limits Go scheduler threads to 4 of 16 available A78 cores.

### IPC Back-Pressure Strategy

When a sub-agent's RSS crosses the `MemoryHigh` threshold (80% of budget):

1. **Cgroup notification** — systemd detects `memory.high` breach. trail-hand monitors via `sd_bus`.
2. **Flow control** — trail-hand stops dispatching new work to the pressured sub-agent.
3. **Shed ephemeral state** — agent evicts LRU-cached flow entries and compiled rule fragments.
4. **Propagate upstream** — if pressure persists >5s, trail-hand reports `RESOURCE_EXHAUSTED` to trail-boss. Control plane can rebalance tenant assignments.
5. **Recovery** — once RSS drops below 70% (sustained 3s), trail-hand resumes dispatching.

### Validation

Memory budgets are validated in CI via integration tests that run all 8 binaries
simultaneously under simulated load (500 tenants, 2000 firewall rules per tenant for
trail-gate) on an ARM64 runner. Assertions:

- No individual binary exceeds its `MemoryMax` within 60 seconds of sustained load
- Combined RSS of all 8 binaries remains below 1000MiB at p99
- Back-pressure activates correctly when synthetic memory pressure is injected

Production metrics exported by trail-hand:

```
trail_agent_rss_bytes{agent="trail-gate"}
trail_agent_gomemlimit_bytes{agent="trail-gate"}
trail_agent_memory_pressure_events_total{agent="trail-gate"}
trail_agent_shed_operations_total{agent="trail-gate"}
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

Semantic-release with Conventional Commits from day one. No manual version bumps, no goreleaser — version numbers are derived from commit history automatically.

### Two Release Trains

The monorepo produces two independently-versioned package groups:

| Train | Contents | Versioning | Rationale |
|-------|----------|------------|-----------|
| `trail-boss` | trail-boss binary, migrations, systemd unit | Independent semver | Control plane ships on its own cadence; unblocked by DPU work |
| `trail-dpu` | All 8 DPU binaries (trail-hand + sub-agents), systemd units, TOML configs | Single package semver | DPU agents deploy together on the same BlueField-3; mixed versions on one DPU are not supported |

This avoids the single-version-blocks-everything problem: a control-plane bugfix ships without waiting for DPU agent stabilization, and vice versa.

### Protocol Version Compatibility

The `agent.proto` AgentHello message carries a `protocol_version` field:

```protobuf
message AgentHello {
  string agent_id = 1;
  string hostname = 2;
  uint32 protocol_version = 3;  // Incremented on breaking wire changes
  string package_version = 4;   // e.g. "1.4.2"
}
```

**Compatibility guarantee:** trail-boss at version N supports trail-dpu at versions N-1 through N+1. This gives a one-version skew window for rolling upgrades.

trail-hand validates on connect:

```go
func (h *HandServer) validateHello(hello *pb.AgentHello) error {
    if hello.ProtocolVersion < minSupportedProtocol ||
       hello.ProtocolVersion > maxSupportedProtocol {
        return connect.NewError(connect.CodeFailedPrecondition,
            fmt.Errorf("protocol_version %d not in supported range [%d, %d]",
                hello.ProtocolVersion, minSupportedProtocol, maxSupportedProtocol))
    }
    return nil
}
```

### CI Pipeline

```
main merge → semantic-release → tag → build + package → publish
```

1. **On main merge:** semantic-release analyzes commits scoped to each package, bumps the appropriate version, creates a Git tag (`trail-boss/v1.2.3` or `trail-dpu/v0.8.1`).
2. **On tag:** CI builds cross-compiled binaries, runs nfpm, publishes to internal package registry.
3. **Separate release configs** ensure boss and dpu packages release independently based on commit scopes.

Conventional Commits enforced via commitlint in CI — PRs with malformed commit messages fail before merge:

```yaml
# .commitlintrc.yaml
extends:
  - "@commitlint/config-conventional"
rules:
  scope-enum:
    - 2
    - always
    - [boss, dpu, hand, corral, gate, pen, path, brand, trough, latch, proto, ci, docs]
  scope-empty:
    - 2
    - never
```

### semantic-release Configuration

```yaml
# release/trail-boss.releaserc.yaml
tagFormat: "trail-boss/v${version}"
plugins:
  - - "@semantic-release/commit-analyzer"
    - preset: conventionalcommits
      releaseRules:
        - { scope: "boss", release: "patch" }
        - { scope: "boss", type: "feat", release: "minor" }
        - { scope: "boss", type: "fix", release: "patch" }
        - { scope: "proto", release: "minor" }
      parserOpts:
        noteKeywords: ["BREAKING CHANGE", "BREAKING-CHANGE"]
  - - "@semantic-release/release-notes-generator"
    - preset: conventionalcommits
  - - "@semantic-release/exec"
    - prepareCmd: |
        make package-boss VERSION=${nextRelease.version}
      publishCmd: |
        deploy/publish-package.sh trail-boss ${nextRelease.version} amd64
  - - "@semantic-release/git"
    - assets: ["CHANGELOG-boss.md"]
      message: "chore(boss): release ${nextRelease.version} [skip ci]"
  - - "@semantic-release/github"
    - assets:
        - path: dist/trail-boss-${nextRelease.version}.amd64.deb
        - path: dist/trail-boss-${nextRelease.version}.amd64.rpm
---
# release/trail-dpu.releaserc.yaml
tagFormat: "trail-dpu/v${version}"
plugins:
  - - "@semantic-release/commit-analyzer"
    - preset: conventionalcommits
      releaseRules:
        - { scope: "dpu", release: "patch" }
        - { scope: "hand", release: "patch" }
        - { scope: "corral", release: "patch" }
        - { scope: "gate", release: "patch" }
        - { scope: "pen", release: "patch" }
        - { scope: "path", release: "patch" }
        - { scope: "brand", release: "patch" }
        - { scope: "trough", release: "patch" }
        - { scope: "latch", release: "patch" }
        - { type: "feat", release: "minor" }
        - { scope: "proto", release: "minor" }
      parserOpts:
        noteKeywords: ["BREAKING CHANGE", "BREAKING-CHANGE"]
  - - "@semantic-release/release-notes-generator"
    - preset: conventionalcommits
  - - "@semantic-release/exec"
    - prepareCmd: |
        make package-dpu VERSION=${nextRelease.version}
      publishCmd: |
        deploy/publish-package.sh trail-dpu ${nextRelease.version} arm64
  - - "@semantic-release/git"
    - assets: ["CHANGELOG-dpu.md"]
      message: "chore(dpu): release ${nextRelease.version} [skip ci]"
  - - "@semantic-release/github"
    - assets:
        - path: dist/trail-dpu-${nextRelease.version}.arm64.deb
        - path: dist/trail-dpu-${nextRelease.version}.arm64.rpm
```

### Package Artifacts

| Artifact | Arch | Repository |
|----------|------|------------|
| `trail-boss-{version}.amd64.deb` | amd64 | Internal apt repo |
| `trail-boss-{version}.amd64.rpm` | amd64 | Internal yum repo |
| `trail-dpu-{version}.arm64.deb` | arm64 | Internal apt repo |
| `trail-dpu-{version}.arm64.rpm` | arm64 | Internal yum repo |

The `trail-dpu` package contains all 8 binaries, their systemd units, default TOML configs under `/etc/trail/`, and a shared `trail-dpu.conf` tmpfiles.d entry.

### Deployment Tiers

| Criterion | Lab | Staging | Production |
|-----------|-----|---------|------------|
| DPU count | < 10 | 10 - 50 | 50+ |
| Tooling | `deploy/deploy-to-lab.sh` | Ansible | NICo lifecycle integration |
| Parallelism | All at once | Batches of 5 | Canary 1% → 5% → 25% → 100% |
| Health gates | None (manual verification) | Ansible health-check task between batches | Automated: metrics + error-rate threshold |
| Rollback trigger | Manual | Ansible `--limit failed` | Automatic on SLO breach |
| Time to full rollout | ~30 seconds | ~10 minutes | ~2 hours |

**Lab (<10 DPUs):** `deploy/deploy-to-lab.sh` — rsync binaries, SSH to restart systemd units:

```bash
#!/usr/bin/env bash
set -euo pipefail
HOSTS="${1:-lab-dpu-01 lab-dpu-02 lab-dpu-03}"
VERSION="${2:-$(git describe --tags --match 'trail-dpu/*' --abbrev=0 | sed 's|trail-dpu/v||')}"

for host in $HOSTS; do
  echo ">>> Deploying trail-dpu ${VERSION} to ${host}"
  rsync -az dist/trail-dpu-${VERSION}.arm64.deb "${host}:/tmp/"
  ssh "$host" "sudo dpkg -i /tmp/trail-dpu-${VERSION}.arm64.deb && sudo systemctl restart trail-hand"
done
```

**Staging (10-50 DPUs):** Ansible playbook with rolling updates and health verification:

```yaml
# deploy/ansible/deploy-dpu.yml
- hosts: staging_dpus
  serial: 5
  tasks:
    - name: Install trail-dpu package
      apt:
        name: "trail-dpu={{ trail_dpu_version }}"
        state: present
        force: yes

    - name: Restart trail-hand (cascades to sub-agents)
      systemd:
        name: trail-hand
        state: restarted

    - name: Wait for health endpoint
      uri:
        url: "http://{{ inventory_hostname }}:9100/healthz"
        status_code: 200
      retries: 10
      delay: 3

    - name: Verify sub-agent registration
      command: trail-hand status --format=json
      register: agent_status
      failed_when: (agent_status.stdout | from_json).registered_agents < expected_agents
```

**Production (50+ DPUs):** NICo lifecycle integration with canary progression:

```
Canary stages: 1% (15 min observe) → 5% (15 min) → 25% (30 min) → 100%
Abort criteria: error_rate > 0.1% OR p99_latency > 50ms OR agent_reconnect_rate > 5/min
```

### Rollback

Rollback at all tiers uses package manager downgrade:

```bash
# Downgrade to previous version
sudo apt install trail-dpu=1.3.7

# trail-hand performs graceful handoff:
# 1. Receives SIGTERM from systemd
# 2. Sends Draining message to sub-agents via Unix socket
# 3. Sub-agents complete in-flight tc-flower operations (max 5s)
# 4. Sub-agents close Unix socket connections
# 5. trail-hand exits; systemd restarts with new (old) binary
# 6. Sub-agents reconnect and re-register via AgentHello
```

The protocol_version compatibility window (N-1 through N+1) ensures that during a rollback, trail-boss still accepts connections from trail-dpu at the previous version.

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

Four phases, each time-boxed to 2-3 weeks. Checkpoints at step 5, 10, 15, and 20 gate forward progress: CI must be green, the monorepo must produce deployable artifacts, and remaining steps are re-baselined against actuals.

---

### Phase 1 — Repository Bootstrap and History Import (weeks 1-3)

**Goal:** New monorepo with full git history from both source repos, CI skeleton, and Go workspace compiling.

**Step 1. Create the monorepo and import trail-boss (network-manager) history.**

```bash
git clone git@github.com:mara-baysn/technicolor-haze-network-manager.git /tmp/nm-export
cd /tmp/nm-export

git filter-repo \
  --path-rename 'cmd/network-manager/:cmd/trail-boss/' \
  --path-rename 'internal/:internal/boss/' \
  --tag-rename '':'boss-'

cd /path/to/technicolor-haze-trail
git remote add nm-import /tmp/nm-export
git fetch nm-import --tags
git merge nm-import/main --allow-unrelated-histories \
  -m "import: trail-boss history via git-filter-repo"
git remote remove nm-import
```

**Step 2. Import trail-gate (firewall) history into the monorepo.**

```bash
git clone git@github.com:mara-baysn/technicolor-haze-firewall.git /tmp/fw-export
cd /tmp/fw-export

git filter-repo \
  --path-rename 'cmd/firewall/:cmd/trail-gate/' \
  --path-rename 'internal/tcflower/:internal/netlink/' \
  --path-rename 'internal/reconciler/:internal/subagent/reconciler/' \
  --path-rename 'internal/':internal/ \
  --tag-rename '':'gate-'

cd /path/to/technicolor-haze-trail
git remote add fw-import /tmp/fw-export
git fetch fw-import --tags
git merge fw-import/main --allow-unrelated-histories \
  -m "import: trail-gate history via git-filter-repo"
git remote remove fw-import
```

**Step 3. Establish unified go.mod and resolve dependency conflicts.**

Create single `go.mod` at repo root. Pin to Go 1.25. Resolve version conflicts between the two imports (keep higher semver). Add `third_party/technicolor-haze-proto` as git submodule.

**Step 4. Scaffold canonical directory layout.**

Create the target directory structure (see Target Structure section). Move imported code into canonical locations. Update all import paths.

**Step 5. CI skeleton: cross-compile check for both architectures.**

Configure CI to: `go build ./...` for both amd64 and arm64, `go vet ./...`, `go test ./...` (unit tests only).

---

> **CHECKPOINT 1 (end of step 5):** CI is green. Both architectures cross-compile. `git log --follow` traces files back to original repos. No functional changes yet. Re-estimate remaining phases.

---

### Phase 2 — Shared Infrastructure and Internal Restructuring (weeks 4-6)

**Goal:** All shared packages extracted, sub-agent framework created, all binaries compile.

**Step 6. Extract shared packages into canonical locations.**

Deduplicate: `internal/domain/`, `internal/obs/`, `internal/config/`, `internal/netlink/`, `internal/transport/`. Update import paths across all cmd/ binaries.

**Step 7a. Move network-manager server code (mechanical file move only).**

Pure file move from `internal/boss/server/` to final location. No API changes. All existing tests must pass.

**Step 7b. Refactor server to ConnectRPC service interface.**

Define ConnectRPC service in proto. Generate stubs. Implement service. Keep old API as thin wrapper — no callers break yet.

**Step 7c. Migrate upstream callers to ConnectRPC client.**

Replace direct function calls with ConnectRPC client calls. Remove compatibility wrapper. All interactions go through protobuf contract.

**Step 8. Implement `internal/boss/bus/` (NATS embedded).**

Wire embedded NATS, JetStream KV buckets, subject routing. Integration test: 3-boss cluster forms, session CAS works.

**Step 9. Create `internal/ipamclient/` (interface + chaos stub).**

**Step 10. Create `internal/subagent/` framework (IPC, reconciler) + `internal/hand/`.**

---

> **CHECKPOINT 2 (end of step 10):** All 4 Phase 1 binaries compile and pass unit tests. Sub-agent IPC framework works in integration test. nfpm packages produced for both architectures. Re-baseline phase 3 estimates.

---

### Phase 3 — Integration, Testing, and Hardware Validation (weeks 7-9)

**Goal:** End-to-end flow working on real BF3 hardware. Integration and E2E tests passing.

**Step 11. Wire trail-hand ↔ trail-boss bidi session end-to-end.**

**Step 12. Wire trail-hand ↔ trail-gate sub-agent IPC end-to-end.**

**Step 13. Wire trail-hand ↔ trail-corral interface lifecycle.**

**Step 14. Create `test/e2e/` (full lifecycle with mock netlink + multi-process E2E).**

**Step 15. Hardware smoke test on lab BF3 DPU.**

Deploy arm64 package to lab DPU. Verify: trail-hand starts, registers with trail-boss, trail-gate programs a tc-flower rule, trail-corral creates a VF.

---

> **CHECKPOINT 3 (end of step 15):** Integration tests pass. Lab DPU runs monorepo-built agents. System is ready for shadow deployment. Re-baseline phase 4.

---

### Phase 4 — Canary Cutover and Decommission (weeks 10-12)

**Goal:** Production traffic validated on monorepo builds. Old repos go read-only.

**Step 16. Greenfield validation deployment (1+ week minimum).**

Deploy trail-boss (K8s or systemd) with synthetic workloads. No existing control plane to shadow against — this is greenfield. Validate: trail-boss correctly compiles intent from upstream controllers (Tenant/VPC, Gateway) into per-DPU desired state. Run synthetic tenant lifecycle scenarios (create VPC → add subnet → attach VF → apply SG → teardown) for 72+ hours under load. Assert: zero state corruption, convergence within SLO, no session ownership flapping.

**Step 17. Canary rack: 5-10 DPUs on trail (1 week minimum).**

Cut canary DPUs over to monorepo-built agents. Monitor convergence time, packet drops, tc-flower errors. Rollback criteria: any p99 regression >10% or data-plane packet loss.

**Step 18. Expand canary to 25% of fleet (3-5 days).**

**Step 19. Full fleet cutover (rolling, one rack at a time).**

**Step 20. Set old repos to read-only (NOT archived).**

Update README in old repos. Keep CI running so existing tags remain reproducible.

---

> **CHECKPOINT 4 (end of step 20):** Production fleet runs on monorepo-built binaries. Old repos are read-only.

---

### Post-Cutover Retention (steps 21-25)

**Step 21.** Dual-build verification (nightly CI in old repos, first 2 weeks).
**Step 22.** Freeze old-repo dependency updates.
**Step 23.** Backport window closes (day 30).
**Step 24.** Remove old-repo CI runners (day 45).
**Step 25.** Final retention review (day 60). Confirm zero rollback events. Old repos remain read-only. Document migration complete.

### Summary Timeline

| Phase | Steps | Duration | Key Gate |
|-------|-------|----------|----------|
| 1 — Bootstrap + History Import | 1-5 | 2-3 weeks | CI green, both arches compile, history preserved |
| 2 — Shared Infrastructure | 6-10 | 2-3 weeks | All binaries build, packages produced |
| 3 — Integration + Hardware | 11-15 | 2-3 weeks | Integration tests pass, lab DPU validated |
| 4 — Canary Cutover | 16-25 | 2-3 weeks active + 60-day retention tail | Fleet migrated, old repos read-only |

**Total active migration work: 8-12 weeks.** The 60-day retention tail runs passively.

---

## What to Implement in Phase 1

Phase 1 delivers a working end-to-end slice: control plane to DPU, with one fully functional sub-agent proving the architecture. No stubs, no placeholders that rot.

| Binary | Phase 1 Scope | Notes |
|--------|--------------|-------|
| `trail-boss` | Full control plane: ConnectRPC API, PostgreSQL state, embedded NATS pub/sub to DPU fleet, desired-state compilation, convergence tracking | amd64, Tier 2 |
| `trail-hand` | Master agent on DPU: manages sub-agent lifecycle, Unix socket IPC multiplexer, health aggregation, NATS uplink to trail-boss, config distribution | arm64, BF3 |
| `trail-corral` | VF/SF pool manager: allocates PCIe VFs, tracks NUMA affinity, binds VFs to tenants, reports capacity to trail-boss | arm64, sub-agent of trail-hand |
| `trail-gate` | **Reference implementation sub-agent.** Firewall: programs per-VF L3/L4 ACL + connection tracking rules into eSwitch via tc-flower/netlink. Implements the full sub-agent contract. | arm64, sub-agent of trail-hand |
| `trail-pen` | Not implemented. Future: per-VM stateful ACLs (security group rules) | Create from trail-gate template when staffed |
| `trail-path` | Not implemented. Future: VPC overlay (VXLAN/Geneve), route programming, VNI management | Create from trail-gate template when staffed |
| `trail-brand` | Not implemented. Future: DHCP server responses on VF representors | Create from trail-gate template when staffed |
| `trail-trough` | Not implemented. Future: L4 DNAT/ECMP rule programming on eSwitch | Create from trail-gate template when staffed |
| `trail-latch` | Not implemented. Future: WireGuard tunnel data-plane, ZTNA auth | Create from trail-gate template when staffed |

**Phase 1 exit criteria:** trail-boss pushes a firewall policy change over NATS, trail-hand receives it, dispatches to trail-gate over Unix socket IPC, trail-gate programs the eSwitch via tc-flower, reports actual state back up the chain, trail-boss confirms convergence. Tested on real BF3 hardware with a VF carrying tenant traffic.

**What we deliberately do NOT ship:** stub `cmd/` entries for unimplemented sub-agents. Stubs rot, give false confidence in CI ("all binaries build!"), and accumulate drift from the real contract. Instead, new sub-agents are created from scratch using trail-gate as the living template when an engineer is staffed to implement them.

---

### Sub-Agent Contract

Every sub-agent that registers with trail-hand must implement two interfaces: the IPC transport interface (how trail-hand communicates with it) and the reconciler interface (how it converges actual state toward desired state). trail-gate is the reference implementation of both.

#### IPC Interface (ConnectRPC over Unix Socket)

Each sub-agent exposes a ConnectRPC service on a Unix domain socket at a well-known path (`/run/trail/{agent-name}.sock`). trail-hand connects as a client.

```protobuf
// trail/agent/v1/subagent.proto

service SubAgent {
  // Called once after socket connection. Sub-agent declares its identity,
  // capabilities, and which pipeline stages it owns.
  rpc Connect(ConnectRequest) returns (ConnectResponse);

  // trail-hand pushes desired state for the pipeline stages this sub-agent owns.
  rpc HandleDesiredState(DesiredStateRequest) returns (DesiredStateResponse);

  // trail-hand polls or sub-agent streams actual state back.
  rpc ReportActualState(ActualStateRequest) returns (ActualStateResponse);

  // Liveness + readiness. trail-hand calls on interval; failure triggers restart.
  rpc HealthCheck(HealthCheckRequest) returns (HealthCheckResponse);
}

message ConnectRequest {
  string hand_version = 1;
  uint64 boot_epoch = 2;
}

message ConnectResponse {
  string agent_name = 1;           // e.g. "trail-gate"
  string agent_version = 2;
  repeated string capabilities = 3; // e.g. ["acl", "conntrack", "stateful-sg"]
  repeated PipelineStage stages = 4; // stages this agent owns
}

message PipelineStage {
  uint32 stage_number = 1;         // matches DPU Orchestrator stage model
  string stage_name = 2;           // e.g. "nacl"
}

message DesiredStateRequest {
  uint64 generation = 1;
  uint32 stage_number = 2;
  bytes config_payload = 3;        // stage-specific protobuf, opaque to trail-hand
  string vni = 4;                  // scope: which tenant/VNI this applies to
}

message DesiredStateResponse {
  bool accepted = 1;
  string error_message = 2;
}

message ActualStateRequest {
  uint32 stage_number = 1;
  string vni = 2;                  // empty = all VNIs for this stage
}

message ActualStateResponse {
  uint64 generation = 1;           // last successfully applied generation
  ConvergenceState state = 2;
  bytes actual_payload = 3;
  repeated string divergence_reasons = 4;
}

enum ConvergenceState {
  CONVERGENCE_STATE_UNSPECIFIED = 0;
  CONVERGED = 1;
  PROGRAMMING = 2;
  DIVERGED = 3;
  ERROR = 4;
}

message HealthCheckRequest {}

message HealthCheckResponse {
  bool healthy = 1;
  bool ready = 2;
  string status_message = 3;
  map<string, string> diagnostics = 4;
}
```

#### Reconciler Interface (Go, internal to each sub-agent)

Each sub-agent implements this Go interface internally — the core loop that converges hardware state toward desired configuration.

```go
// internal/subagent/reconciler/reconciler.go

type Rule interface {
    Key() string
    Equal(other Rule) bool
}

type Reconciler interface {
    Desired() []Rule
    Actual() ([]Rule, error)
    Apply(delta Delta) error
    Rollback() error
}

type Delta struct {
    Add    []Rule
    Remove []Rule
    Modify []RuleChange
}

type RuleChange struct {
    Old Rule
    New Rule
}
```

#### Registration Protocol

When trail-hand starts (or restarts), it discovers sub-agents by scanning `/run/trail/*.sock`. For each socket found:

1. **Connect** — trail-hand calls `SubAgent.Connect()`. The sub-agent responds with its name, version, capabilities, and owned pipeline stages.
2. **Validate** — trail-hand checks that no two sub-agents claim the same pipeline stage. Conflicts are fatal.
3. **Hydrate** — trail-hand immediately calls `HandleDesiredState` with the last-known desired state for each stage the sub-agent owns.
4. **Health loop** — trail-hand calls `HealthCheck` every 5s. Three consecutive failures trigger sub-agent restart via systemd.
5. **Steady state** — On each desired-state update from trail-boss (NATS), trail-hand routes it to the owning sub-agent via `HandleDesiredState`.

**Startup ordering:** trail-hand starts first (systemd `Before=` dependency). Sub-agents start after and create their socket. trail-hand uses inotify on `/run/trail/` to detect new sockets without polling.

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

---

## Agentic Execution Readiness

Checklist for making this plan executable by coding agents without human clarification.

### Architectural Decisions (resolved)

- [x] DECISION: Deployment model — trail-boss produces 2 artifacts: bare-metal systemd (.deb/.rpm) + Dockerfile for K8s. Both use same binary.
- [x] DECISION: Wire protocol — gRPC bidi stream (trail-hand dials out). Boss PUSHES config over established stream. Agent pushes acks/status back. NATS is inter-boss coordination only.
- [x] DECISION: DPU agent packaging — systemd + .deb/.rpm on DPU. No containers on DPU.
- [x] DECISION: Compilation engine — trail-boss compiles intent per available service on each DPU (scoped by registered sub-agent capabilities).
- [x] DECISION: Component identity — trail-boss IS the DPU Orchestrator (same system, new implementation).
- [x] DECISION: Sub-agent management — trail-hand manages sub-agents via systemd on DPU. Confirmed.
- [x] DECISION: Shadow mode — removed. Greenfield deployment (no existing control plane). Replaced with synthetic validation workload.
- [x] DECISION: NICo integration — NICo handles DPU firmware/boot/attestation. Trail agents are installed after NICo confirms DPU is ready. NICo does NOT manage trail agent lifecycle (systemd does).

### Implementation Gaps (for loop agents to resolve)

- [ ] Phase 1 Step 2: filter-repo flags are non-overlapping with expected output tree verified
- [ ] Phase 1 Step 3: go.mod merge strategy concrete (both source go.mods listed, conflict rules explicit)
- [ ] Phase 1 Step 4: complete old-to-new import path mapping table and rewrite script provided
- [ ] Phase 2 Step 6: for each duplicate package, winner specified with merge instructions and acceptance criteria
- [ ] Phase 2 Step 7b: trail-boss proto service definition complete (all RPCs listed with request/response types)
- [ ] Phase 3 Steps 11-13: each wiring step has proto messages, Go interfaces, integration test spec, and mock boundaries
- [ ] Phase 3 Step 14: E2E broken into prioritized sub-steps with must-have vs nice-to-have
- [ ] proto/trail/v1/agent.proto: complete message definitions for bidi session (connect, heartbeat, desired-state poll, actual-state report, recovery)
