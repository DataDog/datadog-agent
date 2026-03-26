> **TL;DR:** `pkg/process/procutil` is a cross-platform library that abstracts OS-level process data collection (Linux `/proc`, Windows PDH, macOS sysctl) behind a common `Probe` interface, providing the `Process`/`Stats` data model used by all process-related checks.

# pkg/process/procutil

## Purpose

`pkg/process/procutil` is a cross-platform library for reading raw process
information from the operating system. It abstracts over Linux `/proc`, Windows
PDH counters, and macOS sysctl calls behind a common `Probe` interface, and
provides the `Process` / `Stats` data model used by all process-related checks
in the agent.

The package has no dependency on the rest of the agent except logging utilities,
making it easy to test in isolation and to use from system-probe (privileged
side) as well as from the process-agent (unprivileged side).

**Related documentation:**
- [process.md](process.md) — package-level overview and how procutil fits in the architecture
- [checks.md](checks.md) — how `ProcessCheck` and `ProcessDiscoveryCheck` create and use the `Probe`
- [pkg/util/cgroups](../util/cgroups.md) — Linux cgroup reader; complements procutil for container-level metrics on Linux

## Key elements

### Key interfaces

#### Probe interface (`probe.go`)

```go
type Probe interface {
    Close()
    // ProcessesByPID returns full process info for all running processes.
    // Set collectStats=false to skip memory stat collection.
    ProcessesByPID(now time.Time, collectStats bool) (map[int32]*Process, error)
    // StatsForPIDs returns lightweight stats for a known list of PIDs.
    // Used by the realtime check and system-probe.
    StatsForPIDs(pids []int32, now time.Time) (map[int32]*Stats, error)
    // StatsWithPermByPID returns stats that require elevated permission
    // (open FD count, IO counters). Called from system-probe on Linux.
    StatsWithPermByPID(pids []int32) (map[int32]*StatsWithPerm, error)
}

type Option func(p Probe)
```

`NewProcessProbe(options ...Option)` is the constructor on every platform.

### Key types

#### Data model (`process_model.go`)

**Process** — full process snapshot:

```go
type Process struct {
    Pid, Ppid, NsPid int32
    Name, Cwd, Exe, Comm string
    Cmdline   []string
    Username  string      // Windows only
    Uids, Gids []int32
    Language  *languagemodels.Language
    TCPPorts, UDPPorts []uint16
    Stats          *Stats
    Service        *Service
    InjectionState InjectionState
    ContainerID    string
}
```

**Stats** — per-process metrics collected on every tick:

```go
type Stats struct {
    CreateTime  int64  // milliseconds since epoch
    Status      string // "R", "S", "D", "T", "Z", "W", "U"
    Nice        int32
    OpenFdCount int32
    NumThreads  int32
    CPUPercent  *CPUPercentStat
    CPUTime     *CPUTimesStat
    MemInfo     *MemoryInfoStat
    MemInfoEx   *MemoryInfoExStat
    IOStat      *IOCountersStat
    IORateStat  *IOCountersRateStat
    CtxSwitches *NumCtxSwitchesStat
}
```

**StatsWithPerm** — subset of `Stats` that requires root to read on Linux
(`/proc/<pid>/fd` for FD count, `/proc/<pid>/io` for IO bytes):

```go
type StatsWithPerm struct {
    OpenFdCount int32
    IOStat      *IOCountersStat
}
```

**IOCountersStat** — a value of `-1` in any field means the agent lacked
permission to read it (not an error). Check with `IOCountersStat.IsZeroValue()`.

**Service** — service-discovery metadata attached to the process: generated
service name, `DD_SERVICE` value, APM tracer metadata, and associated log files.

**InjectionState** — APM auto-injector detection:
`InjectionUnknown`, `InjectionInjected`, `InjectionNotInjected`.

### Key functions

#### Process identity helpers

```go
// Stable string key for a process across exec scenarios where PID is recycled.
func ProcessIdentity(pid int32, createTime int64, cmdline []string) string

// Returns true if two Process values represent the same OS process.
func IsSameProcess(a, b *Process) bool
```

Identity is based on `(pid, createTime, FNV-1a hash of first 100 cmdline args)`.
The 100-arg cap bounds CPU cost for pathological processes that pass tens of
thousands of arguments.

#### DataScrubber (`data_scrubber.go`)

Redacts values of sensitive command-line arguments before they leave the agent.
`ProcessCheck` creates one scrubber per check instance and calls
`ScrubProcessCommand` on every process before building the payload.

```go
scrubber := procutil.NewDefaultDataScrubber()
scrubber.AddCustomSensitiveWords([]string{"my_secret_flag"})
clean := scrubber.ScrubProcessCommand(proc) // returns scrubbed []string
scrubber.IncrementCacheAge()                 // call once per check cycle
```

- Default sensitive words include `*password*`, `*api_key*`, `*secret*`,
  `*access_token*`, `stripetoken`, etc. (defined in
  `data_scrubber_fallback.go` on non-Windows, `data_scrubber_windows.go` on
  Windows).
- Configured by `process_config.custom_sensitive_words` and
  `process_config.strip_proc_arguments` (see [checks.md](checks.md) for the
  full config key table).
- Patterns support single `*` wildcards but not `**` or bare `*`.
- Results are cached by `(pid, createTime)` and the cache is flushed every 25
  check cycles (`cacheMaxCycles`).
- `StripAllArguments: true` removes everything after the executable name.
- The scrubber replaces matched values with `********` in-place on the joined
  cmdline string.

### Configuration and build flags

Platform implementations are selected via build tags (`linux`, `windows`, `darwin`). Options controlling privileged reads, procfs root override, and zombie-process handling are listed below.

#### Platform-specific options (Linux, `process_linux.go`)

All options are no-ops on non-Linux platforms (`option_unsupported.go`).

| Option | Effect |
|---|---|
| `WithPermission(bool)` | When `true`, reads `/proc/<pid>/fd` and `/proc/<pid>/io` (requires root) |
| `WithReturnZeroPermStats(bool)` | When `false` (default in system-probe), omit entries where all perm stats are zero, saving work on the process-agent side |
| `WithProcFSRoot(path)` | Override the procfs root (default: from `kernel.ProcFSRoot()`); useful in containers or tests |
| `WithIgnoreZombieProcesses(bool)` | Skip zombie processes during full scan |
| `WithBootTimeRefreshInterval(duration)` | How often to re-read `/proc/stat` to correct clock drift (default: 1 minute) |

#### Linux implementation notes (`process_linux.go`)

- Reads `/proc/<pid>/stat`, `/proc/<pid>/status`, `/proc/<pid>/statm`,
  `/proc/<pid>/cmdline`, `/proc/<pid>/comm`, `/proc/<pid>/io`.
- Kernel threads (PF_KTHREAD flag set, no cmdline) are silently skipped.
- FD count uses `ReadDirent` directly rather than `os.Readdirnames` to avoid
  allocating a slice of filenames. On Linux 6.2+ it reads the count from
  `stat(2)` on `/proc/<pid>/fd` in a single syscall.
- Boot time is read from `/proc/stat btime`, refreshed periodically to handle
  NTP corrections.
- Clock ticks are retrieved via `getconf CLK_TCK`; fall back to 100 (the
  universal Linux default).
- `ensurePathReadable` enforces a security check before opening any
  `/proc/<pid>/*` symlink: root bypasses it; otherwise the file must be
  world-readable or owned by the running UID/EUID.

Note: container-level cgroup metrics (CPU throttling, memory limits, PSI) are
**not** part of procutil. They are provided by `pkg/util/cgroups`
(see [pkg/util/cgroups](../util/cgroups.md)), which operates on the cgroup
filesystem independently of `/proc`.

#### Windows implementation (`process_windows.go`)

- Uses PDH (Performance Data Helper) counters for CPU, memory, handle count,
  and IO metrics.
- Collects the `Username` field (not available on Linux).
- `process_windows_toolhelp.go` uses the `CreateToolhelp32Snapshot` API as a
  fallback for enumerating processes.
- An LRU cache (`fileDescCache`, capacity 512) stores file descriptions keyed
  by executable path.

#### macOS implementation (`process_darwin.go`)

- Uses `sysctl` and `proc_pidinfo` (via `golang.org/x/sys/unix`) to enumerate
  processes and read per-process stats.
- `StatsWithPermByPID` is not meaningfully implemented (returns empty map);
  privileged stat collection is Linux/Windows only.

## Usage in the codebase

**Process-agent checks** (`pkg/process/checks/`)
`ProcessCheck`, `ProcessDiscoveryCheck`, and the RT process check all call
`probe.ProcessesByPID` or `probe.StatsForPIDs` on each tick. The probe is
created in `Check.Init`:

```go
p.probe = procutil.NewProcessProbe(
    procutil.WithPermission(syscfg.ProcessModuleEnabled),
    procutil.WithIgnoreZombieProcesses(cfg.GetBool("process_config.ignore_zombie_processes")),
)
```

When `syscfg.ProcessModuleEnabled` is `true`, privileged stats (FD count, IO
bytes) are fetched from system-probe instead of reading `/proc/<pid>/fd` and
`/proc/<pid>/io` directly; the check passes `WithPermission(false)` in that
case and merges via `pkg/process/net.GetProcStats`. See [checks.md](checks.md)
and [net.md](net.md) for details.

**System-probe process module** (`cmd/system-probe/modules/process.go`, Linux)
Creates a probe with `WithReturnZeroPermStats(false)` and serves
`StatsWithPermByPID` results over a Unix socket to the process-agent:

```go
p := procutil.NewProcessProbe(procutil.WithReturnZeroPermStats(false))
```

The serialisation format between system-probe and the process-agent is defined
in `pkg/process/encoding` (see [encoding.md](encoding.md)).

**Workloadmeta process collector**
(`comp/core/workloadmeta/collectors/internal/process/`)
Calls `ProcessesByPID` to populate live process entities in workloadmeta.

**Security process list** (`pkg/security/process_list/`)
Uses `procutil.Process` as the entity type for the runtime security agent's
process cache.

**Service extraction** (`pkg/process/metadata/parser/`, `pkg/network/sender/`)
Takes a `*procutil.Process` and derives a service name from the cmdline, env
vars, and APM tracer metadata. See [metadata.md](metadata.md) for the full set
of per-runtime heuristics (Java Spring Boot, Node.js `package.json`, etc.).
