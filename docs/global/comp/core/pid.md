# comp/core/pid — PID File Management Component

**Import path:** `github.com/DataDog/datadog-agent/comp/core/pid`
**Team:** agent-runtimes
**Importers:** ~11 packages

## Purpose

`comp/core/pid` writes the current process PID to a file at startup and removes that file on shutdown. It also checks at startup that the file does not already exist with a PID for a currently-running process, preventing accidental double-starts of the same agent binary.

The actual file-level logic (write, stale-PID detection) lives in [`pkg/pidfile`](../../../pkg/pidfile.md). This component wraps it in the fx lifecycle so that PID file management is automatic and consistent across all agent binaries.

## Package layout

| Package | Role |
|---|---|
| `comp/core/pid/def` | `Component` interface |
| `comp/core/pid/impl` | `NewComponent` constructor, `Params` struct |
| `comp/core/pid/fx` | `Module()` — wires `NewComponent` into fx |
| `comp/core/pid/mock` | Test mock |

## Component interface

```go
type Component interface{}
```

The component has no public methods. Its value is entirely in its lifecycle side effects:

- **At construction** — writes the PID file (fails fast with an error if the path is occupied by a live process).
- **On `OnStop`** — removes the PID file.

## Params

`Params` is the only input the component needs:

```go
type Params struct {
    PIDfilePath string  // path to write the PID file; empty string disables the component
}

func NewParams(pidfilePath string) Params
```

When `PIDfilePath` is empty the component is effectively a no-op (no file is written and no `OnStop` hook is registered).

## fx wiring

```go
// Supply the path at startup
fx.Supply(pidimpl.NewParams(globalParams.PidFilePath)),

// Include the module (via fx or the fx wrapper)
pidfx.Module(),
// or directly:
fxutil.ProvideComponentConstructor(pidimpl.NewComponent)
```

Most agent commands pass a `--pidfile` CLI flag that populates `globalParams.PidFilePath`, which is then forwarded to `NewParams`.

The component does not depend on `comp/core/config`. The PID file path is supplied directly via `Params` at fx graph construction time, before the config component initializes. This makes it safe to place `pidfx.Module()` early in the startup sequence.

## Usage across the codebase

Every long-running agent daemon uses this component:

- **`cmd/agent`** — main agent process (`--pidfile` flag)
- **`cmd/process-agent`** — process agent
- **`cmd/system-probe`** — system probe
- **`cmd/security-agent`** — security agent
- **`cmd/otel-agent`** — OpenTelemetry agent

Example from `cmd/process-agent`:

```go
fx.Supply(pidimpl.NewParams(globalParams.PidFilePath)),
pidfx.Module(),
```

## Platform behavior

Stale-PID detection is platform-specific and implemented in [`pkg/pidfile`](../../../pkg/pidfile.md):

| Platform | Detection mechanism |
|---|---|
| Linux / FreeBSD | `os.Stat("/proc/<pid>")` |
| macOS | `syscall.Kill(pid, 0)` — signal 0 does not kill the process; success means it exists |
| Windows | `winutil.IsProcess(pid)` from `pkg/util/winutil` |

If `WritePID` detects a live conflicting process it returns a descriptive error that includes the PID and file path. The fx constructor propagates this error, causing the entire application to fail immediately rather than silently starting a second instance.

## Related packages

| Package | Relationship |
|---|---|
| [`pkg/pidfile`](../../../pkg/pidfile.md) | Provides `WritePID` — the only exported function. Handles directory creation, stale-PID detection, and atomic write. |
| [`comp/core/config`](config.md) | Not a dependency of this component; the PID path is passed directly through `Params`. However, the `pid_path` configuration key (read elsewhere) conventionally determines the default path passed to `NewParams`. |
