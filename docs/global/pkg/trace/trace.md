> **TL;DR:** `pkg/trace` is the self-contained core of the Datadog Trace Agent: it receives APM and OTel payloads, processes them through normalization/obfuscation/filtering/sampling, computes RED metrics over 100% of spans, and forwards sampled traces plus aggregated stats to the Datadog backend.

# Package `pkg/trace`

## Purpose

`pkg/trace` is the self-contained core of the Datadog Trace Agent. It receives
trace payloads from APM SDKs and OpenTelemetry instrumentation, processes them
through a sequential pipeline (normalization, obfuscation, filtering, sampling),
computes aggregated RED metrics (rate, errors, duration) over the full stream,
and forwards sampled traces plus stats to the Datadog backend.

The package is designed to be reusable outside the main agent binary: any Go
program can embed it to obtain the same trace-processing semantics. The API is
intentionally unstable and subject to major changes.

The trace-agent binary entry point is `cmd/trace-agent/`; it wires all
components together using Uber `fx` (`cmd/trace-agent/subcommands/run/`).

### Pipeline overview

```
APM SDKs / OTel instrumentation
         |
         v
  api.HTTPReceiver / api.OTLPReceiver    (receive, rate-limit, decode)
         |
         v
  agent.Agent (in channel)
    1. Normalize   – validate & default field values
    2. Filter      – blocklist / tag-filter rules
    3. Tag inject  – global DD_TAGS / env tags
    4. Obfuscate   – SQL, HTTP, Redis, Memcached, JSON, credit cards
    5. Truncate    – cap oversized values
    6. Top-level   – mark service entry-point spans
    7. Replace     – user-defined regex tag replacements
         |
    ┌────┴─────────────────────────────────┐
    v                                      v
sampler subsystem                   stats.Concentrator
(keep / drop decision)              (computes RED metrics over all spans)
    |                                      |
    v                                      v
writer.TraceWriter              writer.DatadogStatsWriter
    |                                      |
    └──────────────── Datadog backend ─────┘
```

---

## Key elements

### Core types

| Type | Package | Description |
|---|---|---|
| `Agent` | `agent/` | Central orchestrator. Holds references to all sub-systems. Created with `NewAgent(ctx, conf, ...)`. |
| `HTTPReceiver` | `api/` | HTTP server that accepts trace payloads (v0.3–v0.5 MessagePack, JSON). Exposes channels `out` / `outV1` consumed by `Agent.In` / `Agent.InV1`. |
| `OTLPReceiver` | `api/` | gRPC server for OpenTelemetry OTLP traces. |
| `Payload` / `PayloadV1` | `api/` | Decoded payload enqueued from the receiver to the agent pipeline. |
| `AgentConfig` | `config/` | All agent configuration. Loaded via `config.Load(path)` or `config.LoadConfigFile(path)`. |
| `Endpoint` | `config/` | A backend target with `APIKey` and `Host`. Multiple endpoints allow multi-region failover (MRF). |
| `ObfuscationConfig` | `config/` | Per-span-type obfuscation settings (SQL, HTTP, Redis, Memcached, ES, MongoDB, credit cards). |
| `PrioritySampler` | `sampler/` | Primary sampler. Adjusts per-`(service, env)` rates to hit `TargetTPS`. Returns rates to clients via HTTP response. |
| `ErrorsSampler` | `sampler/` | Keeps traces with error spans that the priority sampler would drop. |
| `RareSampler` | `sampler/` | Keeps the first occurrence of uncommon `(env, service, name, resource, status, error)` tuples. Tags kept spans with `_dd.rare`. |
| `ProbabilisticSampler` | `sampler/` | Rate-based sampler independent of other strategies. |
| `SamplingPriority` | `sampler/` | `int8` type. Values: `PriorityUserDrop (-1)`, `PriorityAutoDrop (0)`, `PriorityAutoKeep (1)`, `PriorityUserKeep (2)`. |
| `DynamicConfig` | `sampler/` | Shared state (e.g. `RateByService`) updated by remote config and read by the HTTP receiver to embed rates in responses. |
| `Concentrator` | `stats/` | Server-side RED metrics computation. Buckets spans by configurable window (default 10 s) and flushes `StatsPayload` to the Stats Writer. |
| `ClientStatsAggregator` | `stats/` | Re-aligns client-computed stats (from newer tracers) to server-side bucket boundaries before forwarding. |
| `TraceWriter` | `writer/` | Buffers `SampledChunks` and flushes to the backend when the buffer reaches ~3.2 MB or after a configurable interval (default 5 s). Supports zstd compression. |
| `DatadogStatsWriter` | `writer/` | Forwards `StatsPayload` messages (from both the Concentrator and `ClientStatsAggregator`) to the backend. |
| `SampledChunks` | `writer/` | The unit handed from the sampler subsystem to the Trace Writer: a `TracerPayload` plus span/event counts. |

### Key interfaces

| Interface | Package | Description |
|---|---|---|
| `agent.Writer` | `agent/` | Base writer interface: `Stop()`, `FlushSync() error`, `UpdateAPIKey(old, new)`. |
| `agent.TraceWriter` | `agent/` | Extends `Writer` with `WriteChunks(*writer.SampledChunks)`. |
| `agent.Concentrator` | `agent/` | `Start()`, `Stop()`, `Add(stats.Input)`, `AddV1(stats.InputV1)`. |
| `agent.SpanModifier` | `agent/` | `ModifySpan(*pb.TraceChunk, *pb.Span)` — called on every span during processing. |
| `agent.TracerPayloadModifier` | `agent/` | Called early in processing, before any filtering or modification. Alias for `payload.TracerPayloadModifier`. |
| `stats.Writer` | `stats/` | `Write(*pb.StatsPayload)` — implemented by `DatadogStatsWriter`. |

### Key functions

| Function | Description |
|---|---|
| `agent.NewAgent(ctx, conf, telemetryCollector, statsd, compression)` | Constructs a fully wired `Agent` ready to be started. |
| `(a *Agent) Run()` | Starts the agent pipeline goroutine. Blocks until ctx is cancelled. |
| `config.Load(path)` | Loads `system-probe.yaml` (or the path provided) and returns an `*AgentConfig`. |
| `sampler.SampleByRate(traceID, rate)` | Deterministic keep/drop decision based on Knuth hashing of the trace ID. |
| `sampler.GetSamplingPriority(chunk)` | Reads the sampling priority embedded in a `TraceChunk`. |
| `sampler.SingleSpanSampling(pt)` | Extracts spans individually kept via `_dd.span_sampling.mechanism`; rebuilds the chunk around them. |
| `stats.NewConcentrator(conf, writer, now, statsd)` | Creates a `Concentrator`. |
| `writer.NewTraceWriter(conf, ...)` | Creates a `TraceWriter` with its sender pool. |

### Important constants / metric keys

| Name | Value | Description |
|---|---|---|
| `sampler.KeySamplingRateGlobal` | `_sample_rate` | Cumulative sampling rate embedded in the root span. |
| `sampler.KeySamplingRateClient` | `_dd1.sr.rcusr` | Client-side sampling rate propagated to the agent. |
| `sampler.KeySpanSamplingMechanism` | `_dd.span_sampling.mechanism` | Present on individually-sampled spans. |
| `sampler.KeyAnalyzedSpans` | `_dd.analyzed` | Marks a span as an APM event (legacy). |
| `writer.MaxPayloadSize` | 3 200 000 B | Maximum accumulated payload before a flush is forced. |
| `config.ServiceName` | `datadog-trace-agent` | OS service name. |

### Sub-packages at a glance

| Path | Description |
|---|---|
| `agent/` | `Agent` struct, pipeline logic, normalizer, obfuscator, truncator. |
| `api/` | `HTTPReceiver`, `OTLPReceiver`, rate limiting, protocol decoders, `/info` and debug endpoints. |
| `config/` | `AgentConfig` and all sub-config structs; loading from `datadog.yaml` under `apm_config`. |
| `sampler/` | All sampling strategies, `ServiceKeyCatalog`, `DynamicConfig`, `RateByService`. |
| `stats/` | `Concentrator`, `ClientStatsAggregator`, `SpanConcentrator`, stats bucket types. |
| `writer/` | `TraceWriter`, `DatadogStatsWriter`, shared `sender` (HTTP + retry + API-key rotation). |
| `event/` | APM event extraction (legacy Trace Search). `event.Processor` + per-second rate limiter. |
| `filters/` | `Blacklister` (resource blocklist) and `Replacer` (regex tag replacement). |
| `pb/` | Protobuf-generated types for the trace protocol (`pb.Span`, `pb.TraceChunk`, `pb.TracerPayload`, `pb.StatsPayload`). |
| `containertags/` | Async enrichment of payloads with Kubernetes/Docker container tags. |
| `info/` | Runtime `/info` endpoint: receiver stats, per-language counts, sampling rates. |
| `remoteconfighandler/` | Applies remote configuration updates (sampling rules, obfuscation toggles) at runtime. |
| `traceutil/` | Span/trace utilities: root-span detection, tag accessors, `ProcessedTrace`. |
| `otel/` | OpenTelemetry trace conversion helpers. |
| `watchdog/` | CPU and memory usage watchdog for the agent process. |
| `timing/` | Latency measurement utility used for internal metrics. |
| `telemetry/` | Internal telemetry (Prometheus-style) for the trace agent. |

---

## Usage

### Embedding the trace pipeline (outside the main agent)

The canonical embedding pattern (as used in `comp/trace/agent/impl/agent.go`):

```go
import (
    pkgagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
    tracecfg  "github.com/DataDog/datadog-agent/pkg/trace/config"
    "github.com/DataDog/datadog-agent/pkg/trace/telemetry"
)

conf, err := tracecfg.Load("/etc/datadog-agent/datadog.yaml")
// ... set conf fields as needed ...

telemetryCollector := telemetry.NewCollector(conf)
agent := pkgagent.NewAgent(ctx, conf, telemetryCollector, statsdClient, compressionComponent)
go agent.Run()
// stop by cancelling ctx
```

`agent.Receiver` is the `*api.HTTPReceiver` and starts automatically. Payloads
flow from the receiver into `agent.In` channel and are processed by `agent.Run`.

### Injecting custom span/payload modifiers

`Agent.SpanModifier` is called on every span during processing (after
normalization, before sampling). `Agent.TracerPayloadModifier` is called early,
before any filtering:

```go
agent.SpanModifier = myModifier          // implements agent.SpanModifier
agent.TracerPayloadModifier = myPayloadModifier  // implements payload.TracerPayloadModifier
```

### Sampling decisions

A trace is forwarded if _any_ sampler decides to keep it. The priority ordering
is: user-set priority (`PriorityUserKeep`) > errors sampler > rare sampler >
priority sampler > probabilistic sampler. The `agent.DiscardSpan` callback is
checked first and can unconditionally drop a span before any sampler runs.

### Configuration

Configuration lives under `apm_config` in `datadog.yaml`. Key knobs:

| Key | Description |
|---|---|
| `apm_config.max_traces_per_second` | Target TPS for the priority sampler. |
| `apm_config.obfuscation.*` | Per-type obfuscation (SQL, HTTP, Redis, Memcached, JSON). |
| `apm_config.ignore_resources` | List of resource-name regex patterns to drop entirely. |
| `apm_config.filter_tags` | Require / reject traces based on tag presence. |
| `apm_config.replace_tags` | Regex replacement rules applied to tag values (PII scrubbing). |
| `apm_config.receiver_port` | Port for the HTTP receiver (default 8126). |
| `apm_config.rare_sampler.*` | Enable/configure the rare sampler. |
| `apm_config.probabilistic_sampler.sampling_percentage` | Rate for the probabilistic sampler. |

### Extending the pipeline

To add a new module to the pipeline (e.g. a new sampler or a new obfuscation
step):

1. Implement the relevant interface (`SpanModifier`, `TracerPayloadModifier`, or
   a new sampler conforming to the pattern in `sampler/`).
2. Wire it into `agent.NewAgent` (or inject it after construction).
3. Add configuration fields to `AgentConfig` and update `config/config.go`.
4. Add unit tests under `agent/` and integration tests pointing at a running
   receiver.

---

## Cross-references

The sub-packages form a strict dependency chain; the table below shows which
doc to read for each area.

| Topic | Document |
|---|---|
| All sampling strategies (`PrioritySampler`, `RareSampler`, `ProbabilisticSampler`, …) | [`pkg/trace/sampler`](sampler.md) |
| RED metrics aggregation and bucket flush | [`pkg/trace/stats`](stats.md) |
| Serialization, compression, and backend delivery | [`pkg/trace/writer`](writer.md) |
| `AgentConfig` fields and defaults | [`pkg/trace/config`](config.md) |
| HTTP / gRPC receivers, protocol endpoints, rate limiting | [`pkg/trace/api`](api.md) |
| `TracerPayloadModifier` interface (zero-import abstraction) | [`pkg/trace/payload`](payload.md) |
| APM event extraction and EPS rate limiter | [`pkg/trace/event`](event.md) |
| Resource blocklist and tag-value scrubbing | [`pkg/trace/filters`](filters.md) |
| OpenTelemetry span conversion and stats bridging | [`pkg/trace/otel`](otel.md) |
| `ProcessedTrace`, span helpers, `normalize` sub-package | [`pkg/trace/traceutil`](traceutil.md) |
| SQL / Redis / HTTP / JSON sensitive-data obfuscation | [`pkg/obfuscate`](../obfuscate.md) |
| Remote configuration updates (sampler TPS, MRF failover) | [`pkg/trace/remoteconfighandler`](remoteconfighandler.md) |
| fx component lifecycle wrapper for the trace pipeline | [`comp/trace/agent`](../../comp/trace/agent.md) |

### Data flow between packages

```
api.HTTPReceiver / api.OTLPReceiver
  │  produces api.Payload / api.PayloadV1
  ▼
agent.Agent.Run()
  │  uses traceutil.ProcessedTrace as the pipeline carrier
  │  calls traceutil.ComputeTopLevel, normalize.*
  │  calls pkg/obfuscate.Obfuscator (SQL, Redis, HTTP, …)
  │  calls filters.Blacklister.Allows → drop / filters.Replacer.Replace → scrub
  │  calls event.Processor.Process → extracts APM events
  ├──▶ sampler.PrioritySampler / ErrorsSampler / RareSampler / ProbabilisticSampler
  │      keep/drop decision; rates written to sampler.DynamicConfig → fed back to receivers
  ├──▶ stats.Concentrator.Add(stats.Input)   → RED metrics → writer.DatadogStatsWriter
  │      also: stats.ClientStatsAggregator.In ← tracers with ClientComputedStats=true
  └──▶ writer.TraceWriter.WriteChunks(writer.SampledChunks) → Datadog backend
```

**OTel ingestion path** (`api.OTLPReceiver`):

```
ptrace.Traces
  │  otel/traceutil: GetOTelService / GetOTelResourceV2 / GetOTelOperationNameV2
  ▼
pb.TracerPayload → main agent pipeline (same as above)
  │
  └─ otel/stats.OTLPTracesToConcentratorInputs
       → stats.Concentrator.Add  (stats computed before full conversion)
```

**Remote configuration** (`remoteconfighandler`):

```
RC APMSampling product
  └─▶ RemoteConfigHandler.onUpdate
        → PrioritySampler.UpdateTargetTPS
        → ErrorsSampler.UpdateTargetTPS
        → RareSampler.SetEnabled
```
