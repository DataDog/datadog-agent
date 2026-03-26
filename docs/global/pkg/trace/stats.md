> **TL;DR:** `pkg/trace/stats` aggregates APM RED metrics (hits, errors, durations, DDSketch latency distributions) from every span regardless of sampling decisions, bucketing them into fixed time windows and sending them independently to the Datadog backend for accurate p99 latency data without 100% trace retention.

# pkg/trace/stats

## Purpose

`pkg/trace/stats` computes APM statistics (hit counts, error counts, total
duration, and latency distributions as DDSketches) from the stream of spans the
agent receives. These stats are aggregated into fixed-width time buckets and
sent to the Datadog backend independently of trace sampling — meaning every span
contributes to statistics even if its parent trace is dropped. This provides
accurate p50/p75/p99 latency data without requiring 100% trace retention.

## Key elements

### Aggregation model

Statistics are keyed on two orthogonal dimensions:

- **`BucketsAggregationKey`** — per-span dimensions: `service`, `name`,
  `resource`, `type`, `span.kind`, HTTP status code, gRPC status code, peer
  tags hash, span-derived primary tags hash, `synthetics` flag, `is_trace_root`.
- **`PayloadAggregationKey`** — per-payload dimensions: `env`, `hostname`,
  `version`, `container_id`, `git_commit_sha`, `image_tag`, `lang`,
  `process_tags_hash`, base service name.

The combination of both is `Aggregation`, which is the map key inside a
`RawBucket`.

### Types

| Type | Description |
|------|-------------|
| `Concentrator` | Main goroutine-based component. Accepts `Input` values via `Add()`, delegates span bucketing to `SpanConcentrator`, and periodically calls `Flush()` to write completed buckets to the `Writer`. Flushes on every `BucketInterval` tick (default 10 s). |
| `SpanConcentrator` | Core bucketing logic. Maintains a `map[int64]*RawBucket` keyed by aligned bucket start timestamp. Eligible spans are placed into the bucket whose end time falls within. Keeps `bufferLen=2` buckets before flushing to tolerate clock skew. |
| `RawBucket` | A single time window. Holds a `map[Aggregation]*groupedStats`. Exported to `map[PayloadAggregationKey]*pb.ClientStatsBucket` via `Export()`. |
| `groupedStats` | Accumulates `hits`, `topLevelHits`, `errors`, `duration` (floats for weighted accumulation) plus two DDSketches: `okDistribution` and `errDistribution` (1% relative accuracy, 2,048 bins). |
| `StatSpan` | Lightweight, immutable span extracted from `pb.Span` or `idx.InternalSpan`. Carries only the fields needed for stats: service, resource, name, type, error flag, parent ID, timestamps, peer tags, span kind, status codes. |
| `SpanConcentratorConfig` | Config for `SpanConcentrator`: `ComputeStatsBySpanKind` and `BucketInterval`. |
| `ClientStatsAggregator` | Secondary aggregator for *client-computed* stats (payloads sent by tracers that pre-aggregate their own stats). Receives `pb.ClientStatsPayload` on its `In` channel, merges colliding buckets (2 s buckets, 20 s old-bucket retention), and forwards de-duplicated payloads to the `Writer`. It intentionally staggers its flush timestamps relative to the Concentrator so agent counts never collide on the same second in the backend. |
| `Writer` (interface) | `Write(*pb.StatsPayload)` — implemented by `writer.DatadogStatsWriter`. |
| `Input` / `InputV1` | Batch of `ProcessedTrace` values plus container/process metadata passed to `Concentrator.Add`. |

### Key functions

| Function | Description |
|----------|-------------|
| `NewConcentrator(conf, writer, now, statsd)` | Creates and returns a `Concentrator`. Call `Start()` to begin the flush loop. |
| `NewSpanConcentrator(cfg, now)` | Creates a standalone `SpanConcentrator` (also used by `dd-trace-go`). |
| `(sc *SpanConcentrator) NewStatSpanFromPB(span, peerTags, derivedTags)` | Builds a `StatSpan` from a `pb.Span`. Returns `(nil, false)` for spans that are not top-level, not measured, and not of an eligible kind. |
| `(sc *SpanConcentrator) NewStatSpanWithConfig(cfg)` | General-purpose `StatSpan` factory; preferred over the deprecated positional `NewStatSpan`. |
| `(sc *SpanConcentrator) AddSpan(s, aggKey, containerID, ctags, origin)` | Public API for external callers (e.g. `dd-trace-go`) to inject spans directly. |
| `(sc *SpanConcentrator) Flush(now, force)` | Returns and removes all buckets that are old enough (`> bufferLen * bsize` ns ago). `force=true` flushes everything (used on shutdown). |
| `(rb *RawBucket) Export()` | Serializes a bucket into `map[PayloadAggregationKey]*pb.ClientStatsBucket` with DDSketch distributions encoded as protobuf bytes. |
| `NewClientStatsAggregator(conf, writer, statsd)` | Creates the aggregator. Start with `Start()`, stop with `Stop()`. |
| `alignTs(ts, bsize)` | Truncates a nanosecond timestamp to the nearest bucket boundary. |
| `KindsComputed` | `map[string]struct{}` listing the span kinds eligible for stats when `ComputeStatsBySpanKind` is enabled: `server`, `consumer`, `client`, `producer`. |

### Span eligibility for stats

A span is included in stats if it satisfies any of:
1. It is a **top-level span** (has the `_top_level` metric set).
2. It is **measured** (has the `_dd.measured` metric set).
3. `ComputeStatsBySpanKind` is enabled and the span's `span.kind` is in
   `KindsComputed`.

Partial snapshot spans (`_dd.partial_version` metric set) are always excluded.

### Peer tag and span-derived primary tag aggregation

When `PeerTagsAggregation` is enabled, client/producer/consumer spans have
additional tag keys (e.g. `peer.service`, `db.instance`) included in their
aggregation key via `matchingPeerTags`. This allows the backend to distinguish
metrics by the downstream peer entity rather than the calling service alone.

`SpanDerivedPrimaryTagKeys` provides a similar mechanism for custom tag
dimensions to be promoted into the aggregation key.

## Usage

The `Concentrator` and `ClientStatsAggregator` are instantiated in
`pkg/trace/agent.NewAgent` and wired to the same `DatadogStatsWriter`:

```go
// pkg/trace/agent/agent.go (simplified)
statsWriter := writer.NewStatsWriter(conf, ...)
a.Concentrator          = stats.NewConcentrator(conf, statsWriter, time.Now(), statsd)
a.ClientStatsAggregator = stats.NewClientStatsAggregator(conf, statsWriter, statsd)
```

During trace processing, the agent calls `Concentrator.Add(input)` for each
decoded payload. `ClientStatsAggregator.In <- payload` is used for payloads
where the tracer set `ClientComputedStats = true`.

The `SpanConcentrator` is also used directly by `dd-trace-go` (via
`NewSpanConcentrator` + `AddSpan`) so that in-process tracing libraries can
contribute stats without going through the full agent pipeline.

---

## Cross-references

| Topic | Document |
|---|---|
| Full pipeline overview showing where `Concentrator.Add` is called | [`pkg/trace`](trace.md) |
| Sampling decisions that run in parallel (stats are computed over all spans regardless of sampling outcome) | [`pkg/trace/sampler`](sampler.md) |
| `DatadogStatsWriter` — receives `StatsPayload` from `Concentrator` and `ClientStatsAggregator` | [`pkg/trace/writer`](writer.md) |
| `AgentConfig` fields governing stats computation (`BucketInterval`, `PeerTagsAggregation`, `ComputeStatsBySpanKind`, `PeerTags`, `SpanDerivedPrimaryTagKeys`) | [`pkg/trace/config`](config.md) |
| OTel stats bridging — `OTLPTracesToConcentratorInputs` produces `stats.Input` from OTLP traces | [`pkg/trace/otel`](otel.md) |

### Stats vs. sampling

A common point of confusion: the `Concentrator` and the sampler subsystem both
receive every span, but they serve different purposes:

- `Concentrator` computes aggregate RED metrics over **100 % of spans** and
  sends them to `DatadogStatsWriter → /api/v0.2/stats`.
- Samplers decide which **full trace chunks** to forward to
  `TraceWriter → /api/v0.2/traces`.

Even if a trace is dropped by all samplers, its spans still contribute to stats
buckets, guaranteeing accurate p99 latency data without 100 % trace retention.

### OTel ingestion

`pkg/trace/otel/stats` provides `OTLPTracesToConcentratorInputs` which converts
`ptrace.Traces` payloads into `[]stats.Input` values that are passed directly to
`Concentrator.Add`. This allows the OTel Collector Datadog exporter to compute
APM stats without going through the full `pb.TracerPayload` conversion:

```go
inputs := otelstats.OTLPTracesToConcentratorInputs(traces, conf, containerTagKeys, peerTagKeys)
for _, inp := range inputs {
    concentrator.Add(inp)
}
```
