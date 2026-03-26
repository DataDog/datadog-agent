> **TL;DR:** Go wrapper around the Windows PDH API that provides locale-aware, lifecycle-managed access to Windows performance counters for core agent checks.

# pkg/util/pdhutil

## Purpose

`pkg/util/pdhutil` wraps the Windows Performance Data Helper (PDH) API, providing Go-idiomatic access to Windows performance counters. It abstracts away:

- Direct `pdh.dll` syscalls via `golang.org/x/sys/windows`
- Locale-aware counter path resolution (Windows counter names are localized; see `README.md` for the full explanation)
- The two-collection requirement for rate-based counters
- Periodic refresh of the Windows PDH object cache to handle counters that become available after boot
- Deduplication of instance names (e.g., multiple `Process` instances with the same name)

The package is only compiled on Windows (`//go:build windows`).

## Key elements

### PDH query lifecycle

The standard usage pattern maps to the Windows PDH query lifecycle:

1. **Open query** — `CreatePdhQuery()` calls `PdhOpenQuery` and returns a `*PdhQuery`.
2. **Register counters** — call one of the `Add*` methods on `*PdhQuery`; this records the counter internally but does NOT yet call the Windows API.
3. **Collect data** — `(*PdhQuery).CollectQueryData()` adds any pending counters to the Windows query (calling `PdhAddEnglishCounter`), refreshes the PDH object cache if needed, and calls `PdhCollectQueryData` (twice if new counters were added, because rate counters need two samples).
4. **Read values** — call `GetValue()` or `GetAllValues()` on the counter handle.
5. **Close** — `(*PdhQuery).Close()` calls `PdhCloseQuery`.

### `PdhQuery`

```go
type PdhQuery struct {
    Handle   PDH_HQUERY
    counters []PdhCounter
}
```

Manages a Windows PDH query handle and a list of registered counters. Key methods:

| Method | Description |
|--------|-------------|
| `CreatePdhQuery()` | Opens a new query; returns `(*PdhQuery, error)` |
| `AddEnglishSingleInstanceCounter(object, counter)` | Registers a single-value counter (no instance) |
| `AddEnglishCounterInstance(object, counter, instance)` | Registers a counter for a specific named instance |
| `AddEnglishMultiInstanceCounter(object, counter, verifyfn)` | Registers a wildcard `(*)` counter for all instances |
| `CollectQueryData()` | Collects data; lazy-initializes any pending counters |
| `Close()` | Frees the Windows query handle |

### `PdhCounter` interface

```go
type PdhCounter interface {
    ShouldInit() bool
    AddToQuery(*PdhQuery) error
    SetInitError(error) error
    Remove() error
}
```

Extended by two sub-interfaces:
- `PdhSingleInstanceCounter` — adds `GetValue() (float64, error)`
- `PdhMultiInstanceCounter` — adds `GetAllValues() (map[string]float64, error)`

Concrete implementations: `PdhEnglishSingleInstanceCounter` and `PdhEnglishMultiInstanceCounter`.

### `PdhFormatter`

A helper for the lower-level PDH API. `(*PdhFormatter).Enum` calls `PdhGetFormattedCounterArray`, iterates instances, skips those in `ignoreInstances`, and invokes a `ValueEnumFunc` callback for each valid counter value.

### PDH object cache refresh

`refreshPdhObjectCache` calls `PdhEnumObjects` with the refresh flag. This updates Windows internals so that counters for newly started services or objects become queryable. The refresh interval is controlled by the `windows_counter_refresh_interval` config key (in seconds; `0` disables it).

### Init failure limiting

If a counter fails to initialize repeatedly, `windows_counter_init_failure_limit` caps the number of retries. After the limit is reached, `ShouldInit()` returns `false` permanently and `GetValue`/`GetAllValues` return a descriptive error.

### Predefined counter path constants

`pdh.go` exports English counter path strings for the `Process` counterset:

```go
CounterAllProcessPctProcessorTime  = `\Process(*)\% Processor Time`
CounterAllProcessWorkingSet        = `\Process(*)\Working Set`
CounterAllProcessPID               = `\Process(*)\ID Process`
// ... and many more
```

### Configuration keys

| Key | Description |
|-----|-------------|
| `windows_counter_refresh_interval` | Seconds between PDH object cache refreshes (0 = disabled) |
| `windows_counter_init_failure_limit` | Max consecutive init failures before giving up on a counter |

### Localization handling

Counter names in Windows are localized. `PdhAddEnglishCounter` (via `PdhAddEnglishCounterW`) accepts English names regardless of the system locale, so the agent always uses English strings in its counter paths. For scenarios where the lower-level `pdhMakeCounterPath` is needed, the README describes the algorithm used to find the correct locale-specific path by trying all candidate translations.

## Usage

The package is used by Windows-only core checks in `pkg/collector/corechecks/system/`:

- **`winproc`** — process CPU, memory, and I/O metrics via multi-instance `Process` counters
- **`cpu`** — CPU utilization via `Processor` counters
- **`memory`** — memory metrics via `Memory` counters
- **`filehandles`** — handle count via `Process` counters
- **`disk/io`** — disk I/O metrics via `PhysicalDisk` counters

Also used by `pkg/process/procutil/process_windows.go` for process-level metrics in the process agent.

Typical pattern for a multi-instance counter:

```go
query, err := pdhutil.CreatePdhQuery()
if err != nil { ... }
defer query.Close()

counter := query.AddEnglishMultiInstanceCounter("Process", "% Processor Time", nil)

// In the check's collect loop:
if err := query.CollectQueryData(); err != nil { ... }
values, err := counter.GetAllValues() // map[instanceName]float64
```

---

## Related packages

| Package / component | Relationship |
|---------------------|--------------|
| [`pkg/util/winutil`](../util/winutil.md) | The parallel Windows-only utility package. `winutil` covers Windows Service lifecycle (SCM), Event Log, and user/SID APIs. `pdhutil` covers performance counter (PDH) access. Core checks that collect Windows metrics often need both: PDH counters for the metric values and `winutil.SCMMonitor` or `winutil.StartService`/`StopService` for service-level operations. Both packages are compiled with `//go:build windows` and live in separate Go modules under `pkg/util/`. |
| [`pkg/collector/corechecks`](../../pkg/collector/corechecks.md) | The Windows system checks in `pkg/collector/corechecks/system/` are the primary consumers of this package. Each check creates a `PdhQuery` in its factory, registers counters, and calls `CollectQueryData()` on every `Run()` tick. The configuration keys `windows_counter_refresh_interval` and `windows_counter_init_failure_limit` are defined in `pkg/config/setup` and control refresh and retry behaviour. |
| `pkg/process/procutil` | `pkg/process/procutil/process_windows.go` uses multi-instance `Process` counters to collect per-process CPU and memory metrics for the process agent. It follows the same `CreatePdhQuery` → `AddEnglishMultiInstanceCounter` → `CollectQueryData` → `GetAllValues` lifecycle as the core checks. |
