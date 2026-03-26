# pkg/trace/sampler

## Purpose

`pkg/trace/sampler` implements all agent-side sampling logic for APM traces. Its
job is to decide which trace chunks to keep and forward to Datadog, and to
compute the per-service sampling rates that are sent back to tracers so they can
participate in a closed-loop feedback system. The package is designed around the
idea that multiple independent samplers contribute decisions; the agent keeps a
chunk if *any* sampler votes to keep it.

## Key elements

### Types

| Type | Description |
|------|-------------|
| `SamplingPriority` | `int8` enum (`PriorityNone`, `PriorityUserDrop=-1`, `PriorityAutoDrop=0`, `PriorityAutoKeep=1`, `PriorityUserKeep=2`). Carried in `TraceChunk.Priority`. |
| `Sampler` | Internal circular-bucket counter. Tracks trace throughput per `Signature` over a 30 s sliding window (6 × 5 s buckets) and derives per-signature sample rates to meet a `targetTPS`. |
| `PrioritySampler` | Feedback-loop sampler. Counts incoming chunks by `ServiceSignature` (service + env) and continuously publishes rates back to tracers via `RateByService`. Only chunks with `PriorityAutoKeep` or `PriorityAutoDrop` participate in the TPS feedback loop; explicit user priorities short-circuit it. |
| `ErrorsSampler` | `ScoreSampler` variant tuned for traces containing error spans. Driven by `conf.ErrorTPS`. |
| `NoPrioritySampler` | `ScoreSampler` variant for traces that carry no priority field. |
| `ScoreSampler` | Base sampler that computes a signature from span fields (service, name, resource, HTTP status, error type) and samples by `traceID` against the derived rate. |
| `RareSampler` | Ensures at least one trace is sampled for each unique combination of `(env, service, name, resource, HTTP status, error type)` that is never kept by priority sampling. Uses a token-bucket rate limiter (`RareSamplerTPS`) and a TTL-based "seen" cache keyed by span hash. |
| `ProbabilisticSampler` | Deterministic sampler that overrides all other samplers. Hashes the trace ID (64- or 128-bit) with a configurable seed and keeps traces below a percentage threshold. Compatible with the OpenTelemetry probabilistic sampler processor when `fullTraceIDMode` is enabled. |
| `DynamicConfig` / `RateByService` | Thread-safe map of `ServiceSignature -> float64` rates. Written by `PrioritySampler.updateRates()` and read by the HTTP receiver to embed rates in responses to tracers. Implements versioning to avoid unnecessary writes. |
| `serviceKeyCatalog` | LRU-bounded (default 5,000 entries) catalog mapping `ServiceSignature` to its hashed `Signature`, used by `PrioritySampler`. |
| `Metrics` | Aggregates `seen`/`kept` counters per `MetricsKey` (sampler name, service, env, priority) and reports them every 10 s to DogStatsD. |

### Key functions

| Function | Description |
|----------|-------------|
| `SampleByRate(traceID, rate)` | Deterministic keep/drop using Knuth hashing. `traceID * 1111111111111111111 < rate * 2^64`. |
| `PrioritySampler.Sample(now, chunk, root, env, clientDropWeight)` | Main entry point; respects client priority, counts the chunk, updates rates. |
| `ScoreSampler.Sample(now, trace, root, env)` | Computes a signature, counts it, applies the rate on `traceID`. |
| `RareSampler.Sample(now, chunk, env)` | Checks the seen-span cache and admits at most `RareSamplerTPS` traces per second. |
| `ProbabilisticSampler.Sample(root)` | FNV-32a hash of `seed || traceID`, keep if below threshold. |
| `SingleSpanSampling(pt)` | Scans a chunk for spans tagged with `_dd.span_sampling.mechanism`; if found, rebuilds the chunk with only those spans and forces `PriorityUserKeep`. |
| `GetSamplingPriority(chunk)` | Reads `TraceChunk.Priority` as a `SamplingPriority`. |

### Metric keys written to spans

| Constant | Span metric key | Meaning |
|----------|-----------------|---------|
| `KeySamplingRateGlobal` | `_sample_rate` | Cumulative pre-sample rate |
| `KeySamplingRateClient` | `_dd1.sr.rcusr` | Tracer-side client rate |
| `KeySamplingRatePreSampler` | `_dd1.sr.rapre` | Agent pre-sampler rate |
| `KeySpanSamplingMechanism` | `_dd.span_sampling.mechanism` | Single-span sampling rule |
| `KeyAnalyzedSpans` | `_dd.analyzed` | Marks a span as an APM event |

### DogStatsD metrics emitted

- `datadog.trace_agent.sampler.seen` — trace chunks evaluated, tagged by sampler/service/env/priority
- `datadog.trace_agent.sampler.kept` — chunks retained
- `datadog.trace_agent.sampler.size` — number of unique signatures tracked
- `datadog.trace_agent.sampler.rare.hits/misses/shrinks`

## Usage

All samplers are instantiated in `pkg/trace/agent.NewAgent` and held as fields on
`Agent`. The processing pipeline calls each sampler sequentially:

```go
// pkg/trace/agent/agent.go (simplified)
dynConf := sampler.NewDynamicConfig()
a.PrioritySampler  = sampler.NewPrioritySampler(conf, dynConf)
a.ErrorsSampler    = sampler.NewErrorsSampler(conf)
a.RareSampler      = sampler.NewRareSampler(conf)
a.NoPrioritySampler = sampler.NewNoPrioritySampler(conf)
a.ProbabilisticSampler = sampler.NewProbabilisticSampler(conf)
a.SamplerMetrics = sampler.NewMetrics(statsd)
a.SamplerMetrics.Add(a.PrioritySampler, a.ErrorsSampler, ...)
```

`RateByService` (inside `DynamicConfig`) is shared with the HTTP receiver, which
embeds current per-service rates in its response headers so tracers adjust
client-side sampling to match the agent's target TPS.

`RemoteConfigHandler` calls `PrioritySampler.UpdateTargetTPS`,
`ErrorsSampler.UpdateTargetTPS`, and `RareSampler.SetEnabled` to apply live
remote configuration updates without restarting the agent.

The `ProbabilisticSampler` is meant as a global override; when enabled it
bypasses all other sampler decisions.

---

## Cross-references

| Topic | Document |
|---|---|
| Pipeline overview and how samplers fit in the full agent | [`pkg/trace`](trace.md) |
| `AgentConfig` sampler fields (`TargetTPS`, `ErrorTPS`, `RareSamplerTPS`, `ProbabilisticSamplerEnabled`, …) | [`pkg/trace/config`](config.md) |
| RED metrics computation — runs in parallel with sampling over *all* spans | [`pkg/trace/stats`](stats.md) |
| Remote configuration that drives `UpdateTargetTPS` / `SetEnabled` at runtime | [`pkg/trace/remoteconfighandler`](remoteconfighandler.md) |

### Relationship to `pkg/trace/config`

Every sampler is constructed directly from `*config.AgentConfig`:

| Sampler config field | AgentConfig field |
|---|---|
| `PrioritySampler` target TPS | `TargetTPS` (default 10) |
| `ErrorsSampler` target TPS | `ErrorTPS` (default 10) |
| `RareSampler` TPS budget | `RareSamplerTPS` (default 5) |
| `RareSampler` cooldown | `RareSamplerCooldownPeriod` (default 5 min) |
| `RareSampler` cardinality cap | `RareSamplerCardinality` (default 200) |
| `ProbabilisticSampler` enabled | `ProbabilisticSamplerEnabled` |
| `ProbabilisticSampler` percentage | `ProbabilisticSamplerSamplingPercentage` |
| `ProbabilisticSampler` seed | `ProbabilisticSamplerHashSeed` |

### Relationship to `pkg/trace/stats`

The stats `Concentrator` and the sampler subsystem operate on the same span
stream but are fully independent: the Concentrator receives *every* span (before
any keep/drop decision) so that RED metrics are computed over 100 % of traffic,
while samplers act on whole trace chunks. This is why `Concentrator.Add` is
called unconditionally and only *then* `PrioritySampler.Sample` determines
whether the trace is forwarded.

### Relationship to `pkg/trace/remoteconfighandler`

`RemoteConfigHandler` subscribes to the `APMSampling` remote configuration
product and calls:

```go
prioritySampler.UpdateTargetTPS(newTPS)
errorsSampler.UpdateTargetTPS(newTPS)
rareSampler.SetEnabled(enabled)
```

Precedence: env-specific RC value > global RC value > local `AgentConfig` default.
