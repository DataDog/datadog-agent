> **TL;DR:** `comp/core/log` is the fx-injectable structured logger for all agent code, wrapping `pkg/util/log` (seelog) so that log level, output targets, and format are driven by configuration and tests can capture output via `testing.TB.Log`.

# comp/core/log — Logging Component

**Import path:** `github.com/DataDog/datadog-agent/comp/core/log/def`
**Team:** agent-runtimes
**Importers:** ~266 packages

## Purpose

`comp/core/log` is the standard structured logger for all agent code. It wraps `pkg/util/log` (a seelog-based logger) and exposes it as an fx-injectable component. Using this component instead of the global logger means:

- Logging configuration is driven by `comp/core/config` (level, output file, syslog URI, JSON format, …).
- Tests can redirect all output to `testing.TB.Log`, making failures easy to read.
- The component flushes pending log entries on graceful shutdown.

## Package layout

| Package | Role |
|---|---|
| `comp/core/log/def` | `Component` interface and `Params` type |
| `comp/core/log/impl` | `NewComponent` constructor; wraps `pkg/util/log/setup.SetupLogger` |
| `comp/core/log/fx` | `Module()` — fx wiring that registers `impl.NewComponent` |
| `comp/core/log/mock` | Test helper `mock.New(t)` — redirects output to `t.Log` |

## Key elements

### Key interfaces

#### Component interface

```go
// Package: comp/core/log/def
type Component interface {
    Trace(v ...interface{})
    Tracef(format string, params ...interface{})

    Debug(v ...interface{})
    Debugf(format string, params ...interface{})

    Info(v ...interface{})
    Infof(format string, params ...interface{})

    Warn(v ...interface{}) error
    Warnf(format string, params ...interface{}) error

    Error(v ...interface{}) error
    Errorf(format string, params ...interface{}) error

    Critical(v ...interface{}) error
    Criticalf(format string, params ...interface{}) error

    Flush()
}
```

`Warn`, `Error`, and `Critical` also return an `error` wrapping the message — useful for one-liner log-and-return patterns.

### Key types

#### Params

`Params` controls how the logger is initialised. It is built with one of two constructors:

### `log.ForDaemon(loggerName, logFileConfig, defaultLogFile string)`

Use for long-running agent processes. All settings are read from `comp/core/config` at startup:

| Config key | Effect |
|---|---|
| `log_level` | Minimum log level |
| `<logFileConfig>` (e.g. `"log_file"`) | Log file path |
| `disable_file_logging` | Disables file output |
| `log_to_syslog` / `syslog_uri` / `syslog_rfc` | Syslog output (Linux/macOS only) |
| `log_to_console` | Stdout output |
| `log_format_json` | JSON-formatted output |

### `log.ForOneShot(loggerName, level string, overrideFromEnv bool)`

Use for CLI subcommands and short-lived tools. File logging and syslog are disabled; console output is always on. When `overrideFromEnv` is true, `DD_LOG_LEVEL` takes precedence over the supplied level.

Both return a `Params` value. After construction you can further adjust it:

```go
params.LogToFile("/tmp/custom.log")
params.LogToConsole(false)
```

### Key functions

#### fx wiring

`logfx.Module()` registers the component and is included in `core.Bundle()`. You supply `Params` through `core.BundleParams`:

```go
fx.Supply(core.BundleParams{
    ConfigParams: config.NewAgentParams(cfgPath),
    LogParams:    log.ForDaemon("AGENT", "log_file", defaultLogFile),
}),
core.Bundle(),
```

The implementation (`impl.NewComponent`) receives `Params` and `config.Component` via dependency injection, calls `pkg/util/log/setup.SetupLogger`, and registers an `OnStop` hook to flush the logger during shutdown.

#### Injecting the component

```go
import logdef "github.com/DataDog/datadog-agent/comp/core/log/def"

type Requires struct {
    fx.In
    Log logdef.Component
}

func NewMyComp(deps Requires) MyComp {
    deps.Log.Infof("Starting MyComp with config value: %v", someValue)
    ...
}
```

### Configuration and build flags

#### Mock

`mock.New(t testing.TB)` returns a `Component` that writes to `t.Log` at trace level. It is gated by the `test` build tag and cleans up after itself via `t.Cleanup`.

```go
import logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"

func TestFoo(t *testing.T) {
    logger := logmock.New(t)
    comp := NewMyComp(logger, ...)
}
```

For fx-based tests use `fxutil.Test` and supply `logmock.New(t)` as the `logdef.Component` value, or use `core.MockBundle()`.

#### Escape hatch: `NewTemporaryLoggerWithoutInit`

When migrating code that cannot yet receive `log.Component` via injection, `logimpl.NewTemporaryLoggerWithoutInit()` returns a component that delegates to the already-initialised global logger:

```go
// comp/core/log/impl
func NewTemporaryLoggerWithoutInit() logdef.Component
```

Use this only as a temporary bridge. Do not use it in new code when the component is reachable within a few stack frames.

## Relationship to pkg/util/log

`comp/core/log` is the fx-managed wrapper around `pkg/util/log`. Understanding the two layers prevents confusion:

| Layer | Package | Role |
|---|---|---|
| Global logger functions | `pkg/util/log` | `log.Infof`, `log.Errorf`, … — package-level functions that delegate to the currently installed `LoggerInterface`. Legacy `pkg/` code calls these directly. |
| Configuration layer | `pkg/util/log/setup` | `setup.SetupLogger` — builds a `LoggerInterface` from config values and installs it as the global logger. Also wires a `cfg.OnUpdate` hook for runtime `log_level` changes. |
| fx component | `comp/core/log/impl` | Calls `setup.SetupLogger` during `OnStart`, registers an `OnStop` hook that calls `Flush`. New `comp/` code injects `logdef.Component` rather than importing `pkg/util/log` directly. |

See [`pkg/util/log`](../../pkg/util/log.md) for the full logger interface, `Limit`/`ShouldLog` rate-limiting helpers, `KlogRedirectLogger`, `zap.NewZapCore`, and the slog handler architecture.

### Config keys read at init

The logger reads these keys from `comp/core/config` ([`comp/core/config`](config.md)) via `Params` during `OnStart`. All are optional; the defaults enable console output at `INFO`:

| Config key | Effect |
|---|---|
| `log_level` | Minimum log level; also watched via `OnUpdate` for runtime changes |
| `log_file` (or custom key) | Log file path |
| `disable_file_logging` | Disables file output |
| `log_to_syslog` / `syslog_uri` / `syslog_rfc` | Syslog output |
| `log_to_console` | Stdout output |
| `log_format_json` | JSON-formatted output |
| `log_file_max_size` / `log_file_max_rolls` | File rotation settings |

### Choosing the right entry point

There are three patterns depending on where the code lives:

| Situation | Recommended approach |
|---|---|
| New `comp/` component | Inject `logdef.Component` (`comp/core/log/def`) via fx |
| Legacy `pkg/` package without fx | Import `pkg/util/log` directly and call package-level functions |
| Migrating a package incrementally | Use `logimpl.NewTemporaryLoggerWithoutInit()` as a bridge |

### fxutil integration

Use `fxutil.Test` with `logmock.New(t)` or `core.MockBundle()` in component unit tests. For commands that call `fxutil.OneShot`, use `fxutil.TestOneShotSubcommand` to verify that `LogParams` is wired correctly without executing the command. See [`pkg/util/fxutil`](../../pkg/util/fxutil.md) for the full test helper API.

## Key dependents

- [`comp/core/config`](config.md) — the logger reads config values during init; config is therefore a hard dependency. Config changes to `log_level` propagate to the logger at runtime via `OnUpdate`.
- Virtually every `comp/` and `pkg/` package that needs structured logging.
- All agent binaries (`cmd/agent`, `cmd/trace-agent`, `cmd/security-agent`, …) supply `LogParams` at startup.
