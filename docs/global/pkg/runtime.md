# pkg/runtime

## Purpose

`pkg/runtime` provides a small set of Go runtime tuning functions that should be called early in agent startup. It handles three concerns:

1. Setting `GOMAXPROCS` to the correct value in containerized environments where the default (`runtime.NumCPU()`) exceeds the allocated vCPU quota.
2. Configuring Go's soft memory limit (`GOMEMLIMIT`) from the cgroup memory limit on Linux.
3. Disabling Transparent Huge Pages (THP) on Linux when the operator opts in.

No types are exported. All public symbols are functions.

## Key elements

### `SetMaxProcs() bool`

Sets `GOMAXPROCS` using the following priority order:

1. **`GOMAXPROCS` environment variable (millicpu notation)**: If the variable is set and ends with `m`, the value is divided by 1000 and clamped to at least 1. For example, `GOMAXPROCS=1500m` → `GOMAXPROCS=1`.
2. **`GOMAXPROCS` environment variable (integer)**: The Go runtime itself already handles this case; the function detects it and returns without further action.
3. **`automaxprocs`**: Uses `go.uber.org/automaxprocs` to read the CPU quota from Linux cgroup `cpu.cfs_quota_us` / `cpu.cfs_period_us`. This is the path taken when no explicit env var is set, and is the correct behavior for Docker and Kubernetes.

Returns `true` if GOMAXPROCS was set (either by env var or automaxprocs). Always logs the final value.

### `NumVCPU() int`

Returns the current `GOMAXPROCS` value, which after `SetMaxProcs()` reflects the actual vCPU allocation rather than the host's physical CPU count. Use this instead of `runtime.NumCPU()` anywhere that needs a concurrency ceiling (e.g. worker pool sizing).

### `SetGoMemLimit(isContainerized bool) (int64, error)` (Linux only)

Reads the cgroup memory hard limit via `pkg/util/cgroups` and calls `debug.SetMemoryLimit` with 90% of that value, allowing the GC to be more aggressive before the container OOM killer fires.

- No-op if `GOMEMLIMIT` is already set in the environment.
- No-op if no cgroup memory limit is configured.
- Returns `(0, error("unsupported"))` on non-Linux platforms.

### `DisableTransparentHugePages() error` (Linux only)

Calls `prctl(PR_SET_THP_DISABLE, 1)` to disable THP for the calling process. THP can cause latency spikes in the eBPF memory allocator and in processes with many small allocations. On non-Linux platforms this function is a no-op that returns `nil`.

## Platform behavior

| Function | Linux | Windows / macOS |
|----------|-------|-----------------|
| `SetMaxProcs` | Full (automaxprocs + env var) | Full (automaxprocs is no-op on non-Linux; env var parsing still works) |
| `NumVCPU` | Full | Full |
| `SetGoMemLimit` | Full | Returns `(0, error("unsupported"))` |
| `DisableTransparentHugePages` | Full | No-op, returns `nil` |

## Usage

All major agent binaries call these functions near the top of their `run` command, before any component initialization:

```go
import ddruntime "github.com/DataDog/datadog-agent/pkg/runtime"

// In cmd/agent/subcommands/run/command.go, cmd/system-probe, cmd/security-agent, etc.
ddruntime.SetMaxProcs()

// Optionally, based on config flag (e.g. system_probe_config.disable_thp):
if err := ddruntime.DisableTransparentHugePages(); err != nil {
    log.Warnf("cannot disable THP: %s", err)
}
```

The trace agent additionally calls `SetGoMemLimit` to take advantage of cgroup-aware GC tuning:

```go
// In comp/trace/agent/impl/agent.go
cgmem, err := agentrt.SetGoMemLimit(env.IsContainerized())
```

`NumVCPU()` is used wherever a concurrency limit should match the process's actual CPU allocation rather than the host's CPU count.

## Dependencies

- `go.uber.org/automaxprocs` — Linux cgroup CPU quota reader.
- `golang.org/x/sys/unix` — `prctl` for THP on Linux.
- `pkg/util/cgroups` — cgroup memory stat reader (Linux only).

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/util/cgroups` | [cgroups.md](../util/cgroups.md) | `SetGoMemLimit` (Linux only) calls `cgroups.NewSelfReader` to obtain the current process's own cgroup, then reads `MemoryStats.Limit` to derive the 90% soft memory ceiling passed to `debug.SetMemoryLimit`. The `SelfCgroupIdentifier` constant (`"self"`) is the lookup key used here. |
| `comp/core/config` | [config.md](../../comp/core/config.md) | Agent binaries read the `system_probe_config.disable_thp` config key (via `comp/core/config` or `pkg/config/setup`) to decide whether to call `DisableTransparentHugePages`. The runtime package itself does not import the config package; the decision is made by the caller in each binary's startup command. |
