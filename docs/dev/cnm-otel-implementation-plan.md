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

## Verification

1. `dda env dev run -- dda inv host-profiler.build` -- binary compiles
2. `dda env dev run -- dda inv linter.go` -- passes
3. `dda env dev run -- dda inv test --targets=./comp/host-profiler/collector/impl/receiver/cnm/...`
4. `dda env dev run -- dda inv test --targets=./comp/otelcol/otlp/components/exporter/cnmexporter/...`
5. KMT: eBPF programs load, connections tracked on real kernel
6. E2E: `CollectorConnections` payloads arrive at fakeintake
7. Comparison: diff payloads against system-probe+process-agent on same host
