> **TL;DR:** `pkg/process` is the core library for the Datadog process-agent, implementing all checks, check scheduling, system-probe IPC, process data collection, metadata extraction, and payload serialization needed to collect and forward process, container, and network connection data.

# pkg/process

## Purpose

`pkg/process` is the core library for the Datadog process-agent. It implements
everything needed to collect, format, and forward data about running processes,
network connections, and containers on the monitored host. The process-agent
binary (`cmd/process-agent`) is built almost entirely on top of this package.

## Package layout

| Sub-package | Responsibility | Detailed doc |
|---|---|---|
| `checks/` | Check implementations (process, container, connections, discovery) and the `Check` interface | [checks.md](checks.md) |
| `runner/` | Check scheduling loop (`CheckRunner`) and payload submission (`CheckSubmitter`) | [runner.md](runner.md) |
| `net/` | HTTP client helpers for fetching privileged stats from system-probe | [net.md](net.md) |
| `procutil/` | Cross-platform process data model and `Probe` abstraction | [procutil.md](procutil.md) |
| `monitor/` | Linux-only netlink wrapper for real-time exec/exit events | [monitor.md](monitor.md) |
| `metadata/` | Service-name extraction and workloadmeta enrichment | [metadata.md](metadata.md) |
| `encoding/` | Protobuf/msgpack marshal/unmarshal for process payloads | [encoding.md](encoding.md) |
| `status/` | Expvar-based status metrics exposed by the process-agent | — |
| `subscribers/` | Event bus subscribers (e.g. GPU subscriber) | — |
| `util/` | Shared utilities (API queue, container helpers, chunking) | — |

## Key elements

### Key types

#### Check names (constants in `checks/checks.go`)

```go
ProcessCheckName     = "process"
RTProcessCheckName   = "rtprocess"
ContainerCheckName   = "container"
RTContainerCheckName = "rtcontainer"
ConnectionsCheckName = "connections"
DiscoveryCheckName   = "process_discovery"
```

### Key interfaces

#### Check interface (`checks/checks.go`)

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

Every check must be `Init`-ed before `Run` is called. `Cleanup` releases any
resources held by the check (e.g. the `procutil.Probe`).

Each concrete check is wrapped in an fx component under `comp/process/<name>impl/`
that implements `comp/process/types.CheckComponent` and registers itself in the
`"check"` fx value group. See [comp/process/types](../../comp/process/types.md)
and [comp/process/runner](../../comp/process/runner.md) for the dependency-injection
wiring.

#### RunResult types

`StandardRunResult` — slice of `model.MessageBody` for standard (non-realtime) payloads.

`CombinedRunResult` — separate slices for standard and realtime payloads,
produced by the process check when both standard and RT collection happen in the
same tick.

### Key functions

#### Concrete checks

**ProcessCheck** (`checks/process.go`)
Collects full process state: cmdline, CPU/memory/IO stats, container
association, service name, APM injection state, and GPU subscription data. Runs
on every standard interval and, when realtime is active, on a shorter realtime
interval to refresh per-PID stats via `procutil.Probe.StatsForPIDs`.

**ProcessDiscoveryCheck** (`checks/process_discovery_check.go`)
Lighter-weight variant that collects basic process metadata (PID, cmdline,
service hints) without full stats. Used when service discovery is enabled but
live process collection is not. Controlled by `discovery.enabled` in the
system-probe config.

**ContainerCheck** (`checks/container.go`)
Collects container metadata and rate metrics from `workloadmeta`. Mutually
exclusive with `ProcessCheck` — enabling process collection disables the
standalone container check.

**ConnectionsCheck** (`checks/net.go`)
Retrieves active TCP/UDP connections from the system-probe network tracer module
via a Unix-socket HTTP call, then resolves PIDs to process names and enriches
with container/DNS metadata.

#### SysProbeConfig (`checks/checks.go`)

```go
type SysProbeConfig struct {
    MaxConnsPerMessage         int
    SystemProbeAddress         string
    ProcessModuleEnabled       bool
    NetworkTracerModuleEnabled bool
}
```

Passed to `Check.Init` to give checks knowledge of system-probe availability.
When `ProcessModuleEnabled` is true the process check delegates privileged stat
collection (open FD count, IO bytes) to system-probe via `net.GetProcStats`.

#### HostInfo (`checks/host_info.go`)

```go
type HostInfo struct {
    SystemInfo        *model.SystemInfo
    HostName          string
    ContainerHostType model.ContainerHostType
}
```

Populated once at startup by `CollectHostInfo` and shared across all checks.
Contains OS version, CPU count, and container host type (e.g. ECS, Fargate).

#### CheckRunner (`runner/runner.go`)

`CheckRunner` is the long-running goroutine that:
1. Ticks on the standard check interval.
2. Calls `Check.Run` for each enabled check.
3. Passes results to `CheckSubmitter`.
4. Manages the real-time interval separately when realtime is enabled.

Real-time can be toggled at runtime via `rtIntervalCh`; the runner adjusts its
tick frequency without restarting.

See [runner.md](runner.md) for complete API documentation including queue
configuration, RT signaling, and endpoint resolution.

#### CheckSubmitter (`runner/submitter.go`)

```go
type Submitter interface {
    Submit(start time.Time, name string, messages *types.Payload)
}
```

Serialises `model.MessageBody` payloads (protobuf or msgpack) and enqueues them
into per-check `WeightedQueue` instances. The forwarder drains these queues and
sends them to the Datadog intake.

#### ProcessMonitor (`monitor/process_monitor.go`, Linux only)

A singleton that listens for kernel process exec/exit events via netlink
(`PROC_EVENT_EXEC`, `PROC_EVENT_EXIT`). Other subsystems (e.g. USM, service
discovery) register callbacks:

```go
type ProcessCallback func(pid uint32)
// Usage:
mon := monitor.GetProcessMonitor()
unsubExec := mon.SubscribeExec(myCallback)
if err := mon.Initialize(false); err != nil { ... }
// on shutdown:
unsubExec()
mon.Stop()
```

The monitor automatically re-connects on netlink errors and backs off with a
scan of `/proc` on reconnect to avoid missing events.

See [monitor.md](monitor.md) for details on the two event-delivery modes
(netlink vs. eBPF event stream), telemetry counters, and USM/uprobe integration.

#### net sub-package (`net/`)

`GetProcStats(client, pids)` — POST to system-probe's `/proc/stats` endpoint to
retrieve `StatsWithPerm` for a list of PIDs (requires root on system-probe side).
Only compiled on Linux and Windows (`//go:build linux || windows`).

`GetNetworkID(client)` — Linux only; GETs the VPC/network ID from system-probe's
network tracer module when the agent cannot determine its own network namespace.

The wire format between the two sides is negotiated by `pkg/process/encoding`
(protobuf preferred, JSON fallback). See [encoding.md](encoding.md) and
[net.md](net.md) for details.

## Configuration keys

| Key | Default | Effect |
|---|---|---|
| `process_config.process_collection.enabled` | `false` | Enable `ProcessCheck` |
| `process_config.container_collection.enabled` | `true` (with container env) | Enable `ContainerCheck` |
| `discovery.enabled` (system-probe) | `false` | Enable `ProcessDiscoveryCheck` |
| `process_config.scrub_args` | `true` | Redact sensitive cmdline arguments |
| `process_config.strip_proc_arguments` | `false` | Strip all cmdline arguments |
| `process_config.custom_sensitive_words` | `[]` | Additional scrubber patterns |
| `process_config.blacklist_patterns` | `[]` | Regex patterns; matching processes are hidden |
| `process_config.ignore_zombie_processes` | `false` | Skip zombie processes |
| `process_config.max_per_message` | 100 | Max processes per intake message |
| `process_config.max_message_bytes` | 1 MiB | Max bytes per intake message |

## Usage in the codebase

The primary consumer is `cmd/process-agent`, which wires checks into the runner
via the component framework (`comp/process/`). A typical startup sequence:

1. `CollectHostInfo` gathers system info once.
2. `NewProcessCheck / NewContainerCheck / NewConnectionsCheck / NewProcessDiscoveryCheck` construct checks.
3. `Check.Init(sysProbeCfg, hostInfo, oneShot)` initialises each check.
4. `CheckRunner` starts its main loop.
5. On each tick, `Check.Run` produces `RunResult`; the `CheckSubmitter` serialises and forwards the payloads.

`cmd/system-probe/modules/process.go` instantiates `procutil.NewProcessProbe`
directly (Linux only) to serve privileged stats to the process-agent over a
Unix socket.

`comp/core/workloadmeta/collectors/internal/process/` uses `ProcessCheck`
output to populate workloadmeta with live process entities.

### Component framework integration

The fx component graph lives under `comp/process/`:

| Component | Role |
|---|---|
| `comp/process/types` | Shared types: `CheckComponent`, `ProvidesCheck`, `Payload`, `RTResponse` |
| `comp/process/runner/runnerimpl` | Wraps `pkg/process/runner.CheckRunner`; manages fx lifecycle |
| `comp/process/submitter/submitterimpl` | Wraps `CheckSubmitter`; publishes `RTResponse` channel |
| `comp/process/*checkimpl` | One per check; implements `CheckComponent`, registers in `"check"` group |

For detailed wiring and usage patterns see [comp/process/types](../../comp/process/types.md)
and [comp/process/runner](../../comp/process/runner.md).

### Data flow summary

```
procutil.Probe
  └── ProcessCheck / ContainerCheck / ConnectionsCheck / ProcessDiscoveryCheck
        │  (RunResult → []model.MessageBody)
        ↓
  CheckRunner  (pkg/process/runner)
        │  (types.Payload)
        ↓
  CheckSubmitter
        │  serialise via pkg/process/encoding
        │  enqueue in WeightedQueue
        ↓
  Forwarder → Datadog intake
```

`pkg/process/net` bridges `ProcessCheck` to the system-probe side for
privileged stats. `pkg/process/metadata` enriches raw `procutil.Process`
objects with service names and language labels before they are included in
payloads.
