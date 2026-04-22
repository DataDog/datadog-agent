# CNM (Cloud Networking Monitoring) on OTel Collector -- Design Document

## 1. Problem Statement

The current CNM architecture requires two cooperating processes:
- **system-probe**: Runs eBPF programs to collect network connection data from the kernel
- **process-agent**: Acts as a proxy, polling system-probe over a Unix socket and forwarding payloads to the Datadog backend

We want a single-process architecture where eBPF collection, signal production, pipeline processing, and backend submission all happen inside one OTel Collector process -- specifically the **host-profiler** binary (`cmd/host-profiler/`). The host-profiler is the right target because it already runs elevated and loads eBPF programs for profiling.

The implementation should be a proper OTel pipeline participant: a **receiver** producing OTel signals that flow through processors, into an **exporter** that converts to the Datadog-native wire format.

---

## 2. Current CNM Architecture (Reference)

### 2.1 System-Probe: eBPF Collection

**Entrypoint:** `cmd/system-probe/main.go` → `cmd/system-probe/subcommands/run/command.go`

**Module system:** `cmd/system-probe/modules/modules.go` registers modules in dependency order. The network tracer module is at `cmd/system-probe/modules/network_tracer.go`.

**Core tracer:** `pkg/network/tracer/tracer.go` -- The `Tracer` struct orchestrates all eBPF-based network monitoring:

| Component | Location | Purpose |
|---|---|---|
| `connection.Tracer` (ebpfTracer) | `pkg/network/tracer/connection/ebpf_tracer.go` | Loads eBPF programs, manages maps |
| `dns.ReverseDNS` | `pkg/network/dns/` | DNS snooping via eBPF |
| `usm.Monitor` | `pkg/network/usm/` | Protocol classification (HTTP, gRPC, TLS, Kafka, etc.) |
| `netlink.Conntracker` | `pkg/network/netlink/` | Connection tracking via netlink |
| `network.State` | `pkg/network/state*.go` | Connection state management |

**eBPF program loading:** `pkg/ebpf/` provides the loading framework:
- **CO-RE (preferred):** `pkg/ebpf/co_re.go` -- loads BTF-enabled programs
- **Fentry (preferred on 5.8+):** `pkg/network/tracer/connection/fentry/tracer.go`
- **Kprobe (fallback):** `pkg/network/tracer/connection/kprobe/tracer.go`
- **Runtime compilation:** `pkg/ebpf/bytecode/runtime/asset.go`

**eBPF probes:** `pkg/network/ebpf/probes/probes.go` defines all kernel hooks:
- TCP: `tcp_connect`, `tcp_finish_connect`, `tcp_sendmsg`, `tcp_recvmsg`, `tcp_close`, `tcp_done`
- UDP: `ip_make_skb`, `ip6_make_skb`, `udp_recvmsg`, `udp_destroy_sock`
- Protocol classification: Socket filters for HTTP, TLS, gRPC, databases

**eBPF maps managed:**
- `conn_stats_map`: Active connection tracking (key: ConnTuple, value: ConnStats)
- `tcp_stats_map`: TCP-specific stats (RTT, retransmits)
- `connection_protocol_map`: Protocol classification results
- `port_bindings_map`, `udp_port_bindings_map`: Listening port tracking
- Various DNS, TLS, and USM maps

**Data exposure:** HTTP API on Unix socket (`/var/run/datadog/sysprobe.sock`):
- `/connections?client_id=X` -- returns active connections (protobuf/JSON)
- `/register?client_id=X` -- client registration
- `/debug/*` -- debug endpoints for maps, state, conntrack

### 2.2 Process-Agent: Proxy/Forwarder

**Entrypoint:** `cmd/process-agent/main.go`

**ConnectionsCheck:** `pkg/process/checks/net.go`
- Polls system-probe's `/connections` endpoint periodically
- Deserializes protobuf `model.Connections`
- Enriches with container tags (via tagger), host tags, service context
- Batches into `model.CollectorConnections` protobuf messages (max 1000 connections per batch)
- Submits via `connectionsforwarder` to `https://process.<DD_SITE>/api/v1/collector`

**System-probe client:** `pkg/system-probe/api/client/client.go`
- HTTP client over Unix domain socket
- Timeout: 10s, max idle connections: 2

### 2.3 Direct Send Mode (Alternative)

When `network_config.direct_send = true`, system-probe bypasses process-agent entirely:
- **Sender:** `pkg/network/sender/sender_linux.go` -- `directSender` struct
- **Interface:** `ConnectionsSource` with `RegisterClient()` and `GetActiveConnections()`
- Periodically calls `GetActiveConnections()`, batches, encodes protobuf, compresses with zstd, submits via `connectionsforwarder`
- This proves the single-process model works and is the closest reference implementation for our OTel approach

### 2.4 Data Model

**Per-connection data** (`pkg/network/event_common.go`):

```
ConnectionStats {
  ConnectionTuple {Source, Dest, SPort, DPort, Pid, NetNS, Type, Family, Direction}
  Monotonic StatCounters {SentBytes, RecvBytes, SentPackets, RecvPackets, Retransmits,
                          TCPEstablished, TCPClosed, TCPRTOCount, TCPRecoveryCount,
                          TCPReordSeen, TCPRcvOOOPack, TCPDeliveredCE, TCPProbe0Count}
  Last StatCounters       // Delta values
  IPTranslation           // NAT info
  Via                     // Routing decision
  RTT, RTTVar             // Round-trip time (microseconds)
  DNSStats                // map[hostname]map[queryType]Stats
  TCPFailures             // map[errno]count
  ProtocolStack           // API/Application/Encryption layers
  ContainerID {Source, Dest}
  TLSTags, CertInfo
  Duration, LastUpdateEpoch
  IntraHost, IsClosed, TCPECNNegotiated
}
```

**Collection-level data** (`Connections` struct):
- `DNS map[Address][]Hostname` -- shared reverse DNS table
- `ResolvConfs map[ContainerID]ResolvConf`
- `ConnTelemetry` -- eBPF health counters (kprobes triggered/missed, etc.)
- `CompilationTelemetryByAsset`, `CORETelemetryByAsset` -- eBPF build results
- `USMData` -- aggregated protocol stats (HTTP, Kafka, Postgres, Redis)

**Wire format:** `model.CollectorConnections` protobuf from `agent-payload/v5/process`:
- 63 fields per Connection message
- Batch semantics (GroupId/GroupSize)
- Tag encoding with interning/deduplication
- DNS encoding with domain database (V2 format)
- USM aggregations as serialized byte buffers

---

## 3. Host-Profiler OTel Collector (Target Binary)

**Entrypoint:** `cmd/host-profiler/subcommands/run/command.go`

**Two modes:**
- **Agent Core mode** (`--core-config`): Full Agent integration with remote tagger, config sync, hostname, trace agent
- **Standalone mode** (`--config`): OTel-only, no Agent Core dependency

**Factory registration:** `comp/host-profiler/collector/impl/otel_col_factories.go`
- `ExtraFactories` interface provides mode-specific factories
- `createFactories()` builds OTel Factories (receivers, processors, exporters)
- Currently: profiling receiver (eBPF), OTLP receiver, Prometheus receiver
- Exporters: debug, OTLP HTTP
- Processors: attributes, cumulative-to-delta, filter, infraattributes (Agent Core), k8s attributes (standalone)

**Existing eBPF receiver pattern:** `comp/host-profiler/collector/impl/receiver/factory.go`
- Uses `xreceiver.NewFactory()` with `component.MustNewType("profiling")`
- Wraps `go.opentelemetry.io/ebpf-profiler/collector`
- Produces OTel Profiles signal (experimental "x" prefix)

**OTel version:** Collector v0.150.0, pdata v1.56.0

---

## 4. Design Options

### Option 1: Map CNM Data to OTLP Metrics

Use OTLP Metrics as the carrier signal, with connection identity encoded as metric attributes.

#### Metric Schema

**Per-connection metrics** (each data point shares connection-identifying attributes):

| Metric Name | Type | Unit | Description |
|---|---|---|---|
| `network.bytes.sent` | Sum (monotonic) | bytes | Bytes sent on this connection |
| `network.bytes.received` | Sum (monotonic) | bytes | Bytes received |
| `network.packets.sent` | Sum (monotonic) | packets | Packets sent |
| `network.packets.received` | Sum (monotonic) | packets | Packets received |
| `network.tcp.retransmits` | Sum (monotonic) | count | TCP retransmits |
| `network.tcp.rtt` | Gauge | us | Round-trip time |
| `network.tcp.rtt_var` | Gauge | us | RTT variance |
| `network.tcp.established` | Sum (monotonic) | count | TCP established transitions |
| `network.tcp.closed` | Sum (monotonic) | count | TCP closed transitions |
| `network.tcp.rto_count` | Sum (monotonic) | count | RTO events |
| `network.tcp.recovery_count` | Sum (monotonic) | count | Recovery events |
| `network.tcp.reorder_seen` | Sum (monotonic) | count | Reorder events |
| `network.tcp.ooo_packets` | Sum (monotonic) | count | Out-of-order packets |

**Per-connection attributes:**

| Attribute | Type | Example |
|---|---|---|
| `network.source.address` | string | `10.0.1.5` |
| `network.source.port` | int | `54321` |
| `network.destination.address` | string | `10.0.2.10` |
| `network.destination.port` | int | `443` |
| `network.transport` | string | `tcp` / `udp` |
| `network.type` | string | `ipv4` / `ipv6` |
| `network.direction` | string | `incoming` / `outgoing` / `local` |
| `network.pid` | int | `1234` |
| `network.netns` | int | `4026531840` |
| `container.id.source` | string | `abc123` |
| `container.id.dest` | string | `def456` |
| `network.intra_host` | bool | `true` |
| `network.is_closed` | bool | `false` |
| `network.protocol.stack` | string | `tls/http2` |

**NAT translation** (additional attributes when present):
- `network.nat.source.address`, `network.nat.source.port`
- `network.nat.destination.address`, `network.nat.destination.port`

**DNS data:**
- As attributes: `network.dns.hostname` (resolved name for dest IP)
- As separate metrics: `network.dns.latency`, `network.dns.timeouts`, `network.dns.responses` keyed by hostname+query_type

**USM/protocol data** (separate metric streams):
- `network.http.requests` (counter; attributes: path, status_code, method)
- `network.http.latency` (histogram; attributes: path, status_code)
- `network.kafka.requests` (counter; attributes: topic, error_code)
- `network.database.requests` (counter; attributes: operation, table)

**Telemetry:** Resource-level attributes or separate low-cardinality metrics (`network.ebpf.kprobes_triggered`, etc.)

#### What Gets Lost

| Data | Status | Mitigation |
|---|---|---|
| Dual counters (Monotonic + Last/delta) | OTLP Sum supports one temporality per stream | Use cumulative; exporter computes deltas |
| Per-connection DNS stats map | Flattened into attributes or separate metrics | Exporter reconstructs map from attribute keys |
| USM aggregations (DDSketch distributions) | Use OTLP ExponentialHistogram | DDSketch → ExponentialHistogram conversion (lossy) |
| TCPFailures map (error_code → count) | One metric per error code attribute value | Reconstructible in exporter |
| Tag encoding/deduplication | Lost in OTLP (no interning) | Exporter re-implements tag encoding |
| GroupId/GroupSize batch semantics | Not in OTLP | Exporter manages batching |
| Via/Route metadata | Resource attributes or separate signal | Exporter reconstructs route index |

#### Pros
- Uses **stable, well-supported OTLP pipeline** -- all existing processors work
- Standard metric exporters can consume the data for non-Datadog backends
- No upstream OTel changes needed -- can ship immediately
- Other vendors could consume the same metrics

#### Cons
- **High cardinality**: 10K-100K connections x 13+ metrics = potentially millions of metric series
- **Lossy for USM**: DDSketch distributions don't round-trip through ExponentialHistogram
- **Reconstruction cost**: DD exporter must rebuild `CollectorConnections` from denormalized metrics
- **Wire efficiency**: OTLP encoding is less compact (no interning, repeated attributes)
- **Semantic mismatch**: Connection snapshots aren't really "metrics"

---

### Option 2: New OTel Signal Type ("Network Flows")

Define a new experimental OTel signal type for network flow/connection data, following the pattern used to add Profiles to OTel. Contribute upstream.

#### Proposed pdata Schema

```protobuf
message NetworkFlowsData {
  repeated ResourceFlows resource_flows = 1;
}

message ResourceFlows {
  Resource resource = 1;
  repeated ScopeFlows scope_flows = 2;
}

message ScopeFlows {
  InstrumentationScope scope = 1;
  repeated ConnectionSnapshot connections = 2;
  DNSContext dns_context = 3;
  repeated Route routes = 4;
  FlowTelemetry telemetry = 5;
}

message ConnectionSnapshot {
  ConnectionTuple tuple = 1;
  TrafficCounters cumulative = 2;
  TrafficCounters delta = 3;
  TCPMetrics tcp = 4;
  repeated DNSStat dns_stats = 5;
  ProtocolStack protocol_stack = 6;
  repeated ApplicationStats app_stats = 7;
  string source_container_id = 8;
  string dest_container_id = 9;
  uint32 route_index = 10;
  NATTranslation nat = 11;
  uint64 last_update_epoch = 12;
  uint64 duration_ns = 13;
  bool is_closed = 14;
  bool intra_host = 15;
  repeated KeyValue attributes = 16;
}

message ConnectionTuple {
  bytes source_address = 1;
  bytes dest_address = 2;
  uint32 source_port = 3;
  uint32 dest_port = 4;
  TransportProtocol transport = 5;
  AddressFamily family = 6;
  Direction direction = 7;
  uint32 pid = 8;
  uint32 netns = 9;
}

message TrafficCounters {
  uint64 bytes_sent = 1;
  uint64 bytes_received = 2;
  uint64 packets_sent = 3;
  uint64 packets_received = 4;
  uint64 retransmits = 5;
  uint64 tcp_established = 6;
  uint64 tcp_closed = 7;
}

message TCPMetrics {
  uint32 rtt_us = 1;
  uint32 rtt_var_us = 2;
  uint32 rto_count = 3;
  uint32 recovery_count = 4;
  uint32 reorder_seen = 5;
  uint32 ooo_packets = 6;
  uint32 delivered_ce = 7;
  uint32 probe0_count = 8;
  bool ecn_negotiated = 9;
  map<uint32, uint32> failures_by_errno = 10;
}

message DNSContext {
  map<bytes, DNSHostnames> reverse_dns = 1;
}

message DNSHostnames {
  repeated string hostnames = 1;
}

message DNSStat {
  string hostname = 1;
  string query_type = 2;
  uint32 timeouts = 3;
  uint64 success_latency_sum_ns = 4;
  uint64 failure_latency_sum_ns = 5;
  map<uint32, uint32> count_by_rcode = 6;
}

message ProtocolStack {
  string api = 1;
  string application = 2;
  string encryption = 3;
}

message ApplicationStats {
  string protocol = 1;
  bytes aggregations = 2;
}

message Route {
  string subnet_alias = 1;
  bytes interface_hw_addr = 2;
}

message NATTranslation {
  bytes translated_source = 1;
  bytes translated_dest = 2;
  uint32 translated_source_port = 3;
  uint32 translated_dest_port = 4;
}

message FlowTelemetry {
  map<string, int64> counters = 1;
  map<string, int64> gauges = 2;
  map<string, string> metadata = 3;
}
```

#### Implementation Pattern (Following Profiles)

Profiles were added via experimental "x" prefix types:
- `go.opentelemetry.io/collector/pdata/pprofile` -- pdata type
- `xconsumer.Profiles`, `xreceiver.WithProfiles`, `xprocessor.WithProfiles`, `xexporter.WithProfiles`

NetworkFlows would follow the same pattern:
- `xconsumer.NetworkFlows`, `xreceiver.WithNetworkFlows`, etc.

#### Pros
- **Lossless**: Full data fidelity for every field in `ConnectionStats`
- **Proper separation**: Receiver → processors → exporter, clean pipeline
- **Vendor-neutral**: Other backends could consume the same signal
- **Upstream contribution**: Positions Datadog as a leader in OTel network observability
- **Efficient**: Wire encoding optimized for network data

#### Cons
- **Upstream dependency**: Requires OTel community acceptance
- **Long timeline**: OTEP → alpha → beta could take 6-12+ months
- **Limited processor support initially**
- **Parallel maintenance** until upstream stabilizes

---

## 5. Recommended Approach: Hybrid Phased

### Phase 1 (Ship Now): OTLP Metrics + Custom DD Exporter

Use Option 1 to deliver value immediately. Accept lossy tradeoffs for USM data.

**Components:**

1. **CNM Receiver** (`comp/host-profiler/collector/impl/receiver/cnm/`)
   - Embeds `pkg/network/tracer.Tracer` directly
   - Produces `pmetric.Metrics` with per-connection data points
   - Periodic collection via ticker (configurable, default 30s)

2. **CNM Datadog Exporter** (`comp/otelcol/otlp/components/exporter/cnmexporter/`)
   - Consumes `pmetric.Metrics` (filtered to `network.*` namespace)
   - Reconstructs `model.CollectorConnections` protobuf
   - Submits via `connectionsforwarder` to `process.<DD_SITE>`

3. **Factory registration** in `otel_col_factories.go` and Fx wiring in `run/command.go`

**Example pipeline configuration:**
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
  datadog/cnm:
    api:
      key: ${DD_API_KEY}
      site: datadoghq.com

service:
  pipelines:
    metrics/cnm:
      receivers: [cnm]
      processors: [infraattributes]
      exporters: [datadog/cnm]
```

### Phase 2 (Parallel): Propose New OTel Signal

1. Write an OTEP for NetworkFlows signal
2. Define protobuf schema (as above)
3. Build reference pdata implementation (can live in this repo initially)
4. Engage OTel SIG Network for feedback
5. Implement "x" prefix experimental pipeline locally

### Phase 3: Migrate to Native Signal

1. Replace OTLP Metrics output with native NetworkFlows signal
2. Adapt DD exporter to consume NetworkFlows directly (simpler, lossless)
3. Build NetworkFlows → Metrics connector for OTLP metric output
4. Adapt infraattributes processor for NetworkFlows

---

## 6. Phase 1 Implementation Details

### New Files

| File | Purpose |
|------|---------|
| `comp/host-profiler/collector/impl/receiver/cnm/factory.go` | OTel receiver factory (produces metrics) |
| `comp/host-profiler/collector/impl/receiver/cnm/config.go` | Config struct, validation, translation to `network/config.Config` |
| `comp/host-profiler/collector/impl/receiver/cnm/receiver.go` | Receiver lifecycle: embeds tracer, runs collection loop |
| `comp/host-profiler/collector/impl/receiver/cnm/metrics.go` | Converts `network.Connections` → `pmetric.Metrics` |
| `comp/otelcol/otlp/components/exporter/cnmexporter/factory.go` | DD exporter factory (consumes metrics) |
| `comp/otelcol/otlp/components/exporter/cnmexporter/config.go` | Exporter config (API key, site, batch settings) |
| `comp/otelcol/otlp/components/exporter/cnmexporter/exporter.go` | Reconstructs `CollectorConnections` protobuf from OTLP metrics |
| `comp/otelcol/otlp/components/exporter/cnmexporter/encoder.go` | Encoding logic adapted from `pkg/network/sender/` |

### Modified Files

| File | Change |
|------|--------|
| `comp/host-profiler/collector/impl/otel_col_factories.go` | Register cnm receiver + cnm exporter factories |
| `cmd/host-profiler/subcommands/run/command.go` | Wire connectionsforwarder, workloadmeta, telemetry Fx deps |
| `comp/host-profiler/collector/impl/agentprovider/` | Auto-generate CNM pipeline config from datadog.yaml |

### Key Code to Reuse

| Existing Code | Reuse For |
|---|---|
| `pkg/network/tracer.Tracer` | Core eBPF collection -- embed directly in receiver |
| `pkg/network/sender/sender_linux.go` | Reference for encoding logic (adapt for exporter) |
| `pkg/network/encoding/marshal/` | Connection → protobuf serialization |
| `agent-payload/v5/process` | `CollectorConnections` protobuf model |
| `comp/host-profiler/collector/impl/receiver/factory.go` | Pattern for receiver factory |
| `comp/otelcol/otlp/components/exporter/serializerexporter/` | Pattern for DD exporter |
| `comp/otelcol/otlp/components/processor/infraattributesprocessor/` | Tag enrichment |
| `comp/forwarder/connectionsforwarder/` | HTTP transport to backend |

### Receiver Lifecycle

```
Start():
  1. Build network/config.Config from OTel config (bypass networkconfig.New())
  2. tracer.IsTracerSupportedByOS() -- graceful degradation if unsupported
  3. tracer.NewTracer(cfg, noopTelemetry, noopStatsd)
  4. tracer.RegisterClient("cnm-otel-receiver")
  5. Start ticker goroutine

Collection loop (every check_interval):
  1. conns, cleanup := tracer.GetActiveConnections("cnm-otel-receiver")
  2. metrics := convertToOTLPMetrics(conns)
  3. nextConsumer.ConsumeMetrics(ctx, metrics)
  4. cleanup()

Shutdown():
  1. Stop ticker
  2. tracer.Stop()
```

### Exporter Reconstruction

The DD exporter consumes `pmetric.Metrics` and rebuilds the native format:
1. Groups metrics by connection tuple (attribute set → connection identity)
2. Reconstructs `ConnectionStats` from metric values + attributes
3. Builds `model.CollectorConnections` protobuf batches (max 1000 per batch)
4. Encodes DNS, tags, routes, telemetry using `pkg/network/encoding/marshal/`
5. Compresses with zstd, submits via `connectionsforwarder`

### Key Technical Challenges

| Challenge | Solution |
|---|---|
| `networkconfig.New()` reads global SystemProbe config | Construct `Config` struct directly from OTel config values |
| `tracer.NewTracer()` needs `telemetryComponent.Component` | Provide no-op implementation for MVP |
| `tracer.NewTracer()` needs `statsd.ClientInterface` | Use `ddgostatsd.NoOpClient{}` (already used in host-profiler) |
| High cardinality in OTLP metrics pipeline | Batch processor + memory limiter; configurable `max_tracked_connections` |
| Binary size growth from network tracer dependencies | Use `cnm` build tag for conditional compilation |
| eBPF conflicts with co-located system-probe | Detect at startup, warn or refuse to start CNM |
| USM DDSketch lossy in ExponentialHistogram | Accept in Phase 1; Phase 3 native signal eliminates |

---

## 7. Module Extraction Plan (Upstreaming Prerequisite)

To upstream the CNM receiver to the OTel Collector contrib repo, `pkg/network/` and
`pkg/ebpf/` must be independently importable Go modules. Today both are part of the
root module, making them impossible to import without pulling in the entire agent.

### Current Blockers

**Circular dependency:** `pkg/ebpf/uprobes/` imports `pkg/network/{go/bininspect,usm/sharedlibraries,usm/utils}`, while `pkg/network/` imports `pkg/ebpf/` (77 files). This must be broken before either can become a module.

**Root-module dependencies of `pkg/ebpf/`:**
- `pkg/remoteconfig/state` (no go.mod)
- `pkg/util/{log,kernel,funcs,archive}` (most already have go.mod)
- `pkg/telemetry`, `pkg/version`, `comp/core/telemetry` (all have go.mod)

**Root-module dependencies of `pkg/network/` (after `pkg/ebpf/` extraction):**
- `pkg/process/util` (no go.mod) -- **hardest blocker**, `Address` type used in 41 files
- `pkg/eventmonitor` (no go.mod) -- 5 files
- `pkg/process/monitor` (no go.mod) -- 6 files
- `pkg/security/secl/model` (no go.mod) -- 6 files
- `comp/core/sysprobeconfig` (no go.mod) -- 5 files
- `pkg/system-probe/config` (no go.mod) -- 3 files
- `pkg/config/setup` global functions (`SystemProbe()`) -- already bypassed by `toNetworkConfig()`

### Extraction Order (Bottom-Up)

| Phase | Extract | Key Work | Size |
|-------|---------|----------|------|
| 1 | `pkg/process/util` | Small package (Address type, IP buffers). Clean deps. | Small |
| 2 | Break `pkg/ebpf/uprobes` circular dep | Move uprobes into `pkg/network/` or behind interface | Medium |
| 3 | `pkg/ebpf/` | Resolve `pkg/remoteconfig/state` dep (extract or interface) | Large |
| 4 | Remaining blockers | `pkg/eventmonitor`, `pkg/process/monitor`, `pkg/security/secl/model`, `comp/core/sysprobeconfig`, `pkg/system-probe/config` | Medium each |
| 5 | `pkg/network/` | All blockers resolved, create module with `used_by_otel: true` | Large |

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

## 8. Verification Plan

1. **Build**: `dda inv agent.build` targeting host-profiler binary
2. **Unit tests**: `dda inv test --targets=./comp/host-profiler/collector/impl/receiver/cnm/...`
3. **Unit tests**: `dda inv test --targets=./comp/otelcol/otlp/components/exporter/cnmexporter/...`
4. **Integration**: Run host-profiler with CNM enabled, verify eBPF programs load, connections tracked
5. **End-to-end**: Verify `CollectorConnections` payloads arrive at fakeintake with correct structure
6. **Comparison test**: Same host running system-probe+process-agent and host-profiler+CNM, compare payloads
7. **Linting**: `dda inv linter.go`
