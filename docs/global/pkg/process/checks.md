# pkg/process/checks

## Purpose

`pkg/process/checks` implements the individual data-collection checks run by the **process-agent**. Each check collects a specific category of observability data (processes, containers, network connections, or lightweight process discovery), serializes it into protobuf payloads, and returns those payloads to the process-agent's submission pipeline.

The package also provides shared helpers for host information collection, payload chunking, realtime/standard check scheduling, and output caching.

**Related documentation:**
- [process.md](process.md) — package-level overview and startup sequence
- [procutil.md](procutil.md) — `Probe` interface and process data model used by all checks
- [net.md](net.md) — IPC client used by `ConnectionsCheck` and `ProcessCheck` to query system-probe
- [metadata.md](metadata.md) — `ServiceExtractor` and `WorkloadMetaExtractor` wired by `ProcessCheck`
- [encoding.md](encoding.md) — serialization used when calling system-probe over the Unix socket
- [runner.md](runner.md) — `CheckRunner` and `CheckSubmitter` that schedule and forward check output
- [comp/process/types](../../comp/process/types.md) — fx types wrapping these checks for dependency injection
- [comp/process/runner](../../comp/process/runner.md) — fx component that owns the check scheduling lifecycle

## Key elements

### Check interface (`checks.go`)

```go
type Check interface {
    Name() string
    IsEnabled() bool
    Realtime() bool
    Init(syscfg *SysProbeConfig, info *HostInfo, oneShot bool) error
    SupportsRunOptions() bool
    Run(nextGroupID func() int32, options *RunOptions) (RunResult, error)
    Cleanup()
    ShouldSaveLastRun() bool
}
```

Every check in the package implements this interface. The process-agent calls `Init` once at startup and then calls `Run` on a timer loop.

### Check names (constants, `checks.go`)

| Constant | Value | Check type |
|----------|-------|------------|
| `ProcessCheckName` | `"process"` | Full process list with CPU/memory/cmdline |
| `RTProcessCheckName` | `"rtprocess"` | High-frequency CPU/memory deltas |
| `ContainerCheckName` | `"container"` | Container metadata and stats |
| `RTContainerCheckName` | `"rtcontainer"` | High-frequency container CPU/memory |
| `ConnectionsCheckName` | `"connections"` | Network connection stats (via system-probe) |
| `DiscoveryCheckName` | `"process_discovery"` | Lightweight process metadata for integration discovery |

### RunResult types (`checks.go`)

| Type | Use |
|------|-----|
| `StandardRunResult` | Single-frequency checks; `Payloads()` returns the payload slice, `RealtimePayloads()` returns nil. |
| `CombinedRunResult` | Dual-frequency checks (process, container); carries both `Standard` and `Realtime` slices. |

### ProcessCheck (`process.go`, `process_rt.go`)

`ProcessCheck` is the primary check. It:

- Reads all running processes via `procutil.Probe` (a `/proc`-based reader on Linux, Windows PDH on Windows).
  See [procutil.md](procutil.md) for the `Probe` interface and platform details.
- Scrubs sensitive command-line arguments with `procutil.DataScrubber`
  (default sensitive words: `*password*`, `*api_key*`, `*secret*`, etc.).
- Filters processes matching `process_config.blacklist_patterns`.
- Enriches processes with container association, service tags via `parser.ServiceExtractor`
  (see [metadata.md](metadata.md)), user info (`LookupIdProbe`), and GPU stats (`gpusubscriber.Component`).
- On realtime runs (`runRealtime`), fetches per-PID stats for the PIDs seen in the last standard run,
  optionally augmented with privileged stats from system-probe via `pkg/process/net.GetProcStats`
  (see [net.md](net.md)).
- Merges with system-probe's extended stats when the process module is enabled in system-probe.

Constructor: `NewProcessCheck(config, sysprobeYamlConfig, wmeta, gpuSubscriber, statsd, grpcTLSConfig, tagger) *ProcessCheck`

### RTProcessCheck

`runRealtime` is a method of `ProcessCheck`, not a separate type. The check is dual-mode: on standard ticks it collects the full process list; on realtime ticks it collects CPU/memory deltas only for the PIDs already known.

### ContainerCheck (`container.go`, `container_rt.go`)

`ContainerCheck` and `RTContainerCheck` collect container metadata and CPU/memory stats respectively. Both are enabled only when `process_config.container_collection.enabled` is true and a container environment is detected. They are mutually exclusive with `ProcessCheck` (enabling process collection disables container-only collection).

- Container data comes from `proccontainers.GetSharedContainerProvider()`, which pulls from workloadmeta.
- Network ID is fetched from system-probe when `NetworkTracerModuleEnabled` is set.

### ConnectionsCheck (`net.go`)

`ConnectionsCheck` collects live TCP/UDP connection stats by querying the **system-probe** over a Unix socket. It:

- Calls the system-probe `/connections` endpoint on every run.
- Resolves DNS names for connection endpoints via `pkg/network/dns`.
- Resolves local process/container associations via `resolver.LocalResolver`.
- Enriches connections with service names via `parser.ServiceExtractor` (see [metadata.md](metadata.md)).
- Chunks output by `maxConnsPerMessage` (configurable via `process_config.max_conns_per_message`).

The underlying HTTP-over-Unix-socket transport is provided by `pkg/process/net`; see [net.md](net.md).
`GetNetworkID` is also called from this check (Linux only) when the local network namespace lookup fails.

### ProcessDiscoveryCheck (`process_discovery_check.go`)

A lightweight alternative to `ProcessCheck`. It collects only process name, PID, and command-line for the purpose of suggesting integrations to customers. It is enabled when `discovery.enabled` is true in the system-probe config and `process_config.process_collection.enabled` is false.

### SysProbeConfig (`checks.go`)

```go
type SysProbeConfig struct {
    MaxConnsPerMessage         int
    SystemProbeAddress         string
    ProcessModuleEnabled       bool
    NetworkTracerModuleEnabled bool
}
```

Passed to `Init` to give checks access to system-probe connectivity settings.

### HostInfo (`host_info.go`)

```go
type HostInfo struct {
    SystemInfo        *model.SystemInfo
    HostName          string
    ContainerHostType model.ContainerHostType
}
```

Collected once at startup via `CollectHostInfo(config, hostnameComp, ipc)`. All checks receive a pointer to the same `HostInfo` in `Init`.

### RunnerWithRealTime (`runner.go`)

`NewRunnerWithRealTime(RunnerConfig) (func(), error)` returns a scheduling loop function that drives dual-frequency checks:

- Ticks on `RtInterval`.
- Runs a standard pass every `CheckInterval / RtInterval` ticks.
- Supports live RT interval updates via `RtIntervalChan`.
- Exits when `ExitChan` is closed.

### Chunking (`chunking.go`)

`chunkProcessesBySizeAndWeight` splits `[]*model.Process` slices into `model.CollectorProc` payloads bounded by both element count (`maxChunkSize`) and total serialized weight (`maxChunkWeight`). Containers are replicated into every chunk that contains one of their processes.

### Output caching (`exports.go`)

`StoreCheckOutput(checkName, payloads)` and `GetCheckOutput(checkName)` are a thread-safe map (`sync.Map`) that caches the most recent `[]model.MessageBody` for each check. These are read by the orchestrator checks (`pkg/collector/corechecks/orchestrator`) and workloadmeta process collector to reuse process data without running the check again.

The cached output also drives the `WorkloadMetaExtractor` language detection pipeline; the `workloadmeta/collector` uses it when the process check is disabled and language detection is the only goal (see [metadata.md](metadata.md)).

## Usage

### In the process-agent

`comp/process/agent/agent_linux.go` constructs check instances using the `New*Check` constructors and registers them with the process-agent's runner. The runner calls `Init` once and then dispatches `Run` via the standard/RT scheduler.

### In component wrappers

Each check has a corresponding `comp/process/` component (e.g. `comp/process/containercheck`, `comp/process/processdiscoverycheck`) that wraps the raw check with the fx dependency-injection lifecycle. These components import `pkg/process/checks` and call the constructors. The components register themselves in the `"check"` fx value group via `comp/process/types.ProvidesCheck`. See [comp/process/types](../../comp/process/types.md) for the fx wiring pattern.

### Check lifecycle in the runner

`comp/process/runner/runnerimpl` collects all `CheckComponent` values from the `"check"` group, filters to those where `IsEnabled()` is true, and passes them to `pkg/process/runner.CheckRunner`. The runner calls `Init` once, then dispatches `Run` on a timer. Results flow to `CheckSubmitter` which serialises payloads and enqueues them for the forwarder. See [runner.md](runner.md) and [comp/process/runner](../../comp/process/runner.md) for details.

### Probe initialisation pattern

All checks that read process data create their `procutil.Probe` inside `Init`:

```go
// pkg/process/checks/process.go (simplified)
p.probe = procutil.NewProcessProbe(
    procutil.WithPermission(syscfg.ProcessModuleEnabled),
    procutil.WithIgnoreZombieProcesses(cfg.GetBool("process_config.ignore_zombie_processes")),
)
```

On Linux, passing `WithPermission(true)` makes the probe read `/proc/<pid>/fd` and `/proc/<pid>/io`, which requires root. When system-probe's process module is enabled, the check delegates this to system-probe via `net.GetProcStats` instead and passes `WithPermission(false)`. See [procutil.md](procutil.md) for option details.

### Relevant configuration keys

| Key | Default | Description |
|-----|---------|-------------|
| `process_config.process_collection.enabled` | `false` | Enable `ProcessCheck` |
| `process_config.container_collection.enabled` | `false` | Enable `ContainerCheck` |
| `discovery.enabled` (sysprobe) | `false` | Enable `ProcessDiscoveryCheck` |
| `process_config.scrub_args` | `true` | Scrub sensitive cmdline arguments |
| `process_config.custom_sensitive_words` | `[]` | Additional words to scrub |
| `process_config.blacklist_patterns` | `[]` | Regex patterns for processes to exclude |
| `process_config.max_per_message` | 100 | Max processes per payload chunk |
| `process_config.max_ctr_procs_per_message` | 10000 | Max container-process pairs per payload |
| `process_config.max_conns_per_message` | 300 | Max connections per connections payload |
