# comp/core/flare

## Purpose

`comp/core/flare` is the fx component responsible for creating and sending **flare archives** — ZIP bundles that Datadog Support uses to diagnose agent issues. When you run `datadog-agent flare <case-id>` or a Remote Config `TaskFlare` task fires, this component orchestrates the collection of logs, configs, runtime state, and diagnostics from all registered providers, packages them into a scrubbed archive, and uploads it to Datadog.

The component also exposes a `POST /flare` IPC endpoint so the CLI process can trigger archive creation inside the running agent.

---

## Package Layout

| Path | Role |
|---|---|
| `comp/core/flare` (root) | `Component` interface, `Params`, `Module()`, implementation (`flare.go`, `providers.go`) |
| `comp/core/flare/types` | Lightweight provider registration types — import this to add data to a flare without pulling in all flare dependencies |
| `comp/core/flare/builder` | `FlareBuilder` and `FlareArgs` interface definitions |
| `comp/core/flare/helpers` | `NewFlareBuilder`, `SendTo`, `FlareSource` — archive creation and upload implementation |

---

## Key Elements

### Component interface

```go
type Component interface {
    Create(pdata types.ProfileData, providerTimeout time.Duration, ipcError error, diagnoseResult []byte) (string, error)
    CreateWithArgs(flareArgs types.FlareArgs, providerTimeout time.Duration, ipcError error, diagnoseResult []byte) (string, error)
    Send(flarePath string, caseID string, email string, source helpers.FlareSource) (string, error)
}
```

- `Create` / `CreateWithArgs` — collect all provider data and write a ZIP file; return its local path.
- `Send` — upload the ZIP to Datadog and delete the local copy (unless `Params.KeepArchiveAfterSend` is set).
- If `providerTimeout <= 0`, the value from `flare_provider_timeout` config key is used.

### Params

```go
type Params struct {
    KeepArchiveAfterSend     bool
    // (private) local, distPath, defaultLogFile, defaultJMXLogFile, etc.
}

// For the running agent process:
flare.NewParams(distPath, pythonChecksPath, logFile, jmxLogFile, dogstatsdLogFile, streamlogsLogFile)

// For the CLI when the agent is unreachable (local fallback mode):
flare.NewLocalParams(distPath, ...)
```

In local mode the archive is built directly in the CLI process without any runtime data; a `local` marker file is added to the archive explaining the reason.

### Module

```go
func Module(params Params) fxutil.Module
```

Supplies `params` via `fx.Supply` and wires `newFlare` as the provider. The implementation also registers:
- a `POST /flare` API endpoint
- a Remote Config `TaskFlare` listener

### Provider pattern (`comp/core/flare/types`)

Any component can contribute files to every flare by returning a `types.Provider` from its `Provides` struct:

```go
// Simplest form — no custom timeout:
flaretypes.NewProvider(func(fb flaretypes.FlareBuilder) error {
    return fb.AddFileFromFunc("my-component.log", collectData)
})

// With a custom timeout:
flaretypes.NewProviderWithTimeout(callback, timeoutFunc)
```

`Provider` carries an `fx.Out` tag `group:"flare"`. The flare component collects all tagged fillers via `deps.Providers []*types.FlareFiller \`group:"flare"\``.

### FlareBuilder interface (`comp/core/flare/builder`)

Passed to every provider callback. Key methods:

| Method | Description |
|---|---|
| `AddFile(dest, content)` | Add in-memory bytes (scrubbed) |
| `AddFileFromFunc(dest, cb)` | Lazy content from a callback (scrubbed) |
| `AddFileWithoutScrubbing(dest, content)` | For binary files (e.g., pprof profiles) |
| `CopyFile(src)` / `CopyFileTo(src, dest)` | Copy a file from disk (scrubbed) |
| `CopyDirTo(src, dest, include)` | Recursively copy a directory (scrubbed) |
| `CopyDirToWithoutScrubbing(src, dest, include)` | Copy pre-scrubbed files (e.g., agent logs) |
| `PrepareFilePath(path)` | Reserve a path for external tools to write to |
| `IsLocal()` | True when running in CLI fallback mode |
| `GetFlareArgs()` | Access caller-supplied `FlareArgs` |
| `Logf(format, ...)` | Write to `flare-creation.log` inside the archive |
| `Save()` | Finalize and write the ZIP |

The builder automatically scrubs known secret patterns from all content. Still, avoid adding data that contains credentials in the first place.

### FlareArgs

```go
type FlareArgs struct {
    StreamLogsDuration   time.Duration
    ProfileDuration      time.Duration
    ProfileMutexFraction int
    ProfileBlockingRate  int
}
```

Available to provider callbacks via `fb.GetFlareArgs()`. Default (zero) values are safe to ingest — providers should degrade gracefully when no profiling/streaming is requested.

### Built-in providers (registered in `newFlare`)

Beyond FX-injected providers, the implementation always adds:
- **Log files** — agent, JMX, DogStatsD logs from their configured directories.
- **Config files** — `.yaml`/`.yml` from `confd_path`, dist `conf.d`, fleet policies, and `checksd`.
- **Legacy providers** from `pkg/flare.ExtraFlareProviders` — system-probe stats, workload lists, goroutine dumps, etc.

The `statusimpl` and `diagnoseimpl` components also self-register flare providers that add `status.log` and `diagnose.log` respectively.

---

## Usage

### Wiring the component into an FX app

```go
import "github.com/DataDog/datadog-agent/comp/core/flare"

fx.Options(
    flare.Module(flare.NewParams(
        distPath,
        pythonChecksPath,
        defaultLogFile,
        defaultJMXLogFile,
        defaultDogstatsdLogFile,
        defaultStreamlogsLogFile,
    )),
)
```

For the CLI local-fallback path use `flare.NewLocalParams(...)`.

### Adding data to every flare from your component

```go
import flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"

type provides struct {
    fx.Out
    FlareProvider flaretypes.Provider
}

func newMyComponent(deps dependencies) provides {
    return provides{
        FlareProvider: flaretypes.NewProvider(func(fb flaretypes.FlareBuilder) error {
            data, err := collectMyData()
            if err != nil {
                return err // logged in flare-creation.log; collection continues
            }
            return fb.AddFile("my-component/data.log", data)
        }),
    }
}
```

### Triggering a flare from the CLI

The CLI calls the agent's `/flare` IPC endpoint (which invokes `Create`) and then calls `Send` with the returned path. See `cmd/agent/subcommands/flare/command.go` for the full flow.

### Remote Config triggered flares

When a `TaskFlare` Remote Config task arrives, `onAgentTaskEvent` inside the flare component reads `case_id`, `user_handle`, and optional `enable_profiling`/`enable_streamlogs` arguments, calls `CreateWithArgs`, then `Send` automatically.

---

## Related packages

- `pkg/flare` — contains the concrete collection functions ("providers") that decide what goes into the archive: config files, log files, expvars, goroutine dumps, environment variables, remote config state, and ECS/Kubernetes metadata. `ExtraFlareProviders` is the legacy registration entry point used by the flare component. See [pkg/flare docs](../../pkg/flare.md).
- `comp/core/status` — `statusimpl` self-registers a flare provider that writes `status.log` (verbose text of the full agent status) into every flare. See [comp/core/status docs](status.md).
- `comp/core/diagnose` — `diagnoseimpl` self-registers a flare provider that writes `diagnose.log` into every flare. See [comp/core/diagnose docs](diagnose.md).
- `pkg/util/scrubber` — the `FlareBuilder` applies the scrubber to every file added to the archive. API keys are partially revealed (last 4 chars), passwords and tokens are fully redacted. `SetPreserveENC(true)` prevents double-scrubbing of `ENC[...]` secret handles. See [scrubber docs](../../pkg/util/scrubber.md).
- `comp/core/config` — the config component itself registers a flare provider (in its constructor) so that configuration files are always included in the archive. See [comp/core/config docs](config.md).
