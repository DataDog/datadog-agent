> **TL;DR:** `pkg/opentelemetry-mapping-go` is the canonical translation layer between the OTel data model and the Datadog data model, covering resource-attribute-to-tag mapping, OTLP metrics conversion (gauges, sums, histograms) to Datadog time series and sketches, and host metadata derivation — published as standalone Go modules so the OTel Collector Datadog exporter can import them independently.

# pkg/opentelemetry-mapping-go

## Purpose

`pkg/opentelemetry-mapping-go` provides the translation layer between the
[OpenTelemetry](https://opentelemetry.io/) data model (OTLP) and the Datadog data model. It
is used wherever the Datadog Agent or the Datadog OpenTelemetry Collector exporter receives
OTLP signals and needs to convert them into the formats accepted by the Datadog backend.

The package covers three distinct concerns:

| Sub-package | Concern |
|-------------|---------|
| `otlp/attributes` | Mapping OTel resource attributes to Datadog tags and hostnames. |
| `otlp/metrics` | Translating OTLP metrics (gauges, sums, histograms, summaries) to Datadog time series and sketches. |
| `inframetadata` | Deriving host metadata payloads from OTel resource attributes and periodically reporting them to the Datadog infrastructure list. |

The package has its own `go.mod` files in each sub-directory because it is also published as a
standalone module used by the OpenTelemetry Collector Datadog exporter outside of the agent.

## Key elements

### Key types

| Type | Sub-package | Description |
|------|-------------|-------------|
| `Translator` | `otlp/attributes` | Resolves a `source.Source` (hostname or cloud resource ID) from OTel resource attributes; tracks missing-source telemetry. |
| `Source` | `otlp/attributes` | `{Kind, Identifier}` pair representing a host identity (hostname or Fargate task ARN). |
| `Dimensions` | `otlp/metrics` | Carries the metric name, host, and tags for a translated data point. |
| `Metadata` | `otlp/metrics` | Returned by `MapMetrics`; includes discovered runtime languages. |
| `Reporter` | `inframetadata` | Periodic reporter that derives `payload.HostMetadata` from OTel resources and pushes them to the Datadog infrastructure list. |

### Key interfaces

| Interface | Sub-package | Description |
|-----------|-------------|-------------|
| `Provider` | `otlp/metrics` | `MapMetrics(ctx, md, consumer, hostHandler)` — primary metrics translation entry point. |
| `Consumer` | `otlp/metrics` | Receives translated time series, sketches, and histograms synchronously from `MapMetrics`. Optional `HostConsumer` / `TagsConsumer` sub-interfaces. |
| `Pusher` | `inframetadata` | `Push(ctx, payload.HostMetadata)` — callers supply their own implementation to deliver metadata to the Datadog API. |
| `HostFromAttributesHandler` | `otlp/attributes` | Provides additional hostname resolution logic; injected into `ResourceToSource`. |

### Key functions

| Function | Sub-package | Description |
|----------|-------------|-------------|
| `NewTranslator(set)` | `otlp/attributes` | Creates an attributes `Translator` with telemetry tracking. |
| `SourceFromAttrs(attrs, handler)` | `otlp/attributes` | Low-level helper that resolves host identity from OTel attributes in priority order. |
| `NewDefaultTranslator(set, attrTranslator, opts...)` | `otlp/metrics` | Stateful translator with cumulative-to-delta conversion and TTL cache. |
| `NewMinimalTranslator(logger, attrTranslator, opts...)` | `otlp/metrics` | Lightweight, stateless translator; skips cumulative monotonic sums and histograms. |
| `(*Translator).StatsToMetrics(sp)` | `otlp/metrics` | Encodes a `StatsPayload` as fake `pmetric.Metrics` for APM stats pipeline passthrough. |
| `NewReporter(logger, pusher, period)` | `inframetadata` | Creates a `Reporter` that periodically flushes host metadata derived via `ConsumeResource`. |

### Configuration and build flags

The `otlp/metrics` translator is configured via `TranslatorOption` functions (see `#### TranslatorOption functions` under `### Sub-packages in detail` below). Key behavioral toggles:

- `WithRemapping()` — enables OTel-to-Datadog metric name remapping; use in Collector deployments without the native Agent.
- `WithOTelPrefix()` — prefixes system/process metrics with `otel.` to avoid collisions with native Agent metrics.
- `WithHistogramMode()` — controls whether histograms are emitted as distributions, counters, or without bucket data.
- `WithNumberMode()` — controls cumulative-to-delta conversion behavior.

The `datadog.host.use_as_metadata` boolean resource attribute overrides the default `shouldUseByDefault` policy in `inframetadata`: set to `"true"` to force a resource to contribute host metadata even when no hostname is detected.

---

### Sub-packages in detail

### `otlp/attributes`

Translates OTel resource attributes to Datadog concepts (hostname, tags, container tags,
Kubernetes tags).

#### `Translator`

```go
// NewTranslator creates an attributes translator that also tracks telemetry
// for missing source (hostname) events.
func NewTranslator(set component.TelemetrySettings) (*Translator, error)

// ResourceToSource derives a source.Source (hostname or cloud resource ID)
// from a resource's attributes. Increments the missing-source counter when
// no hostname is found.
func (p *Translator) ResourceToSource(
    ctx context.Context,
    res pcommon.Resource,
    set attribute.Set,
    hostFromAttributesHandler HostFromAttributesHandler,
) (source.Source, bool)
```

#### Mapping tables (package-level `var`)

These exported maps are consumed by the metrics and trace translators:

| Variable | Description |
|----------|-------------|
| `ContainerMappings` | OTel `container.*`, `cloud.*`, ECS, and Kubernetes semconv → Datadog container tag names. |
| `HTTPMappings` | OTel `http.*` / `network.*` semconv → Datadog span tag names. |

`coreMapping` (unexported) maps `deployment.environment`, `service.name`, and
`service.version` to Datadog UST tags (`env`, `service`, `version`).

#### `source.Source`

```go
type Source struct {
    Kind       SourceKind    // HostnameKind or AWSECSFargateKind
    Identifier string
}
```

`SourceFromAttrs(attrs pcommon.Map, handler HostFromAttributesHandler) (Source, bool)` is the
low-level helper that checks OTel attributes in priority order to determine the host identity.

### `otlp/metrics`

Translates OTLP `pmetric.Metrics` into Datadog time series (`Gauge` / `Count`) and sketches.

#### `Provider` interface

```go
// Provider is the primary interface for translating OTLP metrics.
type Provider interface {
    MapMetrics(
        ctx context.Context,
        md pmetric.Metrics,
        consumer Consumer,
        hostFromAttributesHandler attributes.HostFromAttributesHandler,
    ) (Metadata, error)
}
```

#### Constructors

```go
// NewDefaultTranslator: stateful translator with cumulative-to-delta conversion.
// Maintains an in-memory TTL cache of previous data points.
func NewDefaultTranslator(
    set component.TelemetrySettings,
    attributesTranslator *attributes.Translator,
    options ...TranslatorOption,
) (Provider, error)

// NewMinimalTranslator: lightweight, stateless translator.
// Designed for already-delta metrics or lower-memory environments.
// Skips cumulative monotonic sums and cumulative histograms instead of converting them.
func NewMinimalTranslator(
    logger *zap.Logger,
    attributesTranslator *attributes.Translator,
    options ...TranslatorOption,
) (Provider, error)

// NewTranslator: deprecated wrapper; use NewDefaultTranslator.
func NewTranslator(...) (*Translator, error)
```

#### `Consumer` interface

The caller supplies a `Consumer` to receive the translated data points. All methods are
called synchronously from within `MapMetrics`.

```go
type Consumer interface {
    TimeSeriesConsumer           // ConsumeTimeSeries(ctx, dims, DataType, ts, interval, value)
    SketchConsumer               // ConsumeSketch(ctx, dims, ts, interval, *quantile.Sketch)
    ExplicitBoundHistogramConsumer // ConsumeExplicitBoundHistogram(...)
    ExponentialHistogramConsumer   // ConsumeExponentialHistogram(...)
}

// Optional interfaces checked via type assertion:
type HostConsumer interface { ConsumeHost(host string) }
type TagsConsumer interface  { ConsumeTag(tag string) }
```

`DataType` constants: `Gauge`, `Count`.

#### `Dimensions`

```go
type Dimensions struct {
    Name   string
    Host   string
    Tags   []string
    // origin product metadata (for Datadog-internal attribution)
    // ...
}
```

#### `TranslatorOption` functions

Key options for `NewDefaultTranslator` / `NewMinimalTranslator`:

| Function | Description |
|----------|-------------|
| `WithRemapping()` | Remap OTel container/system/process metrics to Datadog counterparts (for use without the Datadog Agent). |
| `WithOTelPrefix()` | Prefix system/process/Kafka metrics with `otel.` to avoid name collisions. |
| `WithDeltaTTL(int64)` | TTL in seconds for cumulative-to-delta cache entries (default 3600). |
| `WithHistogramMode(HistogramMode)` | Control histogram output: `HistogramModeDistributions` (default), `HistogramModeCounters`, or `HistogramModeNoBuckets`. |
| `WithQuantiles(bool)` | Emit summary quantiles as individual metrics. |
| `WithNumberMode(NumberMode)` | `NumberModeCumulativeToDelta` (default) or `NumberModeRawValue`. |
| `WithFallbackSourceProvider(source.Provider)` | Hostname provider used when resource attributes carry no hostname. |
| `WithOriginProduct(OriginProduct)` | Tag the origin of metrics for Datadog-internal routing. |
| `WithStatsOut(chan<- []byte)` | Channel to receive serialized APM stats payloads embedded in metrics. |

#### APM stats passthrough

`(*Translator).StatsToMetrics(sp *pb.StatsPayload) (pmetric.Metrics, error)` encodes a
serialized `StatsPayload` as a fake `pmetric.Metrics` message for pipeline passthrough. The
translator on the receiving end decodes and forwards it.

#### Runtime metric remapping

`runtime_metric_mappings.go` and `metrics_remapping.go` contain static tables that translate
OTel runtime metrics (e.g. `process.runtime.go.*`) and system/infrastructure metrics to their
Datadog equivalents. Enabled via `WithRemapping()`.

### `inframetadata`

Derives `payload.HostMetadata` from `pcommon.Resource` payloads and reports them to the
Datadog infrastructure list endpoint.

#### `Reporter`

```go
func NewReporter(logger *zap.Logger, pusher Pusher, period time.Duration) (*Reporter, error)

// Run starts the periodic flush loop. Blocks until Stop is called.
func (r *Reporter) Run(ctx context.Context) error

func (r *Reporter) Stop()

// ConsumeResource updates internal host metadata from a resource.
// Returns (changed bool, err error).
func (r *Reporter) ConsumeResource(res pcommon.Resource) (bool, error)
```

#### `Pusher` interface

```go
type Pusher interface {
    Push(context.Context, payload.HostMetadata) error
}
```

Callers provide their own implementation to send metadata to the Datadog API.

#### Resource attribute `datadog.host.use_as_metadata`

When this boolean attribute is present on a resource, it overrides the default
`shouldUseByDefault` policy (`false`). Set it to `"true"` to force the resource to contribute
to host metadata even if no hostname is detected.

## Usage

### In the Datadog Agent OTel pipeline

The primary consumer is the `comp/otelcol/otlp/` component tree (see
[`comp/otelcol/otlp`](../comp/otelcol/otlp.md)), specifically
`comp/otelcol/otlp/components/exporter/serializerexporter/`. The exporter:

1. Creates an `attributes.Translator` once at startup.
2. Creates a `metrics.Provider` (via `NewDefaultTranslator`) with the attributes translator.
3. On each OTLP batch, calls `provider.MapMetrics(ctx, md, consumer, hostHandler)` where
   `consumer` serializes the resulting time series and sketches through `pkg/serializer`.

The `infraattributesprocessor` inside the same pipeline enriches all OTLP signals with
infrastructure tags (from the Tagger) before they reach the exporter. This means the tags
available to `attributes.Translator` via `HostFromAttributesHandler` already include
host-level and container-level attributes injected by the processor.

The full OTel Agent (DDOT, `comp/otelcol/collector` — see
[`comp/otelcol/collector`](../comp/otelcol/collector.md)) uses the same translation path
through the `datadog` exporter factory, but with the standalone `go.mod` variant of this
package so that the module can be published independently for the upstream OTel Collector
Datadog exporter.

When the `comp/otelcol/converter` component is active (see
[`comp/otelcol/converter`](../comp/otelcol/converter.md)), it automatically injects the
`infraattributes` processor into every pipeline that contains a `datadog` exporter. This
means `attributes.Translator` and `metrics.Provider` always receive infrastructure-enriched
data in DDOT deployments without requiring manual config changes.

### In the trace pipeline

`pkg/trace/api/otlp.go` uses `attributes.ContainerMappings`, `HTTPMappings`, and
`SourceFromAttrs` to enrich APM spans with Datadog tag conventions before forwarding them to
the trace writer.

`pkg/trace/transform/transform.go` uses the mapping tables to convert span attributes. See
[`pkg/trace/transform`](trace/transform.md) for the full span conversion API, including
`OtelSpanToDDSpan`, `GetDDKeyForOTLPAttribute`, and how `SetMetaOTLP` routes well-known
Datadog APM convention keys to the correct `pb.Span` struct fields.

### Standalone (OTel Collector Datadog exporter)

Because the sub-packages have their own `go.mod` files, they can be imported directly by the
upstream OpenTelemetry Collector Datadog exporter without taking a dependency on the full
Datadog Agent module.

### Minimal example

```go
// Setup
attrTranslator, _ := attributes.NewTranslator(telemetrySettings)
provider, _ := metrics.NewDefaultTranslator(
    telemetrySettings,
    attrTranslator,
    metrics.WithRemapping(),
    metrics.WithHistogramMode(metrics.HistogramModeDistributions),
)

// Per-batch translation
meta, err := provider.MapMetrics(ctx, otlpMetrics, myConsumer, nil)
if err != nil { ... }
// meta.Languages lists runtime languages found in the batch
```

## Related packages

- [`comp/otelcol/otlp`](../comp/otelcol/otlp.md) — the in-process OTLP ingestion pipeline
  embedded in the core Agent. Its `serializerexporter` sub-package is the primary consumer of
  `otlp/metrics.Provider` (`NewDefaultTranslator`). The exporter creates an
  `attributes.Translator` at startup, calls `provider.MapMetrics` per batch, and hands the
  resulting time series and sketches to `pkg/serializer`. The `infraattributesprocessor` in
  the same pipeline adds infrastructure tags before spans/metrics/logs reach the exporter.
- [`comp/otelcol/collector`](../comp/otelcol/collector.md) — the full OTel Collector component
  used by the Datadog Distribution of OpenTelemetry (DDOT / `cmd/otel-agent`). It wires the
  `datadog` exporter factory which also depends on `pkg/opentelemetry-mapping-go` for
  attribute and metric translation. Because this component runs a user-supplied OTel config
  rather than the fixed in-process pipeline, it can apply `WithRemapping()` and
  `WithOTelPrefix()` options that would collide with native Agent metric names in the
  `comp/otelcol/otlp` path.
- [`pkg/trace/transform`](trace/transform.md) — the OTLP-to-Datadog span conversion layer in
  the trace-agent. It consumes `attributes.ContainerMappings`, `attributes.HTTPMappings`, and
  `attributes.SourceFromAttrs` directly to enrich `pb.Span` objects. The `GetDDKeyForOTLPAttribute`
  helper in `transform` delegates HTTP attribute remapping to
  `attributes.HTTPMappings`. Any change to the mapping tables in `otlp/attributes` therefore
  affects both the metrics and the trace pipelines.
- `pkg/trace/api/otlp.go` — OTLP trace ingestion that calls `transform.OtelSpanToDDSpan`.
  Uses `attributes.ContainerMappings`, `HTTPMappings`, and `SourceFromAttrs` (via `transform`)
  to populate Datadog span tags.
- `pkg/serializer` — downstream consumer of translated Datadog-format metrics and sketches
  produced by `MapMetrics`. Receives `Gauge`/`Count` time series and `*quantile.Sketch` values
  from the `Consumer` implementations inside `serializerexporter`.
- `pkg/util/quantile` — sketch type (`*quantile.Sketch`) used by `SketchConsumer`.
- [`comp/otelcol/converter`](../comp/otelcol/converter.md) — the `ddConverter` confmap converter
  that auto-injects the `infraattributes` processor into DDOT pipelines. When active, it ensures
  that every `datadog` exporter receives infrastructure-enriched OTLP data before
  `attributes.Translator` resolves the hostname. It also backfills `api.key` and `api.site` for
  the `datadog` exporter from `datadog.yaml`, which affects the HTTP destination of payloads
  produced by `metrics.Provider.MapMetrics`.
