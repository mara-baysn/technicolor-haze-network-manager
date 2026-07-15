# Monorepo Plan: `technicolor-haze-network`

## Decision

Full monorepo. One repo for ALL network components (control plane + DPU agents).
Rename `technicolor-haze-network-manager` → `technicolor-haze-network`.

Pattern: Cilium/OVN model — northd + controller in one repo, separate binaries.

---

## Target Structure

```
technicolor-haze-network/
├── cmd/
│   ├── network-manager/           ← Control plane binary (amd64)
│   │   └── main.go
│   ├── dpu-agent/                 ← DPU master agent (arm64)
│   │   └── main.go
│   ├── haze-firewall/             ← Sub-agent: firewall + NAT (arm64)
│   │   └── main.go
│   ├── haze-secgroup/             ← Sub-agent: security groups + ACL (arm64)
│   │   └── main.go
│   ├── haze-overlay/              ← Sub-agent: VPC overlay routing (arm64)
│   │   └── main.go
│   ├── haze-dhcp/                 ← Sub-agent: DHCP responses (arm64)
│   │   └── main.go
│   ├── haze-loadbalancer/         ← Sub-agent: L4/L7 load balancer (arm64)
│   │   └── main.go
│   └── haze-secureaccess/         ← Sub-agent: VPN/ZTNA (arm64)
│       └── main.go
│
├── internal/
│   ├── domain/                    ← NM domain logic (control plane)
│   │   ├── vpc/
│   │   ├── firewall/
│   │   ├── secgroup/
│   │   ├── acl/
│   │   ├── eip/
│   │   ├── lb/
│   │   ├── dns/
│   │   ├── dhcp/
│   │   └── vpn/
│   ├── store/                     ← NM storage backend (memory → postgres)
│   ├── ipam/                      ← IP address management
│   ├── server/                    ← NM gRPC handlers
│   │
│   ├── agent/                     ← DPU master agent logic
│   │   ├── capabilities.go       ← capability discovery + sub-agent spawn
│   │   ├── lifecycle.go          ← systemd management of sub-agents
│   │   └── registry.go           ← running sub-agent registry
│   │
│   ├── subagent/                  ← Shared sub-agent framework
│   │   ├── ipc/                  ← Unix socket + protobuf IPC
│   │   │   ├── server.go        ← sub-agent side (listen on socket)
│   │   │   ├── client.go        ← master agent side (connect to socket)
│   │   │   └── protocol.go      ← wire format (4-byte len + proto)
│   │   ├── reconciler/          ← Pull → diff → apply loop (shared)
│   │   └── health/              ← /healthz /readyz /metrics (shared)
│   │
│   ├── netlink/                   ← Shared netlink/tc-flower primitives
│   │   ├── flower.go            ← tc-flower rule CRUD
│   │   ├── police.go            ← tc police rate limiting
│   │   ├── pedit.go             ← pedit NAT actions
│   │   └── types.go             ← FilterSpec, Action, etc.
│   │
│   └── nmclient/                  ← Shared NM gRPC client (used by all agents)
│       └── client.go
│
├── proto/                          ← Embedded proto definitions
│   ├── agent/v1/agent.proto       ← Master agent ↔ NM
│   ├── ipc/v1/ipc.proto          ← Master agent ↔ sub-agents (local)
│   ├── vpc/v1/vpc.proto
│   ├── firewall/v1/firewall.proto
│   ├── securitygroup/v1/security_group.proto
│   ├── networkacl/v1/network_acl.proto
│   ├── elasticip/v1/elastic_ip.proto
│   ├── loadbalancer/v1/load_balancer.proto
│   ├── dns/v1/dns.proto
│   ├── dhcp/v1/dhcp.proto
│   └── secureaccess/v1/secure_access.proto
│
├── gen/                            ← Generated Go code from proto
│   └── (buf generate output)
│
├── deploy/
│   ├── docker/
│   │   ├── Dockerfile.network-manager    ← amd64
│   │   ├── Dockerfile.dpu-agent          ← arm64 (includes all sub-agents)
│   │   └── Dockerfile.dpu-bundle         ← single fat image with all DPU binaries
│   └── systemd/
│       ├── haze-agent.service
│       ├── haze-firewall.service
│       ├── haze-secgroup.service
│       ├── haze-overlay.service
│       └── haze-dhcp.service
│
├── docs/
│   ├── DESIGN.md                  ← Architecture design (739 lines, exists)
│   ├── DEPLOYMENT.md              ← Where to deploy what
│   └── API.md                     ← Proto API reference
│
├── scripts/
│   ├── setup-dpu.sh              ← DPU initialization
│   └── buf-generate.sh           ← Proto codegen
│
├── go.mod
├── go.sum
├── Makefile
├── buf.yaml                       ← Buf workspace for proto/
├── buf.gen.yaml                   ← Codegen config
├── .github/
│   └── workflows/
│       ├── ci.yml                 ← Test all packages
│       ├── release.yml            ← Semantic release + publish
│       └── docker.yml             ← Build images (amd64 NM + arm64 DPU bundle)
├── .releaserc
├── .gitignore
├── README.md
└── Makefile
```

---

## What Comes From Where

| Source | Destination | Action |
|--------|-------------|--------|
| `technicolor-haze-network-manager/internal/store/` | `internal/store/` | Move |
| `technicolor-haze-network-manager/internal/ipam/` | `internal/ipam/` | Move |
| `technicolor-haze-network-manager/internal/server/` | `internal/server/` | Move |
| `technicolor-haze-network-manager/internal/tenant/` | `internal/domain/vpc/` | Refactor |
| `technicolor-haze-network-manager/cmd/network-manager/` | `cmd/network-manager/` | Move |
| `technicolor-haze-network-manager/proto/` | `proto/` | Move |
| `technicolor-haze-firewall/internal/tcflower/` | `internal/netlink/` | Move + rename |
| `technicolor-haze-firewall/internal/nat/` | `internal/domain/firewall/nat/` or sub-agent | Move |
| `technicolor-haze-firewall/internal/ratelimit/` | `internal/netlink/police.go` | Merge |
| `technicolor-haze-firewall/internal/reconciler/` | `internal/subagent/reconciler/` | Generalize |
| `technicolor-haze-firewall/internal/manager/client.go` | `internal/nmclient/` | Generalize |
| `technicolor-haze-firewall/internal/health/` | `internal/subagent/health/` | Generalize |
| `technicolor-haze-firewall/cmd/firewall/` | `cmd/haze-firewall/` | Move |
| `technicolor-haze-proto/definitions/api/firewall/` | `proto/firewall/` | Copy (or submodule) |

---

## Shared Code (why monorepo wins)

These packages are used by MULTIPLE binaries:

| Package | Used By |
|---------|---------|
| `internal/netlink/` | firewall, secgroup, overlay, dhcp, lb |
| `internal/subagent/ipc/` | ALL sub-agents + master agent |
| `internal/subagent/reconciler/` | ALL sub-agents |
| `internal/subagent/health/` | ALL sub-agents + master agent |
| `internal/nmclient/` | master agent + all sub-agents (pull from NM) |
| `gen/` (proto types) | ALL binaries |

In multi-repo, each of these would be a separate Go module with version coordination hell.
In monorepo: one `go.mod`, one version, one PR to change shared code.

---

## Build Matrix

```makefile
# Single Makefile, multiple targets

build-all: build-nm build-dpu

build-nm:
	GOOS=linux GOARCH=amd64 go build -o bin/network-manager ./cmd/network-manager/

build-dpu: build-agent build-firewall build-secgroup build-overlay build-dhcp build-lb build-vpn

build-agent:
	GOOS=linux GOARCH=arm64 go build -o bin/arm64/dpu-agent ./cmd/dpu-agent/

build-firewall:
	GOOS=linux GOARCH=arm64 go build -o bin/arm64/haze-firewall ./cmd/haze-firewall/

build-secgroup:
	GOOS=linux GOARCH=arm64 go build -o bin/arm64/haze-secgroup ./cmd/haze-secgroup/

# ... etc for each sub-agent
```

---

## Docker Strategy

```dockerfile
# Dockerfile.dpu-bundle — ONE image with all DPU binaries
FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY . .
RUN GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/dpu-agent ./cmd/dpu-agent/
RUN GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/haze-firewall ./cmd/haze-firewall/
RUN GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/haze-secgroup ./cmd/haze-secgroup/
RUN GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/haze-overlay ./cmd/haze-overlay/
RUN GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/haze-dhcp ./cmd/haze-dhcp/
# ... all sub-agents

FROM scratch
COPY --from=builder /out/ /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/dpu-agent"]

# Total image size: ~60MB (all 7 binaries × ~10MB each, stripped)
```

---

## CI Strategy

```yaml
# One CI pipeline tests everything
name: CI
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.22" }
      - run: go test ./... -race -coverprofile=coverage.out
      - run: go vet ./...

  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: golangci/golangci-lint-action@v6

  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.22" }
      - run: make build-all  # builds amd64 NM + arm64 all agents
```

---

## Proto Strategy

Two options:
1. **Embed protos in this repo** (`proto/` directory) — simpler, no submodule
2. **Submodule technicolor-haze-proto** — matches existing platform pattern

Recommendation: **Embed** for domain-specific protos (agent.proto, ipc.proto), **submodule** for shared platform protos (firewall.proto that other teams consume). Or just embed everything and publish to BSR from this repo's CI.

---

## Release Strategy

- Semantic release on main branch
- One version number for ALL binaries (they deploy together)
- Tag: `v1.2.3` → all binaries are v1.2.3
- Docker images: `haze-network-manager:v1.2.3` + `haze-dpu-bundle:v1.2.3`
- Published to arx.infra.voxel.fyi container registry

---

## Migration Steps

1. Rename repo: `technicolor-haze-network-manager` → `technicolor-haze-network`
2. Move existing NM code into new structure
3. Move firewall agent code from `technicolor-haze-firewall`
4. Extract shared netlink code into `internal/netlink/`
5. Create `internal/subagent/` framework (IPC, reconciler, health)
6. Create stub `cmd/` entries for other sub-agents
7. Create Makefile with build targets
8. Create Dockerfiles
9. Create CI/CD workflows
10. Update `technicolor-haze-firewall` README: "DEPRECATED — moved to technicolor-haze-network"
11. Archive `technicolor-haze-firewall` repo

---

## What to Implement in Phase 1

| Binary | Status | Phase 1 |
|--------|--------|---------|
| network-manager | Exists (v1) | Refactor into new structure |
| dpu-agent | New | Implement (capability + spawn + IPC) |
| haze-firewall | Exists | Move from technicolor-haze-firewall |
| haze-secgroup | New | Stub (just reconciler loop + placeholder) |
| haze-overlay | New | Stub |
| haze-dhcp | New | Stub |
| haze-loadbalancer | New | Stub |
| haze-secureaccess | New | Stub |

Phase 1 delivers: NM + master agent + firewall sub-agent working end-to-end.
Other sub-agents are stubs that can be implemented incrementally.
