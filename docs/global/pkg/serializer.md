# pkg/serializer

## Purpose

`pkg/serializer` is responsible for encoding agent data (metrics, sketches, events, service
checks, and metadata) into wire-ready payloads and routing them through the forwarder to
the Datadog intake. It sits between the aggregator and the forwarder: the aggregator calls
`MetricSerializer` methods; the serializer chooses the right encoding format, optionally
compresses the payload, splits it if it would exceed backend size limits, and submits it to
the appropriate forwarder endpoint.

The package also owns the streaming JSON path for large iterating workloads (series, service
checks) and the protobuf path for sketches and orchestrator manifests.

## Key elements

### `MetricSerializer` interface

```go
type MetricSerializer interface {
    SendEvents(e event.Events) error
    SendServiceChecks(serviceChecks servicecheck.ServiceChecks) error
    SendIterableSeries(serieSource metrics.SerieSource) error
    AreSeriesEnabled() bool
    SendSketch(sketches metrics.SketchesSource) error
    AreSketchesEnabled() bool
    SendMetadata(m marshaler.JSONMarshaler) error
    SendHostMetadata(m marshaler.JSONMarshaler) error
    SendProcessesMetadata(data interface{}) error
    SendAgentchecksMetadata(m marshaler.JSONMarshaler) error
    SendOrchestratorMetadata(msgs []types.ProcessMessageBody, hostName, clusterID string, payloadType int) error
    SendOrchestratorManifests(msgs []types.ProcessMessageBody, hostName, clusterID string) error
}
```

This is the primary interface consumed by the aggregator. The `Serializer` struct (see below)
implements it. Test code uses the mock in `pkg/serializer/mocks`.

### `Serializer` struct

`Serializer` (in `serializer.go`) is the concrete implementation. It holds references to the
default forwarder, the orchestrator forwarder, the compression strategy, and per-payload-kind
feature flags. Constructed with:

```go
func NewSerializer(
    forwarder forwarder.Forwarder,
    orchestratorForwarder orchestratorForwarder.Component,
    compressor compression.Compressor,
    config config.Component,
    logger log.Component,
    hostName string,
) *Serializer
```

**Feature flags** (read from config at construction time):

| Config key | Field | Effect when false |
|------------|-------|-------------------|
| `enable_payloads.events` | `enableEvents` | Events are silently dropped |
| `enable_payloads.series` | `enableSeries` | Series are silently dropped |
| `enable_payloads.service_checks` | `enableServiceChecks` | Service checks are silently dropped |
| `enable_payloads.sketches` | `enableSketches` | Sketches are silently dropped |
| `enable_payloads.json_to_v1_intake` | `enableJSONToV1Intake` | Legacy process metadata dropped |

### Payload pipelines

For series (v2 API) and sketches the serializer builds a `metrics.PipelineSet`
(`internal/metrics/pipeline.go`). Each pipeline represents a unique combination of API
version and metric filter, and holds one or more `PipelineDestination`s. After serialization
all pipelines are flushed via `PipelineSet.Send()`.

Multi-region failover (`multi_region_failover.*`) and autoscaling failover
(`autoscaling.failover.*`) inject additional pipelines carrying only the metrics that pass
their respective allowlists.

### `marshaler` sub-package

`pkg/serializer/marshaler` defines the core interfaces that data types implement to plug into
the serializer.

| Interface | Description |
|-----------|-------------|
| `JSONMarshaler` | Simple `MarshalJSON() ([]byte, error)`. Used for metadata and small one-shot payloads. |
| `StreamJSONMarshaler` | Index-based streaming interface: `WriteHeader`, `WriteFooter`, `WriteItem(stream, i)`, `Len`, `DescribeItem`. Used for payloads that can be enumerated by integer index. |
| `IterableStreamJSONMarshaler` | Iterator-based streaming interface: `WriteHeader`, `WriteFooter`, `WriteCurrentItem`, `MoveNext`, `GetCurrentItemPointCount`, `DescribeCurrentItem`. Preferred for large series that are read from a channel or lazy source. |
| `BufferContext` | Reusable `bytes.Buffer` triplet (`CompressorInput`, `CompressorOutput`, `PrecompressionBuf`) passed into `MarshalSplitCompress` to avoid repeated allocation across forwarder flushes. |

`IterableStreamJSONMarshalerAdapter` (in `marshaler_adapter.go`) wraps a
`StreamJSONMarshaler` and exposes it as an `IterableStreamJSONMarshaler`, bridging the two
calling conventions.

### `split` sub-package

`pkg/serializer/split` provides `CheckSizeAndSerialize`, the size-gate used for metadata
payloads that cannot be split across multiple HTTP requests:

```go
func CheckSizeAndSerialize(
    m marshaler.JSONMarshaler,
    compress bool,
    strategy compression.Component,
) (mustSplit bool, compressedPayload []byte, payload []byte, err error)
```

Size limits enforced here:
- Compressed: 2 MiB (`maxPayloadSizeCompressed`)
- Uncompressed: 64 MiB (`maxPayloadSizeUnCompressed`)

If `mustSplit` is `true` for a metadata payload the caller (`sendMetadata`) treats it as a
hard error, since metadata cannot be split.

### `internal/stream` — streaming JSON builder

`JSONPayloadBuilder` (`internal/stream/json_payload_builder.go`) handles the hot path for
large iterating payloads (series, service checks):

1. Writes the header into a temporary buffer.
2. Iterates over items via `IterableStreamJSONMarshaler.MoveNext` / `WriteCurrentItem`.
3. Feeds each serialized item into a `Compressor`. When the compressor signals
   `ErrPayloadFull` the current compressed chunk is sealed and a new one starts.
4. Returns a `transaction.BytesPayloads` slice — one entry per HTTP request needed.

The builder can be constructed in shared-buffer mode
(`enable_json_stream_shared_compressor_buffers = true`) where a single pre-allocated
input/output buffer pair is protected by a mutex, reducing GC pressure on busy hosts.

Key policy constant:

```go
const (
    DropItemOnErrItemTooBig OnErrItemTooBigPolicy = iota
    FailOnErrItemTooBig
)
```

When an individual item is too large to fit even in an empty payload the policy controls
whether the item is silently dropped or the whole batch fails.

### `internal/metrics` — per-metric-kind serializers and pipelines

| File | Type | Purpose |
|------|------|---------|
| `iterable_series.go` | `IterableSeries` | Implements `IterableStreamJSONMarshaler` for v1/v2 JSON series. |
| `iterable_series_v3.go` | — | Protobuf marshalling for v3 series (used when `use_v2_api.series = false`). |
| `sketch_series_list.go` | `SketchSeriesList` | Protobuf marshalling for DDSketch payloads. |
| `events.go` | — | JSON marshalling for events (`MarshalEvents`). |
| `service_checks.go` | `ServiceChecks` | Implements `StreamJSONMarshaler` for service checks. |
| `pipeline.go` | `PipelineSet`, `PipelineConfig`, `PipelineContext`, `PipelineDestination` | Routing layer that maps (filter, API version) to destinations. |

`Filter` / `AllowAllFilter` / `MapFilter` interfaces in `pipeline.go` are used to gate which
metrics enter a given pipeline (e.g. only metrics in the MRF allowlist).

### `types` sub-package

`pkg/serializer/types` provides `ProcessMessageBody` (a protobuf message interface) and
`ProcessPayloadEncoder`, the function that encodes orchestrator messages into protobuf bytes.

### Observability

The serializer exposes expvars under the `serializer` and `jsonstream` namespaces.
Key expvars:

| Name | Meaning |
|------|---------|
| `serializer/SendEventsErrItemTooBigs` | Events dropped because a single event exceeded the max payload size |
| `serializer/SendEventsErrItemTooBigsFallback` | Retries using per-source-type marshaling after a too-big error |
| `jsonstream/TotalCalls` | Total invocations of the streaming JSON builder |
| `jsonstream/TotalItems` | Total items serialized through the streaming path |
| `jsonstream/ItemDrops` | Items silently dropped (too large or unexpected error) |
| `jsonstream/PayloadFulls` | Number of times a new compressed chunk had to be opened |
| `jsonstream/CompressorLocks` | (gauge) goroutines currently waiting for the shared compressor buffer |

### Relevant config keys

| Key | Default | Effect |
|-----|---------|--------|
| `serializer_max_payload_size` | — | Maximum compressed payload size for the streaming builder |
| `serializer_max_uncompressed_payload_size` | — | Maximum uncompressed payload size for the streaming builder |
| `enable_json_stream_shared_compressor_buffers` | `false` | Reuse a single pair of compressor buffers across calls |
| `use_v2_api.series` | `true` | Use the v2 JSON series endpoint; set to `false` for protobuf v3 |
| `serializer_experimental_use_v3_api.series.endpoints` | `[]` | Per-endpoint list of resolvers that use v3 |
| `serializer_experimental_use_v3_api.series.validate` | `false` | Send both v2 and v3 in parallel for validation |

## Usage

### In the aggregator

The aggregator receives a `MetricSerializer` via constructor injection. After each flush cycle
it calls the appropriate `Send*` method:

```go
// agent aggregator (simplified)
if err := s.serializer.SendIterableSeries(serieSource); err != nil {
    log.Warnf("Error flushing series: %v", err)
}
if err := s.serializer.SendSketch(sketchSource); err != nil {
    log.Warnf("Error flushing sketches: %v", err)
}
```

The aggregator never needs to know about compression, payload chunking, or endpoint routing
— those are all handled inside the serializer.

### Implementing a new payload type

1. Implement `marshaler.IterableStreamJSONMarshaler` (or `StreamJSONMarshaler` and wrap it
   with `NewIterableStreamJSONMarshalerAdapter`) in `internal/metrics/`.
2. Add a `Send*` method to `Serializer` that:
   - Checks the relevant feature flag.
   - Calls `s.serializeIterableStreamablePayload(…)` or `buildPipelines` + marshaler.
   - Calls the appropriate forwarder `Submit*` method.
3. Add the method to `MetricSerializer` interface.

### Writing a payload type that supports a single JSON blob (metadata)

Implement `marshaler.JSONMarshaler`, then delegate to `split.CheckSizeAndSerialize` and the
appropriate forwarder `Submit*` method (or call `Serializer.SendMetadata` /
`SendHostMetadata` / `SendAgentchecksMetadata` directly).

## Related packages

- `comp/aggregator/demultiplexer` — constructs the `Serializer` via fx and injects it into
  the aggregator.
- `comp/forwarder/defaultforwarder` — receives `transaction.BytesPayloads` from the
  serializer and manages retry queues and HTTP dispatch.
- `comp/serializer/metricscompression` — defines the `Compressor` interface (`Compress`,
  `ContentEncoding`) used by the serializer for payload compression.
- `pkg/metrics` — defines `SerieSource`, `SketchesSource`, `Serie`, etc. consumed by the
  internal metric serializers.

---

## Cross-references

### How pkg/serializer fits into the wider pipeline

```
pkg/aggregator (BufferedAggregator flush tick)
      │  SendIterableSeries(SerieSource) / SendSketch(SketchesSource)
      ▼
pkg/serializer (Serializer)
      │
      ├─ internal/metrics/IterableSeries  ──  IterableStreamJSONMarshaler
      │       (v1/v2 JSON or v3 protobuf depending on use_v2_api.series)
      ├─ internal/metrics/SketchSeriesList  ──  protobuf
      │
      ├─ internal/stream/JSONPayloadBuilder
      │       │  NewStreamCompressor(buf)  ──  comp/serializer/metricscompression
      │       │  feeds items until ErrPayloadFull → new chunk
      │       └─ returns transaction.BytesPayloads (one entry per HTTP request)
      │
      └─ comp/forwarder/defaultforwarder.SubmitSeries / SubmitSketchSeries
```

### Related documentation

| Document | Relationship |
|----------|--------------|
| [`pkg/metrics`](metrics/metrics.md) | Defines `Serie`, `SerieSource`, `SketchSeries`, `SketchesSource`, `APIMetricType`, and `MetricSource` — the primary inputs to the serializer. `Serie.Tags` is a `tagset.CompositeTags` that the internal serializers iterate via `ForEach` without materialising a flat `[]string`. |
| [`pkg/util/compression`](util/compression.md) | Defines the `Compressor` and `StreamCompressor` interfaces implemented by all compression backends. `internal/stream/JSONPayloadBuilder` calls `Compressor.NewStreamCompressor` to build chunked compressed payloads, and `Compressor.CompressBound` to pre-size output buffers. |
| [`comp/forwarder/defaultforwarder`](../comp/forwarder/defaultforwarder.md) | Downstream consumer. The serializer calls `SubmitSeries`, `SubmitSketchSeries`, `SubmitV1CheckRuns`, etc. and passes `transaction.BytesPayloads`. The forwarder wraps each chunk in an `HTTPTransaction`, fans it out to every configured domain/API-key pair, and manages retry queues. |
| [`comp/serializer/metricscompression`](../comp/serializer/metricscompression.md) | Provides the injectable `Compressor` component. The serializer receives it via fx and passes it to the stream builder. `ContentEncoding()` is used to set the `Content-Encoding` HTTP header on each forwarded payload. |

### Payload size management

The serializer never holds all series in memory at once. `JSONPayloadBuilder` feeds items one by one into a `StreamCompressor`; when the compressor reports `ErrPayloadFull` (compressed size exceeds `serializer_max_payload_size`) a new chunk is started. The result is a `transaction.BytesPayloads` slice — one element per HTTP POST sent to the intake.

For metadata payloads that cannot be split (`SendMetadata`, `SendHostMetadata`), `split.CheckSizeAndSerialize` enforces a hard 2 MiB compressed / 64 MiB uncompressed limit and returns `mustSplit = true` as a hard error if the payload exceeds it.

### Multi-region failover (MRF) and autoscaling pipelines

`PipelineSet` in `internal/metrics/pipeline.go` routes metrics to multiple destinations. Extra pipelines (MRF, autoscaling failover) are injected at `Serializer` construction time. Each pipeline has its own `Filter` (an allowlist of metric names) and `PipelineDestination` (a separate forwarder endpoint). The primary pipeline uses `AllowAllFilter` and the forwarder's primary domain; secondary pipelines use `MapFilter` populated from `multi_region_failover.*` or `autoscaling.failover.*` config keys.
