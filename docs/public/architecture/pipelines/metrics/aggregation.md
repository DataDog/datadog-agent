# Metric aggregation

-----

Metric aggregation is the stage of the metrics pipeline that turns raw samples — DogStatsD packets and check sender calls — into aggregated time series and sketches ready for serialization. Its central object is the `AgentDemultiplexer`, which owns every in-process buffer between metric intake and the [serializer](serialization.md): sharded time samplers for DogStatsD traffic, one check sampler per running check instance, and an optional pass-through pipeline for samples that carry their own timestamps. This page covers everything up to the point where a flush hands `Serie` and `SketchSeries` streams to the serializer; see [Metric serialization](serialization.md) for what happens next and [Forwarder and resilience](../forwarder.md) for the network egress.

The aggregator exists because the backend prices and stores metrics per point, not per sample. A process emitting `my.counter` 50,000 times per second must leave the Agent as one point per 10-second bucket per unique tag set. Everything in this subsystem — context interning, bucketing, sketch accumulation, expiry — serves that reduction while keeping memory bounded on hosts with high tag cardinality.

## Key packages and files

| Path | Purpose |
|---|---|
| [`pkg/aggregator/README.md`](<<<SRC>>>/pkg/aggregator/README.md) | Legacy but still-accurate high-level diagram of the DogStatsD/checks → samplers → serializer flow |
| [`pkg/aggregator/demultiplexer.go`](<<<SRC>>>/pkg/aggregator/demultiplexer.go) | `Demultiplexer` interface, flush trigger types, `createIterableMetrics`, DogStatsD worker/pipeline count heuristics |
| [`pkg/aggregator/demultiplexer_agent.go`](<<<SRC>>>/pkg/aggregator/demultiplexer_agent.go) | `AgentDemultiplexer`: construction, run/flush loops, shutdown sequence |
| [`pkg/aggregator/aggregator.go`](<<<SRC>>>/pkg/aggregator/aggregator.go) | `BufferedAggregator`: check sampler registry, events/service-checks/orchestrator queues, run loop |
| [`pkg/aggregator/sender.go`](<<<SRC>>>/pkg/aggregator/sender.go) | `checkSender`, the concrete implementation of the check-facing `Sender` API |
| [`pkg/aggregator/sender/sender.go`](<<<SRC>>>/pkg/aggregator/sender/sender.go) | `sender.Sender` and `sender.SenderManager` interfaces consumed by the collector |
| [`pkg/aggregator/time_sampler.go`](<<<SRC>>>/pkg/aggregator/time_sampler.go) | `TimeSampler`: 10 s bucketing, counter zero-fill, dedup by `SerieSignature` |
| [`pkg/aggregator/time_sampler_worker.go`](<<<SRC>>>/pkg/aggregator/time_sampler_worker.go) | `timeSamplerWorker`: one goroutine event loop per DogStatsD pipeline shard |
| [`pkg/aggregator/check_sampler.go`](<<<SRC>>>/pkg/aggregator/check_sampler.go) | `CheckSampler`: per-check aggregation, histogram-bucket → sketch conversion |
| [`pkg/aggregator/context_resolver.go`](<<<SRC>>>/pkg/aggregator/context_resolver.go) | `Context`, `contextResolver`, timestamp- and count-based expiry wrappers, tag filtering |
| [`pkg/aggregator/ckey/key.go`](<<<SRC>>>/pkg/aggregator/ckey/key.go) | 64-bit murmur3 `ContextKey` / `TagsKey` generation |
| [`pkg/aggregator/internal/tags/store.go`](<<<SRC>>>/pkg/aggregator/internal/tags/store.go) | Ref-counted interning store for tag slices |
| [`pkg/aggregator/sketch_map.go`](<<<SRC>>>/pkg/aggregator/sketch_map.go) | `sketchMap`: timestamp → context key → `quantile.Agent` accumulation for distributions |
| [`pkg/aggregator/no_aggregation_stream_worker.go`](<<<SRC>>>/pkg/aggregator/no_aggregation_stream_worker.go) | Streaming pipeline for samples with explicit client timestamps |
| [`pkg/metrics/metric_sample.go`](<<<SRC>>>/pkg/metrics/metric_sample.go) | `MetricSample` struct and the `MetricType` enum |
| [`pkg/metrics/context_metrics.go`](<<<SRC>>>/pkg/metrics/context_metrics.go) | `ContextMetrics`: context key → `Metric` map, instantiates the concrete metric per type |
| [`pkg/metrics/check_metrics.go`](<<<SRC>>>/pkg/metrics/check_metrics.go) | `CheckMetrics`: expiry semantics for check metrics, stateful-metric grace period |
| [`pkg/util/quantile/agent.go`](<<<SRC>>>/pkg/util/quantile/agent.go) | Insert-optimized DDSketch (`quantile.Agent`) used for distributions |
| [`comp/aggregator/demultiplexer/impl/demultiplexer.go`](<<<SRC>>>/comp/aggregator/demultiplexer/impl/demultiplexer.go) | Fx wiring: hostname resolution, options from config, lifecycle Stop hook |
| [`comp/dogstatsd/server/impl/batch.go`](<<<SRC>>>/comp/dogstatsd/server/impl/batch.go) | DogStatsD batcher: shards samples per pipeline and feeds the demultiplexer |
| [`pkg/hosttags/host_tag_provider.go`](<<<SRC>>>/pkg/hosttags/host_tag_provider.go) | Caches host tags for `expected_tags_duration`, appended to series at flush |

## Topology

The demultiplexer is built once per process by the Fx component in [`comp/aggregator/demultiplexer`](<<<SRC>>>/comp/aggregator/demultiplexer/impl/demultiplexer.go), which resolves the hostname, derives `AgentDemultiplexerOptions` from config, and calls `aggregator.InitAndStartAgentDemultiplexer` in [`demultiplexer_agent.go`](<<<SRC>>>/pkg/aggregator/demultiplexer_agent.go). Construction wires three parallel ingestion paths around one shared serializer:

```text
                         AgentDemultiplexer
                         ==================

 DogStatsD batcher ──► timeSamplerWorker #0 ─► TimeSampler ─┐
 (shards by context     timeSamplerWorker #1 ─► TimeSampler ─┤ 10 s buckets
  hash)                 ...        (dogstatsd_pipeline_count)│
                                                             │  flush (15 s)
 check senders ─────► BufferedAggregator ────────────────────┤
 (Gauge/Rate/...,       ├─ CheckSampler per check instance   ├─► IterableSeries ──► Serializer
  Commit per run)       ├─ events / service checks buffers   ├─► IterableSketches ─► Serializer
                        └─ orchestrator / event-platform     │
                                                             │
 timestamped samples ─► noAggregationStreamWorker(s) ────────┘  (bypasses flush,
 (dogstatsd |T ext.)    (own Serializer instance each)           streams directly)
```

1. **Sharded time samplers.** `statsdPipelinesCount` `TimeSampler` instances (from `dogstatsd_pipeline_count`, or auto-derived from vCPU count when `dogstatsd_pipeline_autoadjust: true` — see `GetDogStatsDWorkerAndPipelineCount` in [`demultiplexer.go`](<<<SRC>>>/pkg/aggregator/demultiplexer.go)) each run inside their own `timeSamplerWorker` goroutine with a private context resolver and a private `tags.Store` shard, so shards never contend on locks.
1. **The `BufferedAggregator`.** A single goroutine (`NewBufferedAggregator` in [`aggregator.go`](<<<SRC>>>/pkg/aggregator/aggregator.go)) that owns one `CheckSampler` per check instance plus the buffered queues for events, service checks, orchestrator metadata/manifests, and the pass-through for event-platform events.
1. **The no-aggregation pipeline.** Optional `noAggregationStreamWorker` goroutines that convert client-timestamped samples straight into single-point series and stream them into their own dedicated `Serializer` instances, skipping bucketing entirely.

The rest of the Agent only sees the `Demultiplexer` interface ([`demultiplexer.go`](<<<SRC>>>/pkg/aggregator/demultiplexer.go)): `AggregateSample(s)`, `SendSamplesWithoutAggregation`, `ForceFlushToSerializer`, `GetMetricSamplePool`, plus the `sender.SenderManager` surface (`GetSender`/`SetSender`/`DestroySender`/`GetDefaultSender`) that the [check collector](../../checks/collector.md) uses.

## Intake surfaces

### DogStatsD: batcher → time sampler workers

The DogStatsD server's batcher ([`comp/dogstatsd/server/impl/batch.go`](<<<SRC>>>/comp/dogstatsd/server/impl/batch.go)) shards each parsed sample by context hash across the pipeline shards and calls `demux.AggregateSamples(TimeSamplerID(shard), batch)`. Batches are slices borrowed from the shared `metrics.MetricSamplePool` ([`metric_sample_pool.go`](<<<SRC>>>/pkg/metrics/metric_sample_pool.go), batch size 32) and returned to the pool by the consuming worker. Everything upstream of `AggregateSamples` — listeners, packet parsing, origin resolution — belongs to [DogStatsD internals](../../dogstatsd/internals.md).

`timeSamplerWorker.run()` ([`time_sampler_worker.go`](<<<SRC>>>/pkg/aggregator/time_sampler_worker.go)) is a single-goroutine event loop multiplexing the samples channel (capacity `aggregator_buffer_size`), flush triggers, filterlist updates, and contexts-dump requests. By design it processes either samples or a flush, never both concurrently, which is what makes the sampler internals lock-free — and also what lets a slow serializer briefly backpressure DogStatsD through the sample channels (watch the `aggregator.channel_size` telemetry).

For each sample, `TimeSampler.sample()` ([`time_sampler.go`](<<<SRC>>>/pkg/aggregator/time_sampler.go)):

1. Resolves the context via `timestampContextResolver.trackContext`, applying the tag-stripping filterlist to counter and distribution types (results cached in the fixed-capacity `tagFilterCache`).
1. Routes distribution samples into `sketchMap[bucketStart][contextKey]`, inserting into a `quantile.Agent` sketch weighted by `1/SampleRate`.
1. Routes everything else into `metricsByTimestamp[bucketStart]` via `ContextMetrics.AddSample` ([`context_metrics.go`](<<<SRC>>>/pkg/metrics/context_metrics.go)), which lazily instantiates the right `Metric` implementation (Gauge, Counter with interval normalization, Set, Histogram, and so on).
1. If the sample carries `Timestamp > 0` (a late sample falling back to the time sampler when the no-agg pipeline is disabled), that timestamp overrides the wall-clock bucket.

### Checks: sender API → BufferedAggregator → CheckSampler

Checks obtain a `sender.Sender` through `demux.GetSender(checkID)`. The concrete `checkSender` ([`sender.go`](<<<SRC>>>/pkg/aggregator/sender.go)) is created per check ID by the `checkSenderPool`, which simultaneously registers a `CheckSampler` in the aggregator by pushing a `registerSampler` item onto the aggregator's `checkItems` channel.

Every metric call — `Gauge`, `Rate`, `Count`, `MonotonicCount`, `Counter` (deprecated), `Histogram`, `Historate`, `Distribution`, `GaugeWithTimestamp`, `CountWithTimestamp` — builds a `metrics.MetricSample` (default `SampleRate: 1`), appends the check's custom tags (`tags:` from its config), applies the default hostname unless the check opted out via `DisableDefaultHostname`, and sends it as a `senderMetricSample` over `checkItems`. `HistogramBucket` sends pre-aggregated histogram data as a `senderHistogramBucket` item instead. `ServiceCheck` and `Event` go to dedicated channels; `EventPlatformEvent` bypasses aggregation entirely and is forwarded immediately from the aggregator loop to the [event platform forwarder](../event-platform.md).

The crucial call is `Commit()`. It sends a sentinel item that makes the aggregator invoke `CheckSampler.commit(now)`, which materializes series from the accumulated `CheckMetrics` and sketch map into the sampler's `series`/`sketches` slices — this is when context resolution, metric-name filterlist filtering, and `SourceTypeName: "System"` stamping happen. **A check that never calls `Commit()` never sends anything**, and committed series only leave the Agent on the next flush.

`CheckSampler` internals worth knowing ([`check_sampler.go`](<<<SRC>>>/pkg/aggregator/check_sampler.go)):

1. Context expiry is count-based, not time-based: a `countBasedContextResolver` expires a context after `check_sampler_bucket_commits_count_expiry` (default 2) commits without seeing it.
1. `CheckMetrics` ([`check_metrics.go`](<<<SRC>>>/pkg/metrics/check_metrics.go)) removes stateless metrics immediately on context expiry but keeps stateful ones (Rate, MonotonicCount — anything where `isStateful()` is true) for `check_sampler_stateful_metric_expiration_time` (25 h), so a check that runs intermittently doesn't lose the reference value its next delta depends on.
1. Monotonic histogram buckets are converted to deltas against the previous flush value per context (or per context-and-bounds when the check reports Prometheus-style `MultipleBuckets`); deltas are inserted into the sketch by linear interpolation across DDSketch bins (`sketchMap.insertInterp` → `quantile.Agent.InsertInterpolate`). `+Inf` upper bounds are inserted at their lower bound.
1. Deregistration is a deliberate two-phase dance (see the long comment on `deregisterSampler` in [`aggregator.go`](<<<SRC>>>/pkg/aggregator/aggregator.go)): the sampler is only deleted after the *next flush* following deregistration, so in-flight metrics from a check's final run are not lost and a check rescheduled within one flush interval reuses its sampler.

### No-aggregation pipeline: client-timestamped samples

Samples that already carry a timestamp (the DogStatsD `|T<ts>` protocol extension) are routed by the DogStatsD batcher to `SendSamplesWithoutAggregation`, feeding a shared channel (capacity `dogstatsd_queue_size`) drained by `noAggregationStreamWorker` goroutines ([`no_aggregation_stream_worker.go`](<<<SRC>>>/pkg/aggregator/no_aggregation_stream_worker.go)). This path is DogStatsD-only — check senders' `*WithTimestamp` calls go through the regular check path as `MetricWithTimestamp` metrics.

Each worker enriches tags via the [tagger](../../containers/tagger.md), converts each sample directly into a single-point `Serie` with `Interval: 10` (counter and rate values are divided by 10 to become per-second rates, mirroring the time sampler), and streams into its **own** `Serializer` instance. Payloads are force-flushed after `dogstatsd_no_aggregation_pipeline_batch_size` (2048) samples or roughly 2 seconds of silence. The trade-offs of skipping the sampler: only gauge, counter, and rate types are supported (everything else — including the check `Count` type — is dropped with a throttled warning), and there is no context interning, no counter zero-fill, and no deduplication. The pipeline is on by default (`dogstatsd_no_aggregation_pipeline: true`) but only activates in binaries that opted in via `WithDogstatsdNoAggregationPipelineConfig()`.

## Contexts, keys, and the tags store

A **context** is a unique (metric name, hostname, tag set) combination — the unit of aggregation, expiry, and cardinality accounting. `ckey.KeyGenerator.GenerateWithTags2` ([`ckey/key.go`](<<<SRC>>>/pkg/aggregator/ckey/key.go)) hashes name, host, and tags into a 64-bit murmur3 `ContextKey` without allocating; duplicate tags are deduplicated in place during hashing.

Tagger-provided tags (origin detection) and client-provided tags are hashed into two separate `TagsKey` spaces, and the context key combines both. Identical final tag sets reached via different tagger/client splits still collapse into one context, but the tags store interns them as two entries. Keeping the halves separate is what enables per-origin context telemetry (`telemetry.dogstatsd_origin`).

The `contextResolver` ([`context_resolver.go`](<<<SRC>>>/pkg/aggregator/context_resolver.go)) maps `ContextKey` to a `Context` holding name, host, metric type, `NoIndex` flag, source, and two `*tags.Entry` pointers into the ref-counted [`tags.Store`](<<<SRC>>>/pkg/aggregator/internal/tags/store.go) (`aggregator_use_tags_store`, default true). Interning means ten thousand contexts sharing the tag slice `["env:prod", "service:web"]` store it once. `tags.Store.Shrink()` runs after each flush from within the worker and aggregator loops, so entry reclamation never races sample processing.

Two expiry disciplines wrap the resolver:

| Resolver | Used by | Expiry rule |
|---|---|---|
| `timestampContextResolver` | Time samplers | Contexts idle for `dogstatsd_context_expiry_seconds` (20 s) are removed; counter contexts live an extra `dogstatsd_expiry_seconds` (300 s) so zero-filling works across the longer window |
| `countBasedContextResolver` | Check samplers | Expires after `check_sampler_bucket_commits_count_expiry` (2) commits without the context appearing |

Tag stripping via `metric_tag_filterlist` rewrites the context key post-filter (cached in `tagFilterCache`, capacity `aggregator_tag_filter_cache_capacity`); by default the filterlist is only active when `data_plane.enabled` is true because of the `metric_tag_filterlist_adp_only` gate.

## Metric types and flush semantics

The `MetricType` enum lives in [`metric_sample.go`](<<<SRC>>>/pkg/metrics/metric_sample.go); the concrete per-type implementations are the small files in [`pkg/metrics`](<<<SRC>>>/pkg/metrics). Note the collapse at flush time: however rich the intake type, every serie leaves as one of only three `APIMetricType` values (gauge, rate, count), or as a sketch.

| Sender call / DogStatsD type | Aggregation within a bucket | Flushed as |
|---|---|---|
| `Gauge` / `g` | Last value wins | gauge |
| `Rate` | (v2−v1)/(t2−t1) across successive samples; negative deltas dropped | gauge |
| `Count` | Sum within the flush window | count |
| `MonotonicCount` | Delta versus previous value; first sample dropped unless `FlushFirstValue` | count |
| `Counter` / `c` (deprecated for checks) | Sum × (1/SampleRate), normalized by the 10 s bucket interval | rate |
| `Histogram` / `h`, `ms` | Sorted weighted samples → `histogram_aggregates` (max/median/avg/count) plus `histogram_percentiles` (p95) sub-series, distinguished by name suffix | gauges + rate (`.count`) |
| `Historate` | Per-context Rate fed into a Histogram | as histogram |
| `Set` / `s` | Cardinality of unique string values | gauge |
| `Distribution` / `d` | DDSketch accumulation in `quantile.Agent` | sketch |
| `GaugeWithTimestamp` / `CountWithTimestamp` | None — passed through as `MetricWithTimestamp` | gauge / count |

Semantics that regularly surprise people:

1. **DogStatsD `Counter` becomes a per-second rate** (value ÷ 10), while `Count` is submitted as a raw count. The no-aggregation pipeline replicates the ÷10 for counters.
1. **Counters are zero-filled**: a counter context that received no samples still emits 0 for up to `dogstatsd_expiry_seconds` (5 minutes) after its last real sample, so stopping an app does not immediately stop its counter series.
1. **`Rate` and `MonotonicCount` silently swallow their first flush** (`NoSerieError` internally) and negative deltas — a restarted upstream counter loses one interval.

### Distributions and sketches

Distribution samples accumulate into `quantile.Agent` ([`pkg/util/quantile/agent.go`](<<<SRC>>>/pkg/util/quantile/agent.go)), an insert-optimized DDSketch variant: incoming values are mapped to bin keys (gamma configured in [`config.go`](<<<SRC>>>/pkg/util/quantile/config.go) with relative accuracy eps = 1/128, a 4096-bin limit, and a 1e-9 minimum value), buffered in a 512-key array, and merged into a sparse store when the buffer fills. `Finish()` deep-copies the sketch so the sampler can keep accumulating while the flushed copy is serialized. Basic stats (cnt/min/max/sum/avg) ride alongside the bins and are emitted per `SketchPoint` ([`sketch_series.go`](<<<SRC>>>/pkg/metrics/sketch_series.go)). Histograms can additionally be mirrored into distributions with `histogram_copy_to_distribution`.

## The flush cycle

`AgentDemultiplexer.flushLoop()` ticks every `FlushInterval` — `DefaultFlushInterval` is 15 s, overridable per binary via `demultiplexerimpl.WithFlushInterval`. Each flush (`flushToSerializer`):

1. Builds an `IterableSeries` and `IterableSketches` pair via `createIterableMetrics` ([`demultiplexer.go`](<<<SRC>>>/pkg/aggregator/demultiplexer.go)). The per-serie callback appends **host tags** from [`hosttags.HostTagProvider`](<<<SRC>>>/pkg/hosttags/host_tag_provider.go) — non-empty only during the first `expected_tags_duration` after Agent start — and records huge-tagset telemetry. Buffer and channel sizes come from `aggregator_flush_metrics_and_serialize_in_parallel_{buffer,chan}_size` (4000/200).
1. Calls `metrics.Serialize` ([`iterable_metrics.go`](<<<SRC>>>/pkg/metrics/iterable_metrics.go)): the producer runs in the flush goroutine and sequentially triggers each time-sampler worker (through its `flushChan`, waiting for each), then the `BufferedAggregator`. Two consumer goroutines concurrently run `serializer.SendIterableSeries` and `serializer.SendSketch` — serialization and compression overlap sampler flushing, with series streaming through a `BufferedChan` while later samplers are still draining. Samplers are flushed sequentially on purpose: `IterableSeries` is not safe for concurrent producers.
1. `BufferedAggregator.Flush` drains every check sampler's committed series and sketches into the same sinks, appends recurrent series (for example `datadog.djm.agent_host` when `djm_config.enabled`), the `datadog.<flavor>.running` gauge (tagged with `version:` and, if set, `config_id:`), and the HA-agent state metric; it then flushes **service checks** (always adding `datadog.agent.up` OK) and **events**, serialized in a separate goroutine unless the caller asked to wait (shutdown and manual flushes do).
1. Flush telemetry is recorded: the `aggregator.flush` counters and the expvar `aggregator` map that `agent status` renders (see [Status, health, and telemetry](../../operations/introspection.md)).

`TimeSampler.flush` only drains **closed** buckets: a bucket whose `bucketStart + 10s` is still in the future stays open unless `forceFlushAll` (used at shutdown when `dogstatsd_flush_incomplete_buckets: true`). Because buckets are 10 s but the flush ticker is 15 s, a DogStatsD point can wait up to about 25 s before leaving the Agent — timestamped no-agg metrics skip this entirely. During bucket flush, series from the same context are merged and deduplicated by `SerieSignature` (metric type + name suffix), which is how one histogram context yields `.max`, `.avg`, `.count`, `.95percentile` series sharing a context; the metric-name filterlist is applied after aggregation, precisely so those final suffixed names can be filtered.

`ForceFlushToSerializer` shares `flushToSerializer` with the ticker via the flush channel — the code carries an explicit warning that callers outside the flush loop risk deadlock with the parallel-serialization design, which is why the manual trigger goes through the same serialized path.

On `Stop()` the demultiplexer optionally drains in-flight DogStatsD samples, performs one final flush with `waitForSerializer=true`, and sends the staged "Agent Shutdown" event through a direct bounded forwarder path (`SendAgentShutdownEvent`, 1 s timeout), all within `aggregator_stop_timeout` (2 s).

## Configuration

| Key | Default | Effect |
|---|---|---|
| `aggregator_buffer_size` | 100 | Capacity of aggregator input channels and sampler sample channels |
| `aggregator_stop_timeout` | 2 s | Budget for the final flush on shutdown |
| `aggregator_use_tags_store` | true | Ref-counted tag interning; disabling also force-disables DogStatsD origin telemetry |
| `aggregator_tag_filter_cache_capacity` | 1000 | Post-filter context-key cache per resolver |
| `aggregator_flush_metrics_and_serialize_in_parallel_chan_size` / `_buffer_size` | 200 / 4000 | `IterableSeries` channel and buffer sizing |
| `dogstatsd_pipeline_count` / `dogstatsd_pipeline_autoadjust` | 1 / false | Number of time-sampler shards |
| `dogstatsd_context_expiry_seconds` | 20 | Idle context eviction in time samplers |
| `dogstatsd_expiry_seconds` | 300 | Extra counter context lifetime; zero-fill window |
| `dogstatsd_flush_incomplete_buckets` | false | Flush open 10 s buckets on shutdown |
| `dogstatsd_no_aggregation_pipeline` | true | Pass-through pipeline for timestamped samples |
| `dogstatsd_no_aggregation_pipeline_batch_size` | 2048 | Samples per no-agg payload before a forced flush |
| `dogstatsd_no_aggregation_pipeline_workers_count` | 1 | No-agg workers (each with its own serializer) |
| `dogstatsd_queue_size` | 1024 | Shared no-agg input channel capacity |
| `histogram_aggregates` | max, median, avg, count | Histogram sub-series to emit |
| `histogram_percentiles` | 0.95 | Histogram percentile sub-series |
| `histogram_copy_to_distribution` (`_prefix`) | false | Mirror histogram samples into distributions |
| `check_sampler_bucket_commits_count_expiry` | 2 | Commits without a context before check-context expiry |
| `check_sampler_expire_metrics` | true | Enable check metric expiry at all |
| `check_sampler_stateful_metric_expiration_time` | 25 h | Grace period for stateful (rate/monotonic) check metrics |
| `check_sampler_allow_sketch_bucket_reset` | true | Handle monotonic histogram-bucket resets |
| `expected_tags_duration` | 0 | How long after start host tags are attached to every serie and sketch |
| `metric_tag_filterlist` / `metric_tag_filterlist_adp_only` | [] / true | Strip tags on counters/distributions (by default only when `data_plane.enabled`) |
| `telemetry.dogstatsd_origin` | false | Per-origin context-count series (requires the tags store) |
| `basic_telemetry_add_container_tags` | false | Tag the Agent's own telemetry series with its container tags |

## Deployment-mode differences

1. **Host Agent (default)**: everything above; automatic flush every 15 s.
1. **`agent check` CLI**: the demultiplexer is built with `WithFlushInterval(0)` — no automatic flush; the command calls `ForceFlushToSerializer` manually after the check run ([`pkg/cli/subcommands/check/command.go`](<<<SRC>>>/pkg/cli/subcommands/check/command.go)).
1. **Standalone DogStatsD binary** (`cmd/dogstatsd`): the same demultiplexer, opted into the no-agg pipeline; no check senders in practice.
1. **Serverless**: `cmd/serverless-init` builds the demultiplexer with a custom flush interval and `WithContinueOnMissingHostname()`; the Lambda extension layers its own metric path in `pkg/serverless/metrics` on top of the same aggregator primitives.
1. **Cluster Agent**: uses the same demultiplexer for its own telemetry and [cluster checks](../../containers/cluster-checks.md).
1. **HA Agent / MRF / Heroku**: extra recurrent series (`datadog.agent.ha_agent.running`), extra serializer destinations, or a renamed flavor (`datadog.heroku_agent.running`) — all downstream of aggregation; see [Metric serialization](serialization.md) for the multi-destination fan-out.
1. **OS platforms**: no divergence inside this subsystem — DogStatsD transports (UDP, UDS, Windows named pipe) differ upstream of the demultiplexer.

## IPC and introspection

The aggregation pipeline itself is in-process Go channels; it owns no sockets. Ingestion arrives via the DogStatsD server (UDP 8125 / UDS / named pipe) and in-process check senders; egress is the serializer/forwarder. Two introspection surfaces exist:

1. `POST /dogstatsd-contexts-dump` on the Agent command API (port 5001, Bearer-token auth), served by [`comp/aggregator/demultiplexerendpoint`](<<<SRC>>>/comp/aggregator/demultiplexerendpoint/impl/endpoint.go): dumps every live context in every time sampler to `<run_path>/dogstatsd_contexts.json.zstd` and returns the path. Used by [flares](../../operations/flare.md) and cardinality debugging.
1. Expvars on the expvar port (5000): the `aggregator` and `no_aggregation` maps, including the `Flush`/`FlushCount` circular buffers that back `agent status`.

## Gotchas

1. **Two cadences, not one**: 10 s buckets, 15 s flush ticker. Worst-case DogStatsD latency to egress is ~25 s. Only closed buckets flush.
1. **Check series leave one flush after `Commit()`** — commit materializes, flush drains. Metrics sent after `DestroySender` are dropped with only a debug log.
1. **Sampler event loops are strictly serial** (samples or flush, never both); serializer slowness backpressures intake through channel fill.
1. **`NoIndex` series** (origin-detection internal metrics and similar) survive aggregation but are only representable in the v2+ serialization formats; the legacy v1 JSON path drops them.
1. **Host tags on metrics are temporary**: only within `expected_tags_duration` of Agent start; afterward the provider returns nil forever. Persistent host tags reach the backend through host metadata instead.
1. The **anomaly-detection observer** (`comp/anomalydetection`, gated on `anomaly_detection.metrics.enabled`) mirrors every raw sample from time samplers, no-agg workers, and check samplers — worth knowing when tracing where a sample goes.
1. `telemetry.dogstatsd_origin` is silently force-disabled when `aggregator_use_tags_store: false`.
