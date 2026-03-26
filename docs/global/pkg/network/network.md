# `pkg/network` — Network Performance Monitoring (NPM)

## Purpose

`pkg/network` is the core library for Datadog's **Network Performance Monitoring
(NPM)** feature. It provides:

- Shared data types that represent active and closed TCP/UDP network connections.
- A stateful aggregation layer (`State`) that computes per-connection deltas
  between successive poll cycles.
- DNS resolution enrichment and gateway/route-lookup utilities.
- The `Tracer` (in `pkg/network/tracer/`) and its eBPF-level connection tracker
  (`pkg/network/tracer/connection/`), which drive data collection.
- Configuration (`pkg/network/config/`) and encoding/marshaling helpers
  (`pkg/network/encoding/`) that feed the system-probe HTTP API.

The package is consumed primarily by `cmd/system-probe/modules/network_tracer.go`,
which exposes collected data to the Datadog Agent over a Unix socket.

## Key Elements

### Connection representation

| Type | File | Description |
|------|------|-------------|
| `ConnectionTuple` | `event_common.go` | The five-tuple that uniquely identifies a connection: source/dest IP, source/dest port, PID, network namespace, protocol family, and direction. |
| `ConnectionStats` | `event_common.go` | Full per-connection snapshot. Embeds `ConnectionTuple` plus `StatCounters`, DNS stats, `ProtocolStack`, TLS tags, NAT translation (`IPTranslation`), routing (`Via`), RTT, and container metadata. |
| `StatCounters` | `event_common.go` | Monotonic byte/packet counts, retransmits, TCP congestion signals (RTO, fast recovery, ECN, out-of-order, zero-window probes). |
| `Connections` | `event_common.go` | Batch payload returned to callers. Wraps a slice of `ConnectionStats` with associated DNS, resolv.conf, USM application-layer data, and telemetry maps. |
| `BufferedData` / `ClientBuffer` | `event_common.go`, `buffer.go` | A recycled memory buffer used to avoid allocations when building `Connections` payloads. |
| `StatCookie` (`uint64`) | `event_common.go` | A 64-bit hash that uniquely identifies a connection across poll cycles; derived from the eBPF 32-bit cookie via re-hashing. |

### Enumerations

| Type | Values |
|------|--------|
| `ConnectionType` | `TCP`, `UDP` |
| `ConnectionFamily` | `AFINET` (v4), `AFINET6` (v6) |
| `ConnectionDirection` | `UNKNOWN`, `INCOMING`, `OUTGOING`, `LOCAL`, `NONE` |
| `EphemeralPortType` | `EphemeralUnknown`, `EphemeralTrue`, `EphemeralFalse` |
| `ConnTelemetryType` | String constants for internal telemetry metrics emitted in the connections payload (e.g., `MonotonicConnsClosed`, `ConnsBpfMapSize`). |

### Stateful aggregation

| Symbol | File | Description |
|--------|------|-------------|
| `State` (interface) | `state.go` | Per-client stateful tracker. `RegisterClient` / `RemoveClient` manage tracked clients. `GetDelta` merges the latest active connection set with buffered closed connections to produce a `Delta`. `StoreClosedConnection` buffers connections received from the eBPF close event path. |
| `Delta` | `state.go` | Return value of `GetDelta`: `Conns []ConnectionStats` + `USMData`. |
| `networkState` | `state.go` | Concrete implementation of `State`. Handles stats underflow detection, cookie collisions, and per-client USM protocol stat accumulation. |

### Tracer (`pkg/network/tracer/`)

| Symbol | File | Description |
|--------|------|-------------|
| `Tracer` | `tracer/tracer.go` | Top-level orchestrator. Owns an `ebpfTracer` (eBPF or eBPF-less), a `State`, a `ReverseDNS`, a `usm.Monitor`, a `netlink.Conntracker`, a `GatewayLookup`, and an optional process/container cache. `NewTracer` creates and starts all components. `GetActiveConnections` fetches the `Connections` payload for a given client ID. |
| `connection.Tracer` (interface) | `tracer/connection/tracer.go` | Abstraction over the eBPF layer. `Start` / `Stop`, `GetConnections`, `FlushPending`, `Remove`, `GetMap`, `DumpMaps`, `GetType`. Concrete implementations: kprobe prebuilt, runtime-compiled, CO-RE, fentry (Linux), ETW (Windows), and eBPF-less (Fargate). |
| `TracerType` | `tracer/connection/tracer.go` | Enum: `TracerTypeKProbePrebuilt`, `TracerTypeKProbeRuntimeCompiled`, `TracerTypeKProbeCORE`, `TracerTypeFentry`, `TracerTypeEbpfless`. |

### Configuration (`pkg/network/config/`)

`Config` (in `config/config.go`) extends `ebpf.Config` with NPM-specific options:

- `NPMEnabled` — feature gate.
- `CollectTCPv4Conns`, `CollectTCPv6Conns`, `CollectUDPv4Conns`, `CollectUDPv6Conns` — per-protocol collection switches.
- `DNSInspection`, `CollectDNSStats`, `CollectDNSDomains`, `DNSMonitoringPortList` — DNS enrichment.
- `MaxTrackedConnections` — eBPF map size.
- `MaxClosedConnectionsBuffered` — closed-connection ring buffer capacity.
- `EnableConntrack`, `ConntrackMaxStateSize` — NAT tracking via netlink.
- `EnableGatewayLookup` — gateway/subnet annotation.
- `ProtocolClassificationEnabled` — enables USM L7 classification on the same connections.
- `TCPFailedConnectionsEnabled` — track TCP error codes.
- `EnableNPMConnectionRollup` — aggregate ephemeral ports.
- `ExcludedSourceConnections` / `ExcludedDestinationConnections` — CIDR/port blocklists.

`USMConfig` (embedded via `*USMConfig`) adds per-protocol flags for HTTP, HTTP2,
Kafka, Postgres, Redis, TLS (native, Go, Node.js, Istio), and tuning parameters
for eBPF map sizing, ring buffers, and event consumers.

### Supporting sub-packages

| Package | Description |
|---------|-------------|
| `pkg/network/dns/` | DNS packet inspection, reverse-DNS hostname mapping, and per-query stats. `ReverseDNS` interface; `StatsByKeyByNameByType` map type consumed by `State`. |
| `pkg/network/netlink/` | Conntrack integration via netlink. `Conntracker` interface; `ebpfConntracker` and `netlinkConntracker` implementations translate NAT addresses. |
| `pkg/network/ebpf/` | Auto-generated Go types mirroring eBPF C structs (e.g., `ConnTuple`, `ConnStats`, `ProtocolStackWrapper`). |
| `pkg/network/filter/` | Socket-level BPF packet filter attachment used by the DNS and USM raw-socket paths. |
| `pkg/network/encoding/` | Protobuf marshaling of `Connections` payloads for the system-probe HTTP API. |
| `pkg/network/sender/` | Sends encoded connection payloads directly to the Datadog intake (when `DirectSend` is enabled). |
| `pkg/network/usm/` | Universal Service Monitoring (USM) monitor that loads per-protocol eBPF programs. See also `pkg/network/protocols/`. |
| `pkg/network/slice/` | Generic connection-slice utilities (e.g., group connections by type). |

### Build flags

| Build tag | Effect |
|-----------|--------|
| `linux_bpf` | Enables eBPF-based tracers, USM monitor, and the full protocol suite. Required for all eBPF paths. |
| `windows && npm` | Enables ETW-based HTTP monitoring and the Windows network driver interface on Windows. |

## Usage

### Entry point

The production entry point is `cmd/system-probe/modules/network_tracer.go`.
`createNetworkTracerModule` calls:

```go
ncfg := networkconfig.New()          // read datadog.yaml / system-probe.yaml
t, _ := tracer.NewTracer(ncfg, ...)  // start eBPF programs, DNS, USM
```

The resulting `module.Module` exposes HTTP endpoints:
- `GET /connections` — returns a `network.Connections` payload encoded as protobuf.
- `GET /debug/conntrack` — dumps the NAT conntrack table.
- `GET /debug/stats` — returns `State.GetStats()`.

### Typical consumer flow

```go
// 1. Register a polling client (one per upstream consumer, e.g., process-agent)
t.RegisterClient(clientID)

// 2. On each check interval, fetch the delta
conns, _ := t.GetActiveConnections(clientID)
// conns.Conns  — []network.ConnectionStats
// conns.DNS    — map[util.Address][]dns.Hostname (reverse DNS)
// conns.USMData — application-layer stats (HTTP, Kafka, …)
```

### Key data-flow

```
eBPF kprobes / fentry
    |
    v
connection.Tracer.GetConnections()
    |
    v
Tracer.GetActiveConnections()
    +--> State.GetDelta()   (merge active + closed, compute byte deltas)
    +--> ReverseDNS lookup
    +--> GatewayLookup annotation
    +--> usm.Monitor.GetProtocolStats() (USMData)
    |
    v
network.Connections  -->  encoding/marshal  -->  system-probe HTTP response
```

### Adding a new configuration option

1. Add the field to `Config` or `USMConfig` in `pkg/network/config/config.go` /
   `usm_config.go`.
2. Wire it to the YAML key inside `New()` / `NewUSMConfig()`.
3. Propagate the value into the relevant sub-system (tracer, USM monitor, etc.).

## Related packages

The following packages are closely coupled with `pkg/network` and are documented separately:

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/network/tracer` | [tracer.md](tracer.md) | Owns and drives `State`. Calls `State.GetDelta`, `State.StoreClosedConnection`, and `State.RegisterClient`. `Connections` is its primary output type. |
| `pkg/network/config` | [config.md](config.md) | `Config` / `USMConfig` structs that configure every field in `pkg/network`. Created via `config.New()` before the tracer starts. |
| `pkg/network/dns` | [dns.md](dns.md) | Provides the `ReverseDNS` interface consumed by the tracer to populate `Connections.DNS` with IP→hostname mappings and per-query stats. |
| `pkg/network/netlink` | [netlink.md](netlink.md) | Provides `Conntracker`, used by the tracer to fill `ConnectionStats.IPTranslation` with pre-NAT addresses. |
| `pkg/network/usm` | [usm.md](usm.md) | Provides `Monitor.GetProtocolStats()`, whose output populates `Connections.USMData` via `State.GetDelta`. |
| `pkg/network/protocols` | [protocols.md](protocols.md) | Defines `ProtocolType`, `Stack`, and per-protocol stat types embedded in `ConnectionStats` and returned inside `USMData`. |
| `pkg/network/encoding` | [encoding.md](encoding.md) | Marshals `*network.Connections` to protobuf/JSON for the system-probe HTTP API. |
| `pkg/ebpf` | [../../pkg/ebpf.md](../../pkg/ebpf.md) | Provides the shared eBPF infrastructure (`Manager`, `MapCleaner`, `Config`) used by the tracer and USM monitor. |
