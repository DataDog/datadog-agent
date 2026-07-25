# Metric serialization

-----

The serializer ([`pkg/serializer`](<<<SRC>>>/pkg/serializer)) is the stage between [metric aggregation](aggregation.md) and the [forwarder](../forwarder.md): it consumes the `Serie` and `SketchSeries` streams produced by a flush, marshals them into wire payloads (protobuf for series and sketches, JSON for events and service checks), compresses them as it writes, splits them at configured size limits, and submits the resulting byte payloads as forwarder transactions. Its defining design choice is **streaming**: series are marshaled and compressed one at a time as they arrive from the samplers, so a flush never needs the whole series list in memory, and serialization overlaps sampler draining.

## Key packages and files

| Path | Purpose |
|---|---|
| [`pkg/serializer/serializer.go`](<<<SRC>>>/pkg/serializer/serializer.go) | `Serializer` struct and `MetricSerializer` interface: `SendIterableSeries`, `SendSketch`, `SendEvents`, `SendServiceChecks`, `SendHostMetadata`, … |
| [`pkg/serializer/metrics.go`](<<<SRC>>>/pkg/serializer/metrics.go) | Pipeline construction: v2 versus v3 routing, MRF and autoscaling filters, v3 validation/shadow modes |
| [`pkg/serializer/internal/metrics/pipeline.go`](<<<SRC>>>/pkg/serializer/internal/metrics/pipeline.go) | `PipelineSet`/`PipelineConfig`/`PipelineDestination`: fan-out of one marshaling pass to many destinations |
| [`pkg/serializer/internal/metrics/iterable_series.go`](<<<SRC>>>/pkg/serializer/internal/metrics/iterable_series.go) | Hand-rolled protobuf marshaling of series into `MetricPayload` (v2), split-on-full logic |
| [`pkg/serializer/internal/metrics/iterable_series_v3.go`](<<<SRC>>>/pkg/serializer/internal/metrics/iterable_series_v3.go) | v3 columnar format (dictionary plus per-column encoding) |
| [`pkg/serializer/internal/metrics/sketch_series_list.go`](<<<SRC>>>/pkg/serializer/internal/metrics/sketch_series_list.go) | Sketches into `SketchPayload` protobuf |
| [`pkg/serializer/internal/metrics/events.go`](<<<SRC>>>/pkg/serializer/internal/metrics/events.go) | Events into v1 intake JSON grouped by source type |
| [`pkg/serializer/internal/metrics/service_checks.go`](<<<SRC>>>/pkg/serializer/internal/metrics/service_checks.go) | Service checks into a v1 JSON array, stream-marshaled |
| [`pkg/serializer/internal/metrics/origin_mapping.go`](<<<SRC>>>/pkg/serializer/internal/metrics/origin_mapping.go) | `MetricSource` → Origin{Product,Category,Service} enums embedded in payload metadata |
| [`pkg/serializer/internal/stream/compressor.go`](<<<SRC>>>/pkg/serializer/internal/stream/compressor.go) | Streaming payload compressor with size limits, `ErrPayloadFull`/`ErrItemTooBig` |
| [`pkg/serializer/internal/stream/json_payload_builder.go`](<<<SRC>>>/pkg/serializer/internal/stream/json_payload_builder.go) | v1 JSON streaming payload builder (legacy series v1, service checks) |
| [`pkg/serializer/split/split.go`](<<<SRC>>>/pkg/serializer/split/split.go) | Size check for metadata payloads; metadata cannot be split |
| [`comp/serializer/metricscompression/impl/metricscompression.go`](<<<SRC>>>/comp/serializer/metricscompression/impl/metricscompression.go) | Compression component: zstd (default) or zlib per `serializer_compressor_kind` |
| [`comp/forwarder/defaultforwarder/endpoints/endpoints.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/endpoints/endpoints.go) | Declaration of every intake route the serializer targets |
| [`pkg/metrics/series.go`](<<<SRC>>>/pkg/metrics/series.go) | `Serie`, `Point`, and the `device:`/`dd.internal.resource:` tag-to-resource extraction |

The payload schemas themselves (`MetricPayload`, `SketchPayload`) live in the separate [agent-payload](https://github.com/DataDog/agent-payload) repository (`proto/metrics/agent_payload.proto`), vendored as `github.com/DataDog/agent-payload/v5`.

## The Serializer and its construction

`serializer.NewSerializer` binds three things: the shared [forwarder](../forwarder.md) (transaction submission), the orchestrator forwarder, and the metrics-compression component. One instance is shared by the whole demultiplexer, except that each no-aggregation worker gets a private instance (see [aggregation](aggregation.md)). The `MetricSerializer` interface is what the aggregator calls at flush time:

```text
 flush ──► SendIterableSeries(source) ──► series pipelines ──► Forwarder.SubmitTransaction (v1: SubmitV1Series)
       ──► SendSketch(source)         ──► sketch pipelines ──► Forwarder.SubmitTransaction
       ──► SendServiceChecks(...)     ──► v1 JSON          ──► Forwarder.SubmitV1CheckRuns
       ──► SendEvents(...)            ──► v1 intake JSON   ──► Forwarder.SubmitV1Intake
 (metadata providers) ─► SendHostMetadata / SendAgentchecksMetadata ─► SubmitHostMetadata / SubmitAgentChecksMetadata
```

Each data type has a kill switch: `enable_payloads.{series,events,service_checks,sketches,json_to_v1_intake}`. Disabling one logs a warning and silently drops that data type — nothing errors upstream.

## Series formats: v1, v2, v3

### v2 protobuf (the baseline)

`SendIterableSeries` drives `IterableSeries.MarshalSplitCompressPipelines` ([`iterable_series.go`](<<<SRC>>>/pkg/serializer/internal/metrics/iterable_series.go)), which writes the `MetricPayload` protobuf **by hand** using `molecule.ProtoStream` — field numbers are hard-coded constants rather than generated marshaling code, because this is the hottest serialization path in the Agent and the generated code would require materializing every serie as a proto struct. Per serie it emits: resources, metric name, tags, points, the type enum via `MType.SeriesAPIV2Enum()` (count=1, rate=2, gauge=3), `source_type_name`, interval, unit, origin metadata (`MetricSource` mapped through [`origin_mapping.go`](<<<SRC>>>/pkg/serializer/internal/metrics/origin_mapping.go)), and `metric_type_agent_hidden=9` when the serie is `NoIndex`.

Before marshaling, two tag conventions are consumed and converted into first-class payload fields ([`series.go`](<<<SRC>>>/pkg/metrics/series.go)): a `device:` tag becomes the `device` resource, and `dd.internal.resource:<type>:<name>` tags become typed resources. **These tags never reach the backend as literal tags.**

v2 payloads go to `/api/v2/series`, submitted as one `HTTPTransaction` per destination and API key via `Forwarder.SubmitTransaction`.

### v1 JSON (legacy)

Only when `use_v2_api.series: false`: series are streamed as `{"series": [...]}` JSON by the `stream.JSONPayloadBuilder` and submitted via `SubmitV1Series` to `/api/v1/series`. Two behavioral differences matter: the v1 path **drops `NoIndex` series** (they are simply skipped during iteration), and it has no origin metadata.

### v3 columnar (default for Datadog-owned intake)

[`iterable_series_v3.go`](<<<SRC>>>/pkg/serializer/internal/metrics/iterable_series_v3.go) implements a columnar, dictionary-encoded format targeting `/api/intake/metrics/v3/series`. Routing is controlled by `use_v3_api.series.enabled` (default `datadog_only`, meaning Datadog-owned intake endpoints get v3 while every other destination stays on v2) with a per-endpoint override map `use_v3_api.series.endpoints`. Two safety mechanisms exist for the rollout: a **validation** mode that double-sends payloads in both formats with correlation headers (`X-Metrics-Request-ID`/`-Seq`/`-Len`) so intake can diff them, and a **shadow sampling** mode — both under `serializer_experimental_use_v3_api.*`. One hard constraint: v3 requires zstd, so setting `serializer_compressor_kind: zlib` silently forces everything back to v2 (a single info log records this).

## Pipelines: one marshaling pass, many destinations

`buildPipelines` in [`metrics.go`](<<<SRC>>>/pkg/serializer/metrics.go) turns the forwarder's domain resolvers into a `PipelineSet` ([`pipeline.go`](<<<SRC>>>/pkg/serializer/internal/metrics/pipeline.go)). Destinations that share a `PipelineConfig{Filter, V3}` share a marshaling pass; each finished payload then becomes one `HTTPTransaction` per (destination, API key). This is how the same flush fans out to multiple intakes without marshaling twice, and how filtered destinations get different payload contents:

| Destination kind | Selector | Filter |
|---|---|---|
| Primary Datadog intake | default | none |
| Multi-Region Failover | `IsMRF()` on the domain resolver | allowlist `multi_region_failover.metric_allowlist`, active when `multi_region_failover.failover_metrics` is on |
| Cluster Agent (local) | `IsLocal()` | allowlist `autoscaling.failover.metrics`; v2 only — used by the node Agent to feed [autoscaling](../../containers/autoscaling.md) failover with a small metric subset |
| Vector / observability pipelines | ordinary additional resolver | none (full payloads to a different host; stays on v2 unless `vector.metrics.use_v3_api.series` is set) |

## Streaming compression and payload splitting

The stream compressor ([`compressor.go`](<<<SRC>>>/pkg/serializer/internal/stream/compressor.go)) wraps the compression component and enforces two limits simultaneously — a compressed-size cap and an uncompressed-size cap — plus, for series, a point-count cap. The marshaling loop writes one item at a time; when adding an item would exceed a limit, the compressor returns `ErrPayloadFull`, the current payload is finalized (footer written, flushed), and a fresh payload starts with the item retried. An item that cannot fit even in an empty payload yields `ErrItemTooBig` and is **dropped, not errored** — visible only in the `sketch_series` expvar map's `ItemTooBig` counter and the `sketch_series.sketch_too_big` telemetry counter, both shared by the series and sketch marshalers.

| Payload type | Compressed cap | Uncompressed cap | Other cap |
|---|---|---|---|
| Series v2/v3 | `serializer_max_series_payload_size` (512 KB) | `serializer_max_series_uncompressed_payload_size` (5 MB) | `serializer_max_series_points_per_payload` (10,000 points) |
| Sketches, v1 series | `serializer_max_payload_size` (2.5 MB) | `serializer_max_uncompressed_payload_size` (4 MB) | — |
| Metadata | 2 MB | 64 MB | cannot be split at all |

Compression is selected by the [`metricscompression`](<<<SRC>>>/comp/serializer/metricscompression/impl/metricscompression.go) component: `serializer_compressor_kind` is `zstd` by default (level `serializer_zstd_compressor_level`, default 1), with `zlib` as the alternative. The choice sets the `Content-Encoding` header (`zstd` or `deflate`), and protobuf payloads carry the `DD-Agent-Payload` schema-version header.

## Sketches

`SendSketch` drives `SketchSeriesList.MarshalSplitCompressPipelines` ([`sketch_series_list.go`](<<<SRC>>>/pkg/serializer/internal/metrics/sketch_series_list.go)), producing `SketchPayload` protobufs where each sketch carries per-point `dogsketches` — `{ts, cnt, min, max, avg, sum, k[], n[]}`, the bin keys and counts of the DDSketch accumulated during [aggregation](aggregation.md). Sketches go to `/api/beta/sketches` (endpoint name `sketches_v2`), submitted through the same pipeline `SubmitTransaction` mechanism as series, with an experimental v3 route behind `serializer_experimental_use_v3_api.*`.

## Events, service checks, and metadata

These never got the v2/v3 treatment and still use the oldest intake surfaces:

1. **Service checks** (`SendServiceChecks`): a v1 JSON array streamed through the `JSONPayloadBuilder`, split on the same 2.5 MB / 4 MB caps, submitted via `SubmitV1CheckRuns` to `/api/v1/check_run`. Oversized single items are dropped.
1. **Events** (`SendEvents`): the v1 intake envelope `{"apiKey":"","events":{"<source>":[...]},"internalHostname":...}` with events grouped by `SourceTypeName` ([`events.go`](<<<SRC>>>/pkg/serializer/internal/metrics/events.go)), submitted via `SubmitV1Intake` to `/intake/`. The `apiKey` JSON field is intentionally empty — authentication is the header, as everywhere else.
1. **Metadata** (`SendHostMetadata`, `SendAgentchecksMetadata`, and friends): plain JSON, size-checked by `split.CheckSizeAndSerialize` ([`split.go`](<<<SRC>>>/pkg/serializer/split/split.go)). Metadata payloads **cannot be split** — an oversized host-metadata payload is a hard error, unlike series which split transparently. Host metadata also goes to `/intake/` (`SubmitHostMetadata` builds high-priority, not-storable-on-disk transactions against the v1 intake endpoint); the payload content is produced by `comp/metadata/host`.

Orchestrator metadata and manifests take a separate path (`SendOrchestratorMetadata`/`SendOrchestratorManifests` to the orchestrator forwarder) — see [Orchestrator explorer](../../containers/orchestrator.md).

## Handoff to the forwarder

Every finalized payload reaches the forwarder with its target endpoint (declared in [`endpoints.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/endpoints/endpoints.go)), content type, and extra headers — series and sketches as one `HTTPTransaction` per destination and API key via `SubmitTransaction`, everything else as `transaction.BytesPayloads` through the endpoint-specific `Submit*` methods. From that point on, retries, disk buffering, ordering, and multi-domain delivery are the forwarder's problem — see [Forwarder and resilience](../forwarder.md). Route summary:

| Data | Route | Format |
|---|---|---|
| Series (v3, Datadog intake default) | `/api/intake/metrics/v3/series` | columnar v3, zstd only |
| Series (v2, all other destinations) | `/api/v2/series` | protobuf `MetricPayload`, zstd |
| Series (legacy) | `/api/v1/series` | JSON |
| Sketches | `/api/beta/sketches` | protobuf `SketchPayload` |
| Service checks | `/api/v1/check_run` | JSON |
| Events | `/intake/` | v1 intake JSON |
| Host metadata | `/intake/` | JSON |

## Configuration

| Key | Default | Effect |
|---|---|---|
| `use_v2_api.series` | true | v2 protobuf series; false falls back to legacy v1 JSON |
| `use_v3_api.series.enabled` / `.endpoints` | `datadog_only` / {} | v3 columnar routing, per-endpoint overrides |
| `serializer_experimental_use_v3_api.*` | various | v3 sketches opt-in, validation double-send, shadow sampling |
| `serializer_max_payload_size` | 2.5 MB | Compressed cap (sketches and v1 payloads) |
| `serializer_max_uncompressed_payload_size` | 4 MB | Uncompressed cap (sketches and v1 payloads) |
| `serializer_max_series_payload_size` | 512 KB | Compressed cap for v2/v3 series |
| `serializer_max_series_uncompressed_payload_size` | 5 MB | Uncompressed cap for v2/v3 series |
| `serializer_max_series_points_per_payload` | 10000 | Point cap per series payload |
| `serializer_compressor_kind` | `zstd` | `zstd` or `zlib`; zlib disables v3 |
| `serializer_zstd_compressor_level` | 1 | zstd level |
| `enable_payloads.{series,events,service_checks,sketches,json_to_v1_intake}` | true | Per-data-type kill switches |
| `enable_json_stream_shared_compressor_buffers` | true | Shared buffers in the v1 JSON builder |
| `multi_region_failover.failover_metrics` / `.metric_allowlist` | — | MRF dual-shipping filter |
| `autoscaling.failover.enabled` / `.metrics` | — | Local cluster-agent series pipeline |
| `log_payloads` | false | Debug-log every flushed serie, sketch, event, and service check |

## Deployment-mode differences

1. **Embedded OTel collector** ([DDOT](../../otel/ddot.md) and the [serializer exporter](../../otel/otlp-ingest.md)): reuses this serializer but pins the zlib compressor via [`comp/serializer/metricscompression/fx-otel`](<<<SRC>>>/comp/serializer/metricscompression/fx-otel/fx.go), which implicitly disables v3 intake there.
1. **Multi-Region Failover and cluster-agent autoscaling failover**: appear purely as extra pipeline destinations with allowlist filters, as described above; the primary payload bytes are unchanged.
1. Everything else (host, container, Kubernetes, Fargate) uses identical serialization; only the forwarder's domain/proxy configuration differs.

## Gotchas

1. **Dropped is not failed**: items too big for an empty payload vanish silently apart from the `sketch_series.ItemTooBig` expvar and `sketch_series.sketch_too_big` telemetry counter (shared by series and sketches; events have `serializer.SendEventsErrItemTooBigs`). If a customer reports missing high-cardinality series, check these first.
1. **`NoIndex` series only exist in v2+**; flipping `use_v2_api.series` off silently discards them.
1. **`device:` and `dd.internal.resource:` tags are consumed** at serialization time; searching for them as tags in the backend finds nothing.
1. **zlib forces v2** — a single info log (once per serializer, at the first affected flush) is the only trace that every `use_v3_api` setting is being ignored.
1. **Metadata cannot be split**, so a pathologically large host-metadata payload (huge tag sets, thousands of check instances) errors out instead of degrading gracefully.
1. **Events still hit the ancient `/intake/` endpoint** with the empty-`apiKey` envelope; when tracing event delivery, do not look for them on any v2 route.
1. The serializer's expvar maps (`serializer`, `jsonstream`, `compressor`, `sketch_series` on port 5000) are the primary debugging surface; see [Status, health, and telemetry](../../operations/introspection.md).
