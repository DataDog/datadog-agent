# Status, health, and telemetry

-----

A running Agent observes itself through four related mechanisms. The **status system** aggregates human-readable state from every subsystem into `agent status`, the status API, and the GUI status page. The **health registry** powers the Kubernetes liveness/readiness/startup probes and `agent health`. **Internal telemetry** is a per-process Prometheus registry served locally at `/telemetry` for operators and scraped for flares. **Agent telemetry** ships a curated subset of those internal metrics to Datadog's own intake so Datadog can observe its fleet of Agents. All four are built as [components](../components/overview.md) under [`comp/core`](<<<SRC>>>/comp/core), and all are federated: any subsystem can contribute a status section (through an [Fx value group](../components/fx.md)), a health handle, or a metric without the aggregating component knowing about it.

This page covers those four systems plus the raw expvar/pprof debug endpoints each process exposes. The [flare](flare.md) bundles output from all of them, and the [CLI commands](diagnostics.md) that render them are thin HTTPS clients of the API server described in [Inter-process communication](../processes/ipc.md).

## Key packages and files

| Path | Purpose |
|---|---|
| [`comp/core/status/component.go`](<<<SRC>>>/comp/core/status/component.go) | Status component interface; `Provider` and `HeaderProvider` interfaces; Fx groups `status` and `header_status` |
| [`comp/core/status/statusimpl/status.go`](<<<SRC>>>/comp/core/status/statusimpl/status.go) | Aggregates providers, renders text/JSON/HTML, registers the `/agent/status*` endpoints and a `status.log` flare provider |
| [`comp/core/status/statusimpl/common_header_provider.go`](<<<SRC>>>/comp/core/status/statusimpl/common_header_provider.go) | The always-first header: version, flavor, PID, Go version, config paths, log level, Python version |
| [`comp/core/status/render_helpers.go`](<<<SRC>>>/comp/core/status/render_helpers.go) | `RenderText`/`RenderHTML` template helpers used by provider implementations |
| [`pkg/status/health/health.go`](<<<SRC>>>/pkg/status/health/health.go), [`global.go`](<<<SRC>>>/pkg/status/health/global.go) | Token-based health catalogs (liveness, readiness, startup) and the 15 s ping loop |
| [`comp/core/healthprobe/impl/healthprobe.go`](<<<SRC>>>/comp/core/healthprobe/impl/healthprobe.go) | HTTP server on `health_port` serving `/live`, `/ready`, `/startup`; `health.yaml` flare provider |
| [`comp/core/telemetry/impl/telemetry.go`](<<<SRC>>>/comp/core/telemetry/impl/telemetry.go) | Prometheus registry wrapper: `NewCounter`/`NewGauge`/`NewHistogram`, the `/telemetry` handler, `telemetry.log` flare provider |
| [`comp/core/agenttelemetry/impl/agenttelemetry.go`](<<<SRC>>>/comp/core/agenttelemetry/impl/agenttelemetry.go) | Agent telemetry: profile-driven scraping of the internal registry and submission to Datadog intake |
| [`comp/core/agenttelemetry/impl/defaultProfiles.yaml`](<<<SRC>>>/comp/core/agenttelemetry/impl/defaultProfiles.yaml) | Embedded default metric-selection profiles and schedules |
| [`comp/agent/expvarserver/impl/expvarserver.go`](<<<SRC>>>/comp/agent/expvarserver/impl/expvarserver.go) | Core-agent expvar HTTP server on `127.0.0.1:<expvar_port>` serving `http.DefaultServeMux` |
| [`comp/core/profiler/impl/profiler.go`](<<<SRC>>>/comp/core/profiler/impl/profiler.go) | Multi-process pprof collection used by `agent flare --profile` |
| [`comp/core/remoteagentregistry/def/component.go`](<<<SRC>>>/comp/core/remoteagentregistry/def/component.go) | Lets out-of-process agents contribute status sections and flare files over gRPC |
| [`comp/api/api/apiimpl/internal/agent/agent.go`](<<<SRC>>>/comp/api/api/apiimpl/internal/agent/agent.go) | Mounts every `agent_endpoint` provider under `/agent/` on the CMD API server, plus `/agent/status/health` |

## The status system

### Providers and value groups

The status component ([`comp/core/status/component.go`](<<<SRC>>>/comp/core/status/component.go)) defines two contribution interfaces. A `Provider` renders one named section of the status page: it has `Name()`, `Section()`, and three render methods — `JSON(verbose, stats)`, `Text(verbose, buffer)`, and `HTML(verbose, buffer)`. A `HeaderProvider` renders part of the header block above all sections and carries an `Index()` for ordering instead of a section name. Components publish them into the Fx value groups `status` and `header_status` by wrapping them with `status.NewInformationProvider` / `NewHeaderInformationProvider` in their `Provides` struct; the aggregating implementation receives the whole group without knowing who contributed.

Around thirty components contribute sections: the [collector](../checks/collector.md) (running checks and check errors), the demultiplexer/aggregator, the [forwarder](../pipelines/forwarder.md) (endpoints and transaction stats), DogStatsD ([`comp/dogstatsd/status`](<<<SRC>>>/comp/dogstatsd/status)), [autodiscovery](../checks/autodiscovery.md), [secrets](../configuration/secrets.md), [remote configuration](../configuration/remote-config.md), fleet status ([`comp/fleetstatus`](<<<SRC>>>/comp/fleetstatus)), and more. A few non-component providers (JMX status, [Cluster Agent](../containers/cluster-agent.md) connectivity, [system-probe](../ebpf/system-probe.md) status fetched over its socket, NTP, HTTP proxy warnings) are registered directly in [`cmd/agent/subcommands/run/command.go`](<<<SRC>>>/cmd/agent/subcommands/run/command.go). External processes such as the OTel agent register through the [remote agent registry](<<<SRC>>>/comp/core/remoteagentregistry/def/component.go) over gRPC and appear as regular sections.

The extension pattern — a `status_templates/` folder next to the component, rendered with the helpers in [`render_helpers.go`](<<<SRC>>>/comp/core/status/render_helpers.go) — is documented in [the status shared-feature guide](../components/shared_features/status.md).

### Aggregation, ordering, and rendering

[`statusimpl/status.go`](<<<SRC>>>/comp/core/status/statusimpl/status.go) sorts section names alphabetically and case-insensitively (they are stored lowercase), with one hardcoded exception: `status.CollectorSection` (`"collector"`) is always first when present. Providers within a section sort by name; header providers sort by `Index()`, and the common header provider (index 0) is always prepended. `GetStatus(format, verbose, excludeSections...)` walks header providers then section providers, calling the render method matching the requested format. Text output prints dashed `===` section headers; JSON output merges every provider's map into one `map[string]interface{}`; HTML output exists only for the [GUI](diagnostics.md#the-web-gui). A provider that returns an error does not break status generation — errors are collected and appended under an `errors` key at the end of the output.

The common header ([`common_header_provider.go`](<<<SRC>>>/comp/core/status/statusimpl/common_header_provider.go)) reports version, flavor, config file path, PID, Go version, build arch, Agent start time, current `log_level`, FIPS status, and the Python version obtained through `Params.PythonVersionGetFunc`.

### Endpoints and the CLI

The status component registers three routes on the CMD API server via `api.NewAgentEndpointProvider`: `GET /agent/status` (query parameters `verbose=true` and `format=text|json`), `GET /agent/status/sections` (the section list), and `GET /agent/status/section/{component}` (one section). `agent status [section]` ([`cmd/agent/subcommands/status/command.go`](<<<SRC>>>/cmd/agent/subcommands/status/command.go)) is a plain authenticated GET against those routes; text output is additionally passed through the [scrubber](<<<SRC>>>/pkg/util/scrubber) client-side before printing. The status component also registers itself as a flare provider, so every [flare](flare.md) contains the verbose text status as `status.log`.

/// note
`GetStatusBySections` accepts the pseudo-section `header`, which returns only the header providers. The list returned by `/agent/status/sections` therefore includes `"header"` even though no provider declares that section.
///

## Health checks and Kubernetes probes

### The health registry

[`pkg/status/health`](<<<SRC>>>/pkg/status/health/health.go) maintains three process-global catalogs: liveness, readiness-only, and startup. A subsystem calls `health.RegisterLiveness("forwarder")` (or `RegisterReadiness`/`RegisterStartup`) and receives a `Handle` wrapping a buffered channel of capacity 2. A background loop pings every registered channel every 15 seconds by *writing* a deadline timestamp into it. The contract is inverted from most heartbeat APIs: the component proves it is alive by *reading* from `Handle.C` in its main loop. If the component stops consuming, the channel fills, the ping fails, and the component is marked unhealthy — after roughly 30 seconds (2 buffered slots × 15 s). Startup checks are registered with the `Once` option: after the first successful ping they are healthy forever and stop being checked. The checker also watches itself: a `healthcheck` entry goes unhealthy if the ping loop itself stalls for more than twice the ping interval.

`GetLive()` returns the liveness catalog status, `GetStartup()` the startup catalog, and `GetReady()` ([`global.go`](<<<SRC>>>/pkg/status/health/global.go)) the *union* of liveness and readiness-only catalogs. Each returns a `Status{Healthy, Unhealthy}` pair of component-name lists; `*NonBlocking` variants give up after 500 ms so a probe handler can never hang. Registered components include collector check workers, DogStatsD listeners, the forwarder health checker (readiness-only), the logs-agent auditor, autodiscovery, and the tagger — grep for `health.RegisterLiveness` and `health.RegisterReadiness` to see the current census.

### The health probe server

[`comp/core/healthprobe`](<<<SRC>>>/comp/core/healthprobe/impl/healthprobe.go) runs a separate plain-HTTP server bound to `0.0.0.0:<health_port>` with four routes: `GET /live`, `GET /ready`, `GET /startup`, and `GET /` (an alias for `/live` kept for backward compatibility). A healthy result returns 200 with the JSON status; any unhealthy component returns 500 with the same JSON, which is what the kubelet interprets as a probe failure. When `log_all_goroutines_when_unhealthy` is set, every failed probe also dumps all goroutine stacks to the Agent log — invaluable for diagnosing deadlocks that manifest as liveness restarts. The default `health_port` is `0`, meaning **disabled**; the Helm chart and Operator manifests set `DD_HEALTH_PORT=5555` and point the DaemonSet probes at it. The same component runs in the standalone DogStatsD binary and the Cluster Agent (both reusing the `health_port` key) and in system-probe (`system_probe_config.health_port`), and it contributes `health.yaml` to flares.

/// warning
The probe server is unauthenticated and binds `0.0.0.0` by design (the kubelet must reach it). It only exists when `health_port` is non-zero, so on host installs it is off unless explicitly enabled.
///

### `agent health`

`agent health` ([`pkg/cli/subcommands/health`](<<<SRC>>>/pkg/cli/subcommands/health/command.go)) does not use the probe server. It calls `GET /agent/status/health` on the authenticated CMD API ([`internal/agent/agent.go`](<<<SRC>>>/comp/api/api/apiimpl/internal/agent/agent.go)), whose handler evaluates `health.GetReady()` — the readiness union, not liveness. The command prints healthy/unhealthy component lists in color and exits non-zero when anything is unhealthy. A component that is registered readiness-only can therefore fail `agent health` while the Kubernetes liveness probe stays green.

## Internal telemetry

[`comp/core/telemetry`](<<<SRC>>>/comp/core/telemetry/impl/telemetry.go) wraps a process-wide `prometheus.Registry` (pre-loaded with the Go runtime and process collectors) behind constructors like `NewCounter`, `NewGauge`, `NewHistogram` and their `WithOpts`/`Simple*` variants. Metric full names join subsystem and name with a double underscore (`api_server__request_duration_seconds`) unless `NoDoubleUnderscoreSep` is set. A second, rarely used "default" registry holds metrics created with `Options.DefaultMetric: true`; those are excluded from both the HTTP endpoint and flares. In the serverless build the whole component is compiled as a no-op ([`compat_serverless.go`](<<<SRC>>>/comp/core/telemetry/impl/compat_serverless.go)).

The Prometheus text handler is mounted at `/telemetry` on the expvar server (see the port table below), so `curl http://127.0.0.1:5000/telemetry` on the core agent dumps every internal metric. Registration is unconditional, but many *expensive producers* only emit when `telemetry.enabled` is `true` (default `false`): time-sampler and aggregator internals, DogStatsD origin telemetry (`telemetry.dogstatsd_origin`), and per-check telemetry (`telemetry.checks`, a list of check names or `*`); Python interpreter memory stats are gated separately by `telemetry.python_memory` (default `true`). The helper `IsTelemetryEnabled` in [`pkg/config/utils/telemetry.go`](<<<SRC>>>/pkg/config/utils/telemetry.go) returns true when *either* `telemetry.enabled` or `agent_telemetry.enabled` is set, which is why some producers run even with local telemetry off. The full registry dump lands in every flare as `telemetry.log`, and the API server records a `request_duration_seconds` histogram per route, method, status code, and auth mode (`mTLS` vs `token`) through the middleware in [`observability/telemetry.go`](<<<SRC>>>/comp/api/api/apiimpl/observability/telemetry.go). The process-agent has its own expvar/telemetry component in [`comp/process/expvars`](<<<SRC>>>/comp/process/expvars).

/// warning
Scraping `/telemetry` with an OpenMetrics check turns internal metrics into billable custom metrics. That is the reason `telemetry.enabled` defaults to false and is documented with a billing warning.
///

## Agent telemetry

[`comp/core/agenttelemetry`](<<<SRC>>>/comp/core/agenttelemetry/impl/agenttelemetry.go) is the other, easily confused system: it is **enabled by default** (`agent_telemetry.enabled: true`) and ships a curated allow-list of internal metrics to Datadog so the Agent team can observe Agents in the wild. It is force-disabled for FIPS builds and for the `ddog-gov.com` site (`utils.IsAgentTelemetryEnabled`).

Configuration is the embedded [`defaultProfiles.yaml`](<<<SRC>>>/comp/core/agenttelemetry/impl/defaultProfiles.yaml) (replaceable via `agent_telemetry.profiles`). Each named profile selects metrics from the internal Prometheus registry (`checks.execution_time`, `logs.*`, `transactions.*`, `dogstatsd.*`, ...), declares which tags to `preserve_tags` (all others are aggregated away), and carries a schedule — most default profiles start after 30–60 s and repeat every 900 s (a few use longer periods, up to 6 hours). The runner ([`runner.go`](<<<SRC>>>/comp/core/agenttelemetry/impl/runner.go)) is a jitter-scheduled goroutine that calls `telemetry.Gather()`, computes deltas against the previously observed counter/histogram values, aggregates tags per profile, and scrubs strings; the sender ([`sender.go`](<<<SRC>>>/comp/core/agenttelemetry/impl/sender.go)) POSTs zstd-compressed JSON payloads (`request_type: agent-metrics` or `message-batch`) to `https://instrumentation-telemetry-intake.<site>/api/v2/apmtelemetry`, honoring `agent_telemetry.additional_endpoints`.

Three extra features ride on the same component. **Startup traces** (`agent_telemetry.startup_trace_sampling`) wrap the core agent's `startAgent` in a sampled span so slow starts are visible. **Error tracking** (`agent_telemetry.errortracking.enabled`, default false) hooks the logging subsystem ([`pkg/util/log/setup/errortracking.go`](<<<SRC>>>/pkg/util/log/setup/errortracking.go)): error-level logs are captured, deduplicated within a 900 s bouncer window, buffered (2048 entries), and flushed every 60 s as `agent-logs` payloads. **Events**: other components can call `SendEvent(eventName, payload)` for curated one-off events, used for example by the [installer](../deployment/fleet.md). To see exactly what would be sent, run `agent diagnose show-metadata agent-telemetry`, which reads the component's `GET /agent/metadata/agent-telemetry` endpoint (see [Diagnostics and CLI tools](diagnostics.md)).

## Debug endpoints: expvar and pprof

Every Agent process blank-imports `net/http/pprof` and `expvar`, which register `/debug/pprof/*` and `/debug/vars` on `http.DefaultServeMux`. The core agent serves that mux — plus the `/telemetry` handler — with the [`expvarserver` component](<<<SRC>>>/comp/agent/expvarserver/impl/expvarserver.go) on `127.0.0.1:<expvar_port>`, plain HTTP, loopback-only, no auth. The other processes each have an equivalent:

| Process | Setting | Default | Transport | Notes |
|---|---|---|---|---|
| core agent | `expvar_port` | 5000 | HTTP, `127.0.0.1` | expvar, pprof, `/telemetry` |
| process-agent | `process_config.expvar_port` | 6062 | HTTP, localhost | `/telemetry` gated on `telemetry.enabled` |
| security-agent | `security_agent.expvar_port` | 5011 | HTTP, localhost | expvar, pprof |
| trace-agent | `apm_config.debug.port` | 5012 | **HTTPS** with the IPC cert | the only TLS-protected debug server |
| Cluster Agent | `metrics_port` | 5000 | HTTP, `0.0.0.0` | expvar, pprof, Prometheus metrics at `/metrics` |
| system-probe | Unix socket / named pipe | — | `/opt/datadog-agent/run/sysprobe.sock` (Linux), `\\.\pipe\dd_system_probe` (Windows) | pprof and `/telemetry` served on the socket, no TCP |

Two consumers rely on these endpoints. Flares fetch a full goroutine dump (`go-routine-dump.log`) from `/debug/pprof/goroutine?debug=2` on the expvar port ([`comp/core/profiler`](<<<SRC>>>/comp/core/profiler/impl/profiler.go); the logs agent uses the same endpoint through [`pkg/util/goroutinesdump`](<<<SRC>>>/pkg/util/goroutinesdump/goroutinedump.go) when it is force-stopped), and `agent flare --profile N` drives the [profiler component](<<<SRC>>>/comp/core/profiler/impl/profiler.go) to collect heap snapshots, an N-second CPU profile, mutex/block profiles, and an execution trace from **all** running processes in the table above — see [Flare](flare.md#profiling-flares) for the flow. The runtime settings `runtime_mutex_profile_fraction` and `runtime_block_profile_rate` enable contention profiling on a live process without restart, and continuous self-profiling to Datadog APM is available through `internal_profiling.enabled` (with `internal_profiling.capture_all_allocations` setting `runtime.MemProfileRate = 1`); see [Runtime settings](../configuration/runtime-settings.md).

## Configuration

| Key | Default | Effect |
|---|---|---|
| `expvar_port` | 5000 | Core agent expvar/pprof/`/telemetry` server |
| `telemetry.enabled` | `false` | Rich internal Prometheus telemetry (billable if scraped) |
| `telemetry.checks` | `[]` | Per-check telemetry allow-list (`*` for all) |
| `telemetry.dogstatsd_origin` | `false` | DogStatsD origin-detection telemetry |
| `telemetry.python_memory` | `true` | Python interpreter memory stats |
| `agent_telemetry.enabled` | `true` (forced off for FIPS/GovCloud) | Ship curated internal metrics to Datadog |
| `agent_telemetry.profiles` | embedded defaults | Metric selection and schedules |
| `agent_telemetry.errortracking.enabled` | `false` | Forward deduplicated error logs |
| `health_port` | `0` (disabled) | `/live`, `/ready`, `/startup` probe server; Kubernetes manifests set 5555 |
| `log_all_goroutines_when_unhealthy` | `false` | Dump goroutine stacks on failed probes |
| `internal_profiling.enabled` | `false` | Continuous self-profiling to Datadog APM |
| `runtime_mutex_profile_fraction`, `runtime_block_profile_rate` | 0 | Contention profiling, settable at runtime |

## Gotchas

1. **Two "telemetry" configs**: `telemetry.enabled` (local Prometheus detail, off by default) and `agent_telemetry.enabled` (metrics to Datadog, on by default) are independent, but `IsTelemetryEnabled` is the OR of both, and check telemetry is considered enabled for *all* checks whenever agent telemetry is on.
1. The health `Handle.C` semantics are inverted from intuition: the pinger *writes* and the component must *read*. Wrapping a health handle in a goroutine that never drains the channel makes the component permanently unhealthy after ~30 s.
1. `agent health` checks the readiness union, not liveness — it can fail while the Kubernetes liveness probe passes, and vice versa the startup catalog is invisible to it.
1. Section names in status output are case-insensitive and alphabetically sorted, except `collector` is forced first; a provider error never aborts status rendering.
1. The expvar server serves `http.DefaultServeMux`, so any package in the process that registers a handler on the default mux silently exposes it on the expvar port.
1. Metrics created with `DefaultMetric: true` go to a second registry and appear neither in `/telemetry` nor in the flare's `telemetry.log`.
1. On Fx apps, `fxutil.GetAndFilterGroup` drops nil group values — a component opts out of contributing a status or flare provider by returning an untyped nil in its `Provides` struct.
