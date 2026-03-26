> **TL;DR:** Writes the running process PID to a file at startup and guards against duplicate instances — returns an error if the PID file already points to a live process, and silently overwrites stale files.

# pkg/pidfile

## Purpose

Writes the current process PID to a file at startup, and guards against starting a second instance of the same process. If the PID file already exists and the recorded PID belongs to a running process, `WritePID` returns an error; if the PID is stale (process gone), it overwrites the file. The caller is responsible for removing the file at shutdown.

## Key elements

### Key functions

**`WritePID(pidFilePath string) error`**

The only exported function. Behavior:

1. If the file already exists and contains a PID for a currently running process, returns an error with a human-readable message pointing to the conflicting process and file path.
2. Creates any missing parent directories (mode `0755`).
3. Writes the current PID as a decimal string to the file (mode `0644`).

**Platform-specific `isProcess(pid int) bool`**

Internal helper that checks whether a PID corresponds to a running process. Implementations:

| Platform | Mechanism |
|---|---|
| Linux / FreeBSD / Solaris / etc. | `os.Stat("/proc/<pid>")` |
| macOS | `syscall.Kill(pid, 0)` — sends signal 0 (no-op); success means process exists |
| Windows | `winutil.IsProcess(pid)` from `pkg/util/winutil` |

## Usage

The package is consumed by `comp/core/pid/impl/pid.go`, the fx component that wraps PID file management in the Agent's component lifecycle:

```go
// At startup (NewComponent)
err := pidfile.WritePID(pidfilePath)

// At shutdown (lifecycle OnStop hook)
os.Remove(pidfilePath)
```

This component is wired into the main Agent (`cmd/agent`) and the Trace Agent (`comp/trace/agent/impl/agent.go`). The PID file path is typically passed as a CLI flag (e.g. `--pidfile`) and defaults to a path under the Agent's run directory.

Any agent binary that should be guarded against duplicate instances should use the `comp/core/pid` component rather than calling `pidfile.WritePID` directly.
