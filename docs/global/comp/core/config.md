> **TL;DR:** `comp/core/config` is the single fx-injectable source of truth for agent configuration, wrapping a Viper-based store that parses `datadog.yaml`, resolves secrets, and applies CLI and Fleet Policy overrides at startup.

# comp/core/config — Configuration Component

**Import path:** `github.com/DataDog/datadog-agent/comp/core/config`
**Team:** agent-configuration
**Importers:** ~334 packages (most-imported component in the repository)

## Purpose

`comp/core/config` is the single source of truth for agent configuration at runtime. It wraps `pkg/config` (a Viper-based configuration store) and exposes it as an fx-injectable component. Any package that needs to read or write configuration values should depend on this component instead of reaching into `pkg/config` globals directly.

The component initialises the configuration store at startup: it locates and parses `datadog.yaml` (and optional extra files), resolves secrets through `comp/core/secrets`, applies CLI overrides, merges Fleet Policy files, and makes the result available to all downstream components.

## Package layout

| Package | Role |
|---|---|
| `comp/core/config` (root) | Component interface, `Params`, mock helpers, fx `Module()` |
| `config.go` | `cfg` struct (thin wrapper over `pkgconfigmodel.Config`) and `newComponent` constructor |
| `setup.go` | File discovery, YAML parsing, Fleet Policy merging, CLI override application |
| `params.go` | `Params` type and functional option constructors |
| `config_mock.go` | Test helpers (`NewMock`, `NewMockWithOverrides`, `NewMockFromYAML`, …) |

## Key elements

### Key interfaces

#### Component interface

```go
type Component interface {
    pkgconfigmodel.ReaderWriter  // full read + write access to config keys

    Warnings() *pkgconfigmodel.Warnings  // errors collected during setup
}
```

`pkgconfigmodel.ReaderWriter` combines a rich `Reader` (type-safe getters for every Go primitive, `IsSet`, `IsConfigured`, `OnUpdate`, …) with a `Writer` (`Set`, `SetWithoutSource`, `UnsetForSource`). Key reader methods:

- `GetString(key string) string`
- `GetBool(key string) bool`
- `GetInt / GetInt64 / GetFloat64 / GetDuration`
- `GetStringSlice / GetStringMap / GetStringMapString`
- `IsConfigured(key string) bool` — true when set by user (non-default source)
- `OnUpdate(callback NotificationReceiver)` — subscribe to config changes

The `Reader` alias (`config.Reader`) is available for components that only need read access.

### Key functions

#### fx wiring

The component is provided by `config.Module()`, which is included in `core.Bundle()`. You do not normally need to add it manually.

```go
// Typical daemon startup (inside a fxutil.OneShot or fxutil.Run call)
fx.Supply(core.BundleParams{
    ConfigParams: config.NewAgentParams(globalParams.ConfFilePath,
        config.WithExtraConfFiles(globalParams.ExtraConfFilePath),
        config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath),
    ),
    LogParams: log.ForDaemon("AGENT", "log_file", defaultLogFile),
}),
core.Bundle(),
```

The component constructor receives `Params`, `secrets.Component`, and `delegatedauth.Component` via fx dependency injection, and provides both `config.Component` and a `flaretypes.Provider` (so config files are automatically included in flares).

### Key types

#### Params and constructors

`Params` is built with functional options. Pre-built constructors cover each agent flavor:

| Constructor | Use case |
|---|---|
| `NewAgentParams(confFilePath, ...opts)` | Main agent binary |
| `NewClusterAgentParams(configFilePath, ...opts)` | Cluster agent (`datadog-cluster.yaml`) |
| `NewSecurityAgentParams(paths, ...opts)` | Security agent (merges `security-agent.yaml`) |
| `NewParams(defaultConfPath, ...opts)` | Generic / custom use |

Common options:

- `WithConfFilePath(path)` — explicit path to `datadog.yaml`
- `WithExtraConfFiles(paths)` — additional YAML files merged on top
- `WithFleetPoliciesDirPath(dir)` — directory containing Fleet Policy YAML
- `WithCLIOverride(key, value)` — apply a setting from a CLI flag (highest priority)
- `WithIgnoreErrors(true)` — tolerate missing/invalid config (errors stored in `Warnings()`)
- `WithConfigName(name)` — use a different root filename (default `"datadog"`)

#### Mock

The `config_mock.go` file (build tag `test`) provides four helpers, all returning a `Component` backed by an in-memory store:

```go
config.NewMock(t)                                // empty defaults
config.NewMockWithOverrides(t, map[string]any{}) // pre-set keys
config.NewMockFromYAML(t, yamlString)            // parse inline YAML
config.NewMockFromYAMLFile(t, filePath)          // parse YAML file
```

## Usage patterns

**Reading a value from a component constructor:**

```go
type Requires struct {
    fx.In
    Config config.Component
}

func NewMyComp(deps Requires) MyComp {
    enabled := deps.Config.GetBool("my_feature.enabled")
    ...
}
```

**Reacting to config changes at runtime:**

```go
cfg.OnUpdate(func(key string, old, new interface{}) {
    // re-read relevant keys
})
```

**Outside fx (serverless / one-off tools):**

```go
comp, err := config.NewServerlessConfig("/path/to/serverless.yaml")
```

## Relationship to pkg/config

`comp/core/config` is a thin fx wrapper over the lower-level `pkg/config` packages. Understanding the relationship helps when navigating deeper:

| Layer | Package | Role |
|---|---|---|
| Interfaces & types | `pkg/config/model` | `Reader`, `Writer`, `ReaderWriter`, `Config`, `Source`, `Warnings`, `NotificationReceiver` |
| Key registry & defaults | `pkg/config/setup` | `InitConfig`, `LoadDatadog`, `Datadog()` global singleton, all `BindEnvAndSetDefault` calls |
| Backend implementations | `pkg/config/nodetreemodel`, `pkg/config/viperconfig` | Chosen at runtime via `DD_CONF_NODETREEMODEL` |
| Component wrapper | `comp/core/config` | Wraps `pkg/config/setup` globals as an fx-injectable service |

The `Component` interface returned by `comp/core/config` implements `pkgconfigmodel.ReaderWriter` directly, so callers that want to minimize imports can accept `pkgconfigmodel.Reader` (read-only) rather than the full `config.Component`. Prefer `model.Reader` over `config.Component` in function signatures when writes are not needed — see [`pkg/config/model`](../../pkg/config/model.md).

`pkg/config/setup.LoadDatadog` performs the actual file parsing, secret resolution, and feature detection that `comp/core/config`'s `newComponent` orchestrates. Consult [`pkg/config/setup`](../../pkg/config/setup.md) for the full loading sequence including proxy resolution and override functions. For a catalogue of all recognized config keys see [`pkg/config/config`](../../pkg/config/config.md).

## fx and fxutil integration

`comp/core/config` is always consumed through `core.Bundle()`. The bundle is assembled with `fxutil.Run` or `fxutil.OneShot` for daemons and CLI commands respectively. See [`pkg/util/fxutil`](../../pkg/util/fxutil.md) for the full list of test helpers (`fxutil.Test`, `fxutil.TestBundle`, `fxutil.TestOneShotSubcommand`) that let you wire `config.NewMock` into test fx graphs.

The `config_mock.go` helpers (`NewMock`, `NewMockWithOverrides`, `NewMockFromYAML`, `NewMockFromYAMLFile`) wrap `pkg/config/mock.New` with the same in-memory store so tests can call `config.OnUpdate` and observe notifications without touching disk.

## Key dependents

`comp/core/config` is consumed by virtually every other component and command. Prominent examples:

- [`comp/core/log`](log.md) — reads `log_level`, `log_file`, `log_to_console`, syslog keys, and registers an `OnUpdate` hook to change the active log level at runtime without a restart.
- [`comp/core/telemetry`](telemetry.md) — no direct config dependency on the component itself, but many consumers use config to gate telemetry registration.
- [`comp/core/ipc`](ipc.md) — reads `auth_token_file_path`, `cmd_host`, `cmd_port`, and `agent_ipc.socket_path` to construct TLS configs and the HTTP client used for inter-process calls.
- [`comp/core/hostname`](hostname.md) — reads the `hostname` / `hostname_file` config keys and various cloud-provider detection flags.
- `comp/core/secrets` — reads the secret backend configuration.
- `pkg/aggregator`, `pkg/collector`, `comp/logs`, `comp/trace` — read feature-specific settings.
- All agent subcommands (`cmd/agent`, `cmd/trace-agent`, `cmd/security-agent`, …) — supply `Params` at startup.
