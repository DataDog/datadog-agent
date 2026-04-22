# CNM on OTel Collector -- Phase 1 Implementation Plan (OTLP Metrics)

## Context

Implement eBPF-based network monitoring (CNM) as a proper OTel pipeline in the host-profiler binary. A **CNM receiver** embeds the network tracer and produces `pmetric.Metrics`. A **CNM Datadog exporter** consumes those metrics and reconstructs the native `CollectorConnections` protobuf for submission to the Datadog backend. Standard OTel processors (infraattributes, filter, batch) work in between.

```
CNM Receiver --> [infraattributes processor] --> CNM Datadog Exporter --> process.<DD_SITE>
  (eBPF -> pmetric.Metrics)                      (pmetric.Metrics -> CollectorConnections protobuf)
```

For the full design exploration including alternative options (new OTel signal type) and
the current system-probe architecture reference, see `docs/dev/cnm-otel-design.md`.

---

## Development Environment (macOS -> Linux)

eBPF is Linux-only. This repo provides two official mechanisms for macOS developers.

### For writing code, building, and running unit tests: `dda env dev`

```bash
dda env dev start                                    # Start Linux container (bind mount)
dda env dev run -- dda inv install-tools             # One-time tool install
dda env dev run -- dda inv host-profiler.build       # Build host-profiler
dda env dev run -- dda inv test --targets=./comp/host-profiler/collector/impl/receiver/cnm/...
dda env dev run -- dda inv linter.go
```

### For eBPF integration tests on real kernels: KMT

```bash
dda inv -e kmt.init --images=ubuntu_22.04
dda inv -e kmt.gen-config --vms=ubuntu_22.04-local-distro
dda inv -e kmt.launch-stack
dda inv -e kmt.build --vms=ubuntu_22.04-local-distro --component=host-profiler
dda inv -e kmt.test --vms=ubuntu_22.04-local-distro --component=host-profiler \
    --packages ./comp/host-profiler/collector/impl/receiver/cnm
```

### What runs where

| Task | Environment | Command prefix |
|------|-------------|----------------|
| Write/edit code | macOS (local editor) | -- |
| Build host-profiler binary | `dda env dev` container | `dda env dev run --` |
| Run unit tests (mock tracer) | `dda env dev` container | `dda env dev run --` |
| Run linter | `dda env dev` container | `dda env dev run --` |
| eBPF integration tests | KMT local VM | `dda inv -e kmt.test` |
| E2E tests with fakeintake | KMT VM or CI | `dda inv -e kmt.test` |
| Manual testing (run host-profiler) | KMT VM via SSH | SSH into VM |

---

## Step 1: CNM Receiver Package

**New directory:** `comp/host-profiler/collector/impl/receiver/cnm/`

### `factory.go`

Pattern: `comp/host-profiler/collector/impl/receiver/factory.go`

Two constructors:
- `NewFactory()` -- standalone mode (no Agent Core)
- `NewFactoryForAgent(tagger, hostname, config, log)` -- Agent Core mode

Uses standard `receiver.NewFactory` (not `xreceiver`) since we produce stable Metrics signal.

### `config.go`

Config struct mapped from OTel YAML:
- `Enabled`, `CollectTCPv4/v6`, `CollectUDPv4/v6`, `DNSInspection`
- `MaxTrackedConnections` (default 65536), `MaxConnsPerMessage` (default 1000)
- `CheckInterval` (default 30s), `ProtocolClassification`, `ServiceMonitoring`

Key method: `toNetworkConfig()` constructs `*networkconfig.Config` directly, **bypassing** `networkconfig.New()` which reads global `pkgconfigsetup.SystemProbe()`.

### `receiver.go`

Embeds `*tracer.Tracer` (or mock via `ConnectionsSource` interface). On `Start()`:
1. Check kernel support via `tracer.IsTracerSupportedByOS()`
2. Build network config via `toNetworkConfig()`
3. Create tracer with no-op telemetry + statsd
4. Register client, start ticker-based collection goroutine

Collection loop calls `GetActiveConnections()`, converts to `pmetric.Metrics`, pushes via `consumer.ConsumeMetrics()`.

### `metrics.go`

Converts `*network.Connections` -> `pmetric.Metrics`:

**Metrics emitted per connection:**

| Metric | Type | Unit |
|---|---|---|
| `network.bytes.sent` | Sum (cumulative, monotonic) | bytes |
| `network.bytes.received` | Sum (cumulative, monotonic) | bytes |
| `network.packets.sent` | Sum (cumulative, monotonic) | packets |
| `network.packets.received` | Sum (cumulative, monotonic) | packets |
| `network.tcp.retransmits` | Sum (cumulative, monotonic) | count |
| `network.tcp.established` | Sum (cumulative, monotonic) | count |
| `network.tcp.closed` | Sum (cumulative, monotonic) | count |
| `network.tcp.rtt` | Gauge | us |
| `network.tcp.rtt_var` | Gauge | us |

**Attributes per data point:** `network.source.address`, `network.source.port`, `network.destination.address`, `network.destination.port`, `network.transport`, `network.type`, `network.direction`, `network.pid`, `network.netns`, `container.id.source`, `container.id.dest`, `network.intra_host`, `network.is_closed`, `network.protocol.api`, `network.protocol.application`, `network.protocol.encryption`, `network.cookie`, `network.last_update_epoch`, `network.duration_ns`

NAT attributes when present: `network.nat.source.address`, `network.nat.destination.address`

---

## Step 2: CNM Datadog Exporter

**New directory:** `comp/otelcol/otlp/components/exporter/cnmexporter/`

### `factory.go`

Pattern: `comp/otelcol/otlp/components/exporter/serializerexporter/factory.go`

Two constructors mirroring the receiver. Agent Core mode gets `connectionsforwarder.Component`.

### `exporter.go`

Consumes `pmetric.Metrics`, reconstructs `CollectorConnections` protobuf:
1. Groups metrics by connection tuple attributes
2. Reconstructs `ConnectionStats` from metric values + attributes
3. Batches, encodes (DNS, tags, routes), compresses with zstd
4. Submits via `connectionsforwarder`

### `encoder.go`

Encoding logic adapted from `pkg/network/sender/sender_linux.go` and `pkg/network/encoding/marshal/`.

---

## Step 3: Register Factories in Host-Profiler

**Modify:** `comp/host-profiler/collector/impl/otel_col_factories.go`

- Add `GetExporters() []exporter.Factory` to `ExtraFactories` interface
- Add `connectionsForwarder` to `extraFactoriesWithAgentCore` struct
- Return CNM receiver from `GetReceivers()`, CNM exporter from `GetExporters()`
- Append extra exporters in `createFactories()`

---

## Step 4: Wire Fx Dependencies

**Modify:** `cmd/host-profiler/subcommands/run/command.go`

Add `connectionsforwarderfx.Module()` and `secretsfx.Module()` in Agent Core mode.

---

## Step 5: AgentProvider Config Auto-Generation

**Modify:** `comp/host-profiler/collector/impl/agentprovider/config_builder.go`

Add `buildCNMReceiver()`, `buildCNMExporter()`, and a `metrics/cnm` pipeline when `network_config.enabled=true`.

---

## Step 6: Handle `networkconfig.New()` Dependency

`toNetworkConfig()` in `cnm/config.go` constructs the config struct directly, replicating defaults from `pkg/network/config/config.go` lines 226-349.

---

## Step 7: Telemetry and Statsd Shims

- `telemetryComponent.Component` -- no-op for MVP
- `statsd.ClientInterface` -- `&ddgostatsd.NoOpClient{}` (already used in host-profiler)

---

## Files Summary

### New Implementation Files (8)

| File | Description |
|------|-------------|
| `comp/host-profiler/collector/impl/receiver/cnm/factory.go` | Receiver factory |
| `comp/host-profiler/collector/impl/receiver/cnm/config.go` | Config + `toNetworkConfig()` |
| `comp/host-profiler/collector/impl/receiver/cnm/receiver.go` | Receiver lifecycle |
| `comp/host-profiler/collector/impl/receiver/cnm/metrics.go` | Connections -> pmetric.Metrics |
| `comp/otelcol/otlp/components/exporter/cnmexporter/factory.go` | Exporter factory |
| `comp/otelcol/otlp/components/exporter/cnmexporter/config.go` | Exporter config |
| `comp/otelcol/otlp/components/exporter/cnmexporter/exporter.go` | Metrics -> CollectorConnections |
| `comp/otelcol/otlp/components/exporter/cnmexporter/encoder.go` | Protobuf encoding |

### Modified Files (3)

| File | Change |
|------|--------|
| `comp/host-profiler/collector/impl/otel_col_factories.go` | Add GetExporters, register CNM components |
| `cmd/host-profiler/subcommands/run/command.go` | Wire Fx dependencies |
| `comp/host-profiler/collector/impl/agentprovider/config_builder.go` | Auto-generate CNM pipeline |

### New Test Files (11)

| File | Type | Environment |
|------|------|-------------|
| `cnm/config_test.go` | Unit | dev container |
| `cnm/factory_test.go` | Unit | dev container |
| `cnm/receiver_test.go` | Unit (mock tracer) | dev container |
| `cnm/metrics_test.go` | Unit (fidelity) | dev container |
| `cnm/kmt_test.go` | Integration (real eBPF) | KMT VM |
| `cnmexporter/config_test.go` | Unit | dev container |
| `cnmexporter/factory_test.go` | Unit | dev container |
| `cnmexporter/exporter_test.go` | Unit (mock forwarder) | dev container |
| `cnmexporter/encoder_test.go` | Unit (encoding parity) | dev container |
| `test/new-e2e/.../cnm_e2e_test.go` | E2E (fakeintake) | Pulumi VM |
| Shared `testutil_test.go` | Helpers | both |

### Modified Test Infrastructure (4)

| File | Change |
|------|--------|
| `tasks/kmt.py` | Add host-profiler component support |
| `tasks/system_probe.py` (or new) | Host-profiler test package list |
| `test/new-e2e/system-probe/test-runner/main.go` | CNM package timeouts |
| `otel_col_factories_test.go` | CNM factory registration tests |

---

## Example Pipeline Configuration

```yaml
receivers:
  cnm:
    collect_tcp_v4: true
    collect_tcp_v6: true
    collect_udp_v4: true
    collect_udp_v6: true
    dns_inspection: true
    max_tracked_connections: 65536
    check_interval: 30s

processors:
  infraattributes:

exporters:
  datadog_cnm: {}

service:
  pipelines:
    metrics/cnm:
      receivers: [cnm]
      processors: [infraattributes]
      exporters: [datadog_cnm]
```

---

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| High cardinality (100K connections x 13 metrics) | Configurable `max_tracked_connections`; OTel batch + memory limiter |
| OTLP -> DD reconstruction complexity | Round-trip fidelity tests + comparison against direct sender |
| `networkconfig.New()` global config | Construct Config struct directly in `toNetworkConfig()` |
| Binary size from network tracer deps | `cnm` build tag for conditional compilation |
| eBPF conflicts with co-located system-probe | Startup detection, warn or refuse |
| No-op telemetry loses internal tracer metrics | Acceptable for MVP; wire real telemetry later |

---

## Module Extraction (Upstreaming Prerequisite)

To upstream the CNM receiver to OTel Collector contrib, `pkg/network/` and `pkg/ebpf/`
must become independently importable Go modules. Today both are part of the root module,
which means importing them pulls in the entire 250MB+ agent. This section documents the
dependency analysis and extraction plan.

### Current State

The repo has 192 Go modules (managed via `modules.yml`), but the network monitoring
code is entirely in the root module:

| Package | Own Module? | Notes |
|---|---|---|
| `pkg/network/` | No | 594 files, 20+ subdirs, root module |
| `pkg/ebpf/` | No | ~200 files, eBPF loading + C code, root module |
| `pkg/process/util/` | No | Address types used by network, root module |
| `pkg/system-probe/config/` | No | System-probe config, root module |
| `pkg/network/driver/` | Yes | Windows NPM driver (already extracted) |
| `pkg/network/payload/` | Yes | Data structures only (already extracted) |

### Critical Blockers

**1. Circular dependency: `pkg/ebpf/uprobes` <-> `pkg/network`**

`pkg/ebpf/uprobes/` imports `pkg/network/{go/bininspect, usm/sharedlibraries, usm/utils}`,
while `pkg/network/` imports `pkg/ebpf/` from 77 files. This circular dependency must be
broken before either can become a module.

Solution: Move `uprobes/` into `pkg/network/` or decouple via interface.

**2. `pkg/process/util.Address` -- deepest integration (41 files)**

The `Address` type from `pkg/process/util` is embedded in every connection tuple
throughout `pkg/network/`. Functions like `AddressFromNetIP()`, `FromLowHigh()`,
`ToLowHigh()` are used in 41 files.

Solution: Extract `pkg/process/util` as its own module (small, clean deps).

**3. Root-module deps of `pkg/ebpf/` (after circular dep fix)**

| Dependency | Has go.mod? | Usage |
|---|---|---|
| `pkg/util/{log,kernel,funcs,archive}` | Most yes | Core utilities |
| `pkg/telemetry` | Yes | Metrics |
| `pkg/version` | Yes | Version info |
| `comp/core/telemetry` | Yes | DI telemetry |
| `pkg/remoteconfig/state` | **No** | RC-enabled BTF fetching |
| `pkg/config/setup` | Yes | Install paths for clang |

**4. Root-module deps of `pkg/network/` (after `pkg/ebpf/` extraction)**

| Dependency | Has go.mod? | Files importing |
|---|---|---|
| `pkg/process/util` | **No** | 41 files (Address type) |
| `pkg/eventmonitor` | **No** | 5 files |
| `pkg/process/monitor` | **No** | 6 files |
| `pkg/security/secl/model` | **No** | 6 files |
| `comp/core/sysprobeconfig` | **No** | 5 files |
| `pkg/system-probe/config` | **No** | 3 files |
| `pkg/config/setup` globals | Yes (but globals) | ~11 files (already bypassed by `toNetworkConfig()`) |

### Extraction Order (Bottom-Up)

| Phase | Extract | Key Work | Size |
|---|---|---|---|
| 1 | `pkg/process/util` | Small package (Address type, IP buffers). Clean deps. | Small |
| 2 | Break `pkg/ebpf/uprobes` circular dep | Move uprobes into `pkg/network/` or behind interface | Medium |
| 3 | `pkg/ebpf/` as module | Resolve `pkg/remoteconfig/state` dep | Large |
| 4 | Remaining blockers | `pkg/eventmonitor`, `pkg/process/monitor`, `pkg/security/secl/model`, `comp/core/sysprobeconfig`, `pkg/system-probe/config` | Medium each |
| 5 | `pkg/network/` as module | All blockers resolved, add `used_by_otel: true` to modules.yml | Large |

### Mechanical Work per Module (Mostly Automated)

For each new module:
1. Create `go.mod` + add entry to `modules.yml` (manual)
2. `dda inv modules.add-all-replace` -- generates ~180 replace directives (automated)
3. `dda inv tidy` -- fixes all dependent modules (automated)
4. `dda inv modules.go-work` -- updates go.work (automated)

The repo already has 192 modules and mature tooling for managing them. The manual work
is resolving dependency graph issues, not the mechanical go.mod management.

### Relationship to Phase 1

Phase 1 (OTLP Metrics MVP) does NOT require module extraction. The CNM receiver lives
in `comp/host-profiler/` and imports `pkg/network/tracer` directly within the root
module. Module extraction is a parallel workstream that enables upstreaming later.

---

## Verification

1. `dda env dev run -- dda inv host-profiler.build` -- binary compiles
2. `dda env dev run -- dda inv linter.go` -- passes
3. `dda env dev run -- dda inv test --targets=./comp/host-profiler/collector/impl/receiver/cnm/...`
4. `dda env dev run -- dda inv test --targets=./comp/otelcol/otlp/components/exporter/cnmexporter/...`
5. KMT: eBPF programs load, connections tracked on real kernel
6. E2E: `CollectorConnections` payloads arrive at fakeintake
7. Comparison: diff payloads against system-probe+process-agent on same host
