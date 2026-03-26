> **TL;DR:** Provides a Linux-only `PidExists` helper that checks whether a process is still alive by stat-ing its `/proc/<pid>` directory, respecting the container-aware procfs root.

# pkg/util/os

## Purpose

Provides small OS-level utilities that complement the standard library. Currently Linux-only, the package exposes helpers for inspecting live processes via `/proc`.

## Key elements

### Functions

| Function | Build constraint | Description |
|---|---|---|
| `PidExists(pid int) bool` | Linux only | Returns `true` if the process with the given PID is still alive, by stat-ing `/proc/<pid>` via `kernel.HostProc`. |

### Dependencies

- [`pkg/util/kernel`](kernel.md) — used for `HostProc`, which resolves the correct `/proc` path (accounting for the host proc root when running inside a container).

## Relationship to other packages

| Package | Role |
|---|---|
| [`pkg/util/kernel`](kernel.md) | `PidExists` delegates path resolution to `kernel.HostProc`, which handles the `/host/proc` bind-mount used when the agent runs inside a container. For broader Linux process introspection (kernel version, namespace membership, process environment variables) use `pkg/util/kernel` directly. |
| [`pkg/process/procutil`](../../process/procutil.md) | The cross-platform process-information library. `procutil.NewProcessProbe` scans `/proc` for all running processes and reads per-process stats; `pkg/util/os.PidExists` is the lightweight existence check used when only aliveness matters, without reading full process state. |
| [`pkg/network/network.md`](../../network/network.md) | The network tracer (`pkg/network/tracer`) calls `PidExists` to guard Docker-proxy lifecycle management and event-consumer teardown paths, avoiding interactions with already-exited processes. |

## Usage

Used by `pkg/network/tracer` and `pkg/network/sender` to check whether a tracked process is still running before attempting further interactions with it (e.g., Docker proxy lifecycle management and event consumer teardown).

```go
import osutil "github.com/DataDog/datadog-agent/pkg/util/os"

if !osutil.PidExists(pid) {
    // process is gone, clean up
}
```

The function reads `kernel.HostProc(strconv.Itoa(pid))` internally, so it respects the container-aware procfs root set via `$HOST_PROC` or the `/host/proc` bind-mount.

## Platform notes

The package currently contains only a Linux implementation (`process_linux.go`). There is no Windows or macOS equivalent; callers should guard usage with build tags or runtime checks when cross-platform behaviour is needed.

For cross-platform PID existence checks, consider `pkg/util/kernel.ProcessExists` (Linux only, same `/proc`-based approach) or `pkg/process/procutil.NewProcessProbe` which abstracts over all three platforms at the cost of more overhead.
