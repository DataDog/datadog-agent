# Data pipelines overview

-----

The Agent is best understood as a set of mostly independent data pipelines, one per product, that share a small number of egress stacks. Each pipeline has the same overall shape — a collection point, an in-process aggregation or batching stage, and a sender that ships compressed payloads to a product-specific intake domain — but the implementations, buffering semantics, and failure modes differ substantially between products. This page maps every data type the Agent emits, from source to intake, and points to the detailed page for each pipeline. For which binary runs what, see [Binaries and flavors](../processes/binaries.md).

## The big picture

```text
 sources                           collection & aggregation                       egress (HTTPS)

 statsd clients ---UDP/UDS-----> DogStatsD server --+
 checks (Go/Python) -collector-> check senders -----+-> demultiplexer -> serializer -> default forwarder
                                                        (time samplers,     -> <version>-app.agent.<site>
                                                         check samplers)       /api/v2/series, /api/beta/sketches,
                                                                                /api/v1/check_run, /intake/

 files, containers, journald, --> tailers -> decoders -> processors -> batcher -> logs sender
 Windows events, TCP/UDP             -> agent-http-intake.logs.<site> /api/v2/logs
                                        (TCP fallback agent-intake.logs.<site>:10516)

 DBM checks, NDM servers ------> event-platform payloads -> event platform forwarder
 (SNMP, NetFlow, traps,             -> per-track intake hostnames
  network path), SBOM, ...             (dbm-metrics-intake., ndm-intake., ndmflow-intake., ...)

 OTLP apps --gRPC 4317/HTTP 4318--> embedded OTLP collector -> metrics to serializer,
                                                               logs to logs pipeline,
                                                               traces to trace-agent (loopback gRPC :5003)

 tracers ---TCP 8126/UDS-------> trace-agent receiver -> normalize/obfuscate/sample -> trace writer
                                                      +-> concentrator (APM stats) -> stats writer
                                                             -> trace.agent.<site> /api/v0.2/traces, /api/v0.2/stats

 procfs, container runtimes ---> process/container checks -> weighted queues -> process forwarder
 system-probe (NPM) -----------> connections check              -> process.<site> /api/v1/collector,
                                                                   /api/v1/container, /api/v1/connections, ...
```

## Three egress stacks

Although the Agent ships a dozen data types, nearly all of them leave the host through one of three HTTP client stacks:

1. The **default forwarder** ([`comp/forwarder/defaultforwarder`](<<<SRC>>>/comp/forwarder/defaultforwarder)) wraps payloads in retryable transactions with per-endpoint circuit breaking, priority queues, and optional spill-to-disk. It carries metrics, sketches, check runs, events, metadata, orchestrator resources, and process payloads. Detailed in [Forwarder and resilience](forwarder.md).
1. The **logs sender** ([`comp/logs-library`](<<<SRC>>>/comp/logs-library)) streams batched, compressed payloads with blocking backpressure and endless retries against reliable destinations. It carries logs and, through the [event platform forwarder](event-platform.md), every "track"-based product (DBM, NDM, NetFlow, network path, SBOM, container images, and more).
1. The **trace writer** ([`pkg/trace/writer`](<<<SRC>>>/pkg/trace/writer)) is the trace-agent's own sender with bounded retries and its own backpressure chain back to the tracer clients.

Everything else is a variation: the connections forwarder and orchestrator forwarder are `DefaultForwarder` instances pointed at different domains, and the OTel paths re-enter the three stacks above. Intake URLs for every product derive from `site` via [`pkg/config/utils/endpoints.go`](<<<SRC>>>/pkg/config/utils/endpoints.go); dual-shipping (`additional_endpoints`), Multi-Region Failover, and proxy/FIPS handling are covered in [Forwarder and resilience](forwarder.md).

## Product map

### Metrics

Metric samples come from two directions: DogStatsD packets parsed by the server in [`comp/dogstatsd/server`](<<<SRC>>>/comp/dogstatsd/server) (see [DogStatsD internals](../dogstatsd/internals.md)) and check senders driven by the [check collector](../checks/collector.md). Both feed the `AgentDemultiplexer` ([`pkg/aggregator/demultiplexer_agent.go`](<<<SRC>>>/pkg/aggregator/demultiplexer_agent.go)): DogStatsD samples are sharded across `TimeSampler` workers that aggregate into 10-second buckets, check samples go to one `CheckSampler` per check instance inside the `BufferedAggregator`, and client-timestamped samples bypass aggregation entirely through a no-aggregation pipeline. A flush loop (every 15 s) drains the samplers into the serializer ([`pkg/serializer`](<<<SRC>>>/pkg/serializer)), which marshals protobuf `MetricPayload`/`SketchPayload` payloads with streaming zstd compression and submits them as forwarder transactions. Events and service checks ride the same aggregator but are buffered wholesale and serialized to the legacy v1 JSON endpoints. See [Metric aggregation](metrics/aggregation.md) and [Metric serialization](metrics/serialization.md).

### Logs

The logs-agent ([`comp/logs/agent`](<<<SRC>>>/comp/logs/agent), machinery in [`pkg/logs`](<<<SRC>>>/pkg/logs)) has two planes: schedulers turn Autodiscovery configs into log sources, and launchers spawn tailers for files, containers, journald, Windows Event Log, TCP/UDP sockets, and integration logs. Each tailer feeds a decoder (framing, parsing, multiline handling) into one of N parallel pipelines — processor, then batch strategy, then shared sender workers posting to `agent-http-intake.logs.<site>` with zstd compression. Delivered payloads are acked to an auditor that persists offsets in `registry.json` so tailing resumes across restarts. If the bootstrap HTTP connectivity check fails, the pipeline falls back to TCP (port 10516) and upgrades back to HTTP in the background. See [Logs pipeline](logs.md).

### Traces

The trace-agent is a separate binary that receives payloads from APM SDKs on TCP 8126 (plus UDS and Windows named pipes) across a family of versioned endpoints. Every chunk is normalized, filtered, obfuscated, and sampled in [`pkg/trace/agent`](<<<SRC>>>/pkg/trace/agent); a clone of every chunk feeds the `Concentrator` ([`pkg/trace/stats/concentrator.go`](<<<SRC>>>/pkg/trace/stats/concentrator.go)) so APM (RED) stats are computed over all traffic *before* sampling. The priority sampler returns per-service rates to tracers in each HTTP response, forming a feedback loop. Kept traces and stats are shipped by dedicated writers to `trace.agent.<site>`. The trace-agent also hosts a large family of reverse proxies (profiling, telemetry, EVP proxy, debugger, and more) that inject the API key so tracers never need it. See [Trace pipeline (APM)](traces.md).

### Processes and containers

Process, container, and discovery checks live in [`pkg/process/checks`](<<<SRC>>>/pkg/process/checks) and produce protobuf payloads chunked and queued in weighted in-memory queues ([`pkg/process/runner/submitter.go`](<<<SRC>>>/pkg/process/runner/submitter.go)) before shipping to `process.<site>`. On Linux these checks run inside the core agent; on Windows and macOS they run in the standalone process-agent. The NPM connections check pulls connection data from system-probe (see [Network monitoring](../ebpf/network-monitoring.md)) and ships through a dedicated [`connections forwarder`](<<<SRC>>>/comp/forwarder/connectionsforwarder/impl/connectionsforwarder.go). A distinctive behavior: intake responses drive "realtime mode" — when a user watches the Live Processes page, the backend tells the agent to switch from 10 s to 2 s collection. See [Process and container pipeline](processes.md).

### Event platform products

The event platform forwarder ([`comp/forwarder/eventplatform`](<<<SRC>>>/comp/forwarder/eventplatform/impl/epforwarder.go)) reuses the logs sender to run one "passthrough pipeline" per track: DBM samples/metrics/activity, NDM metadata, SNMP traps, NetFlow, network path, container lifecycle/images, SBOM, synthetics, and others. Each track has its own intake hostname (`dbm-metrics-intake.<site>`, `sbom-intake.<site>`, ...) and inherits the full logs endpoint configuration surface under its own config prefix. Producers submit raw events either through the check sender's `EventPlatformEvent` or directly against the forwarder component. Unlike the default forwarder there is no retry queue: pipelines drop on channel overflow. See [Event platform forwarder](event-platform.md).

### Network device monitoring

NDM is a family of collectors inside the core agent that all exit through event platform tracks: the SNMP corecheck ([`pkg/collector/corechecks/snmp`](<<<SRC>>>/pkg/collector/corechecks/snmp)) polls devices for metrics (which ride the normal metrics pipeline) and metadata payloads (track `ndm`); embedded UDP servers collect NetFlow/IPFIX/sFlow ([`comp/netflow`](<<<SRC>>>/comp/netflow), track `ndmflow`, with its own 5-minute flow aggregator) and SNMP traps ([`comp/snmptraps`](<<<SRC>>>/comp/snmptraps), track `ndmtraps`); and [`comp/networkpath`](<<<SRC>>>/comp/networkpath) schedules traceroutes executed by system-probe (track `netpath`). See [Network device monitoring servers](ndm.md).

### OpenTelemetry

OTLP data enters through either of two mechanisms and then merges into the pipelines above. **OTLP ingest** ([`comp/otelcol/otlp`](<<<SRC>>>/comp/otelcol/otlp)) is a small hardcoded collector embedded in the core agent listening on 4317/4318: metrics are translated into the serializer, logs are written into the logs-agent pipeline, and traces are re-exported over loopback gRPC (port 5003) to the trace-agent's `OTLPReceiver` ([`pkg/trace/api/otlp.go`](<<<SRC>>>/pkg/trace/api/otlp.go)). **DDOT** ([`cmd/otel-agent`](<<<SRC>>>/cmd/otel-agent)) is a full user-configurable collector distribution shipped as a separate binary that reuses the same Agent pipelines in-process: serializer plus forwarder for metrics, an embedded logs-agent pipeline for logs, and an in-process trace-agent component for traces. See [OTLP ingest](../otel/otlp-ingest.md) and [DDOT collector](../otel/ddot.md).

### Other emitters

A few data types piggyback on the stacks above rather than owning a pipeline: host and inventory metadata payloads are periodic JSON documents submitted through the default forwarder (`/api/v2/host_metadata` and friends); orchestrator resources collected in Kubernetes are protobuf payloads sent through the orchestrator forwarder ([`comp/forwarder/orchestrator`](<<<SRC>>>/comp/forwarder/orchestrator/impl/forwarder_orchestrator.go)) to `orchestrator.<site>` (see [Orchestrator explorer](../containers/orchestrator.md)); and CWS events and compliance findings from the security stack ship through their own dedicated logs-style senders to security intakes — not through the event platform forwarder (see [Workload Protection](../ebpf/cws.md)).

## Summary table

| Product | Collecting process | Aggregation point | Egress stack | Intake endpoint family |
|---|---|---|---|---|
| Metrics (DogStatsD) | core agent (or standalone `dogstatsd`) | `TimeSampler` 10 s buckets; no-agg pipeline for timestamped samples | default forwarder | `<version>-app.agent.<site>` — `/api/v2/series` (v3 `/api/intake/metrics/v3/series` on Datadog-owned intake), `/api/beta/sketches` |
| Metrics (checks) | core agent (cluster agent, CLC runners) | `CheckSampler` per check instance, drained on flush | default forwarder | same as above |
| Events, service checks | core agent | buffered slices in `BufferedAggregator` | default forwarder | `/intake/`, `/api/v1/check_run` (v1 JSON) |
| Host metadata | core agent | none (periodic providers) | default forwarder | `/api/v2/host_metadata` |
| Logs | core agent (logs-agent) | batch strategy: 1000 msgs / 5 MB / `batch_wait` s | logs sender | `agent-http-intake.logs.<site>` `/api/v2/logs`; TCP fallback `agent-intake.logs.<site>:10516` |
| Traces | trace-agent | trace writer buffer (3.2 MB / 5 s) | trace writer | `trace.agent.<site>` `/api/v0.2/traces` |
| APM stats | trace-agent | `Concentrator` 10 s buckets (pre-sampling) | trace writer | `trace.agent.<site>` `/api/v0.2/stats` |
| Processes, containers | core agent (Linux) / process-agent (Windows, macOS) | chunking + weighted queues | default forwarder (process endpoints) | `process.<site>` `/api/v1/collector`, `/container`, `/discovery` |
| NPM connections | process-agent (data from system-probe) | batching in connections check | connections forwarder | `process.<site>` `/api/v1/connections` |
| Orchestrator resources | cluster agent / node agents | buffered manifests in aggregator | orchestrator forwarder | `orchestrator.<site>` `/api/v2/orch`, `/api/v2/orchmanif` |
| Event platform (DBM, SBOM, container images, ...) | producing check or component | per-track batch strategy | event platform forwarder | per-track intake hostname (e.g. `dbm-metrics-intake.<site>`) |
| NDM metadata, flows, traps, paths | core agent | NetFlow aggregator (5 min); 100-resource metadata batches | event platform forwarder | `ndm-intake.`, `ndmflow-intake.`, `snmp-traps-intake.`, `netpath-intake.` |
| OTLP metrics / logs / traces | core agent (OTLP ingest) or otel-agent (DDOT) | reuses the corresponding product pipeline | as per product | same as the corresponding product |

## Inbound ports at a glance

The pipelines above own the Agent's data-listening surface. Internal control ports (IPC, expvar, health) are covered in [Inter-process communication](../processes/ipc.md).

| Listener | Owner | Data |
|---|---|---|
| UDP 8125, UDS `dsd.socket`, Windows named pipe | DogStatsD server (core agent or standalone) | custom metrics, events, service checks |
| TCP 8126, UDS `apm.socket`, Windows named pipe | trace-agent | traces, client stats, proxied products (profiles, telemetry, EVP, ...) |
| gRPC 4317, HTTP 4318 | OTLP ingest (core agent) or DDOT | OTLP metrics, traces, logs |
| gRPC 5003 (localhost, internal) | trace-agent | OTLP traces forwarded from the embedded OTLP collector |
| UDP 2055 / 4739 / 6343 | NetFlow server (core agent) | NetFlow5/9, IPFIX, sFlow5 |
| UDP 9162 | SNMP traps server (core agent) | SNMP v1/v2c/v3 traps |
| per-source TCP/UDP ports | logs listener launcher | raw and syslog log streams |

## Cross-cutting behaviors

A few properties are worth internalizing because they differ across pipelines in ways that matter operationally:

- **Backpressure semantics diverge.** When the intake is unreachable, metrics accumulate in the forwarder's retry queue (bounded, optionally spilling to disk in the core agent) and are eventually dropped oldest-first; logs *block* all the way back to the tailers, so files keep growing but nothing is lost while the registry holds offsets; traces backpressure through a rendezvous channel to the HTTP handlers, which then answer tracers with 200-but-rejected responses; process payloads evict the oldest payload of the same type from the weighted queue; event platform pipelines simply drop when their input channel fills.
- **Retry guarantees diverge.** Only the default forwarder has durable retries (and only the core agent can spill transactions to disk). The logs sender retries forever against reliable destinations. The trace writer retries a bounded number of times. Event platform pipelines inherit logs semantics without the auditor, so there is no delivery tracking.
- **Aggregation happens at most once, in-process.** Metrics are the only product with true time aggregation (10 s buckets); NetFlow aggregates flows over 5 minutes; APM stats are bucketed in the concentrator; everything else is batching, not aggregation.
- **Compression is per-stack.** Metrics and logs default to streaming zstd (configurable to zlib/gzip); trace payloads are zstd; APM stats are gzip; process payloads use their own encoding. Payload size limits and splitting rules are per-format — see [Metric serialization](metrics/serialization.md).
- **Tagging is resolved at different stages.** Metric contexts resolve origin tags during aggregation via the tagger; logs attach tags per message at processing time; traces attach container tags at the receiver. Host tags are appended to metrics and logs only for `expected_tags_duration` after startup — a recurring source of "tags disappeared" confusion.
- **All endpoints derive from `site`**, and Datadog-owned metric intake domains are rewritten with an agent-version prefix (`7-x-y-app.agent.<site>`), which matters for egress allowlists. Every route the default forwarder knows is registered in [`endpoints.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/endpoints/endpoints.go).

/// note
Where a pipeline runs changes with deployment mode: in Kubernetes the trace-agent and process-agent are separate containers of the agent pod; on Fargate there is no system-probe and several pipelines degrade; the serverless (Lambda) extension embeds trimmed versions of the metrics, logs, and trace pipelines with synchronous per-invocation flushing. Each detailed page has a deployment-mode section.
///
