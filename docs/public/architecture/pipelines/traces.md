# Trace pipeline (APM)

-----

The trace-agent is the process that receives trace payloads from Datadog APM SDKs ("tracers") and OTLP sources, normalizes and obfuscates spans, makes sampling decisions, computes APM (RED) statistics independently of sampling, and forwards both traces and stats to the Datadog intake. It is a sibling process of the core Agent — it is not spawned by it — and it also serves as the local gateway for a family of APM-adjacent products: profiling, dynamic instrumentation, tracer telemetry, and CI Visibility all reach Datadog through reverse proxies hosted on the trace-agent's listener. This page covers the whole pipeline from socket to intake; for how binaries relate to each other see [Binaries and flavors](../processes/binaries.md), and for OTLP ingestion on the core-agent side see [OTLP ingest](../otel/otlp-ingest.md).

## Code layout

The code is deliberately split into three layers so the pipeline can be reused outside this binary (the OSS Datadog exporter for the OpenTelemetry Collector, [DDOT](../otel/ddot.md), and the serverless extension all embed it):

| Layer | Path | Role |
|---|---|---|
| Library | [`pkg/trace`](<<<SRC>>>/pkg/trace) | The entire pipeline: receiver, processing, samplers, stats, writers. A standalone Go module with minimal dependencies. |
| Components | [`comp/trace`](<<<SRC>>>/comp/trace) | Fx components wrapping the library: [`comp/trace/agent`](<<<SRC>>>/comp/trace/agent), [`comp/trace/config`](<<<SRC>>>/comp/trace/config), [`comp/trace/compression`](<<<SRC>>>/comp/trace/compression). |
| Binary | [`cmd/trace-agent`](<<<SRC>>>/cmd/trace-agent) | CLI shell (cobra subcommands `run`, `info`, `config`, …) and service integration. |

Key files, roughly in data-flow order:

| Path | Purpose |
|---|---|
| [`cmd/trace-agent/subcommands/run/command.go`](<<<SRC>>>/cmd/trace-agent/subcommands/run/command.go) | Fx wiring of the process: config, secrets, log, statsd, remote tagger, IPC, zstd compression, the trace bundle |
| [`cmd/loader/main_nix.go`](<<<SRC>>>/cmd/loader/main_nix.go) | `trace-loader`: socket-activation shim that starts the trace-agent lazily (see below) |
| [`comp/trace/agent/impl/agent.go`](<<<SRC>>>/comp/trace/agent/impl/agent.go) | Component wrapping `pkg/trace/agent.Agent`; GOMAXPROCS/GOMEMLIMIT setup; exposes `ReceiveOTLPSpans` and `SendStatsPayload` to embedders |
| [`comp/trace/config/impl/setup.go`](<<<SRC>>>/comp/trace/config/impl/setup.go) | Bridges `datadog.yaml` into `pkg/trace/config.AgentConfig` — every `apm_config.*` key is read here |
| [`pkg/trace/api/api.go`](<<<SRC>>>/pkg/trace/api/api.go) | `HTTPReceiver`: listeners, decode semaphore, trace/stats handlers, memory watchdog |
| [`pkg/trace/api/endpoints.go`](<<<SRC>>>/pkg/trace/api/endpoints.go) | The full endpoint table plus the `AttachEndpoint` extension point |
| [`pkg/trace/api/otlp.go`](<<<SRC>>>/pkg/trace/api/otlp.go) | `OTLPReceiver`: gRPC OTLP server and the `ReceiveResourceSpans` library API |
| [`pkg/trace/agent/agent.go`](<<<SRC>>>/pkg/trace/agent/agent.go) | The `Agent` struct: worker pool, `Process()`, `runSamplers()` |
| [`pkg/trace/agent/normalizer.go`](<<<SRC>>>/pkg/trace/agent/normalizer.go), [`obfuscate.go`](<<<SRC>>>/pkg/trace/agent/obfuscate.go), [`truncator.go`](<<<SRC>>>/pkg/trace/agent/truncator.go) | Per-span normalization, obfuscation dispatch, truncation |
| [`pkg/obfuscate`](<<<SRC>>>/pkg/obfuscate) | Standalone obfuscation module (SQL, Redis, memcached, JSON, URLs, credit cards); shared with DBM |
| [`pkg/trace/sampler`](<<<SRC>>>/pkg/trace/sampler) | Priority, score (errors/no-priority), rare, probabilistic, and single-span samplers |
| [`pkg/trace/stats`](<<<SRC>>>/pkg/trace/stats) | `Concentrator` (agent-computed stats) and `ClientStatsAggregator` (client-computed stats) |
| [`pkg/trace/writer`](<<<SRC>>>/pkg/trace/writer) | `TraceWriter`, `DatadogStatsWriter`, and the shared HTTP `sender` with retry/failover |
| [`pkg/trace/remoteconfighandler/remote_config_handler.go`](<<<SRC>>>/pkg/trace/remoteconfighandler/remote_config_handler.go) | Applies `APM_SAMPLING`, `AGENT_CONFIG`, `APM_SEMANTIC_CORE_DD`, `AGENT_FAILOVER` remote-config updates |
| [`pkg/proto/pbgo/trace`](<<<SRC>>>/pkg/proto/pbgo/trace) | Protobuf/msgpack payload types (`TracerPayload`, `TraceChunk`, `Span`, `StatsPayload`); [`idx/`](<<<SRC>>>/pkg/proto/pbgo/trace/idx) holds the new v1.0 string-table format |
| [`pkg/trace/README.md`](<<<SRC>>>/pkg/trace/README.md) | Upstream overview with a [PlantUML diagram](<<<SRC>>>/pkg/trace/architecture.puml) of the same pipeline |

## Process model and startup

Who starts the trace-agent depends on the platform — see [Process supervision](../processes/supervision.md) for the general story:

1. On Linux package installs, systemd runs [`datadog-agent-trace.service`](<<<SRC>>>/pkg/fleet/installer/packages/embedded/tmpl/gen/debrpm/datadog-agent-trace.service), which is `BindsTo=datadog-agent.service` and execs `trace-loader` rather than the trace-agent directly.
1. On Windows it is the `datadog-trace-agent` service, a dependent service of the main Agent, integrated through `servicemain` in [`cmd/trace-agent/subcommands/run/command_windows.go`](<<<SRC>>>/cmd/trace-agent/subcommands/run/command_windows.go).
1. In containers and Kubernetes it runs as its own container in the Agent pod, with `trace-agent run` as the entrypoint.

The **trace-loader** ([`cmd/loader`](<<<SRC>>>/cmd/loader), gated by `apm_config.socket_activation.enabled`, default true on Linux) exists to avoid paying the trace-agent's memory cost on hosts that never send traces. It parses `datadog.yaml`, opens the TCP, UDS, and OTLP gRPC listeners itself, then blocks in `poll()`. Only when a client actually connects does it `exec()` the real trace-agent, handing the listener file descriptors over through the `DD_APM_NET_RECEIVER_FD`, `DD_APM_UNIX_RECEIVER_FD`, and `DD_OTLP_CONFIG_GRPC_FD` environment variables; [`pkg/trace/api/loader`](<<<SRC>>>/pkg/trace/api/loader) adopts them on the receiving side. If APM is disabled the loader exits without ever starting the trace-agent.

Inside the process, [`comp/trace/agent/impl/agent.go`](<<<SRC>>>/comp/trace/agent/impl/agent.go) returns `ErrAgentDisabled` early when `apm_config.enabled: false`, sets `GOMAXPROCS` from cgroup limits or `apm_config.max_cpu_percent`, sets `GOMEMLIMIT` from cgroup limits or 90% of `apm_config.max_memory`, and wires a `SecretsRefreshFn` so the HTTP senders can refresh API keys from the [secrets backend](../configuration/secrets.md) when the intake returns 403. `Agent.Run()` in [`pkg/trace/agent/agent.go`](<<<SRC>>>/pkg/trace/agent/agent.go) then starts the receiver, concentrator, client-stats aggregator, OTLP receiver, remote-config handler, debug server, and `GOMAXPROCS` worker goroutines each for the v0 (`a.In`) and v1 (`a.InV1`) payload channels. Shutdown is carefully ordered so nothing is lost: stop accepting (OTLP receiver, then HTTP receiver), close `In`, drain workers, flush the stats producers into the stats writer, then stop the writers.

The trace-agent depends on the core Agent at runtime for three things, all described in [Inter-process communication](../processes/ipc.md): the remote [tagger](../containers/tagger.md) stream over gRPC (container tags for payload enrichment and origin detection), the [remote configuration](../configuration/remote-config.md) service, and hostname resolution. Its own internal metrics go out via DogStatsD to the core Agent.

## The receiver

`HTTPReceiver` ([`pkg/trace/api/api.go`](<<<SRC>>>/pkg/trace/api/api.go)) serves a single `http.Server` across up to three listeners: TCP `apm_config.receiver_host:receiver_port` (default `localhost:8126`, connection-rate-limited by `apm_config.connection_limit`, default 2000), a Unix socket `apm_config.receiver_socket` (Linux default `/var/run/datadog/apm.socket`), and a Windows named pipe `apm_config.windows_pipe_name`. The connection's `ConnContext` records which listener it came from; on Linux, UDS connections also capture peer credentials (`SO_PEERCRED`) so the container-ID provider ([`pkg/trace/api/container.go`](<<<SRC>>>/pkg/trace/api/container.go)) can resolve the calling container from its cgroup — the foundation of [origin detection](../containers/origin-detection.md) for traces.

### Trace intake API versions

The endpoint set ([`pkg/trace/api/endpoints.go`](<<<SRC>>>/pkg/trace/api/endpoints.go)) has grown one version at a time; all remain served:

| Endpoint | Codec | Payload shape |
|---|---|---|
| `/spans`, `/services`, `/v0.1/*` | JSON | Flat span list, grouped into traces server-side (hidden from `/info`) |
| `/v0.2/*`, `/v0.3/*` | JSON, msgpack (v0.3) | `pb.Traces`, that is `[][]Span` |
| `/v0.4/traces` | msgpack or JSON | `pb.Traces`; the most widely used legacy version |
| `/v0.5/traces` | msgpack | `pb.Traces` with a string-dictionary encoding ([`decoder_v05.go`](<<<SRC>>>/pkg/proto/pbgo/trace/decoder_v05.go)) |
| `/v0.7/traces` | msgpack | Full `pb.TracerPayload` — chunks arrive with priority, origin, and tags already set |
| `/v1.0/traces` | msgpack | `idx.InternalTracerPayload`, the new string-table ("idx") format, handled by `handleTracesV1` into the separate `outV1` channel |
| `/v0.6/stats` | msgpack | `pb.ClientStatsPayload` — stats precomputed inside the tracer |
| `/v0.7/config` | JSON | Remote-config passthrough for tracers (attached at runtime, see below) |
| `/info` | JSON | Discovery: what this agent supports and how it is configured |

Tracers describe themselves and their payloads through request headers defined in [`pkg/trace/api/internal/header/headers.go`](<<<SRC>>>/pkg/trace/api/internal/header/headers.go): language and tracer version (`Datadog-Meta-*`), `X-Datadog-Trace-Count`, capability flags (`Datadog-Client-Computed-Top-Level`, `Datadog-Client-Computed-Stats`), the counts of P0 traces the client dropped locally (`Datadog-Client-Dropped-P0-Traces/-Spans`, which feed sampler weighting), and container identity headers (`Datadog-Entity-ID`, Local/External Data) for origin detection.

### Backpressure

The receiver is engineered to shed load without triggering client retry storms:

1. `handleTraces` must first acquire one of `apm_config.decoders` semaphore slots (default `GOMAXPROCS/2`, minimum 1). If none frees up within `apm_config.decoder_timeout` (default 1000 ms), the payload is **rejected but answered `200 OK`** — old tracers treat non-200 as "retry", which is exactly the wrong response to an overloaded agent. Tracers that send `Datadog-Send-Real-Http-Status: true` (and agents with the `429` feature flag) get an honest `429` instead. The `PayloadRefused` vs `PayloadAccepted` receiver stats tell the truth regardless.
1. Decode buffers are pooled and pre-sized from `Content-Length`, capped by `apm_config.max_payload_size` (25 MB).
1. Decoded payloads are pushed into the `out` channel, whose buffer is `apm_config.trace_buffer` — **default 0, a rendezvous channel**. HTTP handlers block until a worker picks the payload up, so backpressure propagates from the writer through the workers all the way back to the tracer's TCP connection.
1. A watchdog ticker (`apm_config.watchdog_check_delay`, default 10s) monitors memory: if heap alloc exceeds 1.5× `apm_config.max_memory` (default 500 MB) the process **kills itself** (`datadog.trace_agent.receiver.oom_kill` metric, exit 1) and relies on systemd/SCM/kubelet to restart it.

### The `/info` endpoint and the rates feedback response

The response body of every trace POST on v0.4+ is the JSON `rate_by_service` map — the transport of the priority-sampler feedback loop described below (v0.1–v0.3 clients just get `OK`). Tracers use the `Datadog-Rates-Payload-Version` header to skip re-parsing unchanged rate maps.

`/info` ([`pkg/trace/api/info.go`](<<<SRC>>>/pkg/trace/api/info.go)) is an unauthenticated discovery document: agent version, visible endpoints, feature flags, span-events/span-meta-structs capabilities, the reduced obfuscation config (so tracers can obfuscate client-side), peer tags, and EVP-proxy allowed headers. Every response from every endpoint carries `Datadog-Agent-State`, a SHA-256 of the `/info` body, so tracers detect configuration changes by watching that header and re-fetching `/info`. Note that the hash changes asynchronously shortly after startup when the Org Propagation Marker — a truncated hash of the org UUID fetched in the background from `/api/v2/validate` ([`pkg/trace/api/opm.go`](<<<SRC>>>/pkg/trace/api/opm.go)) — arrives.

## The processing pipeline

Worker goroutines pull `*api.Payload` values off the channel and run `Agent.Process()` ([`pkg/trace/agent/agent.go`](<<<SRC>>>/pkg/trace/agent/agent.go)):

```text
 tracer POST /v0.4/traces
        |
        v
 HTTPReceiver (decode semaphore, container tags attach)
        |
        v  a.In (rendezvous channel)
 worker: Process()
   normalize -> filter (ignore_resources, filter_tags)
   -> obfuscate -> truncate -> replace_tags -> top-level
        |                          \
        |                           +--> clone of every chunk --> Concentrator --> StatsWriter
        v                                (unless client computed stats)
   runSamplers() ---- dropped chunks discarded
        |
        v  kept chunks
   TraceWriter (buffer to 3.2MB / 5s) --> sender --> trace.agent.{site}/api/v0.2/traces
```

Per trace chunk, in order:

1. **Normalization** ([`normalizer.go`](<<<SRC>>>/pkg/trace/agent/normalizer.go), utilities in [`pkg/trace/traceutil/normalize`](<<<SRC>>>/pkg/trace/traceutil/normalize)): service and operation names are length-limited and character-sanitized (fallbacks `unnamed-service` / `unnamed_operation`), resources UTF-8-truncated at `max_resource_len` (5000 chars), trace/span IDs must be nonzero, timestamps are sanity-checked. Each malformation reason is counted in per-tracer `SpansMalformed.*` receiver stats.
1. **Root-span resolution**: `traceutil.GetRoot` finds the chunk's root (the last span without a parent in the chunk); for pre-v0.7 protocols, chunk attributes (sampling priority, origin, decision maker) are lifted from root-span tags by `setChunkAttributes`.
1. **Filtering**: the `Blacklister` drops chunks whose root resource matches any `apm_config.ignore_resources` regex, then `filteredByTags` applies `apm_config.filter_tags` / `filter_tags_regex` require/reject lists against root-span meta.
1. **Per-span enrichment**: the `GlobalTags` from `config.AgentConfig` are injected into every span (this is how Azure App Services metadata lands on spans), then the fx-injectable `SpanModifier` hook runs (see [`comp/trace/payload-modifier`](<<<SRC>>>/comp/trace/payload-modifier)).
1. **Obfuscation** ([`obfuscate.go`](<<<SRC>>>/pkg/trace/agent/obfuscate.go), engine in [`pkg/obfuscate`](<<<SRC>>>/pkg/obfuscate)): dispatched on `span.Type` — `sql`/`cassandra` runs the SQL obfuscator over the resource and `sql.query` tag (a parse failure replaces the resource with `Non-parsable SQL query`); `redis`/`valkey` quantizes the resource; `memcached`, `mongodb`, `elasticsearch`/`opensearch` (JSON), and `web`/`http` (URLs) each have their own path. Credit-card scanning runs across all meta keys when `apm_config.obfuscation.credit_cards.enabled` (default true). `apm_config.sql_obfuscation_mode` selects the newer sqllexer-based `obfuscate_and_normalize` path. The same obfuscator settings are published in `/info`, and tracers that declare `Datadog-Obfuscation-Version` obfuscate client-side, letting the agent skip the work.
1. **Truncation** ([`truncator.go`](<<<SRC>>>/pkg/trace/agent/truncator.go)) and `apm_config.replace_tags` regex rewrites.
1. **Top-level computation**: unless the tracer declared `Datadog-Client-Computed-Top-Level`, `traceutil.ComputeTopLevel` marks service-entry spans (a span is top-level when it has no parent or its parent belongs to a different service). Top-level flags drive which spans count toward APM stats.
1. **Stats fork**: a clone of the chunk is appended to the stats input **unless the client already computed stats** — stats are computed over all received chunks, *before* sampling.
1. **Sampling** (`runSamplers`, next section) marks each chunk kept or dropped; kept chunks accumulate into `writer.SampledChunks` and go to the `TraceWriter`.

One subtlety: when `apm_config.enable_container_tags_buffer` is on (default), payloads carrying a container ID whose tags are not yet complete in the tagger are *held* by [`pkg/trace/containertags/buffer.go`](<<<SRC>>>/pkg/trace/containertags/buffer.go) for up to ~12 seconds — this papers over the Kubernetes cold-start race where a pod sends traces before the kubelet has reported it.

## Sampling and the priority feedback loop

The sampler stack lives in [`pkg/trace/sampler`](<<<SRC>>>/pkg/trace/sampler). The order in `runSamplers` matters:

| Sampler | File | Trigger | Behavior |
|---|---|---|---|
| Error Tracking Standalone | — | `apm_config.error_tracking_standalone.enabled` | Only the errors sampler runs; chunks without error spans are dropped outright |
| Rare sampler | [`rare_sampler.go`](<<<SRC>>>/pkg/trace/sampler/rare_sampler.go) | `apm_config.enable_rare_sampler` (**default off**) | Runs first so it observes every chunk; keeps traces presenting an unseen `(env, service, name, resource, http.status, error.type)` combination within a TTL; kept spans tagged `_dd.rare` |
| Probabilistic sampler | [`probabilistic.go`](<<<SRC>>>/pkg/trace/sampler/probabilistic.go) | `apm_config.probabilistic_sampler.enabled` | Deterministic hash of the trace ID (lower 64 bits by default; the full 128 bits behind the `probabilistic_sampler_full_trace_id` feature flag) vs `sampling_percentage`; sets decision maker `_dd.p.dm: -9`; disables the priority feedback loop |
| Priority sampler | [`prioritysampler.go`](<<<SRC>>>/pkg/trace/sampler/prioritysampler.go) | Default path, chunk has a priority | The feedback loop, described below |
| No-priority sampler | [`scoresampler.go`](<<<SRC>>>/pkg/trace/sampler/scoresampler.go) | Chunk has no priority (very old tracers) | Same signature-scoring machinery as the priority sampler |
| Errors sampler | [`scoresampler.go`](<<<SRC>>>/pkg/trace/sampler/scoresampler.go) | Spans with `Error != 0` | Rescues error traces up to `apm_config.errors_per_second` (default 10, 0 disables) even when otherwise dropped |
| Single-span sampling | [`sampler.go`](<<<SRC>>>/pkg/trace/sampler/sampler.go) | Trace dropped but spans carry `_dd.span_sampling.mechanism` | The chunk is rewritten to contain only those spans and force-kept |
| Analytics events (deprecated) | [`pkg/trace/event`](<<<SRC>>>/pkg/trace/event) | `apm_config.analyzed_spans` | Legacy App Analytics extraction, rate-limited by `apm_config.max_events_per_second` (default 200) |

### How the priority feedback loop works

Tracers stamp each chunk with a priority: `USER_DROP (-1)`, `AUTO_DROP (0)`, `AUTO_KEEP (1)`, or `USER_KEEP (2)`. User decisions are always respected. For the automatic priorities, the core sampler ([`coresampler.go`](<<<SRC>>>/pkg/trace/sampler/coresampler.go)) counts seen traces per *signature* (a hash of service and env) in a circular buffer of six 5-second buckets, and on each bucket rotation recomputes per-signature sampling rates — distributing `apm_config.target_traces_per_second` (default 10) for the whole agent across signatures based on the moving max of the buckets, with rate increases capped at 1.2× per step. The resulting rates are stored in `DynamicConfig.RateByService` ([`dynamic_config.go`](<<<SRC>>>/pkg/trace/sampler/dynamic_config.go)), keyed `service:<svc>,env:<env>`, and returned to tracers **in the HTTP response of every trace POST** ([`responses.go`](<<<SRC>>>/pkg/trace/api/responses.go)).

Tracers apply these rates locally: they pre-sample, assign `AUTO_KEEP`/`AUTO_DROP` themselves, and — on modern tracers — do not even send the dropped P0 traces. Instead they report how many they dropped via the `Datadog-Client-Dropped-P0-Traces` header, and the agent re-weights its counters with those numbers so the computed rates stay correct despite never seeing the dropped traffic. The applied rate is recorded on the root span (`_dd.agent_psr`) for backend accounting. Remote configuration can override the local TPS target (see below), capped by `apm_config.max_remote_traces_per_second`.

/// note
"Dropped" never means "unmeasured". Stats are computed before sampling, and client-side P0 drops are compensated through the header counts — so hit counts and latency distributions in Datadog reflect *all* traffic, regardless of what fraction of traces is kept.
///

`SamplerMetrics` publishes per-`(service, env, sampler, priority)` kept/seen counts as `datadog.trace_agent.sampler.kept/seen`.

## APM stats

Stats generation is what makes the sampling model safe, and it runs on two parallel paths in [`pkg/trace/stats`](<<<SRC>>>/pkg/trace/stats):

1. **Agent-computed stats — `Concentrator`** ([`concentrator.go`](<<<SRC>>>/pkg/trace/stats/concentrator.go), [`span_concentrator.go`](<<<SRC>>>/pkg/trace/stats/span_concentrator.go)): consumes the pre-sampling clones of every chunk and aggregates them into 10-second time buckets (`apm_config.bucket_size_seconds`). Only top-level spans, measured spans (`_dd.measured: 1`), and partial-snapshot spans are counted. The aggregation key combines a payload key (env, hostname, version, container ID, git commit SHA, image tag, language, process-tags hash) with a bucket key (service, name, resource, type, span kind, HTTP status, synthetics flag, peer-tags hash). Each key accumulates hit/error/duration counters plus two DDSketch distributions (ok/error latency, [`statsraw.go`](<<<SRC>>>/pkg/trace/stats/statsraw.go)). Every span is weighted by the inverse of its client sampling rate — the `_sample_rate` span metric ([`weight.go`](<<<SRC>>>/pkg/trace/stats/weight.go)) — to compensate for client-side sampling. Peer tags (`peer.service`, `db.instance`, …) are aggregated when `apm_config.peer_tags_aggregation` is on (default), with the tag list sourced from the RC-updatable semantics registry ([`pkg/trace/semantics`](<<<SRC>>>/pkg/trace/semantics)); `apm_config.compute_stats_by_span_kind` (default true) additionally computes stats for client/server/producer/consumer spans even when they are not top-level. The concentrator keeps the two most recent buckets unflushed to absorb late spans and force-flushes at shutdown.
1. **Client-computed stats — `ClientStatsAggregator`** ([`client_stats_aggregator.go`](<<<SRC>>>/pkg/trace/stats/client_stats_aggregator.go)): tracers that send `Datadog-Client-Computed-Stats` POST `pb.ClientStatsPayload` objects to `/v0.6/stats`; their trace chunks then bypass the concentrator entirely. `Agent.ProcessStats` normalizes service/env, applies the blacklist and replace-tags, and obfuscates resources for older tracers; the aggregator then re-buckets client payloads into 2-second agent buckets and merges colliding counts so the backend sees at most one count point per second per agent (latency distributions pass through untouched).

Both paths flush into the `DatadogStatsWriter` ([`writer/stats.go`](<<<SRC>>>/pkg/trace/writer/stats.go)), which groups entries into payloads of at most 4000 stats entries, resolves container tags, gzip-compresses, and sends to `/api/v0.2/stats` using the same sender machinery as traces.

## Trace writer and sender

`TraceWriter` ([`writer/trace.go`](<<<SRC>>>/pkg/trace/writer/trace.go)) buffers sampled chunks until 3.2 MB (`writer.MaxPayloadSize`, the intake's limit) or a 5-second tick, then serializes a `pb.AgentPayload` — hostname, env, the tracer payloads, agent version, and current sampler state (`TargetTPS`, `ErrorTPS`, `RareSamplerEnabled`) — compresses it with the compression component (zstd in the agent binary; the `Content-Encoding` header follows whichever implementation is wired), and POSTs to `https://trace.agent.{site}/api/v0.2/traces` with the `DD-Api-Key` header. [`tracev1.go`](<<<SRC>>>/pkg/trace/writer/tracev1.go) is the parallel writer for the v1.0 idx format.

The sender ([`writer/sender.go`](<<<SRC>>>/pkg/trace/writer/sender.go)) maintains one sender per endpoint — the main endpoint, each of `apm_config.additional_endpoints`, and the multi-region-failover endpoint — with exponential backoff and up to `apm_config.max_sender_retries` (4) retries. A 403 from the intake triggers the `SecretsRefreshFn` API-key refresh (throttled). Queue-full drops are counted as `datadog.trace_agent.trace_writer.dropped`. MRF endpoints (`multi_region_failover.*`) only receive traffic when APM failover is enabled statically or through the `AGENT_FAILOVER` remote-config product.

/// warning
The trace-agent does **not** use the core Agent's [forwarder](forwarder.md). Its sender has no disk buffering and a queue size of 1 for the trace writer: sustained intake unavailability propagates as backpressure to tracers (or drops after retries) rather than spooling to disk.
///

Serverless environments set `apm_config.sync_flushing`: writers buffer everything until an explicit `FlushSync()` call at the end of the Lambda invocation.

## OTLP ingest in the trace-agent

The `OTLPReceiver` ([`pkg/trace/api/otlp.go`](<<<SRC>>>/pkg/trace/api/otlp.go)) is a gRPC `ptraceotlp` server on `otlp_config.traces.internal_port` (default **5003**, bound to the receiver host). This is an *internal* port: applications never talk to it. The core Agent's OTLP pipeline receives OTLP on 4317/4318 and forwards the trace portion to the trace-agent over this port — see [OTLP ingest](../otel/otlp-ingest.md) for that side. The server sets `grpc.MaxConcurrentStreams(1)`, deliberately processing one payload at a time so OTLP ingestion participates in the same backpressure chain as HTTP intake.

In embedded contexts ([DDOT](../otel/ddot.md), the OSS Datadog exporter, serverless) there is no gRPC hop at all: `ReceiveResourceSpans` is called in-process through `comp/trace/agent`'s `ReceiveOTLPSpans`. Either way, conversion resolves the source hostname and container identity from OTel resource attributes, maps spans through [`transform.OtelSpanToDDSpan`](<<<SRC>>>/pkg/trace/transform/transform.go), regroups them by 64-bit trace ID, and honors a `sampling.priority` attribute if present — otherwise the OTLP probabilistic sampler (`otlp_config.traces.probabilistic_sampler.sampling_percentage`) assigns `AUTO_KEEP`/`AUTO_DROP` by trace-ID hash. The resource attribute `_dd.stats_computed` marks chunks as client-computed-stats, which is how the OTel stats connector avoids double-counting. Converted payloads then enter the same `a.In` channel as native traces.

## Proxy endpoints

Beyond trace intake, the receiver hosts a catalog of reverse proxies ([`httputil.ReverseProxy`](https://pkg.go.dev/net/http/httputil#ReverseProxy) instances). They exist so that tracer libraries need only one local endpoint and never need the API key: each proxy injects the agent's `DD-Api-Key`, container tags, and hostname/env, and honors `proxy`/`no_proxy` settings.

| Local endpoint | Destination | Notes |
|---|---|---|
| `/profiling/v1/input` | `intake.profile.{site}/api/v2/profile` | Continuous profiler uploads; fan-out via `apm_config.profiling_additional_endpoints` ([`profiles.go`](<<<SRC>>>/pkg/trace/api/profiles.go)) |
| `/telemetry/proxy/api/v2/apmtelemetry` | `instrumentation-telemetry-intake.{site}` | Tracer telemetry; async forwarder with a 25 MB in-flight buffer budget; scrubs secrets from SSI `command_line` fields ([`telemetry.go`](<<<SRC>>>/pkg/trace/api/telemetry.go)); `apm_config.telemetry.enabled`, default true |
| `/evp_proxy/v1` … `/v4` | `<X-Datadog-EVP-Subdomain>.{site}` | Generic event-platform proxy used by CI Visibility, DBM, and others; strict charset validation, 5-header allowlist, 10 MB cap (`evp_proxy_config.max_payload_size`) ([`evp_proxy.go`](<<<SRC>>>/pkg/trace/api/evp_proxy.go)); distinct from the core Agent's [event platform forwarder](event-platform.md) |
| `/debugger/v1/input` | `http-intake.logs.{site}/api/v2/logs` | Dynamic Instrumentation "dynamic logs"; gated on logs being enabled ([`debugger.go`](<<<SRC>>>/pkg/trace/api/debugger.go)) |
| `/debugger/v1/diagnostics`, `/debugger/v2/input` | `debugger-intake.{site}/api/v2/debugger` | Dynamic Instrumentation snapshots/diagnostics |
| `/symdb/v1/input` | `debugger-intake.{site}` | Symbol database uploads ([`symdb.go`](<<<SRC>>>/pkg/trace/api/symdb.go)) |
| `/v0.1/pipeline_stats` | `trace.agent.{site}/api/v0.1/pipeline_stats` | Data Streams Monitoring ([`pipeline_stats.go`](<<<SRC>>>/pkg/trace/api/pipeline_stats.go)) |
| `/openlineage/api/v1/lineage` | `data-obs-intake.{site}` | Data Jobs Monitoring / OpenLineage ([`openlineage.go`](<<<SRC>>>/pkg/trace/api/openlineage.go), `ol_proxy_config.*`) |
| `/dogstatsd/v2/proxy` | UDP relay to the core Agent's DogStatsD | Lets tracers submit StatsD metrics through the APM socket ([`dogstatsd.go`](<<<SRC>>>/pkg/trace/api/dogstatsd.go)); see [DogStatsD internals](../dogstatsd/internals.md) |
| `/tracer_flare/v1` | `app.{site}/api/ui/support/serverless/flare` | Tracer flare uploads ([`tracer_flare.go`](<<<SRC>>>/pkg/trace/api/tracer_flare.go)); see [Flare](../operations/flare.md) |

## Remote configuration

The trace-agent both *proxies* remote config for tracers and *consumes* it itself — both through the core Agent's RC service (see [Remote configuration](../configuration/remote-config.md)):

1. **Tracer-facing proxy** — `/v0.7/config` ([`cmd/trace-agent/config/remote/config.go`](<<<SRC>>>/cmd/trace-agent/config/remote/config.go)): tracers POST `ClientGetConfigsRequest`; the trace-agent normalizes service/env, injects container tags, and forwards over gRPC to the core Agent. This is how tracer libraries receive `APM_TRACING` products (sampling rules, library configuration).
1. **Agent-side subscriptions** ([`pkg/trace/remoteconfighandler`](<<<SRC>>>/pkg/trace/remoteconfighandler)): `APM_SAMPLING` overrides the priority-sampler target TPS, error TPS, and rare-sampler enablement per env or globally; `AGENT_CONFIG` changes the log level at runtime (implemented, curiously, by POSTing to the trace-agent's own debug server `/config/set` with the IPC bearer token); `APM_SEMANTIC_CORE_DD` replaces the semantic-conventions registry wholesale; and a dedicated MRF client watches `AGENT_FAILOVER` to toggle `failover_apm`.

## Ports and observability

| Port / socket | Direction | Purpose |
|---|---|---|
| TCP 8126 (localhost by default) | in | Trace intake + all proxy endpoints; unauthenticated (local trust); cross-site browser requests rejected via `Sec-Fetch-Site` |
| `/var/run/datadog/apm.socket` (UDS, Linux) | in | Same server; peer credentials feed container-ID resolution |
| `\\.\pipe\<apm_config.windows_pipe_name>` | in | Same server on Windows (off by default) |
| TCP 5003 (gRPC, localhost) | in | OTLP traces forwarded from the core Agent's OTLP receiver |
| TCP 5012 (127.0.0.1, TLS with IPC cert) | in | Debug server ([`debug_server.go`](<<<SRC>>>/pkg/trace/api/debug_server.go)): pprof, expvar, `/config`, `/config/set`, `/secret/refresh` |
| Core Agent gRPC (cmd port 5001) | out | Remote tagger stream, remote-config fetch, hostname; IPC auth token + TLS |
| UDP 8125 (or UDS/pipe) | out | Internal `datadog.trace_agent.*` metrics via DogStatsD |
| HTTPS `trace.agent.{site}` and other intakes | out | Traces, stats, and all proxied products |

The `agent status` command renders its trace-agent section by scraping the trace-agent's expvar through the debug server, and `trace-agent info` prints the same data ([`pkg/trace/info`](<<<SRC>>>/pkg/trace/info)); [flares](../operations/flare.md) capture the debug server's `/config` output. See [Status, health, and telemetry](../operations/introspection.md) for the general pattern. Configuration defaults live in [`pkg/config/setup/apm_settings.go`](<<<SRC>>>/pkg/config/setup/apm_settings.go), and the config-to-`AgentConfig` mapping in [`comp/trace/config/impl/setup.go`](<<<SRC>>>/comp/trace/config/impl/setup.go) is the authoritative list of every `apm_config.*` key.

## Deployment-mode differences

| Mode | What changes |
|---|---|
| Linux host | Separate systemd unit with socket activation via trace-loader; TCP + UDS listeners; `AmbientCapabilities=CAP_NET_BIND_SERVICE` allows receiver ports below 1024 |
| Windows | `datadog-trace-agent` Windows service; optional named-pipe listener; [`comp/trace/etwtracer`](<<<SRC>>>/comp/trace/etwtracer) forwards ETW events to the .NET tracer; no UDS peer-credential container detection |
| Containers / Kubernetes | Own container in the Agent pod; `IsContainerized()` auto-binds `0.0.0.0`; container-ID resolution via UDS peer creds → cgroup or origin-detection headers; standard wiring is hostPort 8126 and/or the `apm.socket` hostPath |
| Fargate (ECS/EKS) | No cgroup-based container detection (headers only); profiles and proxies tagged with the Fargate orchestrator |
| Serverless (Lambda) | Not this binary: the serverless extension embeds `pkg/trace` directly with `SynchronousFlushing` and `DiscardSpan`/`SpanModifier` hooks; the debug server is compiled out |
| Azure App Services extension | trace-agent + DogStatsD run **without a core Agent**; the remote tagger is a noop and AAS metadata is injected as global tags |
| DDOT / OSS Datadog exporter | `comp/trace/agent` embedded with the HTTP receiver disabled; spans enter via direct `ReceiveOTLPSpans` calls |
| Multi-region failover | Second intake endpoint; sending is toggled by `multi_region_failover.*` config or `AGENT_FAILOVER` remote config |

See [Runtime environments](../deployment/environments.md) for how these environments are detected.

## Gotchas

1. **A `200 OK` does not mean the payload was accepted.** Overload rejections answer 200 by design; check the `PayloadRefused` receiver stat (or opt into `Datadog-Send-Real-Http-Status`).
1. **Stats are computed before sampling, once.** Dropping a trace does not affect hit/latency metrics — but a tracer that sets `Datadog-Client-Computed-Stats` (or the `_dd.stats_computed` OTLP resource attribute) bypasses the concentrator, so setting it while the agent also computes stats produces doubles.
1. The **rare sampler is disabled by default** (`apm_config.enable_rare_sampler`), despite older docs suggesting otherwise; `disable_rare_sampler` is a deprecated no-op. Similarly, `apm_config.trace_writer.queue_size` is accepted but explicitly ignored with a warning.
1. The priority sampler **only counts automatic priorities (0/1)** in the feedback loop; user decisions (-1/2) are honored but unweighted.
1. The **OOM watchdog kills the entire process** at 1.5× `max_memory` and expects the supervisor to restart it — a crash-looping trace-agent with `oom_kill` metrics is a memory-sizing problem, not a bug in the restart logic.
1. The container-tags buffer (default on) can **delay trace writes by up to ~12 s** on Kubernetes cold starts; disable with `apm_config.enable_container_tags_buffer: false` if that latency matters.
1. The `convert-traces` feature flag (in `apm_config.features` / `DD_APM_FEATURES`) reroutes **all** API versions through the v1 idx pipeline, not just `/v1.0/traces`.
1. The debug server silently degrades without the IPC certificate, and the `AGENT_CONFIG` remote-config log-level product does nothing when `apm_config.debug.port` is 0 — the RC handler applies log levels *through* the debug server.
1. `/v0.x/services` endpoints are no-ops (since 2019); service metadata is derived from traces.
