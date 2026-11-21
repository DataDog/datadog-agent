# Core Agent and system-probe startup traces

Set `DD_FX_TRACING_ENABLED=true` on an Agent with a local trace-agent enabled.
Alongside the existing Fx startup trace, the core Agent sends a diagnostic trace
covering the entire synchronous `startAgent()` call, from function entry until
it returns, including early error returns. No extra configuration is needed in
`agent_telemetry`; this uses the Fx diagnostic transport.

Search APM for:

```text
service:dd-agent-fx-init operation_name:startAgent resource_name:agent
```

The root ends when `startAgent()` returns, even if initialization takes more
than a minute. This differs from Agent Telemetry's `agent.startup` root, which
uses a one-minute timer. The existing Fx `operation_name:startup` root continues
to end at Fx Started, before `startAgent()` executes.

The root and children have `fx.trace_id` linking to the Fx trace,
`process_name` identifying the process, and `agent.version` identifying the
build. Children have operation `startAgent.phase` and fixed resource names:

| Resource | Synchronous work measured |
| --- | --- |
| `startup.log` | Initial version logging. |
| `coredump.Setup` | Core dump setup. |
| `profiling.configure_allocations` | Allocation profiling configuration. |
| `common.SetupInternalProfiling` | Internal profiler setup. |
| `common.GetMainCtxCancel` | Main context retrieval. |
| `procmgr.StartReporter` | Process manager reporter setup. |
| `telemetry.Handler` | Telemetry handler creation and HTTP registration. |
| `hostname.Get` | Hostname lookup. |
| `rcclient.SubscribeAgentTask` | Remote configuration task subscription, when enabled. |
| `datastreams.NewActionsController` | Agent task provider construction, when enabled. |
| `autodiscovery.AddConfigProvider.agent_tasks` | Agent task provider registration, when enabled. |
| `datasecurity.NewController` | Data security provider construction, when enabled. |
| `autodiscovery.AddConfigProvider.data_security` | Data security provider registration, when enabled. |
| `clcrunnerapi.StartCLCRunnerServer` | Cluster check runner API startup, when enabled. |
| `leaderelection.CreateGlobalLeaderEngine` | Leader engine construction, when enabled. |
| `demultiplexer.GetDefaultSender` | Default sender retrieval. |
| `telemetry.RegisterStatsSender` | Stats sender registration, if a sender is available. |
| `installinfo.LogVersionHistory` | Version history update. |
| `check.InitializeInventoryChecksContext` | Inventory checks context initialization. |
| `jmxfetch.InitRunner` | JMX runner initialization. |
| `jmxfetch.RegisterWith` | JMX registration with Autodiscovery. |
| `commonchecks.RegisterChecks` | Core check registration. |
| `collector.InitCheckScheduler` | Check scheduler initialization. |
| `metriclookback.NewSenderManager` | Lookback sender manager creation and scheduler attachment. |
| `autodiscovery.AddScheduler` | Check scheduler registration and existing configuration replay. |
| `demultiplexer.AddAgentStartupTelemetry` | Startup telemetry registration. |
| `autodiscovery.LoadAndRun` | Waiting for background preparation and starting initial providers. |
| `misconfig.ToLog` | Misconfiguration logging. |
| `diagnose.register` | Diagnose callback registration; callbacks are not executed here. |
| `startDependentServicesAsync.launch` | Dispatching dependent service startup; background service startup is not awaited. |

The phase spans measure elapsed call time, including any blocking waits, rather
than CPU time. A call that starts background work measures only dispatch/setup.
In particular, `autodiscovery.LoadAndRun` measures the caller's time waiting for
preparation, not background preparation that already overlapped Fx startup.
The root includes small gaps between phases, including tracing overhead.

An error flag on a phase marks a returned error; a recoverable error such as
core dump setup failure does not mark the root as failed if startup succeeds.
No configuration values, filenames, or error messages are added to these traces.

## System-probe startup

The same opt-in setting enables a `startSystemProbe` root for system-probe,
using its configured local trace-agent port:

```text
service:dd-agent-fx-init operation_name:startSystemProbe resource_name:system-probe
```

On Windows the executable resource name can be `system-probe.exe`. Both the CLI
and Windows service startup paths are instrumented. The CLI root starts before
transparent huge page configuration and signal-handler setup, and ends when
`startSystemProbe()` returns, before the main loop waits for shutdown. An
intentionally disabled or externally managed system-probe returns without a root
error flag. An enabled system-probe that fails to initialize records an error.

Children use operation `startSystemProbe.phase`. In addition to configuration
access and startup logging, the root contains these phases when applicable:

| Resource | Synchronous work measured |
| --- | --- |
| `runtime.DisableTransparentHugePages` | Configuring transparent huge pages (CLI path only). |
| `signals.setup` | Installing signal handlers (CLI path only). |
| `logUserAndGroupID` | User/group lookup and logging. |
| `coredump.Setup` | Core dump setup. |
| `utils.NewMemoryMonitor` | Reading configuration and creating the memory monitor. |
| `MemoryMonitor.Start` | Starting the memory monitor. |
| `setupInternalProfiling` | Applying profiling settings. |
| `telemetry.register_collectors` | Creating and registering debug/eBPF collectors. |
| `expvar.launch` | Dispatching the debug HTTP server goroutine. |
| `api.StartServer` | Creating the listener, loading modules and setting up HTTP serving. |

`api.StartServer` has nested phases for `server.NewListener`, `module.Register`,
`api.register_handlers`, `startPacketCaptureServer` and `http.Serve.launch`.
The last two measure synchronous setup/dispatch, not completion of background
HTTP serving or readiness of its listener.

Within `module.Register`, `modules.preRegister` measures shared eBPF setup on
Linux or driver initialization on Windows. Each enabled module has separate
`module.<name>.create` and `module.<name>.register` spans for its constructor and
endpoint registration. Names are bounded identifiers such as `network_tracer`,
`event_monitor` and `gpu`; disabled modules have no per-module spans. This separates
common setup from individual module work, including kernel operations awaited by
those calls.

`modules.postRegister` measures post-registration work, including lock contention
collector initialization and BTF cache flushing on Linux. Module selection,
telemetry configuration and launching periodic statistics updates are also timed.
Recoverable module errors flag the affected phase while other modules continue
starting. If no module loads, the API phase and startup root are marked failed.

Nested phase durations include their children: do not add a parent's duration
to its children's durations. System-probe-lite's early exec handoff happens
before the Go post-Fx startup path and is outside this trace.

## Delivery and limitations

Traces are opt-in, buffered in memory and sent asynchronously through the
existing Fx transport after completion. A crash, forced exit, unavailable
trace-agent or APM sampling can leave a startup attempt absent. This diagnostic
trace does not measure image pulls, init containers, Fx initialization, first
successful check or first delivered heartbeat, and is not a fleet completeness
metric.

## Core Agent and system-probe shutdown traces

The same `DD_FX_TRACING_ENABLED=true` opt-in also records graceful shutdown:

```text
service:dd-agent-fx-init operation_name:stopAgent
service:dd-agent-fx-init operation_name:stopSystemProbe
```

Each root starts before explicit process teardown and ends after Fx's component
`OnStop` hooks. Children use `stopAgent.phase` or `stopSystemProbe.phase`.
Core phases cover health retrieval, the CLC runner, JMX, profiling, main-context
cancellation, dependent services and log flushing. System-probe phases cover
`module.Close`, individual `module.<name>.unregister` and `module.<name>.close`
calls, the expvar server, profiling, memory monitoring and log flushing.
Both roots also include each executed Fx stop hook, named by its function.
Nested durations include their children; do not sum parents and children.

The `module.event_monitor.close` span contains finer shutdown phases:

- `event_monitor.Probe.Stop`: producer cancellation, worker waits and, on Linux,
  `ebpf_probe.Manager.StopReaders`.
- `event_monitor.consumer.<ID>.Stop`: each registered consumer. The CWS consumer
  also records reloader, rule-engine, worker, API-server and gRPC-server teardown
  under `cws.*` resources.
- `event_monitor.stats.cancel` and `event_monitor.stats.Wait`: stats sender teardown.
- `event_monitor.Probe.Close`: on Linux, separate `ebpf_probe.*` resources cover
  namespace-resolver stop, raw-packet collection close, name-mapping removal,
  telemetry unregister, `ebpf_probe.Manager.Stop`, eRPC close and resolver close.

All of these are `operation_name:stopSystemProbe.phase` spans in the same trace.
`ebpf_probe.Manager.Stop` includes the dependency's reader/probe/map cleanup;
it does not create a span for each probe or each concurrent detach worker.
Existing cleanup ordering, error handling and the tracing opt-in are unchanged.

Shutdown delivery is independent of the local core/trace-agent processes. After
Fx stops, the process sends a gzip-compressed AgentPayload directly to the
configured primary APM intake (`site` / `apm_config.apm_dd_url`), or the configured
trace pipeline worker, plus `apm_config.additional_endpoints`. It uses the Agent HTTP transport's proxy/TLS settings,
cached hostname and the resolved API key already held in configuration. It
makes no shutdown-time calls to the core Agent for credentials or metadata.
The export is synchronous, with one attempt per destination and a shared
two-second deadline; destinations are attempted concurrently so a slow primary
does not prevent delivery to a secondary. That export time is outside the
measured shutdown root. Spans carry analytics
sampling metadata so analytics-enabled retention rules can retain them without
the local trace-agent processing the payload.

Resolved API keys are required. `apm_config.traces_send_to_main_endpoint:false`
skips only the primary destination; configured additional endpoints still receive
the trace. Each destination uses its own configured API key. Unresolved secret
backend or delegated credentials are skipped without preventing delivery to other
resolved destinations. This exporter does not resolve or refresh credentials
during shutdown. A configured local pipeline or proxy must remain reachable. Network
failures, expired credentials and `SIGKILL` can still leave traces missing.
When Fx returns a stop timeout, the root is marked failed and the unfinished
hook is included up to that deadline with `shutdown.incomplete:true`. A late
hook completion does not mutate the exported trace. In particular, no completed
trace is exported if the process is killed during teardown before Fx returns.
The traces diagnose completed graceful shutdowns; they do not guarantee an
inventory of all termination attempts.
