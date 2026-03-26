> **TL;DR:** Linux-only, zero-dependency cgroup v1/v2 filesystem parser that exposes CPU, memory, I/O, and PID statistics through stable interfaces, used as the foundation for all container-level resource metrics on Linux, with a `memorymonitor` sub-package for event-driven memory pressure notifications.

# pkg/util/cgroups

## Purpose

`pkg/util/cgroups` provides a Linux-only, zero-dependency cgroup filesystem
parser that supports both cgroup v1 and cgroup v2. It reads CPU, memory, I/O,
and PID statistics directly from the cgroup pseudo-filesystem and exposes them
through a small set of stable interfaces. It is the foundation for all
container-level resource metrics collected by the agent on Linux.

The sub-package `memorymonitor/` builds on this to provide event-driven memory
pressure and OOM threshold notifications via cgroup v1 `memory.event` + epoll.

All non-test files carry `//go:build linux`; the package is not usable on other
platforms.

## Key elements

### Key types

**`CPUStats`**, **`MemoryStats`**, **`IOStats`**, **`PIDStats`**, **`PSIStats`** — stat structs with `*uint64`/`*float64` optional fields representing per-cgroup resource usage. See `### Stats types` below for field details.

**`Stats`** — top-level wrapper: `CPU *CPUStats`, `Memory *MemoryStats`, `IO *IOStats`, `PID *PIDStats`.

**Error types** (see `### Error types` below): `InvalidInputError`, `ControllerNotFoundError`, `FileSystemError`, `ValueError`.

### Key interfaces

### `Cgroup` interface (`cgroup.go`)

The primary interface returned by the `Reader`. Every implementation (v1 or v2)
satisfies it.

```go
type Cgroup interface {
    Identifier() string
    Inode() uint64
    GetParent() (Cgroup, error)
    GetCPUStats(*CPUStats) error
    GetMemoryStats(*MemoryStats) error
    GetIOStats(*IOStats) error
    GetPIDStats(*PIDStats) error
    GetPIDs(cacheValidity time.Duration) ([]int, error)
}
```

Each `Get*Stats` call reads directly from the filesystem (no caching). The
caller pre-allocates the stats struct; on partial failure the struct keeps
previously filled fields.

`GetStats(c Cgroup, stats *Stats) (allFailed bool, errs []error)` is a
convenience wrapper that calls all four methods and returns whether every one
failed.

### Key functions

See `### Reader`, `### SelfReader / NewSelfReader`, `### Filters`, and `### PID mapping` below for the full function reference.

### Stats types (`stats.go`)

All types use `*uint64` (or `*float64`) for optional fields — a `nil` pointer
means the kernel did not expose that value.

**`MemoryStats`** — hierarchical memory data. Sources: `memory.*` under cgroup
v1, `memory.*` under cgroup v2. Key fields:

| Field | Notes |
|---|---|
| `UsageTotal`, `RSS`, `Cache`, `Swap` | Bytes |
| `Limit`, `SwapLimit` | Configured limits (bytes) |
| `LowThreshold`, `HighThreshold` | v1 soft limit / v2 `memory.high` |
| `OOMEvents` | v1: `memory.failcnt`; v2: `oom` in `memory.events` |
| `OOMKiilEvents` | v2 only (`oom_kill` in `memory.events`) |
| `Peak` | v1: `max_usage_in_bytes`; v2: `memory.peak` |
| `PSISome`, `PSIFull` | Pressure Stall Information (v2 only) |

**`CPUStats`** — nanosecond-resolution CPU time. Sources: `cpu`, `cpuacct`,
`cpuset` (v1) or `cpu`, `cpuset` (v2).

| Field | Notes |
|---|---|
| `User`, `System`, `Total` | Nanoseconds |
| `Shares` | v1 only (raw share value) |
| `Weight` | v2 only (analogous to shares) |
| `ThrottledPeriods`, `ThrottledTime` | Throttling counters |
| `SchedulerPeriod`, `SchedulerQuota` | CFS bandwidth control |
| `CPUCount` | Logical CPUs from `cpuset.cpus` |
| `PSISome` | CPU pressure (v2 only) |

**`IOStats`** — block I/O. Source: `blkio.*` (v1) or `io.*` (v2). Devices are
keyed by `"MAJOR:MINOR"` in `Devices map[string]DeviceIOStats`.

**`PIDStats`** — thread/process counts. Sources: `pids.*` (both versions).

**`PSIStats`** — Pressure Stall Information sub-struct. Fields: `Avg10`,
`Avg60`, `Avg300` (percentage 0–100), `Total` (nanoseconds). Only populated
on cgroup v2.

**`Stats`** — top-level wrapper: `CPU *CPUStats`, `Memory *MemoryStats`,
`IO *IOStats`, `PID *PIDStats`.

### `Reader` (`reader.go`)

The `Reader` discovers and tracks all cgroups on the system. It is safe for
concurrent use.

**Constructor:**

```go
r, err := cgroups.NewReader(opts ...ReaderOption)
```

**Options (builder pattern):**

| Option | Description |
|---|---|
| `WithHostPrefix(prefix)` | Where host paths are mounted (e.g. `/host` inside a container) |
| `WithProcPath(path)` | Full path to `/proc`; defaults to `$hostPrefix/proc` |
| `WithReaderFilter(rf)` | Selects which cgroup folders to include and assigns identifiers |
| `WithCgroupV1BaseController(ctrl)` | Which v1 controller to use as the reference hierarchy (default: `"memory"`) |
| `WithPIDMapper(id)` | Forces a specific PID mapper strategy (`"proc"` or auto) |

**Lifecycle:**

```go
err = r.RefreshCgroups(cacheValidity time.Duration)
```

Rescans the filesystem if data is older than `cacheValidity`. Pass `0` to
always refresh. Must be called before `ListCgroups` / `GetCgroup`.

**Lookup:**

```go
cg := r.GetCgroup(id string) Cgroup
cg := r.GetCgroupByInode(inode uint64) Cgroup
list := r.ListCgroups() []Cgroup
version := r.CgroupVersion() int   // 1 or 2
```

### `SelfReader` / `NewSelfReader` (`self_reader.go`)

A `Reader` pre-configured to track only the cgroup of the current process.

```go
selfReader, err := cgroups.NewSelfReader("/proc", isContainerized, opts...)
cg := selfReader.GetCgroup(cgroups.SelfCgroupIdentifier)
```

`SelfCgroupIdentifier` is the constant `"self"`. Used by `pkg/runtime` to set
`GOMEMLIMIT` and by dogstatsd for rate-limiting.

### Filters (`reader.go`)

A `ReaderFilter` is `func(path, name string) (identifier string, error)`.
Return an empty string to skip a cgroup folder.

| Pre-built filter | Description |
|---|---|
| `DefaultFilter` | Accepts all cgroup folders; uses the full path as identifier |
| `ContainerFilter` | Matches folders whose name contains a container ID (standard hex-64, ECS hex-32, or Garden UUID formats); skips `.mount` suffixes and conmon entries |

`ContainerRegexp` and `ContainerRegexpStr` are exported for callers that need
to match container IDs independently.

### PID mapping (`pid_mapper.go`)

`GetPIDs` on a `Cgroup` uses one of two strategies, selected automatically:

- **`cgroupProcsPidMapper`** — reads `cgroup.procs` directly. Used when the
  agent runs in the host PID namespace (fast, no caching needed).
- **`procPidMapper`** — walks `/proc/<pid>/cgroup` for every process. Used
  when running in a container without host PID namespace. Results are cached
  for `cacheValidity`.

`StandalonePIDMapper` / `NewStandalonePIDMapper` provide the same capability
without a `Cgroup` object, for serverless-like scenarios where the PID and
cgroup namespaces are split.

`IdentiferFromCgroupReferences(procPath, pid, controller, filter)` parses
`/proc/<pid>/cgroup` and returns the identifier produced by the filter.

### Error types (`errors.go`)

| Type | When returned |
|---|---|
| `InvalidInputError` | Nil pointer passed to a stat method |
| `ControllerNotFoundError` | v1 controller not visible in `/proc/mounts` |
| `FileSystemError` | Error reading a cgroup pseudo-file; wraps the underlying `error` |
| `ValueError` | Unexpected content in a cgroup file; non-blocking, reported via the reporter |

`ValueError` is intentionally non-fatal and delivered through the global
reporter (`reporter.go`) rather than returned to callers, so a single
malformed file does not break collection.

### `memorymonitor` sub-package

**Build tag:** `//go:build linux`. Uses cgroup v1 only via
`containerd/cgroups/v3/cgroup1`.

`MemoryController` wraps an epoll file descriptor and a map of registered
memory events. It calls registered callbacks when a kernel memory event fires.

```go
mc, err := memorymonitor.NewMemoryController(
    "systemd",          // or "v1"
    containerized,
    memorymonitor.MemoryPercentageThresholdMonitor(onHighMem, 90, false),
    memorymonitor.MemoryPressureMonitor(onPressure, "medium"),
)
mc.Start()
defer mc.Stop()
```

Pre-built monitor factories:

| Factory | Description |
|---|---|
| `MemoryPercentageThresholdMonitor(cb, pct, swap)` | Fires when usage exceeds `pct`% of the cgroup limit |
| `MemoryThresholdMonitor(cb, limit, swap)` | Fires when usage exceeds an absolute byte threshold |
| `MemoryPressureMonitor(cb, level)` | Fires on memory pressure at `"low"`, `"medium"`, or `"critical"` level |

`MemoryMonitor` is a `func(cgroupsv1.Cgroup) (cgroupsv1.MemoryEvent, func(), error)`.
Custom monitors can be provided.

### Configuration and build flags

All non-test files carry `//go:build linux`. The package is not usable on other platforms.

The `memorymonitor/` sub-package also requires `//go:build linux` and uses cgroup v1 only via `containerd/cgroups/v3/cgroup1`.

The package has its own `go.mod` (`pkg/util/cgroups/go.mod`) and can be imported as a standalone module. There are no agent-configuration keys — all behavior is controlled programmatically through `ReaderOption` builder functions.

## Usage

### Container metrics collector (system collector)

The primary consumer is `pkg/util/containers/metrics/system/collector_linux.go`,
which creates a `Reader` scoped to container cgroups and calls `RefreshCgroups`
on each collection cycle:

```go
reader, err := cgroups.NewReader(
    cgroups.WithCgroupV1BaseController("memory"),
    cgroups.WithProcPath(procPath),
    cgroups.WithHostPrefix(hostPrefix),
    cgroups.WithReaderFilter(cgroups.ContainerFilter),
)
// per-collection-cycle:
reader.RefreshCgroups(cacheValidity)
cg := reader.GetCgroup(containerID)
var stats cgroups.Stats
cgroups.GetStats(cg, &stats)
```

### Go memory limit from cgroup (`pkg/runtime`)

```go
// pkg/runtime/gomemlimit_linux.go
selfReader, _ := cgroups.NewSelfReader("/proc", isContainerized)
cg := selfReader.GetCgroup(cgroups.SelfCgroupIdentifier)
var mem cgroups.MemoryStats
cg.GetMemoryStats(&mem)
debug.SetMemoryLimit(int64(0.9 * float64(*mem.Limit)))
```

### eBPF check cgroup lookup

`pkg/collector/corechecks/ebpf/oomkill/oom_kill.go` and similar checks use
`cgroups.ContainerFilter` together with `Reader` to map container IDs to
cgroup paths.

### dogstatsd rate-limiting

`comp/dogstatsd/listeners/ratelimit/cgroup_memory_usage_linux.go` uses
`NewSelfReader` to read the agent's own memory usage for adaptive rate
limiting.

### Memory pressure monitoring

`pkg/security` uses `memorymonitor.NewMemoryController` to trigger eviction
of security event data when the process approaches its cgroup memory limit.

## Notes

- All stat fields are pointer types. Always nil-check before dereferencing.
- `GetStats` returns `allFailed=true` only if every individual call failed;
  partial results are normal when some controllers are absent.
- When running in a container without `--cgroupns=host` (cgroup v2), PID
  resolution falls back to `/proc` walking and emits a startup warning. Some
  features (PID-to-container mapping) will be degraded.
- The package has its own `go.mod`
  (`pkg/util/cgroups/go.mod`) and can be imported as a standalone module.

---

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/util/kernel` | [pkg/util/kernel.md](kernel.md) | `pkg/util/cgroups` uses `kernel.ProcFSRoot()` / `kernel.SysFSRoot()` as the default `procPath` and `hostPrefix` when creating a `Reader` inside a containerised agent. `kernel.HostVersion()` is used by the `memorymonitor` sub-package to detect whether the host supports the `memory.event` API (cgroup v1 only). |
| `pkg/util/containers` | [pkg/util/containers.md](containers.md) | The `metrics/system` collector is the highest-priority container-stats backend on Linux. It creates a `Reader` with `ContainerFilter` to track per-container cgroup paths and calls `RefreshCgroups` / `GetStats` on each collection cycle. The generic `ContainerStats` types returned by the `Collector` interface mirror the fields in `CPUStats`, `MemoryStats`, and `IOStats`. |
| `pkg/security/resolvers` | [pkg/security/resolvers.md](../security/resolvers.md) | The CWS `cgroup/Resolver` maps PIDs and container IDs to `cgroupModel.CacheEntry` objects. It uses `cgroups.ContainerFilter` and `IdentiferFromCgroupReferences` to parse `/proc/<pid>/cgroup` and derive container identifiers from the cgroup hierarchy, mirroring the same mechanism used by the `metrics/system` collector. The `memorymonitor` sub-package is used by `pkg/security` to trigger eviction of event data when the agent process approaches its cgroup memory limit. |
