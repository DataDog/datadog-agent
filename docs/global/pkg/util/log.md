# pkg/util/log

## Purpose

`pkg/util/log` is the agent-wide logging library. It wraps a pluggable `LoggerInterface` behind a set of package-level functions (`log.Infof`, `log.Errorf`, …), so every component can emit structured log messages without holding a reference to a concrete logger.

Key design properties that matter to contributors:

- **Pre-init buffering.** Messages logged before `SetupLogger` is called are buffered in memory and flushed once the logger is ready. This covers the window between process start and config loading.
- **Automatic secret scrubbing.** Every message passes through `pkg/util/scrubber` before being written, so credentials that end up in log calls are redacted automatically. The scrub function is stored in the package-level `scrubBytesFunc` variable (defaults to `scrubber.ScrubBytes`), which lets tests substitute a no-op scrubber.
- **Dynamic log level.** The active level is stored in a `slog.LevelVar` and can be changed at runtime via `ChangeLogLevel` (wired to the `log_level` config key via an `OnUpdate` hook set up in `setup/`).
- **`Warn`/`Error`/`Critical` return an `error`.** The returned error carries the (scrubbed) log message, making it convenient to log-and-return in a single statement.

## Relationship to other packages

- **`pkg/util/scrubber`** — provides `ScrubBytes`, the function used to clean every log message before it is written. The logger stores a reference to this function. See `docs/global/pkg/util/scrubber.md` for the full list of credential patterns and runtime key registration.
- **`comp/core/log`** — the FX-injectable component that wraps this package. New components in `comp/` should declare a `logdef.Component` dependency rather than importing `pkg/util/log` directly. The `impl.NewComponent` constructor calls `setup.SetupLogger` and registers an `OnStop` hook that calls `Flush`. See `docs/global/comp/core/log.md`.
- **`pkg/config/model`** — `setup.SetupLogger` accepts a `pkgconfigmodel.Reader` and calls `cfg.OnUpdate` to propagate `log_level` changes at runtime without a restart.

---

## Key elements

### `pkg/util/log` (root package)

**`DatadogLogger`** — the singleton struct that holds the active `LoggerInterface` and the current `*slog.LevelVar`. Never used directly; all access goes through the package-level functions.

**`LoggerInterface`** (alias for `types.LoggerInterface`) — the interface a backend must implement to be plugged in. Methods: `Trace/Debug/Info/Warn/Error/Critical` (and `f` variants), `Close`, `Flush`, `SetAdditionalStackDepth`, `SetContext`.

**`LogLevel`** (alias for `types.LogLevel`) — an integer type backed by `slog.Level`. Constants: `TraceLvl`, `DebugLvl`, `InfoLvl`, `WarnLvl`, `ErrorLvl`, `CriticalLvl`, `Off`. String names: `"trace"`, `"debug"`, `"info"`, `"warn"`, `"error"`, `"critical"`, `"off"`.

**Setup functions**

| Function | When to use |
|---|---|
| `SetupLogger(i LoggerInterface, level string)` | Point the global logger at a `LoggerInterface` instance. |
| `SetupLoggerWithLevelVar(i LoggerInterface, level *slog.LevelVar)` | Same, but shares a `LevelVar` so the caller can change the level atomically later. |
| `ChangeLogLevel(level LogLevel) error` | Update the active level at runtime. |
| `ValidateLogLevel(s string) (LogLevel, error)` | Parse and validate a level string; accepts `"warning"` as an alias for `"warn"`. |

**Logging functions** — one family per level, each with three variants:

| Pattern | Example | Notes |
|---|---|---|
| Bare | `log.Info(v...)` | args joined with spaces |
| Formatted | `log.Infof(format, params...)` | `fmt.Sprintf`-style |
| Contextual | `log.Infoc(message, key, value, ...)` | appends `(key:value, …)` to the message |
| StackDepth | `log.InfoStackDepth(depth, v...)` | adjusts caller frame shown in the log line |
| Lazy | `log.InfoFunc(func() string)` | closure is only called when the level is enabled; avoids expensive `Sprintf` at trace/debug |

`Warn`, `Error`, and `Critical` variants additionally return an `error` whose message is the scrubbed log text.

**`Limit` / `NewLogLimit`** — rate-limits a log site. `NewLogLimit(n, interval)` logs the first `n` calls unconditionally, then at most once per `interval`. Combine with `ShouldLog`:

```go
lim := log.NewLogLimit(10, 10*time.Minute)
// later, inside a hot loop:
if log.ShouldLog(log.DebugLvl) && lim.ShouldLog() {
    log.Debugf("noisy event: %v", event)
}
```

**`KlogRedirectLogger`** — implements `io.Writer`; parses klog's `Lmmdd …] msg` format and forwards to the appropriate agent log level. Used to silence klog's default stderr output in Kubernetes components.

**`Wrapper`** — thin struct that re-exports all log functions with a configurable stack depth offset. Used by `comp/core/log` to expose `LoggerInterface` to FX-managed components without adding spurious frames to stack traces. Prefer the component interface (`comp/core/log/def`) in new component code.

---

### `pkg/util/log/setup`

The **configuration layer** that builds a `LoggerInterface` from agent config and installs it as the global logger. Most binaries should not call this directly — they go through `comp/core/log/impl`.

**`SetupLogger(name, level, logFile, syslogURI, syslogRFC, console, json, cfg)`** — the main entry point. Constructs an slog-backed logger with the requested outputs (file, console, syslog), registers a `cfg.OnUpdate` hook to propagate `log_level` changes, and also installs the logger as `log/slog`'s default handler.

**`LoggerName`** constants: `CoreLoggerName` (`"CORE"`), `JMXLoggerName` (`"JMXFETCH"`), `DogstatsDLoggerName` (`"DOGSTATSD"`).

**`BuildJMXLogger`** — builds a `LoggerInterface` dedicated to JMX output. The JMX logger always runs at `InfoLvl` because JMXFetch does its own level filtering. Used by the JMX check runner to capture JMXFetch output separately from the main agent log.

**`NewLogWriter(depth int, level LogLevel) (io.Writer, error)`** — wraps the global logger as an `io.Writer`. Useful for redirecting stdlib `log.Logger` or other writer-based loggers into the agent log.

**`NewTLSHandshakeErrorWriter`** — same as `NewLogWriter` but downgrades TLS handshake errors to `DEBUG` to avoid noise.

Relevant config keys read by `setup/`:

| Key | Effect |
|---|---|
| `log_level` | Active log level |
| `log_file_max_size` | Max size before file rotation |
| `log_file_max_rolls` | Number of rotated files to keep |
| `log_format_rfc3339` | Use RFC 3339 timestamps |
| `dogstatsd_log_file_max_size` / `_max_rolls` | DogStatsD-specific rotation |

Default log format (non-JSON, non-serverless):
```
2006-01-02 15:04:05 MST | CORE | INFO | (file.go:42 in funcName) | message
```

JSON format adds `"agent"`, `"time"`, `"level"`, `"file"`, `"line"`, `"func"`, `"msg"` fields.

The `serverless` build tag selects a leaner formatter without file/line information.

---

### `pkg/util/log/types`

Shared type definitions imported by all sub-packages to avoid import cycles:

- `LoggerInterface` interface
- `LogLevel` type and constants
- `ToSlogLevel` / `FromSlogLevel` conversion helpers

---

### `pkg/util/log/slog`

A `slog.Handler`-based implementation of `LoggerInterface`. This is the backend used in production.

**`slog.Wrapper`** — wraps any `slog.Handler`, handles stack-depth adjustments via `runtime.Callers`, and stores per-record context attributes atomically. Created by `slog.NewWrapper(handler)` or `slog.NewWrapperWithCloseAndFlush(handler, flush, close)`.

**`slog.Disabled()`** — returns a logger that drops all messages. Useful for tests and for disabling optional loggers.

The `handlers/` sub-package provides composable `slog.Handler` implementations:

| Handler | Purpose |
|---|---|
| `handlers.NewFormat(writer, formatter)` | Applies a custom `func(ctx, slog.Record) string` to produce each log line |
| `handlers.NewLevel(handler, levelVar)` | Adds level filtering in front of another handler |
| `handlers.NewMulti(handlers...)` | Fans out to multiple handlers (file + console + syslog) |
| `handlers.NewAsync(handler)` | Queues records and writes them in a background goroutine |
| `handlers.NewLocking(handler)` | Serialises concurrent writes with a mutex |
| `handlers.NewDisabled()` | Drops everything |

The `formatters/` sub-package provides building blocks (`Date`, `Frame`, `ShortFilePath`, `ShortFunction`, `UppercaseLevel`, `ExtraTextContext`, `ExtraJSONContext`, `Quote`, …) used by the format functions in `setup/`.

The `filewriter/` sub-package provides a size-rotating file writer consumed by `handlers.NewFormat`.

---

### `pkg/util/log/syslog`

**`syslog.Receiver`** — an `io.Writer` that writes to a local or remote syslog socket (UDP, TCP, Unix). Created with `NewReceiver(uri)`. URI examples: `"udp://localhost:514"`, `"unix:///dev/log"`.

**`syslog.HeaderFormatter(facility, rfc bool)`** — returns a function that produces RFC 5424 (or BSD-style) syslog headers for a given `LogLevel`.

---

### `pkg/util/log/zap`

**`zap.NewZapCore()`** — creates a `zapcore.Core` that routes zap log records into the agent's global logger. Used by OTel Collector components embedded in the agent.

**`zap.NewZapCoreWithDepth(depth int)`** — same, with a custom stack depth for use when the zap logger is itself wrapped by additional layers (e.g. an slog bridge).

---

## Usage

### Choosing the right entry point

There are three ways to use the logging library, and choosing correctly keeps the dependency graph clean:

| Situation | Recommended approach |
|---|---|
| New `comp/` component | Inject `logdef.Component` from `comp/core/log/def` |
| Legacy `pkg/` package without FX | Import `pkg/util/log` directly |
| Migrating an existing package | Use `logimpl.NewTemporaryLoggerWithoutInit()` as a bridge; remove once reachable via injection |

### Normal component code (pkg/)

Most components in `pkg/` import `pkg/util/log` directly and call the package-level functions:

```go
import pkglog "github.com/DataDog/datadog-agent/pkg/util/log"

pkglog.Infof("starting check %s", checkName)
if err != nil {
    return pkglog.Errorf("check %s failed: %v", checkName, err)
}
```

### FX component code (comp/)

New components built on the FX framework should declare a `log.Component` dependency (from `comp/core/log/def`) and receive it via injection:

```go
import logdef "github.com/DataDog/datadog-agent/comp/core/log/def"

type Requires struct {
    fx.In
    Log logdef.Component
}

func NewMyComp(deps Requires) MyComp {
    deps.Log.Infof("starting with value: %v", someValue)
    ...
}
```

The implementation (`comp/core/log/impl`) calls `setup.SetupLogger` internally and wraps the result in a `pkg/util/log.Wrapper` to keep stack depths consistent.

### Initialising the logger (binary entry points)

Most agent binaries go through `comp/core/log` (see `docs/global/comp/core/log.md`) which calls `setup.SetupLogger` internally. For binaries that manage this themselves, the call looks like:

```go
err := pkglogsetup.SetupLogger(
    pkglogsetup.CoreLoggerName,
    cfg.GetString("log_level"),
    cfg.GetString("log_file"),
    cfg.GetString("syslog_uri"),
    cfg.GetBool("syslog_rfc"),
    cfg.GetBool("log_to_console"),
    cfg.GetBool("log_format_json"),
    cfg,
)
```

After this call, all buffered pre-init messages are replayed. `setup.SetupLogger` also registers a `cfg.OnUpdate` callback so that changes to the `log_level` config key take effect immediately without a restart.

### Rate-limiting noisy log sites

```go
var logLimiter = log.NewLogLimit(10, 10*time.Minute)

func onEvent(e Event) {
    if log.ShouldLog(log.DebugLvl) && logLimiter.ShouldLog() {
        log.Debugf("received event: %v", e)
    }
}
```

`NewLogLimit` is initialised once (e.g. at struct creation), not per call.

### Redirecting third-party loggers

```go
// klog (Kubernetes client-go)
klog.SetOutput(log.NewKlogRedirectLogger(2))

// stdlib log.Logger
w, _ := pkglogsetup.NewLogWriter(1, pkglog.InfoLvl)
stdLogger := stdlog.New(w, "", 0)

// zap (OTel Collector)
zapLogger := zap.New(zaplog.NewZapCore())
```

### Testing

The `test` build tag activates `log_test_init.go`, which automatically calls `SetupLogger(Default(), "debug")` in an `init()` function. Tests that need to capture log output can construct a logger from a writer:

```go
var buf bytes.Buffer
logger, _ := log.LoggerFromWriterWithMinLevel(&buf, log.DebugLvl)
log.SetupLogger(logger, "debug")
```

For FX-based tests, use `logmock.New(t)` from `comp/core/log/mock` instead. It redirects all output to `t.Log` and auto-cleans up on test completion:

```go
import logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"

func TestFoo(t *testing.T) {
    logger := logmock.New(t)
    comp := NewMyComp(logger, ...)
}
```

The `slog.Disabled()` function in `pkg/util/log/slog` returns a no-op `LoggerInterface` useful when you need to call `setup.SetupLogger` in a test but don't care about output.
