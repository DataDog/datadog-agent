# pkg/util/profiling

## Purpose

`pkg/util/profiling` provides a thread-safe wrapper around the [`dd-trace-go` profiler](https://pkg.go.dev/gopkg.in/DataDog/dd-trace-go.v1/profiler) for enabling **continuous internal profiling** of agent processes. It manages a single global profiler instance (start/stop/status), exposes the `Settings` configuration struct consumed by all agent binaries, and owns the thread-safe wrappers for Go runtime block/mutex profiling rate getters and setters.

---

## Key elements

### `profiling.go` — lifecycle management

**`Start(settings Settings) error`** — starts the profiler with the given settings. Thread-safe; a second call while the profiler is already running is a no-op. Profiles enabled by default: CPU and heap. Optional profiles controlled by `Settings` flags: goroutine, block, mutex.

**`Stop()`** — stops the profiler if it is running. Idempotent and thread-safe.

**`IsRunning() bool`** — returns whether the profiler is currently active.

**`GetBaseProfilingTags(extraTags []string) []string`** — builds the standard tag set that all agent processes attach to their profiles:
- `version:<AgentVersion>`
- `__dd_internal_profiling:datadog-agent`
- any caller-supplied extra tags

### URL constants

| Constant | Value |
|---|---|
| `ProfilingURLTemplate` | `"https://intake.profile.%s/v1/input"` — for agentless submission; substitute a Datadog site (e.g. `datadoghq.com`) |
| `ProfilingLocalURLTemplate` | `"http://%v/profiling/v1/input"` — for routing through a local trace-agent |

### `settings.go` — `Settings` struct

Passed to `Start`. All fields are optional; `applyDefaults()` fills in zero values before forwarding to the profiler.

| Field | Type | Description |
|---|---|---|
| `Socket` | `string` | Unix socket path for UDS submission |
| `ProfilingURL` | `string` | HTTP endpoint (agentless mode) |
| `Env` | `string` | `env` tag on profiles |
| `Service` | `string` | Service name tag |
| `Period` | `time.Duration` | How often to upload a profile |
| `CPUDuration` | `time.Duration` | Length of each CPU profile (defaults to `profiler.DefaultDuration`) |
| `MutexProfileFraction` | `int` | Sampling rate for mutex contention events |
| `BlockProfileRate` | `int` | Nanosecond block profile rate |
| `WithGoroutineProfile` | `bool` | Enable goroutine profiles |
| `WithBlockProfile` | `bool` | Enable block profiles |
| `WithMutexProfile` | `bool` | Enable mutex profiles |
| `WithDeltaProfiles` | `bool` | Upload deltas rather than full profiles |
| `Tags` | `[]string` | Additional tags |
| `CustomAttributes` | `[]string` | Goroutine label names shown as custom attributes in the Datadog Profiling UI |

### `runtime.go` — runtime profiling rate accessors

These wrap `runtime.SetBlockProfileRate` / `runtime.SetMutexProfileFraction` behind the same mutex used by `Start`/`Stop`, so the runtime settings are always consistent with what was passed to the profiler:

| Function | Description |
|---|---|
| `SetBlockProfileRate(rate int)` | Sets block profile rate and records the value so it can be restored |
| `GetBlockProfileRate() int` | Returns the last value set (Go runtime provides no getter) |
| `SetMutexProfileFraction(fraction int)` | Sets mutex profile fraction |
| `GetMutexProfileFraction() int` | Returns the current fraction via `runtime.SetMutexProfileFraction(-1)` |

---

## Usage

### Starting internal profiling in an agent binary

Every agent entry point follows the same pattern, illustrated by `cmd/agent/subcommands/run/command.go`:

```go
if cfg.GetBool("internal_profiling.enabled") {
    settings := profiling.Settings{
        ProfilingURL: fmt.Sprintf(profiling.ProfilingURLTemplate, cfg.GetString("site")),
        Env:          cfg.GetString("env"),
        Service:      "datadog-agent",
        Period:       cfg.GetDuration("internal_profiling.period"),
        Tags:         profiling.GetBaseProfilingTags(extraTags),
    }
    if err := profiling.Start(settings); err != nil {
        log.Warnf("Could not start internal profiling: %v", err)
    }
    defer profiling.Stop()
}
```

The same pattern appears in `cmd/system-probe`, `cmd/security-agent`, `cmd/process-agent`, and the trace agent (`comp/trace/agent/impl/run.go`).

### Runtime configuration settings

`pkg/config/settings/runtime_setting_profiling.go` exposes `internal_profiling.enabled` as a runtime-changeable setting. It calls `profiling.Start` / `profiling.Stop` in response to config updates. `runtime_setting_block_profile_rate.go` and `runtime_setting_mutex_profile_fraction.go` use `SetBlockProfileRate` / `SetMutexProfileFraction` to expose these values as live-tunable settings.

---

## Relationship to other packages

| Package / component | Relationship |
|---|---|
| `comp/host-profiler` ([docs](../../comp/host-profiler.md)) | A separate eBPF-based host profiler for continuous **system-wide** profiling of all processes on a Linux host. `pkg/util/profiling` profiles the **agent process itself** using the dd-trace-go profiler and submits to the Datadog profiling backend; `comp/host-profiler` profiles the entire host via eBPF and ships profiles through an embedded OTel Collector. They are complementary, not alternatives. |
| `pkg/util/log` ([docs](log.md)) | All diagnostic output from `Start`, `Stop`, and the runtime rate setters is emitted via `pkg/util/log`. |
| `pkg/config/settings` | Exposes `internal_profiling.enabled`, `internal_profiling.block_profile_rate`, and `internal_profiling.mutex_profile_fraction` as live-tunable runtime settings by calling the functions in this package. |
