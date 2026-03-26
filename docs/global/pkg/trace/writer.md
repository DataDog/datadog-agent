# pkg/trace/writer

## Purpose

`pkg/trace/writer` is responsible for serializing, compressing, and delivering
APM payloads to the Datadog backend. It provides two concrete writers:

- **`TraceWriter`** — buffers sampled trace chunks, serializes them as protobuf
  (`AgentPayload`), compresses them, and sends to `/api/v0.2/traces`.
- **`DatadogStatsWriter`** — accepts `pb.StatsPayload` values from the stats
  pipeline, serializes them as gzip-compressed msgpack, and sends to
  `/api/v0.2/stats`.

Both writers delegate outgoing HTTP delivery to a shared `sender` abstraction
that provides connection limiting, a retry queue, and exponential-with-jitter
backoff.

## Key elements

### TraceWriter

| Item | Description |
|------|-------------|
| `TraceWriter` struct | Holds a slice of `[]*pb.TracerPayload` (the in-flight buffer), flush ticker (default 5 s), and a slice of `sender` instances. |
| `NewTraceWriter(cfg, prioritySampler, errorsSampler, rareSampler, ...)` | Constructor. Starts `timeFlush` and `reporter` goroutines. |
| `WriteChunks(pkg *SampledChunks)` | Primary ingestion point. Appends chunks to the internal buffer; triggers an immediate flush if the buffer exceeds `MaxPayloadSize` (3.2 MB). |
| `SampledChunks` | Carries a `*pb.TracerPayload`, an estimated byte size, a span count, and an event count. Produced by `pkg/trace/agent` after sampling decisions. |
| `flush()` / `flushPayloads(payloads)` | Serializes all buffered `TracerPayload`s into a single `pb.AgentPayload` (protobuf), compresses it via the injected `compression.Component`, and calls `sendPayloads`. The `AgentPayload` also embeds live sampler state (`TargetTPS`, `ErrorTPS`, `RareSamplerEnabled`) so the backend can understand current sampling intent. |
| `FlushSync()` | Blocking flush used in serverless mode (`SynchronousFlushing = true`). |
| `MaxPayloadSize` | Package-level variable (default 3.2 MB). Can be overridden in tests. |
| `pathTraces` | `"/api/v0.2/traces"` |

### DatadogStatsWriter (stats writer)

| Item | Description |
|------|-------------|
| `DatadogStatsWriter` struct | Wraps a sender slice and an in-memory payload list for sync mode. |
| `NewStatsWriter(cfg, telemetryCollector, statsd, timing, containerTagsBuffer)` | Constructor. Does not start a goroutine; caller must call `Run()`. |
| `Write(sp *pb.StatsPayload)` | Implements `stats.Writer`. If container tags enrichment is enabled and the payload has container IDs, enrichment happens asynchronously before the payload is written. |
| `Run()` | Ticker-based loop that reports DogStatsD metrics every 5 s and handles sync-mode flush requests. |
| `buildPayloads(sp, maxEntries)` | Splits large `StatsPayload`s into smaller ones, each holding at most `maxEntriesPerPayload = 4,000` grouped stats entries (~1.5 MB compressed). |
| `encodePayload(w, payload)` | Encodes a `pb.StatsPayload` as gzip-compressed msgpack. |
| `pathStats` | `"/api/v0.2/stats"` |

### sender (internal)

`sender` is a per-endpoint HTTP delivery worker. It is not exported but
is central to both writers.

| Item | Description |
|------|-------------|
| `senderConfig` | URL, max concurrent connections, queue depth, max retries, user-agent string, MRF failover flag. |
| `sender.Push(p *payload)` | Enqueues a payload. Multiple goroutines (`maxConns`) dequeue and call `sendPayload`. |
| `sendPayload` / `sendOnce` | Sends one HTTP POST. On 5xx or timeout, wraps the error in `retriableError` and retries with "Full Jitter" backoff (base 100 ms, cap 10 s). On 4xx (except 403/429), drops the payload. On 403, triggers optional API key refresh via `apiKeyManager.refresh()`. |
| `backoffDuration(attempt)` | `random(0, min(10 s, 100 ms * 2^attempt))`. |
| `newSenders(cfg, recorder, path, climit, qsize, ...)` | Creates one sender per configured endpoint. Connection limit is split evenly across non-MRF endpoints. |
| `stopSenders(senders)` | Gracefully drains in-flight payloads (5 s timeout) then closes queues. |
| `eventRecorder` interface | Implemented by both `TraceWriter` and `DatadogStatsWriter`. Called on `retry`, `sent`, `rejected`, and `dropped` events to update DogStatsD counters and histograms. |
| MRF support | Senders for Multi-Region Failover endpoints are created but disabled unless `MRFFailoverAPM()` returns `true`. |

### payload / pool

`payload` holds a `bytes.Buffer` (the compressed request body) and a header map.
A `sync.Pool` (`ppool`) is used to reuse allocations across requests. When
multiple senders exist (e.g. additional endpoints), the payload is cloned before
pushing to each sender.

### DogStatsD metrics emitted

**TraceWriter:**
- `datadog.trace_agent.trace_writer.payloads/bytes/bytes_uncompressed/spans/traces/events/errors/retries`
- `datadog.trace_agent.trace_writer.dropped/dropped_bytes`
- `datadog.trace_agent.trace_writer.connection_fill/queue_fill` (histograms)
- `datadog.trace_agent.trace_writer.encode_ms/flush_duration` (timing)

**DatadogStatsWriter:**
- `datadog.trace_agent.stats_writer.client_payloads/payloads/stats_buckets/stats_entries/bytes/retries/splits/errors`
- `datadog.trace_agent.stats_writer.dropped/dropped_bytes`

## Usage

Both writers are created in `pkg/trace/agent.NewAgent` and wired to samplers
and the stats pipeline:

```go
// pkg/trace/agent/agent.go (simplified)
statsWriter := writer.NewStatsWriter(conf, telemetryCollector, statsd, timing, containerTagsBuffer)
a.StatsWriter  = statsWriter
a.Concentrator = stats.NewConcentrator(conf, statsWriter, time.Now(), statsd)  // shares statsWriter
a.TraceWriter  = writer.NewTraceWriter(conf, a.PrioritySampler, a.ErrorsSampler,
                     a.RareSampler, telemetryCollector, statsd, timing, comp)
```

The agent calls `a.TraceWriter.WriteChunks(pkg)` after each sampling decision.
On shutdown the agent calls `a.TraceWriter.Stop()` which drains buffered payloads
before closing sender queues.

`DatadogStatsWriter.Run()` is started as a goroutine. On shutdown
`DatadogStatsWriter.Stop()` is called to flush remaining stats.

**Serverless mode:** When `conf.SynchronousFlushing = true`, the writers do not
flush on a timer. The Lambda extension calls `FlushSync()` explicitly after each
invocation to ensure all data is sent before the function suspends.

**API key rotation:** Both writers expose `UpdateAPIKey(oldKey, newKey)` which
iterates over their senders and replaces the key when it matches `oldKey`.

---

## Cross-references

| Topic | Document |
|---|---|
| Full pipeline overview and where writers fit | [`pkg/trace`](trace.md) |
| `AgentConfig` writer settings (`TraceWriter`, `StatsWriter`, `SynchronousFlushing`, `MaxSenderRetries`, `ConnectionResetInterval`) | [`pkg/trace/config`](config.md) |
| `SampledChunks` produced by the sampler subsystem and consumed by `TraceWriter.WriteChunks` | [`pkg/trace/sampler`](sampler.md) (see `SingleSpanSampling` and `SampledChunks`) |
| `StatsPayload` produced by `Concentrator` and `ClientStatsAggregator`, consumed by `DatadogStatsWriter.Write` | [`pkg/trace/stats`](stats.md) |
| `pb.TracerPayload` / `pb.StatsPayload` protobuf wire types serialized by the writers | [`pkg/trace/payload`](payload.md) |

### `TraceWriter` and sampler state embedding

When `TraceWriter.flush()` serializes the buffered `TracerPayload` list into a
`pb.AgentPayload`, it also embeds the live sampler state read directly from the
`PrioritySampler`, `ErrorsSampler`, and `RareSampler` instances:

```
pb.AgentPayload
  ├── TracerPayloads []*pb.TracerPayload   (from sampler → SampledChunks)
  ├── TargetTPS      float32               (from PrioritySampler)
  ├── ErrorTPS       float32               (from ErrorsSampler)
  └── RareSamplerEnabled bool              (from RareSampler)
```

This lets the backend understand the current sampling intent alongside the
sampled traces, enabling accurate rate computation on the backend side.

### `AgentConfig` tuning knobs for writers

| Config field | Effect |
|---|---|
| `TraceWriter.ConnectionLimit` | Max concurrent HTTP connections for `TraceWriter` senders. |
| `TraceWriter.QueueSize` | In-memory queue depth per sender. |
| `TraceWriter.FlushPeriodSeconds` | Flush interval (default 5 s). |
| `StatsWriter.ConnectionLimit` | Max concurrent HTTP connections for `DatadogStatsWriter` senders. |
| `StatsWriter.FlushPeriodSeconds` | DogStatsD reporting interval (default 5 s). |
| `SynchronousFlushing` | Serverless mode: disables timers, requires explicit `FlushSync()` calls. |
| `MaxSenderRetries` | Maximum retry attempts per payload on 5xx/timeout (default 4). |
| `ConnectionResetInterval` | How often `ResetClient` recreates its `http.Client` to close idle TCP connections. |
