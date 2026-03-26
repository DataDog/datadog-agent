# comp/agent/jmxlogger

**Package:** `github.com/DataDog/datadog-agent/comp/agent/jmxlogger`
**Team:** agent-metric-pipelines

## Purpose

`jmxlogger` provides a dedicated logger for the JMXFetch subprocess. JMXFetch is the Java daemon the agent spawns to collect JMX-based metrics (JVM, Tomcat, Kafka, etc.). Its output is piped back to the Go agent and needs to be routed to the correct log destination — either the agent's standard log file (when running normally) or a separate file when invoked from the CLI for debugging (`agent jmx collect`, `agent jmx list`, etc.).

Without this component the JMXFetch output would have no typed logger and callers would have to manage log file setup themselves.

## Key Elements

### Interface

```go
// component.go
type Component interface {
    JMXInfo(v ...interface{})
    JMXError(v ...interface{}) error
    Flush()
}
```

| Method | Description |
|--------|-------------|
| `JMXInfo` | Writes an info-level log message from JMXFetch output |
| `JMXError` | Writes an error-level log message; returns any logging error |
| `Flush` | Flushes buffered log output (called on graceful shutdown) |

### `Params`

Controls how the logger is configured:

```go
// Two constructors in jmxloggerimpl/params.go
func NewCliParams(logFile string) Params  // CLI mode: log to the given file
func NewDefaultParams() Params            // Normal mode: respect agent config
```

In CLI mode (`fromCLI: true`) the logger writes only to the specified file (typically placed in the JMX flare directory). In normal mode it honours `jmx_log_file`, `disable_file_logging`, `syslog_rfc`, `log_to_console`, and `log_format_json` from `datadog.yaml`.

### Lifecycle

A `fx.Hook` registered on `OnStop` calls `Flush()` and then `close()` (which releases the underlying seelog instance) so that no log lines are dropped during agent shutdown.

### FX wiring

`jmxloggerimpl.Module(params)` wires `newJMXLogger` into the fx graph. The `Params` value is supplied at the call site so that CLI subcommands can inject a different params than the long-running agent process.

## Usage

The component lives in `comp/agent/bundle.go` and is wired with caller-supplied `Params`:

```go
// comp/agent/bundle.go
func Bundle(params jmxloggerimpl.Params) fxutil.BundleOptions {
    return fxutil.BundleOptions{
        ...
        jmxloggerimpl.Module(params),
        ...
    }
}
```

**Normal agent startup** (`cmd/agent/subcommands/run/command.go`) passes `NewDefaultParams()`, routing JMX logs to the configured JMX log file.

**CLI JMX subcommands** (`cmd/agent/subcommands/jmx/command.go`) pass `NewCliParams(logFile)` with a timestamped file in the flare directory:

```go
agent.Bundle(jmxloggerimpl.NewCliParams(cliParams.logFile))
```

**Primary consumer** — `pkg/jmxfetch`:

```go
func NewJMXFetch(logger jmxlogger.Component, ipc ipc.Component) *JMXFetch {
    ...
}
// During JMXFetch.Start():
j.Output = j.logger.JMXInfo        // stdout of JMXFetch process -> JMXInfo
// Error lines from stderr:
_ = j.logger.JMXError(in.Text())
```

Other callers: `pkg/cli/standalone/jmx.go`, `pkg/cli/subcommands/check/command.go`, `pkg/jmxfetch/state.go`, `pkg/jmxfetch/runner.go`.

## Cross-references

| Related package / component | Relationship |
|-----------------------------|--------------|
| [`pkg/jmxfetch`](../../pkg/jmxfetch.md) | Primary consumer. `jmxfetch.NewJMXFetch(logger, ipc)` injects `jmxlogger.Component` and pipes the subprocess stdout to `logger.JMXInfo` and stderr to `logger.JMXError`. The `JMXLoggerName = "JMXFETCH"` constant (defined in `pkg/util/log/setup`) tags all JMX log lines in the agent's log output. The logger runs at `InfoLvl` regardless of the agent's global log level because JMXFetch performs its own level filtering internally. |
| [`comp/core/log`](../core/log.md) | `jmxlogger` is a companion, not a wrapper, of `comp/core/log`. In normal daemon mode, `newJMXLogger` calls `pkg/util/log/setup.BuildJMXLogger` which creates a **separate** seelog instance that writes to `jmx_log_file` (or inherits `log_file` when `jmx_log_file` is unset). This keeps JMXFetch output out of the main agent log. The same `disable_file_logging`, `syslog_rfc`, `log_to_console`, and `log_format_json` config keys that `comp/core/log` reads are also honoured by the JMX logger. `Flush()` is called on `OnStop` — mirroring the flush hook registered by `comp/core/log` — to ensure no JMX log lines are dropped during shutdown. |
| [`comp/core/config`](../core/config.md) | In default (non-CLI) mode, `newJMXLogger` reads `jmx_log_file`, `disable_file_logging`, `syslog_rfc`, `log_to_console`, and `log_format_json` from `config.Component` to construct the seelog configuration. In CLI mode (`NewCliParams`) these keys are ignored and the logger writes exclusively to the caller-supplied file path. |
