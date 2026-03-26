# Package `pkg/aggregator`

## Purpose

The `aggregator` package is the central hub that receives raw metric samples from two sources — DogStatsD and checks (Python/Go) — and converts them into fully aggregated, serializable payloads that are forwarded to the Datadog backend.

It sits between the collection layer (checks, DogStatsD server) and the serialization layer (`pkg/serializer`). Its main responsibilities are:

- **Buffering** incoming metrics, events, and service checks from concurrent producers.
- **Sampling** metrics over time into buckets (for DogStatsD) or per check run (for checks), computing derived types such as rates, histograms, and monotonic counts.
- **Flushing** accumulated data to the serializer at a configurable interval (default 15 seconds), which then forwards it to the intake.
- **Routing** special payloads (events, service checks, orchestrator metadata, event-platform events) to their respective forwarders.

### Related documentation

| Document | What it covers |
|---|---|
| [`pkg/aggregator/sender`](sender.md) | `Sender` and `SenderManager` interfaces used by checks to submit metrics |
| [`pkg/aggregator/ckey`](ckey.md) | `ContextKey` hashing — how the aggregator keys metrics to a context |
| [`pkg/metrics`](../metrics/metrics.md) | `MetricSample`, `Serie`, `SketchSeries`, `ContextMetrics` — the metric data model |
| [`pkg/tagset`](../tagset.md) | `HashingTagsAccumulator`, `CompositeTags` — tag deduplication used in context resolution |
| [`pkg/serializer`](../serializer.md) | `MetricSerializer` — the downstream consumer of aggregated series and sketches |
| [`comp/aggregator/demultiplexer`](../../comp/aggregator/demultiplexer.md) | fx component wrapping this package; the entry point used at agent startup |

### Pipeline overview

```
DogStatsD server          Checks (Python / Go)
      |                           |
      | MetricSampleBatch         | via Sender (see sender.md)
      v                           v
AgentDemultiplexer ──────────────────────────────────────
  |  TimeSampler (sharded, one per pipeline)             |
  |    └─ 10-second buckets, context-keyed (ckey.md)    |
  |  BufferedAggregator                                   |
  |    └─ CheckSampler (one per check instance)          |
  |    └─ Events / ServiceChecks / OrchestratorManifests |
  v                                                      v
Serializer (serializer.md) ──────────────────────> Intake
```

---

## Key elements

### Types

| Type | File | Description |
|---|---|---|
| `Demultiplexer` | `demultiplexer.go` | Top-level interface; every consumer should depend on this rather than a concrete type. Embeds `sender.SenderManager`. |
| `DemultiplexerWithAggregator` | `demultiplexer_agent.go` | Extended interface for the main Agent, adds `Aggregator()`, `AggregateCheckSample()`, and event-platform access. |
| `AgentDemultiplexer` | `demultiplexer_agent.go` | Concrete implementation used in production. Owns the `BufferedAggregator`, sharded `TimeSampler` workers, and the optional no-aggregation pipeline. |
| `AgentDemultiplexerOptions` | `demultiplexer_agent.go` | Configuration struct passed to `InitAndStartAgentDemultiplexer`. Key fields: `FlushInterval`, `EnableNoAggregationPipeline`, `DontStartForwarders`. |
| `BufferedAggregator` | `aggregator.go` | Runs its own goroutine. Owns one `CheckSampler` per registered check, plus the event/service-check/orchestrator queues. Receives data from checks via Go channels. |
| `CheckSampler` | `check_sampler.go` | Aggregates metrics for a single check instance. One instance per `checkid.ID`. Accumulates samples between `Commit()` calls and produces `Series` / `SketchSeriesList` at flush time. |
| `TimeSampler` | `time_sampler.go` | Aggregates DogStatsD metrics into fixed-size time buckets (10 s by default). One instance per pipeline shard. |
| `TimeSamplerID` | `time_sampler.go` | Integer type used to identify a shard when calling `AggregateSamples`. |
| `FlushAndSerializeInParallel` | `aggregator.go` | Controls the buffer and channel sizes used when flushing series to the serializer in a parallel streaming mode. |
| `Stats` | `aggregator.go` | Circular buffer of the last 32 flush statistics (count or duration). Exposed via `expvar`. |

### Functions

| Function | Description |
|---|---|
| `InitAndStartAgentDemultiplexer(...)` | Creates and starts the `AgentDemultiplexer` (and its goroutines). The primary entry point for the main Agent. |
| `DefaultAgentDemultiplexerOptions()` | Returns safe defaults: 15 s flush interval, no-aggregation pipeline disabled. |
| `NewBufferedAggregator(...)` | Creates a `BufferedAggregator`. Called internally by `InitAndStartAgentDemultiplexer`. |
| `AddRecurrentSeries(serie)` | Registers a `*metrics.Serie` that is appended to every flush. Used for "agent is running" heartbeat metrics. |
| `GetDogStatsDWorkerAndPipelineCount()` | Returns the recommended number of DogStatsD worker goroutines and pipeline shards based on vCPU count and `dogstatsd_pipeline_autoadjust`. |
| `(d *AgentDemultiplexer) ForceFlushToSerializer(start, wait)` | Triggers an immediate, synchronous flush from all samplers to the serializer. Blocks until the flush is acknowledged. |
| `(d *AgentDemultiplexer) Stop(flush bool)` | Stops the demultiplexer. If `flush` is true, performs a final flush first. |
| `(d *AgentDemultiplexer) AggregateSamples(shard, batch)` | Sends a batch of `MetricSample` to the given DogStatsD pipeline shard. |
| `(d *AgentDemultiplexer) SendSamplesWithoutAggregation(batch)` | Pushes pre-timestamped samples directly to the no-aggregation pipeline (bypasses sampling). |

### Constants

| Constant | Value | Description |
|---|---|---|
| `DefaultFlushInterval` | 15 s | How often `BufferedAggregator` flushes to the serializer. |
| `MetricSamplePoolBatchSize` | 32 | Batch size for the `MetricSamplePool` shared between DogStatsD and the time sampler. |

### Configuration keys

| Key | Default | Description |
|---|---|---|
| `aggregator_buffer_size` | 100 | Depth of the Go channels feeding the `BufferedAggregator`. |
| `aggregator_use_tags_store` | false | Enables a shared tag store for memory deduplication. |
| `aggregator_stop_timeout` | – | Seconds to wait for a graceful flush on `Stop()`. |
| `aggregator_flush_metrics_and_serialize_in_parallel_buffer_size` | – | Buffer size for the parallel flush/serialize path. |
| `aggregator_flush_metrics_and_serialize_in_parallel_chan_size` | – | Channel size for the parallel flush/serialize path. |
| `dogstatsd_pipeline_count` | 1 | Number of DogStatsD aggregation pipeline shards. |
| `dogstatsd_pipeline_autoadjust` | false | Automatically size pipelines and worker count relative to vCPUs. |
| `check_sampler_bucket_commits_count_expiry` | 2 | Commits without activity before a context is expired in `CheckSampler`. |
| `check_sampler_expire_metrics` | – | Whether to expire stale stateful metrics in `CheckSampler`. |

### Internal sub-packages

| Path | Description |
|---|---|
| `pkg/aggregator/ckey` | Provides `ContextKey`, a compact hash of metric name + host + sorted tags used as a map key in samplers. See [`ckey.md`](ckey.md) for full details. |
| `pkg/aggregator/internal/tags` | Shared tag store used to deduplicate tag slices across contexts (enabled via `aggregator_use_tags_store`). Uses `tagset.TagsKey` from [`pkg/tagset`](../tagset.md) as the deduplication key. |
| `pkg/aggregator/mocksender` | Test-only package. Provides `MockSender` (testify mock) and `CreateDefaultDemultiplexer()` for unit testing checks without a real forwarder. Requires the `test` build tag. |
| `pkg/aggregator/sender` | Defines the `Sender` and `SenderManager` interfaces. See [`sender.md`](sender.md). |

---

## Usage

### Starting the demultiplexer (main Agent)

In practice, the main Agent never calls `InitAndStartAgentDemultiplexer` directly. Instead, the `comp/aggregator/demultiplexer` fx component (see [`demultiplexer.md`](../../comp/aggregator/demultiplexer.md)) does it:

```go
// cmd/agent/subcommands/run/command.go
demultiplexerimpl.Module(demultiplexerimpl.NewDefaultParams(
    demultiplexerimpl.WithDogstatsdNoAggregationPipelineConfig(),
))
```

Under the hood, `demultiplexerimpl.newDemultiplexer` calls `InitAndStartAgentDemultiplexer`:

```go
demux := aggregator.InitAndStartAgentDemultiplexer(
    log, sharedForwarder, orchestratorForwarder,
    aggregator.DefaultAgentDemultiplexerOptions(),
    eventPlatformForwarder, haAgent, compressor, tagger, filterList,
    hostname,
)
// demux implements both Demultiplexer and sender.SenderManager
```

### Sending DogStatsD metrics

The DogStatsD server calls `AggregateSamples` with a pre-allocated batch from the shared pool:

```go
batch := demux.GetMetricSamplePool().GetBatch()
// fill batch[0..n]
demux.AggregateSamples(TimeSamplerID(shardIndex), batch[:n])
```

### Accessing a Sender for checks

Checks receive a `sender.SenderManager` at configuration time and obtain a per-instance sender:

```go
// inside Configure():
func (c *MyCheck) Configure(senderManager sender.SenderManager, ...) error {
    c.senderManager = senderManager
    return nil
}

// inside Run():
sender, err := c.GetSender()   // wrapper around senderManager.GetSender(c.ID())
sender.Gauge("my.metric", value, hostname, tags)
sender.Commit()
```

`DestroySender` is called by the collector scheduler when a check is unscheduled, which deregisters the `CheckSampler` from the `BufferedAggregator` after the next flush.

### Adding a heartbeat series

Any package can register a series that gets appended to every flush:

```go
aggregator.AddRecurrentSeries(&metrics.Serie{
    Name:   "datadog.mycomponent.running",
    Points: []metrics.Point{{Value: 1.0}},
    MType:  metrics.APIGaugeType,
})
```

### How context keys are computed

Every incoming sample is reduced to a `ckey.ContextKey` (`uint64`) by `TimeSampler` and `CheckSampler` using `ckey.KeyGenerator.GenerateWithTags`. The key is a chained Murmur3 hash of (name, hostname, sorted-deduplicated tags). Samples that hash to the same key are accumulated into the same `ContextMetrics` bucket. See [`ckey.md`](ckey.md) for the full algorithm and design rationale.

Tag deduplication before hashing uses `tagset.HashingTagsAccumulator` and `tagset.HashGenerator`. Tagger-sourced tags and metric-level tags are split into two accumulators and deduplicated with `Dedup2` to avoid any tag appearing twice. See [`tagset.md`](../tagset.md).

### How aggregated series flow to the serializer

After each flush tick, `BufferedAggregator` calls `ContextMetrics.Flush()` (from `pkg/metrics`) which returns `[]*metrics.Serie`. These are wrapped in `metrics.IterableSeries` (a `SerieSink`/`SerieSource` pair) and passed to `serializer.SendIterableSeries()`. For distribution metrics the analogous path uses `SketchSeriesList` → `serializer.SendSketch()`. The serializer never buffers all series at once; it streams them item-by-item to the forwarder. See [`serializer.md`](../serializer.md) and [`metrics.md`](../metrics/metrics.md).

### Testing

Use `mocksender.NewMockSender(id)` to get a testify-based mock that records all calls without starting a real aggregator pipeline. See also [`sender.md`](sender.md) for the `SetSender` injection mechanism.

```go
// build tag: test
mockSender := mocksender.NewMockSender(checkID)
mockSender.SetupAcceptAll()
// run your check, then assert:
mockSender.AssertMetric(t, "Gauge", "my.metric", expectedValue, "", expectedTags)
```

### Serverless variant

`demultiplexer_serverless.go` provides `ServerlessDemultiplexer`, a stripped-down demultiplexer for AWS Lambda. It omits the orchestrator and event-platform forwarders and uses a shorter flush interval appropriate for short-lived function invocations.
