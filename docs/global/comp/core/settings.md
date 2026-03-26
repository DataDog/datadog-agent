# comp/core/settings — Runtime Settings Component

**Import path:** `github.com/DataDog/datadog-agent/comp/core/settings`
**Team:** agent-configuration
**Importers:** ~26 packages

## Purpose

`comp/core/settings` manages configuration values that can be read and changed while the agent is running, without a restart. It maintains a registry of named `RuntimeSetting` entries, exposes them through a Go API, and automatically registers HTTP endpoints on the agent's internal API so that `datadog-agent config` CLI commands and the GUI can inspect and modify them at runtime.

Values written through this component are stored in the underlying `config.Component` with source `model.SourceCLI`, so they take effect immediately and can be read back through the normal config API.

## Package layout

| Package | Role |
|---|---|
| `comp/core/settings` (root) | `Component` interface, `RuntimeSetting` interface, `Params`, `RuntimeSettingProvider` fx helper |
| `comp/core/settings/settingsimpl` | `settingsRegistry` implementation, HTTP handlers, `Module()` |

## Component interface

```go
type Component interface {
    // Enumerate all registered settings
    RuntimeSettings() map[string]RuntimeSetting

    // Read / write individual settings by name
    GetRuntimeSetting(setting string) (interface{}, error)
    SetRuntimeSetting(setting string, value interface{}, source model.Source) error

    // HTTP handler factories — used by the API component
    GetFullConfig(namespaces ...string) http.HandlerFunc
    GetFullConfigWithoutDefaults(namespaces ...string) http.HandlerFunc
    GetFullConfigBySource() http.HandlerFunc
    GetValue(w http.ResponseWriter, r *http.Request)
    SetValue(w http.ResponseWriter, r *http.Request)
    ListConfigurable(w http.ResponseWriter, r *http.Request)
}
```

`GetRuntimeSetting` / `SetRuntimeSetting` return `*SettingNotFoundError` when the requested name is not registered.

### RuntimeSetting interface

Every individual setting implements:

```go
type RuntimeSetting interface {
    Get(config config.Component) (interface{}, error)
    Set(config config.Component, v interface{}, source model.Source) error
    Description() string
    Hidden() bool
}
```

Built-in implementations live in `pkg/config/settings` and include `LogLevelRuntimeSetting`, `RuntimeMutexProfileFraction`, `RuntimeBlockProfileRate`, `ProfilingGoroutines`, and `LogPayloadsRuntimeSetting`.

## HTTP endpoints registered

The implementation registers these routes on the agent API automatically:

| Method | Path | Handler |
|---|---|---|
| GET | `/config` | Full config dump (YAML, sensitive fields scrubbed) |
| GET | `/config/without-defaults` | Config dump excluding default values |
| GET | `/config/list-runtime` | List all runtime-configurable settings |
| GET | `/config/{setting}` | Read a single setting (add `?sources=true` for per-source breakdown) |
| POST | `/config/{setting}` | Write a single setting (`value` form field) |

## fx wiring

`settingsimpl.Module()` provides `settings.Component` and auto-registers the five HTTP endpoints as `api.AgentEndpointProvider` values consumed by `comp/api`.

```go
// Supply the Params and include the module
fx.Provide(func(serverDebug dogstatsddebug.Component, config config.Component) settings.Params {
    return settings.Params{
        Settings: map[string]settings.RuntimeSetting{
            "log_level": commonsettings.NewLogLevelRuntimeSetting(),
            // ... other settings
        },
        Config:     config,
        Namespaces: []string{"logs_config"}, // optional namespace filter for /config
    }
}),
settingsimpl.Module(),
```

`Namespaces` restricts the `/config` endpoint to return only the listed top-level keys instead of the full configuration.

## Implementing a new RuntimeSetting

```go
type MyRuntimeSetting struct{}

func (s *MyRuntimeSetting) Description() string { return "Enable or disable my feature" }
func (s *MyRuntimeSetting) Hidden() bool        { return false }

func (s *MyRuntimeSetting) Get(cfg config.Component) (interface{}, error) {
    return cfg.GetBool("my_feature.enabled"), nil
}

func (s *MyRuntimeSetting) Set(cfg config.Component, v interface{}, source model.Source) error {
    cfg.Set("my_feature.enabled", v, source)
    return nil
}
```

Register it by adding it to the `Settings` map in `Params` at startup (see the agent run command for examples).

## Usage across the codebase

- **`cmd/agent`** — registers ~10 settings including `log_level`, `dogstatsd_stats`, `dogstatsd_capture_duration`, and multi-region failover toggles.
- **`cmd/system-probe`** — registers system-probe-specific settings.
- **`cmd/security-agent`** — registers security-agent-specific settings.
- **`cmd/process-agent`** — registers process-agent-specific settings.
- **`comp/remote-config/rcclient`** — uses `settings.Component` to apply remote-config-driven setting changes at runtime. The RC client calls `SetRuntimeSetting` with `model.SourceRC` when the backend pushes a new value for an `AGENT_CONFIG` product update.
- **`comp/core/profiler`** — uses `settings.Component` to expose profiling knobs.

## Related components

| Component / Package | Doc | Relationship |
|---|---|---|
| `comp/core/config` | [config.md](config.md) | The backing store for all runtime settings. `SetRuntimeSetting` writes values with `model.SourceCLI` (or `model.SourceRC`) directly into the `config.Component`; `GetRuntimeSetting` reads from the same store. The `RuntimeSetting.Set` and `Get` methods receive `config.Component` as a parameter. |
| `comp/api/api` | [../api/api.md](../api/api.md) | The CMD API server that hosts the `/config` endpoints. `settingsimpl.Module()` automatically exports five `api.AgentEndpointProvider` values via the `"agent_endpoint"` fx group, so the routes are registered without any explicit wiring in the agent run command. |
| `pkg/config/settings` | — | Contains the built-in `RuntimeSetting` implementations (`LogLevelRuntimeSetting`, `RuntimeMutexProfileFraction`, `RuntimeBlockProfileRate`, `ProfilingGoroutines`, `LogPayloadsRuntimeSetting`). These are the concrete types placed in the `Settings` map inside `Params`; they delegate their `Get`/`Set` logic to `config.Component`. |
| `comp/remote-config/rcclient` | [../remote-config/rcclient.md](../remote-config/rcclient.md) | The RC client applies `AGENT_CONFIG` product updates by calling `settings.Component.SetRuntimeSetting`. This is the integration point between Remote Configuration and the live config store. |
