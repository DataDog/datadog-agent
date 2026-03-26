# pkg/network/encoding

## Purpose

`pkg/network/encoding` serializes and deserializes the network connections payload that flows between **system-probe** and the **process-agent** (and optionally the core agent). The package is split into two deliberate sub-packages:

- `marshal/` — used exclusively inside **system-probe** to turn internal `network.Connections` structs into protobuf or JSON wire bytes.
- `unmarshal/` — used inside **process-agent** (and in tests) to decode those wire bytes back into the shared `model.Connections` protobuf type from `github.com/DataDog/agent-payload/v5/process`.

The split exists to avoid pulling all system-probe-internal imports (eBPF maps, USM protocol state, etc.) into the process-agent binary. The two sub-packages share only the protobuf schema; neither depends on the other.

## Key elements

### Top-level package (`encoding`)

The root `encoding.go` file contains only the package doc comment; its purpose is to explain the architectural rationale for the split. There is no exported API at this level.

### `marshal/`

| Symbol | Description |
|---|---|
| `Marshaler` interface | `Marshal(*network.Connections, io.Writer, *ConnectionsModeler) error` + `ContentType() string`. All serialization backends implement this. |
| `GetMarshaler(accept string) Marshaler` | Returns a `protoSerializer` when the `Accept` header contains `application/protobuf`, otherwise returns a `jsonSerializer`. |
| `ContentTypeProtobuf` | Constant `"application/protobuf"`. |
| `ConnectionsModeler` | Aggregates all per-batch encoding state: IP cache, route index, DNS formatter, resolv-conf formatter, USM encoders, deduped tags set, and the system-probe PID. |
| `NewConnectionsModeler(*network.Connections) (*ConnectionsModeler, error)` | Initializes all formatting helpers for a given snapshot of connections. Must be closed with `Close()` when done to flush USM orphan telemetry. |
| `FormatConnection(...)` | Converts a single `network.ConnectionStats` into a protobuf `model.ConnectionBuilder`, including addresses, TCP metrics, NAT translation, routes, protocol stack, DNS, USM stats, and tags. |
| `FormatCompilationTelemetry`, `FormatConnectionTelemetry`, `FormatCORETelemetry` | Helper functions that populate telemetry maps on the `model.ConnectionsBuilder`. |
| `FormatType`, `FormatProtocolStack` | Convert internal enums to protobuf equivalents. |
| `RouteIdx` | Pairs a `model.Route` with its position in the deduplicated route slice. |
| `USMEncoder` interface | `EncodeConnection(network.ConnectionStats, *model.ConnectionBuilder) (staticTags uint64, dynamicTags map[string]struct{})`. Implemented by each application-layer protocol encoder (HTTP, HTTP/2, Kafka, Postgres, Redis). |
| `USMConnectionIndex[K, V]` | Generic container that groups USM aggregation objects by `types.ConnectionKey`. Populated via `GroupByConnection`. Handles PID collision via `IsPIDCollision`. |
| `USMConnectionData[K, V]` | Per-connection bucket of `USMKeyValue` entries with PID-collision tracking. |
| `GroupByConnection[K, V](protocol, data, keyGen)` | Builds a `USMConnectionIndex` from a flat protocol-keyed map. |
| `InitializeUSMEncoders(*network.Connections)` | Platform-specific factory (Linux / Windows / unsupported) that constructs all active `USMEncoder` implementations for the current connections snapshot. |

**Platform-gated files:**
- `usm_encoders_linux.go` — HTTP, HTTP/2, Kafka, Postgres, Redis encoders on Linux.
- `usm_encoders_windows.go` — HTTP/HTTP/2 encoders on Windows.
- `usm_encoders_unsupported.go` — returns an empty slice on other platforms.

### `unmarshal/`

| Symbol | Description |
|---|---|
| `Unmarshaler` interface | `Unmarshal([]byte) (*model.Connections, error)` + `ContentType() string`. |
| `GetUnmarshaler(ctype string) Unmarshaler` | Returns protobuf or JSON deserializer based on `Content-Type`. |
| `ContentTypeProtobuf` | Constant `"application/protobuf"`. |

## Usage

**In system-probe** (`cmd/system-probe/modules/network_tracer.go`, `pkg/network/sender/`):

1. A `ConnectionsModeler` is created from the current connections snapshot.
2. `GetMarshaler` selects the serializer based on the caller's `Accept` header.
3. `Marshaler.Marshal` writes the protobuf payload to a `http.ResponseWriter` or buffer.
4. `ConnectionsModeler.Close()` is called to emit USM orphan-aggregation telemetry.

**In process-agent** (`pkg/process/checks/net.go`):

1. Raw bytes arrive from the system-probe HTTP endpoint.
2. `GetUnmarshaler` is called with the `Content-Type` header.
3. `Unmarshaler.Unmarshal` decodes the bytes into `*model.Connections` for further processing and forwarding to the Datadog backend.

### End-to-end serialisation flow

```
tracer.GetActiveConnections()
    |
    v
*network.Connections  (internal Go types: ConnectionStats, dns.StatsByKeyByNameByType, USMData)
    |
    v
marshal.NewConnectionsModeler(conns)
    +--> newDNSFormatter(conns, ipc)        ← converts dns.Hostname / StatsByKeyByNameByType
    +--> newResolvConfFormatter(conns)      ← per-container DNS resolv.conf
    +--> InitializeUSMEncoders(conns)       ← one USMEncoder per active protocol (HTTP, Kafka, …)
    +--> ipCache (Address → string memoiser)
    +--> routeIndex (Via → RouteIdx dedup)
    |
    v
Marshaler.Marshal(conns, writer, modeler)
    |
    +--> For each conn: FormatConnection(builder, conn, ...)
    |       +--> USMEncoder.EncodeConnection()  ← HTTP/Kafka/Postgres/Redis/HTTP2 stats
    |       +--> dnsFormatter.encode()          ← DNS stats for this connection
    |       +--> FormatProtocolStack()          ← protocols.Stack → protobuf enum
    |       +--> NAT translation, routes, tags
    |
    v
protobuf bytes  →  system-probe HTTP response (Content-Type: application/protobuf)
    |
    v
process-agent: Unmarshaler.Unmarshal() → *model.Connections  →  Datadog backend
```

### DNS formatting detail

`dnsFormatter` (in `marshal/dns.go`) is responsible for converting the `dns.StatsByKeyByNameByType` map into the protobuf `model.DNSStats` message attached to each connection. It groups stats by `dns.Key` (server/client IP + port + protocol) and encodes latency sums, timeout counts, and per-RCODE counters. The hostname strings are encoded as indices into a shared string table to reduce payload size.

### `AgentConfiguration` stamping

`ConnectionsModeler.modelConnections` reads four system-probe configuration flags exactly once (via `sync.Once`) and stamps them onto every `model.Connections` payload:

| Flag | Config key |
|------|------------|
| `NpmEnabled` | `network_config.enabled` |
| `UsmEnabled` | `service_monitoring_config.enabled` |
| `CcmEnabled` | `ccm_network_config.enabled` |
| `CsmEnabled` | `runtime_security_config.enabled` |

This allows the process-agent and backend to know which features are active without a separate API call.

### Adding a new USM protocol encoder

1. Implement the `USMEncoder` interface: `EncodeConnection`, `Close`.
2. Add the encoder to `InitializeUSMEncoders` in `usm_encoders_linux.go` (and `_windows.go` if applicable).
3. The `USMConnectionIndex[K, V]` / `GroupByConnection` helpers handle key-by-connection grouping; PID collision detection is provided by `IsPIDCollision`.

---

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/network` | [network.md](network.md) | Defines `network.Connections`, `network.ConnectionStats`, and all internal types that `marshal/` consumes. `encoding/marshal` imports `pkg/network` directly; `encoding/unmarshal` does not — it works only with the protobuf `model` types. |
| `pkg/network/usm` | [usm.md](usm.md) | `USMData` in `network.Connections` is populated by `usm.Monitor.GetProtocolStats()`. Each entry in `USMData` is consumed by the corresponding `USMEncoder` implementation in `marshal/usm_encoders_linux.go`. |
| `pkg/proto` | [../../pkg/proto/proto.md](../../pkg/proto/proto.md) | The wire format is defined in `datadog/process/process.proto` (in `pkg/proto`). The generated Go types live in `github.com/DataDog/agent-payload/v5/process` (`model.Connections`, `model.Connection`, `model.DNSStats`, etc.). `marshal/` writes these types; `unmarshal/` reads them. |
