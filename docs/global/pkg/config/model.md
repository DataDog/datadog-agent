> **TL;DR:** Defines the public interfaces (`Reader`, `Writer`, `Config`, `BuildableConfig`) and shared types (`Source`, `Proxy`, `Warnings`) that the entire agent configuration system is built upon — importable without pulling in the full setup dependency graph.

# pkg/config/model

## Purpose

`pkg/config/model` contains the public interfaces, types, and shared primitives for the agent
configuration system. It is deliberately free of implementation logic so that any package that
only needs to *read or write* configuration can import this tiny module without pulling in the
full `pkg/config/setup` dependency graph.

## Relationship to other packages

- **`pkg/config/setup`** — owns the two global `Config` singletons and all key registrations.
  It calls `model.ApplyOverrideFuncs` at the end of `LoadDatadog`. See `docs/global/pkg/config/setup.md`.
- **`comp/core/config`** — exposes the config as an FX-injectable component. Its `Component`
  interface embeds `model.ReaderWriter`. See `docs/global/comp/core/config.md`.
- **`pkg/config/env`** — calls `model.Reader.IsConfigured` and `model.Reader.GetBool` to detect
  the runtime environment during feature detection. See `docs/global/pkg/config/env.md`.
- **`pkg/util/log/setup`** — accepts `model.Reader` and calls `OnUpdate` to track `log_level`
  changes. See `docs/global/pkg/util/log.md`.

## Key elements

### Key interfaces

### Interfaces

The package defines a set of interfaces arranged in a hierarchy from most restricted to most
general:

| Interface | Allowed operations |
|---|---|
| `Reader` | Read-only access: `Get*`, `IsSet`, `IsConfigured`, `AllSettings*`, `OnUpdate`, `GetSequenceID`, `GetSource`, `GetAllSources`, `GetEnvVars`, … |
| `Writer` | Write-only access: `Set`, `SetWithoutSource`, `UnsetForSource` |
| `ReaderWriter` | Embeds `Reader` + `Writer` |
| `Setup` | Schema-building operations: `BindEnvAndSetDefault`, `SetDefault`, `BindEnv`, `SetKnown`, `BuildSchema`, `ParseEnvAs*` |
| `Compound` | File/stream loading: `ReadInConfig`, `MergeConfig`, `MergeFleetPolicy`, `RevertFinishedBackToBuilder` |
| `Config` | `ReaderWriter` + `Compound` + `SetKnown` — the everyday interface consumed by most of the codebase |
| `BuildableConfig` | `ReaderWriter` + `Setup` + `Compound` — used during startup to build the schema from scratch |

In practice, code that only reads config should accept a `Reader`; code that reads and writes
(e.g. in tests or during load) should accept `ReaderWriter` or `Config`. `BuildableConfig` is
used only in `pkg/config/setup`, `pkg/config/create`, and `pkg/config/mock`.

`Reader` also exposes several diagnostic helpers:
- `AllSettingsWithoutSecrets()` — all settings with the `SourceSecretBackend` layer excluded; safe for logging and flares.
- `AllSettingsWithoutDefaultOrSecrets()` — user-visible non-secret values only.
- `GetSecretSettingPaths()` — flattened list of keys that have a resolved secret value; used by the scrubber.
- `AllFlattenedSettingsWithSequenceID()` — atomic snapshot of all flattened leaf keys plus sequence ID for race-free reads.
- `ConfigFileUsed()` / `ExtraConfigFilesUsed()` — the file paths that were loaded.
- `GetLibType()` — returns `"viper"`, `"nodetreemodel"`, or `"tee"` for diagnostics.

### Key types

### `Source`

```go
type Source string

const (
    SourceSchema             Source = "schema"          // defined in schema but no default
    SourceDefault            Source = "default"
    SourceUnknown            Source = "unknown"         // test helpers / SetWithoutSource
    SourceInfraMode          Source = "infra-mode"
    SourceFile               Source = "file"
    SourceEnvVar             Source = "environment-variable"
    SourceFleetPolicies      Source = "fleet-policies"
    SourceAgentRuntime       Source = "agent-runtime"
    SourceSecretBackend      Source = "secret-backend"
    SourceLocalConfigProcess Source = "local-config-process"
    SourceRC                 Source = "remote-config"
    SourceCLI                Source = "cli"
    SourceProvided           Source = "provided"        // pseudo: any non-default source
)
```

A `Source` is attached to every value stored in the config. Higher-priority sources override
lower-priority ones. The ordered slice `Sources` exposes the priority ordering in ascending order;
`Source.IsGreaterThan(x)` and `Source.PreviousSource()` provide programmatic comparisons.

`SourceSecretBackend` is stored as a separate layer so that config dumps can exclude secrets
(via `AllSettingsWithoutSecrets()`) without changing effective values. `SourceUnknown` is
reserved for `SetWithoutSource` and should only appear in test code.

### `ValueWithSource`

```go
type ValueWithSource struct {
    Source Source
    Value  interface{}
}
```

Returned by `Reader.GetAllSources(key)` to show every value ever set for a key, regardless of
which one is currently effective. Useful for debugging.

### `Proxy`

```go
type Proxy struct {
    HTTP    string   `mapstructure:"http"`
    HTTPS   string   `mapstructure:"https"`
    NoProxy []string `mapstructure:"no_proxy"`
}
```

A convenience struct for the `proxy` config subtree, returned by `Reader.GetProxies()`.

### `Warnings`

```go
type Warnings struct {
    Errors []error
}
```

A list of non-fatal errors accumulated during config loading (e.g. unknown keys, unexpected
Unicode in values). Accessible via `Reader.Warnings()`. In `comp/core/config`, these are also
surfaced through `config.Component.Warnings()`.

### `NotificationReceiver`

```go
type NotificationReceiver func(setting string, source Source, oldValue, newValue any, sequenceID uint64)
```

A callback registered with `Reader.OnUpdate`. The config calls all registered receivers in
sequence each time `Set` is called. Receivers must not block. The `sequenceID` is a monotonically
increasing counter that can be paired with `Reader.GetSequenceID()` to detect stale reads.

### Key functions

### Override mechanism

```go
model.AddOverride(name string, value interface{})
model.AddOverrides(vars map[string]interface{})
model.AddOverrideFunc(f func(Config))
model.ApplyOverrideFuncs(config Config)
```

These package-level functions allow code outside of `pkg/config/setup` to inject config values
before `LoadDatadog` finalizes the configuration. Overrides are applied with `SourceAgentRuntime`
priority. `AddOverrideFunc` is the more flexible variant; it receives the full `Config` and can
apply conditional logic. `ApplyOverrideFuncs` is called by `LoadDatadog` at the end of loading.

### Error sentinel

```go
var ErrConfigFileNotFound = errors.New("Config File Not Found")
func NewConfigFileNotFoundError(err error) error
```

`LoadDatadog` (and `comp/core/config`) wrap missing config files in this sentinel error. Callers
can use `errors.Is(err, model.ErrConfigFileNotFound)` to distinguish a missing file (which is
usually non-fatal) from other I/O errors.

### `StringifyOption`

Controls the output of `Reader.Stringify`, which is available only under the `test` build tag:

```go
cfg.Stringify(model.SourceFile, model.DedupPointerAddr, model.OmitPointerAddr)
cfg.Stringify(model.SourceDefault, model.FilterSettings([]string{"api_key", "site"}))
```

## Usage

### Accepting config as a parameter

Prefer the narrowest interface that satisfies your needs:

```go
// Read-only component — most common
func NewMyComponent(cfg model.Reader) *MyComponent { ... }

// Component that also writes (e.g. applies runtime overrides)
func Setup(cfg model.ReaderWriter) error { ... }

// Startup / schema construction only
func RegisterKeys(cfg model.Setup) { ... }
```

Accepting `model.Reader` or `model.ReaderWriter` avoids importing `pkg/config/setup` and keeps
the dependency graph clean. `config.Component` from `comp/core/config` implements
`model.ReaderWriter`, so components injected with the FX component satisfy these signatures
directly.

### Calling Set with a source

Always supply an explicit source when writing:

```go
cfg.Set("logs_config.enabled", true, model.SourceAgentRuntime)
```

`SetWithoutSource` exists for test helpers and uses `SourceUnknown`.

### Registering an update callback

```go
cfg.OnUpdate(func(key string, src model.Source, old, new any, seq uint64) {
    if key == "log_level" {
        applyNewLogLevel(new.(string))
    }
})
```

This is how `pkg/util/log/setup` reacts to runtime log-level changes without requiring a restart.
The callback runs synchronously inside the `Set` call and must not block. Use the `sequenceID`
parameter together with `cfg.GetSequenceID()` to verify that a read performed outside the
callback is not stale.

### Override functions

Packages that need to force a config value based on runtime conditions (e.g. the container
environment detector) register an override function during `init()`:

```go
func init() {
    model.AddOverrideFunc(func(cfg model.Config) {
        if isRunningInFargate() {
            cfg.Set("ecs_fargate", true, model.SourceAgentRuntime)
        }
    })
}
```

`model.ApplyOverrideFuncs` is called by `pkg/config/setup.LoadDatadog` after the YAML file is
read and secrets are resolved, so overrides see the user's file values when making decisions.
Prefer `AddOverrideFunc` over `AddOverride` when conditional logic is needed. Both set values
with `SourceAgentRuntime` priority, which is above `SourceFile` but below `SourceCLI`.
