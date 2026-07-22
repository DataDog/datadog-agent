# Process and container pipeline

-----

The process and container pipeline powers Live Processes, Live Containers, and (together with system-probe) Network Performance Monitoring in the Datadog app. It periodically snapshots every process and container on the host, computes rate metrics between snapshots, scrubs sensitive command-line arguments, and ships protobuf payloads to a dedicated intake at `process.<site>`. It is historically the domain of the standalone `process-agent` binary, but on Linux the checks now run inside the core agent, and the standalone binary survives only as the host for the NPM connections check.

The code is organized in three layers:

1. [`pkg/process`](<<<SRC>>>/pkg/process) — check implementations, per-platform process probes, payload formatting and chunking, submission queues.
1. [`comp/process`](<<<SRC>>>/comp/process) — the Fx component bundle wiring checks, runner, submitter, forwarders, API server, and expvars (see the [component framework overview](../components/overview.md)).
1. [`cmd/process-agent`](<<<SRC>>>/cmd/process-agent) — CLI, boot sequence, and the Windows service wrapper for the standalone binary.

The same [`process.Bundle()`](<<<SRC>>>/comp/process/bundle.go) is embedded in both the standalone process-agent and the core agent; a platform switch decides where the checks actually execute (see [where checks run](#where-checks-run-linux-vs-everywhere-else)).

## Key packages

| Path | Purpose |
|---|---|
| [`pkg/process/checks/checks.go`](<<<SRC>>>/pkg/process/checks/checks.go) | `Check` interface, check names, `RunResult`/`CombinedRunResult` |
| [`pkg/process/checks/process.go`](<<<SRC>>>/pkg/process/checks/process.go) | `ProcessCheck`: full process collection, scrubbing, formatting, chunking |
| [`pkg/process/checks/process_rt.go`](<<<SRC>>>/pkg/process/checks/process_rt.go) | Realtime (stats-only) run path emitting `CollectorRealTime` payloads |
| [`pkg/process/checks/container.go`](<<<SRC>>>/pkg/process/checks/container.go), [`container_rt.go`](<<<SRC>>>/pkg/process/checks/container_rt.go) | Container and realtime-container checks via the shared `ContainerProvider` |
| [`pkg/process/checks/process_discovery_check.go`](<<<SRC>>>/pkg/process/checks/process_discovery_check.go) | Lightweight 4-hourly process census |
| [`pkg/process/checks/net.go`](<<<SRC>>>/pkg/process/checks/net.go) | Connections check: fetches NPM data from system-probe, resolves, batches |
| [`pkg/process/checks/runner.go`](<<<SRC>>>/pkg/process/checks/runner.go) | Interleaved standard/realtime ticker (`NewRunnerWithRealTime`) |
| [`pkg/process/runner/runner.go`](<<<SRC>>>/pkg/process/runner/runner.go) | `CheckRunner`: one goroutine per check, realtime-mode switching |
| [`pkg/process/runner/submitter.go`](<<<SRC>>>/pkg/process/runner/submitter.go) | `CheckSubmitter`: weighted queues, intake headers, request IDs |
| [`pkg/process/runner/endpoint/endpoints.go`](<<<SRC>>>/pkg/process/runner/endpoint/endpoints.go) | Main and additional process intake endpoints from config |
| [`pkg/process/procutil`](<<<SRC>>>/pkg/process/procutil) | Per-platform process probes (procfs, Toolhelp/PDH, `ps`) and the `DataScrubber` |
| [`pkg/process/util/containers/containers.go`](<<<SRC>>>/pkg/process/util/containers/containers.go) | Shared `ContainerProvider` (workloadmeta metadata + metrics + tagger + filters) |
| [`pkg/process/util/api`](<<<SRC>>>/pkg/process/util/api) | `WeightedQueue`, payload encoding, intake HTTP headers |
| [`pkg/process/util/coreagent`](<<<SRC>>>/pkg/process/util/coreagent) | Compile-time platform switch: do process checks run in the core agent? |
| [`pkg/process/metadata/workloadmeta`](<<<SRC>>>/pkg/process/metadata/workloadmeta) | `WorkloadMetaExtractor` (language detection, entity diffs) and the `ProcessEntityStream` gRPC server |
| [`comp/process/bundle.go`](<<<SRC>>>/comp/process/bundle.go) | Fx bundle: runner, submitter, five check components, agent, hostinfo, expvars, apiserver, forwarders, gpusubscriber |
| [`comp/process/agent/agent_linux.go`](<<<SRC>>>/comp/process/agent/agent_linux.go), [`agent_fallback.go`](<<<SRC>>>/comp/process/agent/agent_fallback.go) | Platform gate deciding which binary flavor runs the process component |
| [`comp/process/forwarders/impl/forwarders.go`](<<<SRC>>>/comp/process/forwarders/impl/forwarders.go) | Process and realtime-process forwarders |
| [`comp/forwarder/connectionsforwarder/impl/connectionsforwarder.go`](<<<SRC>>>/comp/forwarder/connectionsforwarder/impl/connectionsforwarder.go) | Separate forwarder for connections payloads |
| [`cmd/process-agent/command/main_common.go`](<<<SRC>>>/cmd/process-agent/command/main_common.go) | Standalone binary Fx app assembly |
| [`pkg/config/setup/process.go`](<<<SRC>>>/pkg/config/setup/process.go), [`process_settings.go`](<<<SRC>>>/pkg/config/setup/process_settings.go) | All `process_config.*` defaults and env bindings |
| [`pkg/languagedetection/detector.go`](<<<SRC>>>/pkg/languagedetection/detector.go) | Cmdline-based language detection plus privileged detection RPC to system-probe |

## Check inventory

Checks implement the `checks.Check` interface in [`pkg/process/checks/checks.go`](<<<SRC>>>/pkg/process/checks/checks.go) (`Init`, `IsEnabled`, `Run`, `Cleanup`, `Realtime`, `SupportsRunOptions`, `ShouldSaveLastRun`) and are wrapped as Fx components contributing to the `group:"check"` value group. Payload types come from the [`agent-payload`](https://github.com/DataDog/agent-payload) repository (`github.com/DataDog/agent-payload/v5/process`).

| Check | Payload | Intake route | Default interval | Notes |
|---|---|---|---|---|
| `process` | `CollectorProc` | `/api/v1/collector` | 10 s | Full process metadata + stats; embeds containers |
| `rtprocess` | `CollectorRealTime` | `/api/v1/collector` | 2 s | Stats-only deltas; not a separate check object — the `ProcessCheck` runs it via `RunOptions{RunRealtime}` |
| `container` | `CollectorContainer` | `/api/v1/container` | 10 s | Only when the process check is disabled (process payloads already embed containers) |
| `rtcontainer` | `CollectorContainerRealTime` | `/api/v1/container` | 2 s | Realtime container stats; runs only while realtime mode is on |
| `connections` | `CollectorConnections` | `/api/v1/connections` | 30 s | NPM connections pulled from system-probe; standalone process-agent only |
| `process_discovery` | `CollectorProcDiscovery` | `/api/v1/discovery` | 4 h | Lightweight census (pid, cmdline, user, create time); mutually exclusive with the process check; disabled on ECS Fargate |

A former `process_events` check no longer exists; live process events moved to the event monitor / CWS stack (see [Workload Protection](../ebpf/cws.md)). Similarly, `pkg/process/monitor.ProcessMonitor` (a netlink proc-event helper) belongs to system-probe's USM, not to these checks.

## Where checks run: Linux vs everywhere else

[`pkg/process/util/coreagent`](<<<SRC>>>/pkg/process/util/coreagent) is a compile-time switch: `ProcessChecksRunInCoreAgent()` returns `true` on Linux ([`coreagent_linux.go`](<<<SRC>>>/pkg/process/util/coreagent/coreagent_linux.go)) and `false` elsewhere ([`coreagent_other.go`](<<<SRC>>>/pkg/process/util/coreagent/coreagent_other.go)). Every check's `IsEnabled()` consults it together with the binary flavor, so on Linux the process, container, and discovery checks refuse to run in the standalone process-agent and enable themselves in the core agent instead.

`Enabled()` in [`comp/process/agent/agent_linux.go`](<<<SRC>>>/comp/process/agent/agent_linux.go) then decides whether the process component is active at all for a given flavor:

- Core agent on Linux: always active (the runner logs "Starting process-component").
- Standalone process-agent on Linux: active only if the connections (NPM) check is enabled — NPM never runs in the core agent.
- Cluster check runner (`clc_runner_enabled`): never active.
- Non-Linux ([`agent_fallback.go`](<<<SRC>>>/comp/process/agent/agent_fallback.go)): active only in the process-agent flavor; AIX has its own variant.

/// warning
The historical `process_config.run_in_core_agent.enabled` toggle has been **removed**. Placement is hardcoded by platform now, and documentation referencing the old flag is stale. If a Helm chart still schedules a process-agent container on Linux without NPM, `shouldStayAlive()` in [`main_common.go`](<<<SRC>>>/cmd/process-agent/command/main_common.go) keeps the idle process alive (with an error log) purely to avoid a crash-looping container.
///

### Standalone binary boot

`runApp` in [`cmd/process-agent/command/main_common.go`](<<<SRC>>>/cmd/process-agent/command/main_common.go) builds one Fx app containing the core bundle (config, secrets, log, IPC), `process.Bundle()`, a **remote** [workloadmeta](../containers/workloadmeta.md) (`workloadmeta.Params{AgentType: workloadmeta.Remote}`), a **remote** [tagger](../containers/tagger.md), a remote workloadfilter, the statsd client, remote config, runtime settings, and the networkpath collector. The standalone process-agent never talks to container runtimes and never builds its own tagger — it consumes the core agent's workloadmeta and tagger stores over authenticated gRPC (core agent `cmd_port` 5001, see [IPC](../processes/ipc.md)). If neither the process component nor the standalone workloadmeta process collector is enabled, the app exits with `errAgentDisabled` unless `shouldStayAlive()` applies.

## Runner and realtime mode

`CheckRunner.Run()` in [`pkg/process/runner/runner.go`](<<<SRC>>>/pkg/process/runner/runner.go) spawns one goroutine per enabled check:

- Checks without run options (`container`, `connections`, `process_discovery`, `rtcontainer`) use a basic runner: run once immediately (realtime-only checks skip this priming run), then tick at the interval from [`interval.go`](<<<SRC>>>/pkg/process/checks/interval.go). Realtime-only checks (`rtcontainer`) execute only while realtime mode is on.
- The process check uses `NewRunnerWithRealTime` ([`pkg/process/checks/runner.go`](<<<SRC>>>/pkg/process/checks/runner.go)): a single ticker at the realtime interval (2 s); every `ratio = checkInterval/rtInterval` ticks (default 5) it performs a standard run, and the other ticks perform realtime runs — but only while realtime mode is enabled. The two intervals must divide evenly or both are reset to defaults with a warning.

```text
ticks (2 s):   1    2    3    4    5    6    7    8    9    10   11
RT mode off:   STD  -    -    -    -    STD  -    -    -    -    STD
RT mode on:    STD  RT   RT   RT   RT   STD  RT   RT   RT   RT   STD
```

Realtime mode is **backend-driven**; no local configuration turns it on. Every payload POST to the process intake returns a `ResCollector` body carrying `CollectorStatus{ActiveClients, Interval}`, decoded by `readResponseStatuses` in [`runner.go`](<<<SRC>>>/pkg/process/runner/runner.go). The `CheckSubmitter` forwards these statuses on a buffered channel (`GetRTNotifierChan`), and the runner's `UpdateRTStatus` enables realtime mode when any endpoint reports `ActiveClients > 0` — that is, when someone has the Live Processes page open in the Datadog app — and adjusts the realtime interval to the maximum the backend requests. `process_config.disable_realtime_checks: true` disables the whole mechanism. During a standard run with realtime enabled, the process check emits both `CollectorProc` and `CollectorRealTime` payloads in one pass (`CombinedRunResult`).

## Process collection per platform

The [`procutil.Probe`](<<<SRC>>>/pkg/process/procutil) interface (`ProcessesByPID`, `StatsForPIDs`, `StatsWithPermByPID`) has one implementation per platform:

| Platform | File | Mechanism |
|---|---|---|
| Linux | [`process_linux.go`](<<<SRC>>>/pkg/process/procutil/process_linux.go) | Hand-rolled procfs parser (no gopsutil on the hot path): `/proc/<pid>/{stat,status,statm,cmdline,io,cwd,exe,comm}` with a reused buffer pool; honors `HOST_PROC` in containers |
| Windows | [`process_windows_toolhelp.go`](<<<SRC>>>/pkg/process/procutil/process_windows_toolhelp.go) | Default: `CreateToolhelp32Snapshot` plus per-process handles (`GetProcessMemoryInfo`, `GetProcessIoCounters`, `GetProcessHandleCount`); caches the exe "file description" as a friendly name |
| Windows (alt) | [`process_windows.go`](<<<SRC>>>/pkg/process/procutil/process_windows.go) | PDH performance-counter probe, selected by `process_config.windows.use_perf_counters: true` ([`process_probe_windows.go`](<<<SRC>>>/pkg/process/checks/process_probe_windows.go)) |
| macOS | [`process_darwin.go`](<<<SRC>>>/pkg/process/procutil/process_darwin.go) | Shells out to `ps` plus sysctl; no IO stats |
| AIX | [`process_aix.go`](<<<SRC>>>/pkg/process/procutil/process_aix.go) | Reads the binary `psinfo_t` structure from `/proc/<pid>/psinfo` directly (world-readable, no elevated privileges) |
| Fallback | [`process_fallback.go`](<<<SRC>>>/pkg/process/procutil/process_fallback.go) | gopsutil-based |

On Linux, reading other users' `/proc/<pid>/io` and file-descriptor counts requires elevated permissions the agent does not have. When system-probe's process module is enabled (`system_probe_config.process_config.enabled` in `system-probe.yaml`), the check fetches privileged stats from system-probe (`GetProcStats` in [`pkg/process/net`](<<<SRC>>>/pkg/process/net)) and merges `OpenFdCount` and IO counters via `mergeProcWithSysprobeStats`. IO values of `-1` in payloads are a sentinel for "permission denied", not zero I/O.

The [`DataScrubber`](<<<SRC>>>/pkg/process/procutil/data_scrubber.go) scrubs command-line arguments matching default sensitive words plus `process_config.custom_sensitive_words` (wildcards allowed), caching scrubbed cmdlines per `(pid, createTime)` for 25 check cycles. `process_config.strip_proc_arguments` drops all arguments outright.

### The process check run pipeline

A standard run of `ProcessCheck.run` in [`process.go`](<<<SRC>>>/pkg/process/checks/process.go):

1. Samples system CPU times for delta computation.
1. Collects processes via `processesByPID()`. This is where Linux core-agent builds diverge: with build tags `linux && systemprobechecks` ([`process_linux.go`](<<<SRC>>>/pkg/process/checks/process_linux.go)) the check reads process *entities* from the [workloadmeta store](../containers/workloadmeta.md) (`wmeta.ListProcesses()`) and calls `probe.StatsForPIDs` for live stats, dropping entities whose create time mismatches (dead or PID-reused). Those entities carry service-discovery data (ports, generated service names, tracer metadata, APM instrumentation state, log files) and detected language, which populate `PortInfo`, `ServiceDiscovery`, `Language`, and `InjectionState` on `model.Process`. Everywhere else ([`process_fallback.go`](<<<SRC>>>/pkg/process/checks/process_fallback.go)) the probe is queried directly and those fields stay nil.
1. Merges system-probe privileged stats (Linux, process module enabled).
1. Fetches containers from the shared [`ContainerProvider`](<<<SRC>>>/pkg/process/util/containers/containers.go): running containers from workloadmeta, filtered by `container_include`/`container_exclude`, tagged (high cardinality), with stats from `pkg/util/containers/metrics` and rates computed against the previous run, plus a pid-to-container-ID map.
1. Feeds metadata extractors: the [`ServiceExtractor`](<<<SRC>>>/pkg/process/metadata/parser/service.go) (infers `process_context` service names from cmdlines) and, on non-Linux with language detection on, the `WorkloadMetaExtractor`.
1. `fmtProcesses`: scrub cmdlines, apply the `process_config.blacklist_patterns` disallow-list, optionally skip zombies, compute CPU%/memory/IO rates against the previous run, resolve users, attach per-PID GPU tags (from [`gpusubscriber`](<<<SRC>>>/comp/process/gpusubscriber), Linux only) and per-PID tagger tags (entity `process://<pid>`, high cardinality). Processes are grouped by container ID.
1. `createProcCtrMessages` chunks by both count (`process_config.max_per_message`, default 100) and byte weight (`process_config.max_message_bytes`, default 1 MB) via [`chunking.go`](<<<SRC>>>/pkg/process/checks/chunking.go), keeping container processes next to their container. Each `CollectorProc` carries the hostname, `NetworkId`, `SystemInfo`, group ID/size, `ContainerHostType` (`fargateECS`/`fargateEKS`), and a `HintMask` — every `process_config.process_discovery.hint_frequency`-th run (default 60) sets the discovery-hint bit so the backend refreshes discovery data.

The first run only primes rate caches and returns nothing, and `skipProcess` drops processes absent from the previous run: a process must be seen in **two consecutive collections** to be reported, so short-lived processes (under roughly one interval) never appear, and the first payload lands one interval after startup.

Realtime runs ([`process_rt.go`](<<<SRC>>>/pkg/process/checks/process_rt.go)) call `StatsForPIDs` on the PIDs remembered from the last standard run and emit compact `CollectorRealTime` payloads with 500 ms container-cache validity.

## Connections check and system-probe

The connections check ([`pkg/process/checks/net.go`](<<<SRC>>>/pkg/process/checks/net.go)) is the reason the standalone process-agent still exists on Linux. It is enabled only in the process-agent flavor, when [system-probe](../ebpf/system-probe.md)'s `network_tracer` module is on (`network_config.enabled`) and `network_config.direct_send` is false — with direct send, system-probe submits `CollectorConnections` itself and the process-agent may not be needed at all.

Flow per run:

1. `Init` registers a client with system-probe (`GET /network_tracer/register?client_id=...` over the system-probe socket) so buffering starts immediately.
1. Each run fetches `GET /network_tracer/connections` (protobuf) from system-probe.
1. Filters docker-proxy artifacts using process data, then resolves local container remote addresses with the `LocalResolver` in [`pkg/process/net`](<<<SRC>>>/pkg/process/net) (address-to-container and pid-to-container caches from the `ContainerProvider`).
1. Schedules network-path traceroute tests via the `npcollector` component.
1. Enriches with container tags, per-PID process tags from the remote tagger, host tags, and service names from the `ServiceExtractor` (governed by `system_probe_config.process_service_inference.*`). On Windows, [`net_windows.go`](<<<SRC>>>/pkg/process/checks/net_windows.go) additionally fetches IIS and remote-service tags from system-probe HTTP endpoints; [`net_linux.go`](<<<SRC>>>/pkg/process/checks/net_linux.go) only computes a listening-port-to-pid map.
1. Batches into `CollectorConnections` messages of at most `system_probe_config.max_conns_per_message` connections, with deduplicated DNS/domain string databases (offset encoding) and route/tag indices.

The mechanics of what system-probe collects are covered in [Network monitoring (NPM and USM)](../ebpf/network-monitoring.md).

## Submission path

`CheckRunner` hands results to the `CheckSubmitter` ([`pkg/process/runner/submitter.go`](<<<SRC>>>/pkg/process/runner/submitter.go)), which maintains three [`WeightedQueue`](<<<SRC>>>/pkg/process/util/api/weighted_queue.go)s: process (also carrying container and discovery payloads), rtprocess (also rtcontainer), and connections. Queue capacities are `process_config.queue_size` (256 items), `process_config.rt_queue_size` (5), and a per-queue byte budget `process_config.process_queue_bytes` (60 MB). When full, a queue evicts the oldest items **of the same payload type** first, so a burst of discovery or container payloads cannot starve process payloads in the queue they share — but sustained backpressure silently drops data (queue stats are visible in status output and expvars).

Consumers encode payloads as compressed protobuf and set the intake headers from [`pkg/process/util/api/headers`](<<<SRC>>>/pkg/process/util/api/headers): `X-DD-Agent-Timestamp`, `X-Dd-Hostname`, `X-Dd-Processagentversion`, `X-Dd-ContainerCount`, agent start time, payload source (binary flavor), and for process and connections payloads a 64-bit `X-DD-Request-ID` (22 bits seconds-in-month, 28 bits FNV hash of hostname+pid, 14 bits chunk index) that the backend uses to deduplicate messages across agent restarts.

Delivery goes through dedicated `DefaultForwarder` instances (built in [`comp/process/forwarders`](<<<SRC>>>/comp/process/forwarders/impl/forwarders.go) and [`comp/forwarder/connectionsforwarder`](<<<SRC>>>/comp/forwarder/connectionsforwarder/impl/connectionsforwarder.go)) with per-check submit methods (`SubmitProcessChecks` and friends in [`default_forwarder.go`](<<<SRC>>>/comp/forwarder/defaultforwarder/impl/default_forwarder.go)). Realtime payloads are **non-retryable** — stale realtime stats are worthless — while all other payloads use the normal retry machinery described in [Forwarder and resilience](forwarder.md). Endpoints resolve via [`endpoint.GetAPIEndpoints`](<<<SRC>>>/pkg/process/runner/endpoint/endpoints.go): the main endpoint is `https://process.<site>` (overridable with `process_config.process_dd_url`), plus `process_config.additional_endpoints` for fan-out. `process_config.drop_check_payloads` can blackhole selected checks' payloads for debugging.

## Language detection and workloadmeta process entities

The pipeline is also the producer of process entities for [workloadmeta](../containers/workloadmeta.md), which feed Kubernetes language detection (the Cluster Agent annotating deployments) and service discovery. There are three plumbing modes:

1. **Linux core agent (the default path)**: the `process-collector` workloadmeta collector ([`comp/core/workloadmeta/collectors/internal/process`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal/process/process_collector.go), build tag `linux && systemprobechecks`) runs when any of process collection, service discovery (`discovery.enabled` in `system-probe.yaml`), `language_detection.enabled`, or `gpu.enabled` is on. It scans with a procutil probe, detects languages via [`pkg/languagedetection`](<<<SRC>>>/pkg/languagedetection/detector.go), pulls service-discovery data from system-probe's `discovery` module in batches, maps pids to containers, and pushes `workloadmeta.Process` entities into the store. The process check then *reads* those entities, closing the loop.
1. **Non-Linux process-agent with the process check enabled**: the [`WorkloadMetaExtractor`](<<<SRC>>>/pkg/process/metadata/workloadmeta/extractor.go) registers as an extractor of the process check, hashes process identity (`pid|createTime|cmdHash`), detects languages for new processes (cmdline heuristics), and produces versioned cache diffs. A [gRPC server](<<<SRC>>>/pkg/process/metadata/workloadmeta/grpc.go) serves `ProcessEntityStream.StreamEntities` on `process_config.language_detection.grpc_port` (default **6262**, TLS with IPC certificates): on connect it sends a full SET snapshot, then incremental SET/UNSET diffs; a version gap forces the client to resync by dropping the stream. The core agent's `remote-process-collector` ([`comp/core/workloadmeta/collectors/internal/remote/processcollector`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal/remote/processcollector/process_collector.go), Windows-only build) consumes the stream and inserts entities into the core agent's workloadmeta.
1. **Non-Linux process-agent with the process check disabled**: a standalone `local-process` collector ([`pkg/process/metadata/workloadmeta/collector/process.go`](<<<SRC>>>/pkg/process/metadata/workloadmeta/collector/process.go)) runs inside the process-agent when `language_detection.enabled && !process_config.process_collection.enabled`, fetching processes every `workloadmeta.local_process_collector.collection_interval` (default 1 min) through its own instances of the same extractor and gRPC server.

The detector itself ([`pkg/languagedetection/detector.go`](<<<SRC>>>/pkg/languagedetection/detector.go)) uses cmdline and exe-name heuristics (python, java, node, ruby, php, dotnet, and so on) and can optionally ask system-probe's `language_detection` module (Linux only, `system_probe_config.language_detection.enabled`) for binary analysis of still-unknown PIDs, which also yields runtime versions. `language_detection.enabled` is rejected on macOS.

## Configuration

All keys live in `datadog.yaml` unless noted; environment variables follow both `DD_PROCESS_CONFIG_*` and legacy `DD_PROCESS_AGENT_*` prefixes (see `procBindEnvAndSetDefault` in [`process_settings.go`](<<<SRC>>>/pkg/config/setup/process_settings.go)).

| Key | Default | Effect |
|---|---|---|
| `process_config.process_collection.enabled` | `false` | Enables the process check (also enabled implicitly when system-probe `discovery.enabled` is on) |
| `process_config.container_collection.enabled` | `true` | Container checks (only if a container environment is detected and the process check is off) |
| `process_config.process_discovery.enabled` | `true` | Discovery census check |
| `process_config.process_discovery.interval` | 4 h (min 10 m) | Discovery interval |
| `process_config.intervals.process` / `process_realtime` | 10 s / 2 s | Standard/realtime intervals (must divide evenly) |
| `process_config.intervals.container` / `container_realtime` | 10 s / 2 s | Container check intervals |
| `process_config.intervals.connections` | 30 s | Connections check interval |
| `process_config.disable_realtime_checks` | `false` | Disables realtime mode entirely |
| `process_config.process_dd_url` | `https://process.<site>` | Intake override |
| `process_config.additional_endpoints` | `{}` | URL-to-API-keys fan-out |
| `process_config.queue_size` / `rt_queue_size` / `process_queue_bytes` | 256 / 5 / 60 MB | Submission queue caps |
| `process_config.max_per_message` / `max_message_bytes` | 100 / 1 MB (caps 10 000 / 4 MB) | Payload chunking |
| `process_config.scrub_args` / `custom_sensitive_words` / `strip_proc_arguments` | `true` / `[]` / `false` | Cmdline scrubbing |
| `process_config.blacklist_patterns` | `[]` | Regex disallow-list for processes |
| `process_config.ignore_zombie_processes` | `false` | Skip zombie processes (Linux) |
| `process_config.windows.use_perf_counters` | `false` | PDH probe instead of Toolhelp (Windows) |
| `process_config.cmd_port` | 6162 | Process-agent API server port |
| `process_config.expvar_port` | 6062 | Expvar/status port |
| `process_config.language_detection.grpc_port` | 6262 | `ProcessEntityStream` gRPC port |
| `process_config.cache_lookupid` | `false` | Cache uid-to-username lookups |
| `process_config.drop_check_payloads` | `[]` | Drop payloads of named checks (debug) |
| `language_detection.enabled` | `false` | Master switch for process entities / language detection (not supported on macOS) |
| `workloadmeta.local_process_collector.collection_interval` | 1 m | Standalone collector interval |

System-probe configuration (`system-probe.yaml`) read by this area: `network_config.enabled` (plus `network_config.direct_send`), `system_probe_config.process_config.enabled`, `system_probe_config.max_conns_per_message`, `system_probe_config.process_service_inference.*`, `system_probe_config.language_detection.enabled`, `system_probe_config.expected_tags_duration`, and `discovery.enabled` with `discovery.service_collection_*`.

/// warning
The deprecated `process_config.enabled` string (`"true"`/`"false"`/`"disabled"`) is still transformed at config load (`loadProcessTransforms` in [`pkg/config/setup/process.go`](<<<SRC>>>/pkg/config/setup/process.go)): `"true"` enables process collection but **disables** container collection — a common surprise when migrating old configurations.
///

## Deployment-mode differences

- **Linux host, container, or Kubernetes DaemonSet**: process, container, and discovery checks run in the **core agent** ("process-component"). The process-agent binary or container runs only when NPM is enabled, and only runs the connections check. Process data comes from workloadmeta on this path, enabling service-discovery and language enrichment of `CollectorProc` payloads.
- **Windows**: everything runs in the standalone process-agent, installed as the `datadog-process-agent` service ([`main_windows.go`](<<<SRC>>>/cmd/process-agent/command/main_windows.go)). Toolhelp or PDH probes; privileged system-probe stats are Linux-only. The core agent consumes process entities over gRPC port 6262 when language detection is on.
- **macOS**: standalone process-agent with the `ps`-based probe; no IO stats, no language detection, no system-probe.
- **Heroku buildpack agent**: built from `AGENT_HEROKU_TAGS` in [`tasks/build_tags.bzl`](<<<SRC>>>/tasks/build_tags.bzl), which keeps `systemprobechecks` but drops the container-runtime tags (`docker`, `containerd`, `kubelet`, and so on), so process checks follow the standard Linux core-agent path minus container data.
- **ECS Fargate / EKS Fargate**: `ContainerHostType` is set to `fargateECS`/`fargateEKS` in payloads; in sidecar mode the hostname may resolve to the task ARN ([`host_info.go`](<<<SRC>>>/pkg/process/checks/host_info.go)); the process discovery check is disabled on ECS Fargate.
- **Cluster check runner**: the process component is always disabled.

See [Binaries and flavors](../processes/binaries.md) for how the process-agent fits into the overall process topology, and [Runtime environments](../deployment/environments.md) for environment detection.

## IPC and ports

| Channel | Direction | Details |
|---|---|---|
| TCP 6162 (`process_config.cmd_port`) | CLI/status → process-agent | HTTPS with IPC TLS certificates and auth-token middleware; routes in [`cmd/process-agent/api/server.go`](<<<SRC>>>/cmd/process-agent/api/server.go): `/config`, `/agent/status`, `/agent/tagger-list`, `/agent/workload-list`, `/check/{check}`, `/secret/refresh` |
| TCP 6062 (`process_config.expvar_port`) | localhost only | Plain HTTP expvars ([`comp/process/expvars`](<<<SRC>>>/comp/process/expvars/impl/expvars.go)) rendered by `process-agent status` |
| TCP 6262 (`process_config.language_detection.grpc_port`) | core agent → process-agent | gRPC `ProcessEntityStream`, TLS from IPC certificates, snapshot-plus-diff protocol, single client stream |
| Core agent gRPC (5001) | process-agent → core agent | Hostname resolution, remote tagger, remote workloadmeta, remote workloadfilter streams; authenticated via the IPC component |
| system-probe socket | process/core agent → system-probe | Unix socket `${run_path}/sysprobe.sock` (Linux/macOS) or named pipe `\\.\pipe\dd_system_probe` (Windows); REST routes `/network_tracer/*`, `/process/stats`, `/language_detection/detect`, `/discovery/*` |
| Intake HTTPS | → `process.<site>` | `/api/v1/collector`, `/api/v1/container`, `/api/v1/connections`, `/api/v1/discovery`; protobuf with `X-Dd-*` headers; responses carry realtime-mode statuses |

The one-shot debugging entry points are `process-agent check <name>` and, on the core agent, `agent processchecks <name>` (shared implementation in [`pkg/cli/subcommands/processchecks`](<<<SRC>>>/pkg/cli/subcommands/processchecks/command.go)); both build a throwaway in-process Fx app with remote workloadmeta and tagger. In core-agent mode, process status appears as a section of `agent status`, and flares capture the last standard-run payloads of each check ([`comp/process/agent/impl/flare.go`](<<<SRC>>>/comp/process/agent/impl/flare.go)).

## Gotchas

- **Two consecutive sightings required.** `skipProcess` drops processes absent from the previous run, so the process check never reports processes living less than roughly one interval (10–20 s), and the first run after boot reports nothing.
- **Standalone and core-agent builds differ on Linux.** The process-agent binary lacks the `systemprobechecks` build tag, so identical code behaves differently (`processesByPID` falls back to the direct probe). When debugging, account for build tags, not just `GOOS`.
- **Process and container checks are mutually exclusive**, because process payloads embed containers; process discovery is likewise disabled whenever the process check is on. Enabling both yields only the process check.
- **Realtime payloads are never retried** on forwarder failure — by design, since stale realtime stats are useless.
- **The realtime interval must evenly divide the standard interval** or both are reset to defaults with a warning.
- **The `WorkloadMetaExtractor` diff channel has size 1**: if the gRPC consumer stalls, older diffs are discarded (visible as `stale_diffs` telemetry; a contended write increments `diffs_dropped`) and version gaps force the remote collector to a full resync via stream reconnect. Only one `ProcessEntityStream` client is allowed; a second `StreamEntities` call cancels the first.
- **`GetSharedContainerProvider` errors if `InitSharedContainerProvider` was not called first** — an Fx invoke-ordering constraint stemming from a tagger/workloadmeta circular dependency.
- **Hostname resolution degrades silently** in the standalone agent (config → gRPC to core agent → `dd_agent_bin hostname` subprocess → `os.Hostname()`, in [`host_info.go`](<<<SRC>>>/pkg/process/checks/host_info.go)); broken core-agent IPC can produce payloads with a mismatched hostname that split Live Processes views.
- **Statsd gauges `datadog.process.processes.host_count` and `datadog.process.containers.host_count`** are tagged `agent:<flavor>`, which is how Datadog distinguishes core-agent from process-agent execution; a 15 s heartbeat gauge `datadog.process.agent` is emitted only by the standalone flavor.
