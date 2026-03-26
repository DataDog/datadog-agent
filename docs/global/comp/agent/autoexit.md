> **TL;DR:** `autoexit` automatically shuts down the agent when all non-agent processes on the host have exited, enabling clean container sidecar lifecycle management.

# comp/agent/autoexit

**Package:** `github.com/DataDog/datadog-agent/comp/agent/autoexit`
**Team:** agent-runtimes

## Purpose

`autoexit` implements an automatic agent shutdown mechanism for containerized environments. When the agent runs as a sidecar (e.g., in a Kubernetes pod or with s6-based supervision), it should exit automatically once its companion workload processes are gone — rather than waiting for an explicit stop signal. Without this component the agent would continue running indefinitely after the main workload exits, consuming resources and blocking pod termination.

The component reads configuration at startup and, if enabled, launches a background goroutine that polls the process list. When it detects that only excluded (known agent/supervisor) processes remain, it sends `SIGINT` to itself, triggering the normal graceful shutdown sequence.

## Key Elements

### Key interfaces

```go
// def/component.go
type Component interface{}
```

The interface carries no methods; the component's entire value is the side effect of registering and starting the background watcher during `fx` application startup.

### Key types

**`noProcessExit`** — the only built-in shutdown detector. On each tick (every 30 seconds) it calls `gopsutil/process.Processes()` and checks whether every running process name matches at least one excluded-process regex. The built-in exclusion list covers:

- Supervisor processes: `pause`, `s6-svscan`, `s6-supervise`
- Datadog processes: `agent`, `process-agent`, `trace-agent`, `security-agent`, `system-probe`, `privateactionrunner`

Extra patterns can be added via `auto_exit.noprocess.excluded_processes`.

### Key functions

**`startAutoExit`** — internal function that starts the polling goroutine. It respects the application context (`ctx.Done()`), so the watcher stops cleanly when the fx lifecycle ends. If `SIGINT` cannot be sent, it falls back to `os.Exit(1)`.

`autoexitfx.Module()` wires `NewComponent` (from `autoexitimpl`) into the fx graph. `NewComponent` calls `configureAutoExit` immediately during startup, so the watcher is active as soon as the component is instantiated.

### Configuration and build flags

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `auto_exit.noprocess.enabled` | bool | `false` | Enable the no-process exit detector |
| `auto_exit.validation_period` | int (seconds) | — | How long the exit condition must be continuously true before triggering shutdown |
| `auto_exit.noprocess.excluded_processes` | `[]string` | — | Additional regex patterns for processes that should be ignored when deciding whether to exit |

## Usage

The component is included in `comp/agent/bundle.go` and therefore available to every binary that uses the agent bundle:

```go
// comp/agent/bundle.go
autoexitfx.Module()
```

Binaries that use it:

- **`cmd/agent`** — main agent (`run` subcommand)
- **`cmd/trace-agent`** — trace agent (`run` subcommand)
- **`cmd/system-probe`** — system probe (`run` subcommand)
- **`cmd/security-agent`** — security agent (`start` subcommand)
- **`cmd/process-agent`** — process agent

Consumers declare a dependency on `autoexit.Component` via an `fx.Invoke` call to ensure the component is instantiated even though it exposes no callable methods:

```go
fx.Invoke(func(_ autoexit.Component) {})
```

To enable the feature, add to `datadog.yaml`:

```yaml
auto_exit:
  noprocess:
    enabled: true
  validation_period: 60   # seconds the condition must hold before exiting
```

## Cross-references

| Related package / component | Relationship |
|-----------------------------|--------------|
| [`comp/core/config`](../core/config.md) | `autoexit` depends on `config.Component` to read `auto_exit.noprocess.enabled`, `auto_exit.validation_period`, and `auto_exit.noprocess.excluded_processes` at startup. Configuration is read once during `NewComponent`; no runtime `OnUpdate` hook is registered. |
| [`comp/core/log`](../core/log.md) | `autoexit` injects `log.Component` to emit `Info`/`Debug`/`Error` messages from the background goroutine (process scan results, shutdown trigger, signal errors). |
| [`pkg/util/system`](../../pkg/util/system.md) | `gopsutil/v4/process.Processes()` (used by `noProcessExit.check()`) is the same process-listing primitive that `pkg/util/system` builds on for host-CPU and network-namespace helpers. Both packages share the implicit constraint that `/proc` must be readable in containerised environments for reliable operation. |
