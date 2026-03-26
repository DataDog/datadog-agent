# `pkg/network/protocols` — Application-Layer Protocol Classification (USM)

## Purpose

`pkg/network/protocols` is the application-layer monitoring component of
Datadog's **Universal Service Monitoring (USM)**. It classifies the L7 protocol
carried by each TCP/UDP connection and collects per-request statistics (latency,
status code, method, topic, command, etc.) for those connections.

Each supported protocol is implemented as a standalone eBPF program that:
1. Classifies traffic via a kernel-side tail-call dispatcher.
2. Emits structured events through a shared batched/ring-buffer event pipeline.
3. Feeds a userspace `StatKeeper` that aggregates request stats by connection key.

The resulting stats are returned through `Protocol.GetStats()` and surfaced to
`pkg/network`'s `State` layer via `USMProtocolsData`, then serialized into the
connections payload.

## Key Elements

### Core interfaces and types (root package)

| Symbol | File | Description |
|--------|------|-------------|
| `Protocol` (interface) | `protocols.go` (`linux_bpf`) | Contract every protocol implementation must satisfy. Lifecycle methods: `ConfigureOptions`, `PreStart`, `PostStart`, `Stop`. Data methods: `GetStats`, `DumpMaps`, `Name`, `IsBuildModeSupported`. |
| `ProtocolSpec` | `protocols.go` | Registration record for a protocol: `Factory` function, `Instance`, eBPF `Maps`, `Probes`, and `TailCalls`. Used by `usm.ebpfProgram` to load and wire protocols at startup. |
| `ProtocolFactory` | `protocols.go` | `func(*manager.Manager, *config.Config) (Protocol, error)` — called once during USM monitor initialization. |
| `ProtocolStats` | `protocols.go` | Return type of `GetStats()`: a `{Type ProtocolType, Stats interface{}}` pair. |
| `ModifierProvider` (interface) | `modifier.go` (`linux_bpf`) | Optional interface a `Protocol` can implement to attach additional `ddebpf.Modifier` instances (e.g., a perf event handler) to the eBPF manager. |
| `ProtocolType` (enum) | `types.go` | `Unknown`, `HTTP`, `HTTP2`, `Kafka`, `TLS`, `Mongo`, `Postgres`, `AMQP`, `Redis`, `MySQL`, `GRPC`. |
| `Stack` | `types.go` | Three-layer protocol descriptor attached to every `ConnectionStats`: `API` (e.g., gRPC), `Application` (e.g., HTTP2), `Encryption` (e.g., TLS). `MergeWith` / `Contains` / `IsUnknown` helpers are provided. |

### eBPF dispatcher constants

Defined in `protocols.go` and `ebpf_types.go` (cgo-generated from C headers):

| Constant | Description |
|----------|-------------|
| `ProtocolDispatcherProgramsMap` | eBPF map name for per-protocol tail-call programs (`"protocols_progs"`). |
| `TLSDispatcherProgramsMap` | eBPF map name for TLS tail-call programs (`"tls_process_progs"`). |
| `ProtocolDispatcherClassificationPrograms` | Map name for classification programs (`"dispatcher_classification_progs"`). |
| `TLSProtocolDispatcherClassificationPrograms` | Map name for TLS classification programs. |
| `DefaultMapCleanerBatchSize` | Default batch size (100) for eBPF map cleanup. |
| `ProgramType` / `DispatcherProgramType` | cgo enums mirroring C `protocol_prog_t` / `dispatcher_prog_t` used to reference tail-call slots. |

### Supported protocols

#### `protocols/http/`

| Symbol | Description |
|--------|-------------|
| `Spec` | `*protocols.ProtocolSpec` — registration entry for HTTP/1.x. |
| `Transaction` (interface) | Abstraction over an HTTP transaction (Linux eBPF or Windows ETW). Methods: `Method`, `StatusCode`, `Path`, `RequestLatency`, `ConnTuple`, `RequestStarted`, `ResponseLastSeen`. |
| `Method` (enum) | `MethodGet`, `MethodPost`, `MethodPut`, `MethodDelete`, `MethodHead`, `MethodOptions`, `MethodPatch`, `MethodTrace`, `MethodConnect`. |
| `Key` | Aggregation key: `ConnectionKey` + `Method` + `Path`. Used as the map key for request stats. |
| `RequestStat` | Per-bucket stats for a single (connection, method, path): latency DDSketch, request count, TLS tag bitfield. |
| `RequestStats` | Collection of `RequestStat` entries indexed by HTTP status-code class (1xx–5xx). `CombineWith` merges two `RequestStats` objects. |
| `StatKeeper` | Aggregates `Transaction` events into `map[Key]*RequestStats`. Supports path quantization (`URLQuantizer`), replace rules, and connection rollup. `GetAndResetAllStats` returns and resets current stats. |
| `StatKeeper.NewStatkeeper` | Constructor — reads `Config` for `EnableUSMQuantization`, `EnableUSMConnectionRollup`, `HTTPReplaceRules`. |

HTTP2 lives in `protocols/http2/` and follows the same pattern with its own
`Spec`, `StatKeeper`, and `Key` type.

#### `protocols/kafka/`

| Symbol | Description |
|--------|-------------|
| `Spec` | `*protocols.ProtocolSpec` — registration entry for Kafka. |
| `KafkaTransaction` | eBPF-side transaction struct mirroring the C type; carries topic name, API key, API version, error code. |
| `KafkaTransactionKey` | Connection tuple + topic name, used as the in-flight map key. |
| `StatKeeper` | Aggregates Kafka request/response events into per-`(connection, topic, APIVersion, error)` stats. |
| `Telemetry` / `kernelTelemetry` | Kernel-side and userspace telemetry helpers for Kafka. |

Multiple tail-call programs handle Kafka's multi-step response parsing
(fetch response partition parser v0/v12, record batch parser, produce response
parser). These are declared as constants in `protocol.go` (`fetchResponsePartitionParserV0TailCall`, etc.).

#### `protocols/postgres/`

Monitors PostgreSQL extended query protocol. Provides a `Spec`, `StatKeeper`,
and shared C type `POSTGRES_MAX_TOTAL_MESSAGES` (exposed in Go as
`PostgresMaxTotalMessages`). Uses multiple tail-call programs for request/
response parsing and handles the `PARSE` message type.

#### `protocols/redis/`

Monitors Redis RESP protocol. `RedisTrackResources` config flag controls whether
individual command keys are tracked (or only the command verb).

#### `protocols/tls/`

The TLS package provides tagging rather than a full `Protocol` implementation.

| Symbol | File | Description |
|--------|------|-------------|
| `ConnTag` (`uint64`) | `types.go` | Bit-flag identifying the TLS library in use. |
| `GnuTLS`, `OpenSSL`, `Go`, `TLS`, `Istio`, `NodeJS` | `types.go` | Predefined `ConnTag` constants sourced from `pkg/ebpf/c/protocols/tls/tags-types.h`. |
| `StaticTags` | `types.go` | Map from `ConnTag` to Datadog tag string (e.g., `"tls.library:openssl"`). Attached to `ConnectionStats.TLSTags`. |
| `Tags` | `tags.go` | Struct encoding TLS tag bits; embedded in `ConnectionStats`. |

TLS uprobes for **Go TLS** live in `tls/gotls/`; Node.js uprobes in `tls/nodejs/`.

#### Other protocols

| Directory | Protocol | Notes |
|-----------|----------|-------|
| `protocols/amqp/` | AMQP 0-9-1 (RabbitMQ) | Classifies producer/consumer operations. |
| `protocols/mongo/` | MongoDB wire protocol | Identifies query operations. |
| `protocols/mysql/` | MySQL text/binary protocol | Classifies statement types. |
| `protocols/http2/` | HTTP/2 (gRPC, h2c) | Dynamic HPACK table tracked in a per-connection eBPF map. |

### Event pipeline (`protocols/events/`)

The `events` sub-package standardizes kernel-to-userspace data transfer across
all USM protocols.

| Symbol | Description |
|--------|-------------|
| `BatchConsumer[T]` | Reads structured event batches from a perf buffer or ring buffer. Calls a user-supplied `func([]T)` callback on each batch. Safe for concurrent use. |
| `KernelAdaptiveConsumer[T]` | Selects between `BatchConsumer` and `DirectConsumer` based on kernel version (>=5.8 enables ring-buffer-based direct delivery). |
| `Consumer.Sync()` | Forces a flush of all buffered events. Called at each network check interval so that in-flight events reach the `StatKeeper` before stats are collected. |

On the kernel side each protocol uses the `USM_EVENTS_INIT` C macro to declare
the event batch maps and enqueue/flush helpers; the flush hook is attached to
`tracepoint/net/netif_receive_skb` (or `kprobe/__netif_receive_skb_core` on
4.14).

### Telemetry utilities (`protocols/telemetry/`)

Provides DDSketch-based latency utilities shared across protocols:

- `NSTimestampToFloat(ns uint64) float64` — truncates a nanosecond timestamp to
  10-bit mantissa precision for DDSketch insertion.
- `GetSketchQuantile(sketch, percentile) float64` — safe quantile accessor that
  returns 0 for nil sketches.

## Build flags

| Build tag | Effect |
|-----------|--------|
| `linux_bpf` | Full protocol suite: `Protocol` interface, all `Spec` variables, eBPF program loading, USM monitor. Required for any protocol monitoring. |
| `windows && npm` | HTTP/1.x monitoring via ETW (`http/etw_http_service.go`). Subset of features available on Linux. |

Files without a build tag (e.g., `types.go`, `common.go`) are compiled on all
platforms and define the cross-platform data types (`ProtocolType`, `Stack`,
`Method`, `RequestStat`, etc.) used by encoding and tests.

## Usage

### Enabling protocol monitoring

In `system-probe.yaml` (under the `service_monitoring_config` namespace):

```yaml
service_monitoring_config:
  enabled: true
  enable_http_monitoring: true
  enable_http2_monitoring: true
  enable_kafka_monitoring: true
  enable_postgres_monitoring: true
  enable_redis_monitoring: true
  enable_go_tls_support: true
  enable_native_tls_monitoring: true
```

Each flag corresponds directly to a `USMConfig` field (see `pkg/network/config/usm_config.go`).

### How a protocol is loaded

`usm.Monitor` (in `pkg/network/usm/monitor.go`) iterates over all registered
`ProtocolSpec` entries, invokes each `ProtocolFactory`, and calls
`Protocol.ConfigureOptions` then `PreStart` / `PostStart`. The resulting
per-protocol tail-call programs are inserted into the dispatcher maps
(`ProtocolDispatcherProgramsMap`, etc.).

### Adding a new protocol

1. Create a new sub-package under `pkg/network/protocols/<name>/`.
2. Implement the `Protocol` interface (all seven methods).
3. Define a package-level `Spec *protocols.ProtocolSpec` with `Factory`, `Maps`,
   `Probes`, and `TailCalls`.
4. Register `Spec` in `pkg/network/usm/ebpf_main.go` (the list of known
   protocol specs).
5. Add a `ProtocolType` constant to `protocols/types.go`.
6. Wire the eBPF C classifier into the dispatcher via `protocols/ebpf/c/`.
7. Add a `Enable<Name>Monitoring bool` field to `USMConfig` and read it in
   `NewUSMConfig`.

### Data flow

```
Network packet
    |
    v
eBPF socket filter (dispatcher tail-call)
    +--> protocol classifier  (sets Stack on conn_tuple)
    +--> protocol parser      (enqueues events via USM_EVENTS_INIT)
    |
    v
events.BatchConsumer / KernelAdaptiveConsumer  (userspace)
    |
    v
Protocol.GetStats() -> *ProtocolStats
    |
    v
network.State.GetDelta()  (merges into USMProtocolsData)
    |
    v
encoding/marshal  -->  system-probe HTTP response
```

### Relationship to `pkg/network`

`protocols.Stack` is embedded in every `network.ConnectionStats` so that each
connection record carries its detected protocol layers. The per-request stat maps
(e.g., `map[http.Key]*http.RequestStats`) are collected separately and surfaced
through `Connections.USMData`, which is populated by `network.State` calling
`storeUSMStats` for each protocol type.

---

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/network/usm` | [usm.md](usm.md) | `usm.Monitor` loads and orchestrates all `ProtocolSpec` instances. Protocol factories are registered in `usm/ebpf_main.go` and called during `usm.NewMonitor`. |
| `pkg/network/tracer` | [tracer.md](tracer.md) | The tracer owns the `usm.Monitor` and calls `GetProtocolStats()` to collect per-protocol stats at each check interval. Protocol `Stack` data flows into `ConnectionStats` via the shared `connection_protocol` eBPF map. |
| `pkg/network/go` | [go.md](go.md) | Used by `protocols/tls/gotls/` to inspect compiled Go binaries (function entry/return offsets, struct field offsets, ABI) before attaching Go TLS uprobes. |
| `pkg/ebpf` | [../../pkg/ebpf.md](../../pkg/ebpf.md) | Provides `Manager`, `MapCleaner`, CO-RE/RC/prebuilt loading, and `UprobeAttacher`. Every protocol's eBPF program is loaded through a `pkg/ebpf.Manager` instance created by `usm.ebpfProgram`. |
| `pkg/network` | [network.md](network.md) | Defines `ConnectionStats.ProtocolStack` (of type `protocols.Stack`) and the `USMData` map inside `Connections` that carries per-protocol stat snapshots. |
