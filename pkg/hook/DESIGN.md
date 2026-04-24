# `pkg/hook` — Pipeline Observation System

## Goal

The Agent's data pipelines (Metrics, Logs, Traces) were designed for a single purpose: collect,
aggregate, and forward data to the Datadog backend. As new platform needs emerge — anomaly
detection, real-time analysis, diagnostics — there is a growing need to tap into this data at its
source, without modifying the pipelines themselves.

`pkg/hook` introduces a lightweight, generic publish/subscribe mechanism that any component can use
to observe pipeline data. Producers (the pipelines) publish payloads; consumers attach to them
transparently, without being coupled to pipeline internals.

## Naming Convention

**Data Pipelines** refers to the Logs, Traces, and Metrics ingestion-to-forwarding paths: from the
point of collection (DogStatsD socket, trace agent receiver, log tailing) through processing
(aggregation, sampling, filtering) to serialization and dispatch to the backend.

## Requirements

### No coupling, zero dependencies

A pipeline should require no knowledge of who consumes its data, or even whether anyone does. The
hook system is injectable via fx groups and optional by design: pipelines depend on a `Hook[T]`
interface and can swap in a noop implementation with no behavioral change. Consumer packages are
never imported by producer packages.

### Minimal overhead

The performance cost of publishing to the hook must be negligible on the hot path — particularly
for the Metrics pipeline, which processes hundreds of thousands of samples per second at peak
DogStatsD rates.

### Drop if not able to send

A slow or backlogged consumer must never propagate back-pressure to the pipeline. If a consumer
falls behind, payloads are silently dropped for that consumer only. Other consumers and the
pipeline itself are unaffected.

## Use Cases

### Recording pipeline

As part of the Agent Q branch Anomaly Detection project, we need to evaluate and improve our
detection system using real-world data from staging and production environments. Alongside synthetic
data generated from Gensim Scenarios, this live pipeline data will be stored in Parquet files for
offline analysis.

We chose to capture data **at its source, inside the Agent**, for two reasons: we need **raw
pre-aggregation metrics** (not the rolled-up series the backend receives), and **unparsed logs**
(not the structured records produced after the log processing pipeline). `pkg/hook` is the
integration point that makes this possible with no changes to the pipeline code itself.

### Observer pipeline

These hooks will also feed the Agent's Observer system, which will analyze pipeline data in real
time and emit structured events to the backend for further processing — for example, anomaly
signals for BitsAI analysis.

## Scope: Agent Data Plane is not covered

While ADP is planned to become the de-facto DogStatsD pipeline and already runs on ~55% of the
us1.prod.dog fleet (~74,500 of 136,000 hosts), it is explicitly out of scope for this system for
two reasons.

First, the Agent codebase is predominantly Go, which means the hook system can be reused across
all data pipelines with no language boundary overhead. Second, the Observer — the primary consumer
of these hooks — lives in the core Agent, making it a natural direct consumer of `pkg/hook`. A
similar hook/tap system in ADP may be introduced at a later date.

---

## Implementation

### Core abstraction: `Hook[T]`

`pkg/hook` exposes a single generic interface:

```go
type Hook[T any] interface {
    Publish(producerName string, payload T)
    Subscribe(consumerName string, callback func(T), opts ...Option[T]) (unsubscribe func())
    HasSubscribers() bool
    Name() string
}
```

`T` is the payload type — `[]MetricSampleSnapshot` for the Metrics pipeline, a log record for
Logs, trace stats for Traces.

The live implementation is created with [`hook.NewHook[T](name)`](https://github.com/DataDog/datadog-agent/blob/misteriaud/pkg-hook-minimal/pkg/hook/hook.go) and distributed via fx groups
(`group:"hook"`). Components that do not need observation — tests, disabled pipelines, the
serverless agent — use [`hook.NewNoopHook[T]()`](https://github.com/DataDog/datadog-agent/blob/misteriaud/pkg-hook-minimal/pkg/hook/noop_impl.go), which discards every call with zero overhead,
satisfying the no-coupling requirement without conditional logic in the pipeline code.

### Publish: non-blocking fan-out

`Publish` delivers a payload to all currently registered subscribers in two steps:

1. An `atomic.Int32` subscriber counter is read first. If zero, `Publish` returns immediately —
   **no allocation, no lock, no channel operation**. This is the common case when no consumer is
   active, and is the primary mechanism satisfying the minimal-overhead requirement.
2. When subscribers are present, a single `sync.RWMutex` read lock is acquired (multiple
   concurrent producers do not block each other). Each subscriber's buffered channel receives the
   payload via a non-blocking `select`. If a channel is full, the payload is dropped for that
   subscriber only, its `recycle` function is called if set, and a drop telemetry counter is
   incremented.

The producer never waits on any consumer, directly satisfying the drop-if-not-able-to-send
requirement.

### Subscribe / Unsubscribe lifecycle

`Subscribe` registers a named callback, allocates a buffered channel, and starts a dedicated
goroutine that drains the channel and invokes the callback. The consumer name must be unique per
hook; a duplicate panics immediately (a programming error, analogous to double-closing a channel).
The returned `unsubscribe` function stops the goroutine, removes the consumer entry, and decrements
the subscriber counter atomically.

The buffer capacity defaults to 100 and can be tuned per-subscriber with `WithBufferSize(n)` for
consumers that need to absorb larger bursts without dropping — for example a file writer that
flushes in batches.

### Zero-GC delivery with `WithRecycle`

For latency-sensitive consumers, `WithRecycle(clone, recycle)` enables pool-based delivery. Before
enqueuing, `Publish` calls `clone` to create a private copy of the payload for that subscriber.
After the callback returns, the consumer goroutine calls `recycle` to return the copy to the pool.
On the steady-state path — buffer not full, pool has capacity — this eliminates all heap
allocations on the delivery path. Note that `clone` runs in the producer goroutine, so the copy cost is borne by the publisher.
With one active subscriber this is one slice copy per batch: `MetricSampleSnapshot` is 72 bytes,
a batch of 32 samples is 2.25 KB — fitting in L1 cache — so the copy takes roughly 10–20 ns plus
a `pool.Get()`. At 1 M samples/sec (~31 K batches/sec) this amounts to well under 0.1% of
producer CPU.

### Pool-safe payloads: `MetricSampleSnapshot`

Pipeline objects such as `metrics.MetricSample` are pooled: the backing memory is recycled
immediately after each worker iteration. Publishing a pointer into pooled memory would be a
use-after-free. `MetricSampleSnapshot` is a value-type copy of the observable fields:

```go
type MetricSampleSnapshot struct {
    Name       string
    Value      float64
    RawTags    []string
    Timestamp  float64
    SampleRate float64
    ContextKey uint64  // precomputed aggregator context key; 0 for pipelines without a context resolver
}
```

`ContextKey` is the aggregator's murmur3 hash of (name, hostname, tags), computed for free inside
[`TimeSampler.sample()`](https://github.com/DataDog/datadog-agent/blob/misteriaud/pkg-hook-minimal/pkg/aggregator/time_sampler.go). Subscribers can use it directly as a deduplication key, avoiding a
redundant hash computation per sample.

Subscribers receive owned copies that are safe to retain indefinitely.

## Pipeline tap points

### Metrics pipeline

The Metrics pipeline has three distinct ingestion paths, each hooked independently with the same
`Hook[[]MetricSampleSnapshot]` instance injected via fx.

**DogStatsD (pre-aggregation)** — [`pkg/aggregator/time_sampler.go`](https://github.com/DataDog/datadog-agent/blob/misteriaud/pkg-hook-minimal/pkg/aggregator/time_sampler.go)

The primary tap point. [`TimeSampler.sample()`](https://github.com/DataDog/datadog-agent/blob/misteriaud/pkg-hook-minimal/pkg/aggregator/time_sampler.go) is called once per metric sample received from the
DogStatsD socket. The hook captures the sample **before aggregation** (bucketing, counter
accumulation, histogram computation), so subscribers receive the raw stream as the client emitted
it. Samples are accumulated into a `hookBatch` slice and published in one call per worker batch
(typically 8–32 samples) under producer name `"dogstatsd"`.

This is the most valuable tap point for the Recording and Observer use cases: it provides the
pre-rollup data that our systems need to operate, and is the highest-volume path in the agent.

The integration uses an **accumulator pattern** to avoid a second iteration over the sample batch:
`TimeSampler` holds a `hookBatch []MetricSampleSnapshot` slice reused across batches. Inside
`sample()` — where `ContextKey` is computed anyway — each processed sample is appended inline.
After the worker finishes the full pooled batch, it calls [`publishHookBatch()`](https://github.com/DataDog/datadog-agent/blob/misteriaud/pkg-hook-minimal/pkg/aggregator/time_sampler.go), which publishes the
accumulated slice and resets it to `[:0]`. `hookBatch` grows to the maximum batch size on the first
burst and stays allocated; subsequent batches write into pre-allocated capacity with zero heap
allocation.

**DogStatsD no-aggregation stream** — [`pkg/aggregator/no_aggregation_stream_worker.go`](https://github.com/DataDog/datadog-agent/blob/misteriaud/pkg-hook-minimal/pkg/aggregator/no_aggregation_stream_worker.go)

Metrics that carry the no-aggregation flag bypass `TimeSampler` entirely and are serialized
directly. They are published under producer name `"dogstatsd-no-aggr"` in batches, guarded by a
`HasSubscribers()` check since this path does not benefit from the hookBatch accumulator pattern.
`ContextKey` is 0 (no context resolver runs on this path).

**Check metrics** — [`pkg/aggregator/check_sampler.go`](https://github.com/DataDog/datadog-agent/blob/misteriaud/pkg-hook-minimal/pkg/aggregator/check_sampler.go)

Metrics collected by Python and Go checks pass through [`checkSampler.addSample()`](https://github.com/DataDog/datadog-agent/blob/misteriaud/pkg-hook-minimal/pkg/aggregator/check_sampler.go). They are
published as single-element batches under producer name `"checks"`, guarded by `HasSubscribers()`.
The volume is orders of magnitude lower than DogStatsD, so the per-sample allocation is acceptable.
`ContextKey` is 0 (the check sampler has its own context resolver that is not yet wired to the
hook).

### Traces pipeline

**Trace stats** — [`pkg/trace/writer/stats.go`](https://github.com/DataDog/datadog-agent/blob/misteriaud/poc_signal_recorder_rebase/pkg/trace/writer/stats.go)

The trace stats writer is tapped to capture `ClientGroupedStats` entries — per-service,
per-resource aggregated statistics computed by the trace agent — before they are forwarded to the
backend. Each `StatsPayload` is fanned out per `ClientGroupedStats` entry under producer name
`"stats-writer"`. The hook type is `Hook[TraceStatsView]`, a read-only interface wrapping the
protobuf types without copying them.

Raw spans are not tapped; the stats tap provides the pre-aggregated signal that is most relevant
for anomaly detection.

### Logs pipeline

**Log processor** — [`pkg/logs/processor/processor.go`](https://github.com/DataDog/datadog-agent/blob/misteriaud/poc_signal_recorder_rebase/pkg/logs/processor/processor.go)

The tap point is inside the log processor, after redaction rules are applied and the message is
rendered but **before encoding and forwarding**. Each processed log message is published under
producer name `"logs"` as a `LogView` — a read-only interface exposing content, status, tags, and
hostname. The hook is created in [`comp/logs/agent/agentimpl/agent.go`](https://github.com/DataDog/datadog-agent/blob/misteriaud/poc_signal_recorder_rebase/comp/logs/agent/agentimpl/agent.go) and injected via fx.

### Telemetry

Three Prometheus metrics are exposed under the `hooks` subsystem:

| Metric | Labels | Description |
|---|---|---|
| `hooks_gauge` | `hook_name` | Number of live hooks |
| `subscribed_callbacks_gauge` | `hook_name` | Active subscribers per hook |
| `drops_counter` | `hook_name`, `consumer_name` | Payloads dropped due to full subscriber buffer |

`drops_counter` is the primary signal for detecting a lagging consumer.

### Requirement compliance

| Requirement | How it is satisfied |
|---|---|
| No coupling, zero dependencies | `Hook[T]` is an interface; pipelines never import consumer packages. `NewNoopHook` makes the hook fully optional with no pipeline code changes. |
| Minimal overhead | Atomic zero-subscriber fast path exits before any lock or allocation. `hookBatch` accumulator eliminates a second iteration over samples. |
| Drop if not able to send | Non-blocking channel send (`select default`); only the lagging consumer drops payloads; the pipeline and other consumers are unaffected. |
