# OTLP ingest

-----

OTLP ingest is a minimal OpenTelemetry Collector embedded inside the core Agent process. It runs the upstream `otlp` receiver on the standard OTLP ports (4317 gRPC, 4318 HTTP) and routes whatever arrives into the Agent's classic pipelines: metrics into the shared serializer, logs into the logs-agent channel, and traces over a loopback gRPC hop into the trace-agent. Its pipeline topology is not user-configurable — the pipelines are hardcoded YAML templates parameterized by `otlp_config.*` keys in `datadog.yaml`. It exists so that applications instrumented with OpenTelemetry SDKs can point their OTLP exporter at the local Agent without deploying a separate collector. For the fully configurable collector distribution that ships as its own binary, see [DDOT collector](ddot.md).

## OTLP ingest vs DDOT

Both features live under [`comp/otelcol`](<<<SRC>>>/comp/otelcol) and share the exporter/processor components and the [`pkg/opentelemetry-mapping-go`](<<<SRC>>>/pkg/opentelemetry-mapping-go) translation library, which makes them easy to confuse:

| | OTLP ingest | DDOT |
|---|---|---|
| Process | Inside the core `agent` binary | Separate `otel-agent` binary |
| Collector config | Hardcoded templates, tuned via `otlp_config.*` | Full user-supplied collector YAML |
| Components | `otlp` receiver + Datadog exporters only | Curated collector-contrib set + Datadog components |
| Fx component | [`comp/otelcol/collector/fx-pipeline`](<<<SRC>>>/comp/otelcol/collector/fx-pipeline/fx.go) / [`impl-pipeline`](<<<SRC>>>/comp/otelcol/collector/impl-pipeline/pipeline.go) | [`comp/otelcol/collector/fx`](<<<SRC>>>/comp/otelcol/collector/fx/fx.go) / [`impl`](<<<SRC>>>/comp/otelcol/collector/impl/collector.go) |
| Enabled by | Presence of the `otlp_config.receiver` section | `otelcollector.enabled: true` |
| Trace hand-off | OTLP/gRPC over loopback to the trace-agent process | In-process trace-agent component |

## Key packages

| Path | Purpose |
|---|---|
| [`comp/otelcol/otlp`](<<<SRC>>>/comp/otelcol/otlp) | The embedded collector: pipeline construction, config mapping, templates |
| [`comp/otelcol/otlp/config.go`](<<<SRC>>>/comp/otelcol/otlp/config.go) | `FromAgentConfig`: converts `otlp_config.*` into a `PipelineConfig` |
| [`comp/otelcol/otlp/map_provider.go`](<<<SRC>>>/comp/otelcol/otlp/map_provider.go), [`map_provider_config_not_serverless.go`](<<<SRC>>>/comp/otelcol/otlp/map_provider_config_not_serverless.go) | `buildMap`: merges the hardcoded pipeline templates with user values |
| [`comp/otelcol/otlp/collector.go`](<<<SRC>>>/comp/otelcol/otlp/collector.go) | `Pipeline`: assembles factories and runs the collector service |
| [`comp/otelcol/otlp/configcheck`](<<<SRC>>>/comp/otelcol/otlp/configcheck) | `IsConfigEnabled` (the on/off detection) and the no-`otlp`-build fallback |
| [`comp/otelcol/collector/impl-pipeline/pipeline.go`](<<<SRC>>>/comp/otelcol/collector/impl-pipeline/pipeline.go) | Core-agent component that starts OTLP ingest, plus flare filler and status provider |
| [`comp/otelcol/bundle.go`](<<<SRC>>>/comp/otelcol/bundle.go) | Fx bundle pulling `fx-pipeline` into the core agent's graph |
| [`comp/otelcol/otlp/components/exporter/serializerexporter`](<<<SRC>>>/comp/otelcol/otlp/components/exporter/serializerexporter) | `serializer` exporter: OTLP metrics → Datadog series/sketches via `pkg/serializer` |
| [`comp/otelcol/otlp/components/exporter/logsagentexporter`](<<<SRC>>>/comp/otelcol/otlp/components/exporter/logsagentexporter) | `logsagent` exporter: OTLP log records → `*message.Message` into the logs-agent pipeline |
| [`comp/otelcol/otlp/components/processor/infraattributesprocessor`](<<<SRC>>>/comp/otelcol/otlp/components/processor/infraattributesprocessor) | Tagger-backed processor adding container/Kubernetes tags as resource attributes |
| [`pkg/trace/api/otlp.go`](<<<SRC>>>/pkg/trace/api/otlp.go) | The trace-agent's `OTLPReceiver`, target of the loopback trace hop |
| [`pkg/opentelemetry-mapping-go`](<<<SRC>>>/pkg/opentelemetry-mapping-go) | OTLP↔Datadog translation: [`otlp/attributes`](<<<SRC>>>/pkg/opentelemetry-mapping-go/otlp/attributes) (source/hostname resolution), [`otlp/metrics`](<<<SRC>>>/pkg/opentelemetry-mapping-go/otlp/metrics) (metric translator), [`otlp/logs`](<<<SRC>>>/pkg/opentelemetry-mapping-go/otlp/logs) (log mapping) |
| [`pkg/config/setup/otlp_settings.go`](<<<SRC>>>/pkg/config/setup/otlp_settings.go) | All `otlp_config.*` keys and defaults |
| [`pkg/serverless/otlp/otlp.go`](<<<SRC>>>/pkg/serverless/otlp/otlp.go) | Serverless (Lambda extension) flavor of the same pipeline |

## Enablement

There is no `otlp_config.enabled` boolean. OTLP ingest turns on when the `otlp_config.receiver` section *exists* in the resolved configuration — an empty `receiver:` key in `datadog.yaml`, or any bound environment variable such as `DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT`, is sufficient. The check is `configcheck.IsConfigEnabled` in [`configcheck_common.go`](<<<SRC>>>/comp/otelcol/otlp/configcheck/configcheck_common.go), which calls `HasSection`/`IsConfigured` on `otlp_config.receiver`. The per-signal switches `otlp_config.traces.enabled` and `otlp_config.metrics.enabled` default to `true` and `otlp_config.logs.enabled` defaults to `false`, but they only matter once the receiver section exists; declaring the receiver with all three signals disabled is rejected as a configuration error.

The component that hosts the collector, [`comp/otelcol/collector/impl-pipeline/pipeline.go`](<<<SRC>>>/comp/otelcol/collector/impl-pipeline/pipeline.go), is always part of the core agent's Fx graph (via [`comp/otelcol/bundle.go`](<<<SRC>>>/comp/otelcol/bundle.go)). At startup, when ingest is enabled, it:

1. Sets the `feature_otlp_enabled` inventory flag (visible in Fleet Automation and metadata payloads).
1. Fetches a logs channel from the logs agent with `GetPipelineProvider().NextPipelineChan()` — whenever the [logs agent](../pipelines/logs.md) is running; setting `otlp_config.logs.enabled: true` without a running logs agent is a pipeline error.
1. Calls `otlp.NewPipelineFromAgentConfig` with the shared serializer, the logs channel, the [tagger](../containers/tagger.md), and the hostname component.

A failure to build or run the OTLP pipeline logs an error but does **not** fail agent startup; the error is captured and surfaced in the OTLP section of `agent status` (see [Status, health, and telemetry](../operations/introspection.md)).

## Pipeline assembly

`FromAgentConfig` ([`config.go`](<<<SRC>>>/comp/otelcol/otlp/config.go)) reads `otlp_config.*` into a `PipelineConfig`; `buildMap` ([`map_provider.go`](<<<SRC>>>/comp/otelcol/otlp/map_provider.go)) then merges up to three hardcoded YAML pipeline templates from [`map_provider_config_not_serverless.go`](<<<SRC>>>/comp/otelcol/otlp/map_provider_config_not_serverless.go) with the user's receiver/batch/debug settings. The result is fed to the collector through an in-memory confmap provider ([`internal/configutils`](<<<SRC>>>/comp/otelcol/otlp/internal/configutils/utils.go)) — no YAML file ever exists on disk. Collector-internal logs are routed into the Agent logger through a zap-core adapter ([`pkg/util/log/zap`](<<<SRC>>>/pkg/util/log/zap)), and a `debug` exporter is appended to every enabled pipeline unless `otlp_config.debug.verbosity` is `none`.

The assembled service looks like this:

```text
                    +--------------------------------- core agent process ----------------------------------+
                    |                                                                                        |
 OTLP/gRPC :4317 -->|  otlp        traces:  infraattributes/traces --> otlp exporter ---(gRPC, loopback)----+---> trace-agent
 OTLP/HTTP :4318 -->|  receiver -> metrics: infraattributes --------> serializer exporter -> pkg/serializer |     OTLPReceiver :5003
                    |              logs:    infraattributes --------> logsagent exporter --> logs-agent     |
                    |                                                    channel                             |
                    +----------------------------------------------------------------------------------------+
```

### Traces

The traces pipeline is `otlp` receiver → `infraattributes/traces` processor (on by default via `otlp_config.traces.infra_attributes.enabled`) → an upstream `otlp` **exporter** pointing at `localhost:<otlp_config.traces.internal_port>` (default `5003`), plaintext gRPC with compression off and the sending queue disabled. On the other side of that loopback hop, the trace-agent — a separate process on host installs, see [Trace pipeline (APM)](../pipelines/traces.md) — opens its `OTLPReceiver` gRPC server on that port only when OTLP ingest is enabled ([`comp/trace/config/impl/setup.go`](<<<SRC>>>/comp/trace/config/impl/setup.go) reads the port from `pkgconfigsetup.OTLPTracePort`). [`pkg/trace/api/otlp.go`](<<<SRC>>>/pkg/trace/api/otlp.go) converts OTel spans to Datadog spans and hands them to the normal trace-processing pipeline (obfuscation, sampling, stats). `otlp_config.traces.probabilistic_sampler.sampling_percentage` applies probabilistic sampling to this traffic inside the trace-agent.

### Metrics

The metrics pipeline is `otlp` receiver → `infraattributes` → `serializer` exporter. The [`serializerexporter`](<<<SRC>>>/comp/otelcol/otlp/components/exporter/serializerexporter) translates OTLP metrics through [`pkg/opentelemetry-mapping-go/otlp/metrics`](<<<SRC>>>/pkg/opentelemetry-mapping-go/otlp/metrics) — OTLP histograms become distributions (sketches) by default, cumulative monotonic sums are converted to deltas with per-timeseries state kept for `otlp_config.metrics.delta_ttl` seconds, summaries become gauges — and hands the resulting `Series` and `Sketches` directly to the shared `serializer.MetricSerializer`. OTLP metrics therefore join the classic pipeline at the [serialization](../pipelines/metrics/serialization.md) stage and ride the normal [forwarder](../pipelines/forwarder.md) to the intake; they bypass the [aggregator](../pipelines/metrics/aggregation.md) entirely (no time-based resampling happens in the Agent).

There is one side channel: APM stats computed client-side by OTel SDK/collector setups arrive as a special OTLP metric and are re-posted to the trace-agent at `otlp_config.metrics.apm_stats_receiver_addr`, which defaults to `http://localhost:<apm_config.receiver_port>/v0.6/stats`.

### Logs

The logs pipeline (opt-in, `otlp_config.logs.enabled: true`) is `otlp` receiver → `infraattributes` → `logsagent` exporter. The [`logsagentexporter`](<<<SRC>>>/comp/otelcol/otlp/components/exporter/logsagentexporter) maps OTLP log records through [`pkg/opentelemetry-mapping-go/otlp/logs`](<<<SRC>>>/pkg/opentelemetry-mapping-go/otlp/logs) into `*message.Message` values and writes them to the logs-agent channel obtained at startup, with log source `otlp_log_ingestion` and the `otel_source:datadog_agent` tag. From there they follow the standard [logs pipeline](../pipelines/logs.md) processing and intake path.

### The infraattributes processor

All three pipelines run the [`infraattributesprocessor`](<<<SRC>>>/comp/otelcol/otlp/components/processor/infraattributesprocessor), which resolves entities named by resource attributes (`container.id`, `k8s.pod.uid`, `aws.ecs.task.arn`, and others) against the local [tagger](../containers/tagger.md) and merges the resulting container/Kubernetes tags back into resource attributes. `otlp_config.metrics.tag_cardinality` (`low` by default) selects how granular the container tags attached to OTLP metrics are. The processor is shared with DDOT, where it is user-visible configuration — see the [DDOT page](ddot.md#the-infraattributes-processor-and-the-datadog-connector) for its full behavior.

## Configuration

All keys and defaults live in [`pkg/config/setup/otlp_settings.go`](<<<SRC>>>/pkg/config/setup/otlp_settings.go); the config-key path constants (`OTLPTracePort` and friends) are in [`pkg/config/setup/otlp.go`](<<<SRC>>>/pkg/config/setup/otlp.go). The most important ones:

| Key | Default | Effect |
|---|---|---|
| `otlp_config.receiver.protocols.grpc.endpoint` | `localhost:4317` | OTLP/gRPC bind address; the presence of the `receiver` section is the on/off switch |
| `otlp_config.receiver.protocols.http.endpoint` | `localhost:4318` | OTLP/HTTP bind address (plus CORS and request-size knobs; buffer and keepalive knobs live under `grpc`) |
| `otlp_config.traces.enabled` | `true` | Build the traces pipeline |
| `otlp_config.traces.internal_port` | `5003` | Loopback gRPC port of the trace-agent's `OTLPReceiver`; `0` disables the hop |
| `otlp_config.traces.probabilistic_sampler.sampling_percentage` | `100` | Probabilistic sampling applied in the trace-agent |
| `otlp_config.traces.infra_attributes.enabled` | `true` | Run infraattributes on the traces pipeline |
| `otlp_config.traces.infra_attributes.container_tag_promotion` | `off` | Promote custom container tags into `_dd.tags.container` (`off`/`duplicate`/`rename`) |
| `otlp_config.metrics.enabled` | `true` | Build the metrics pipeline |
| `otlp_config.metrics.tag_cardinality` | `low` | Granularity of container tags on OTLP metrics (`low`/`orchestrator`/`high`) |
| `otlp_config.metrics.delta_ttl` | `3600` | Seconds to remember cumulative-to-delta state per timeseries |
| `otlp_config.metrics.histograms.mode` | `distributions` | Also: `counters`, `nobuckets` |
| `otlp_config.metrics.sums.cumulative_monotonic_mode` | `to_delta` | Or `raw_value` |
| `otlp_config.metrics.summaries.mode` | `gauges` | Summary datapoint handling |
| `otlp_config.metrics.resource_attributes_as_tags` | `false` | Copy all resource attributes onto metric tags |
| `otlp_config.metrics.instrumentation_scope_metadata_as_tags` | `true` | Add scope name/version tags |
| `otlp_config.metrics.batch.*`, `otlp_config.logs.batch.*` | `min_size: 8192`, `flush_timeout: 200ms` | Exporter-side batching |
| `otlp_config.metrics.apm_stats_receiver_addr` | `http://localhost:<apm receiver port>/v0.6/stats` | Where client-computed APM stats are re-posted |
| `otlp_config.logs.enabled` | `false` | Build the logs pipeline |
| `otlp_config.debug.verbosity` | `basic` | `none` removes the debug exporter from all pipelines |
| `data_plane.otlp.proxy.enabled` | `false` | Agent Data Plane fronts the public OTLP gRPC endpoint |

When the Agent Data Plane proxy is enabled, ADP owns the public OTLP gRPC endpoint and the embedded receiver rebinds to `data_plane.otlp.proxy.receiver.protocols.grpc.endpoint`; `FromAgentConfig` validates that the two endpoints do not collide.

## Deployment modes

1. **Host agent (Linux, Windows, macOS)**: OTLP ingest is compiled into the main agent binary behind the `otlp` build tag, and the receiver binds to localhost by default. The trace path requires the trace-agent process to be running (see [Process supervision](../processes/supervision.md)) since the hop targets its loopback port.
1. **Containers and Kubernetes**: the Helm chart and Operator expose the OTLP ports on the core-agent container and set the endpoint env vars to bind on `0.0.0.0` so application pods can reach them; the setting mechanics are identical.
1. **Serverless (Lambda extension)**: [`pkg/serverless/otlp/otlp.go`](<<<SRC>>>/pkg/serverless/otlp/otlp.go) builds a trimmed variant (build tags `otlp && serverless` select [`map_provider_config_serverless.go`](<<<SRC>>>/comp/otelcol/otlp/map_provider_config_serverless.go)): no infraattributes processor, no logs pipeline, no internal telemetry.
1. **Flavors without OTLP**: the iot and dogstatsd [flavors](../processes/binaries.md) exclude the `otlp` build tag; [`configcheck_no_otlp.go`](<<<SRC>>>/comp/otelcol/otlp/configcheck/configcheck_no_otlp.go) compiles `IsEnabled` to a constant `false`, so ingest silently stays off — and the OTLP status section is hidden — even if the section is configured.

## Ports

| Port | Protocol | Purpose |
|---|---|---|
| 4317 | gRPC | Public OTLP intake (`otlp_config.receiver.protocols.grpc.endpoint`, localhost-bound by default) |
| 4318 | HTTP | Public OTLP intake (`otlp_config.receiver.protocols.http.endpoint`) |
| 5003 | gRPC | Internal hop from the embedded collector to the trace-agent's `OTLPReceiver` (the exporter targets `localhost`; the receiver binds `apm_config.receiver_host`) |
| `apm_config.receiver_port` (8126) | HTTP | `/v0.6/stats` side channel for client-computed APM stats |

## Gotchas

1. **There is no `enabled` flag.** Declaring the `otlp_config.receiver` section — even implicitly through a single env var — is what enables ingest. Conversely, removing every `receiver` key (not just setting sub-keys to defaults) is the only way to disable it.
1. **Traces fail silently without a trace-agent.** The loopback `otlp` exporter has its queue disabled; if the trace-agent is not running (for example `apm_config.enabled: false`), OTLP trace exports error out inside the embedded collector without affecting the rest of the agent. Check `agent status` for the OTLP section.
1. **Pipeline failures do not stop the agent.** Build/run errors and panics are captured in the component's `pipelineError` and only visible in status output; the agent process keeps running.
1. **`otlp_config.metrics.tag_cardinality` has a quirky env binding.** It answers to two env vars (`DD_OTLP_CONFIG_METRICS_TAG_CARDINALITY` and the legacy `DD_OTLP_TAG_CARDINALITY`), and [`otlp_settings.go`](<<<SRC>>>/pkg/config/setup/otlp_settings.go) carries a warning that the binding only partially works; don't assume the standard config machinery covers it.
1. **No aggregation.** OTLP metrics skip the aggregator; a misbehaving OTLP producer sending high-frequency datapoints translates directly into intake traffic, moderated only by the exporter batcher.
1. **Version reporting differs from DDOT.** OTLP ingest reports the Agent's own version as the collector build info, whereas DDOT reports the pinned upstream collector version.
