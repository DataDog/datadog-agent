> **TL;DR:** Central configuration system for the Datadog Agent — owns the schema of every recognized key, its defaults, env-var bindings, and the loading/merging logic that produces the two global `Config` singletons consumed by all agent components.

# pkg/config

## Purpose

`pkg/config` is the central configuration system for the Datadog Agent. It owns the agent-wide
configuration schema: every recognized config key, its default value, its environment-variable
binding, and the logic that loads, merges, and validates the final configuration at startup. It
also exposes the two global `model.Config` singletons (`Datadog` and `SystemProbe`) that are
consumed throughout the codebase.

For usage from new FX-based code, prefer `comp/core/config` (see `docs/global/comp/core/config.md`),
which wraps this package as an injectable component. This package is the authoritative source for
the schema and loading logic that the component builds upon.

The package is organized into focused sub-packages rather than a single flat library:

| Sub-package | Responsibility |
|---|---|
| `model` | Interfaces (`Reader`, `Writer`, `Config`, `BuildableConfig`) and shared types (`Source`, `Proxy`, `Warnings`) |
| `create` | Factory (`NewConfig`) that chooses the backing implementation at runtime |
| `setup` | Schema registration, defaults, and load functions for all agent settings |
| `nodetreemodel` | Default in-memory backend (node-tree) |
| `viperconfig` | Legacy Viper-backed backend |
| `teeconfig` | Dual-write backend used to compare implementations during migration |
| `remote` | Remote Configuration overlay |
| `env` | Runtime environment detection (container, Kubernetes, ECS, …) |
| `structure` | `UnmarshalKey` helper for decoding config subtrees into structs |
| `fetcher` | IPC-based config fetching (agent → system-probe / tracers) |
| `settings` | Runtime-settable keys exposed through the agent API |
| `mock` | Test helpers (`mock.New`, `mock.NewFromYAML`, `mock.NewSystemProbe`) |
| `utils` | Derived-value helpers (API key, endpoints, tags, …) |
| `legacy` | Agent 5 → 6 migration helpers |

## Sub-package cross-references

| Sub-package | Full doc |
|---|---|
| `pkg/config/model` | `docs/global/pkg/config/model.md` |
| `pkg/config/setup` | `docs/global/pkg/config/setup.md` |
| `pkg/config/env` | `docs/global/pkg/config/env.md` |
| `pkg/config/remote` | `docs/global/pkg/config/remote.md` |
| `comp/core/config` | `docs/global/comp/core/config.md` |

## Key elements

### Key types

See `model.Source`, `model.Proxy`, `model.Warnings`, and `model.ValueWithSource` in `docs/global/pkg/config/model.md`.

### Key functions

### Global singletons (`pkg/config/setup`)

```go
setup.Datadog()     // model.Config — main agent configuration
setup.SystemProbe() // model.Config — system-probe configuration
```

Both objects are populated during `init()` by `InitConfigObjects` and are safe to read from
anywhere after that point. They must not be written outside of startup and test code.

### `InitConfigObjects` / `InitConfig` / `LoadDatadog`

- `InitConfigObjects(cliPath, defaultDir string)` — called once from `main`; creates the backend,
  registers all defaults, and freezes the schema with `BuildSchema`.
- `InitConfig(config model.Setup)` — registers defaults and env-var bindings for all core-agent
  settings (delegates to `initCommonConfigComponents` + `initCoreAgentFull`). The `serverless`
  build tag causes only `initCommonConfigComponents` to run, producing a smaller schema.
- `LoadDatadog(config, secretResolver, delegatedAuth, extraEnvVars)` — reads the YAML file,
  merges fleet policies, resolves `ENC[]` secrets, loads proxy settings, applies feature
  detection, and runs all registered `model.AddOverrideFunc` callbacks. This is the step that
  makes the config ready to use. Full details in `docs/global/pkg/config/setup.md`.

In FX-based binaries `comp/core/config/impl.newComponent` orchestrates the same steps via
dependency injection, so most agent binaries never call these functions directly.

### Backend selection (`pkg/config/create`)

`create.NewConfig(name, configLib)` chooses the implementation at runtime. The env var
`DD_CONF_NODETREEMODEL` (or the `conf_nodetreemodel` YAML key) controls this:

| Value | Effect |
|---|---|
| `"viper"` (or default) | Viper backend |
| `"enable"` | Node-tree backend |
| `"tee"` | Both backends; read from Viper, log differences |
| `"enable-tee"` | Both backends; read from node-tree, log differences |
| Agent version string (e.g. `"7.60"`) | Enable node-tree if agent version >= given value |

### Configuration and build flags

- `serverless` — Omits core-agent-only settings from `initConfig`; only `initCommonConfigComponents`
  runs, producing a smaller schema for the serverless agent.
- `test` — Enables `Reader.Stringify` and `SetTestOnlyDynamicSchema`, used by the mock helpers.

### `BindEnvAndSetDefault`

The primary primitive for declaring a config key:

```go
config.BindEnvAndSetDefault("logs_config.enabled", false)
```

It registers a default value and binds the corresponding `DD_LOGS_CONFIG_ENABLED` environment
variable in one call. All key registrations live in `setup/common_settings.go` (shared) and
`setup/config.go` (core-agent only).

### Source priority

Every value in the config carries a `model.Source` tag. When multiple sources provide the same
key the higher-priority source wins (full definition in `docs/global/pkg/config/model.md`):

```
schema < default < unknown < infra-mode < file < environment-variable < fleet-policies
       < agent-runtime < secret-backend < local-config-process < remote-config < cli
```

`SourceSecretBackend` is stored as a separate layer so that `AllSettingsWithoutSecrets()` can
exclude resolved secrets from diagnostic dumps without changing the effective values.
`SourceProvided` is a pseudo-source meaning "any source except default", useful in filters.

### `model.ApplyOverrideFuncs`

Called at the end of `LoadDatadog`. Code that needs to override config values programmatically
(e.g. to force a setting based on the detected environment) registers a function with
`model.AddOverrideFunc` before `Load` is called.

### `structure.UnmarshalKey`

Used throughout the codebase to decode a config sub-tree into a typed struct:

```go
var p model.Proxy
structure.UnmarshalKey(config, "proxy", &p)
```

Options (`EnableSquash`, `EnableStringUnmarshal`, `ConvertEmptyStringToNil`, …) handle edge-cases
inherited from the Viper era.

### `env` package

Provides runtime environment predicates (`env.IsContainerized()`, `env.IsKubernetes()`,
`env.IsECS()`, …) and `env.DetectFeatures(config)`, which applies environment-driven config
overrides at the end of `LoadDatadog`. Feature detection is gated behind the
`autoconfig_from_environment` config key (default: `true`) and panics if called before the
config is loaded. Full details in `docs/global/pkg/config/env.md`.

### Mock helpers (`pkg/config/mock`)

```go
cfg := mock.New(t)                       // fresh config with all defaults, auto-cleaned up
cfg := mock.NewFromYAML(t, yamlString)   // pre-populated from a YAML string
cfg := mock.NewSystemProbe(t)            // mock for the system-probe config
```

`mock.New` replaces the global `setup.Datadog()` singleton for the duration of the test and
restores the original on cleanup. For FX-based component tests, use the equivalent helpers in
`comp/core/config`: `config.NewMock(t)`, `config.NewMockWithOverrides(t, map[string]any{})`, or
`config.NewMockFromYAML(t, yaml)`. These return a `config.Component` (which embeds
`model.ReaderWriter`) and are therefore compatible with any function that accepts `model.Reader`.

## Usage

### Reading a config value

Prefer accepting a `model.Reader` (read-only) parameter over importing the global singleton
directly, so the code stays testable:

```go
// Preferred: accept model.Reader via dependency injection
func NewMyComponent(cfg model.Reader) *MyComponent {
    return &MyComponent{endpoint: cfg.GetString("dd_url")}
}
```

When injection is not yet possible (e.g. in legacy code), use the global accessor:

```go
import pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

apiKey := pkgconfigsetup.Datadog().GetString("api_key")
```

Components in `comp/` receive a `config.Component` (which implements `model.ReaderWriter`) via
Fx injection. `config.Component` is equivalent to `model.ReaderWriter` for most read use-cases.
The narrower `config.Reader` alias is available for components that only need read access.

### Adding a new config key

1. Add a `BindEnvAndSetDefault` call in `pkg/config/setup/common_settings.go` (shared) or in
   the relevant feature file (e.g. `apm.go`, `system_probe.go`).
2. If the key exposes a custom env-var format, add a `ParseEnvAs*` call alongside it.
3. Run `dda inv linter.go` — unknown keys loaded from file are reported as warnings by
   `findUnknownKeys`.

### Cross-process config (fetcher)

Side-agents (security-agent, trace-agent) can pull their effective configuration from the
core-agent over IPC instead of re-reading the YAML. `pkg/config/fetcher` provides the client
side; the server side is exposed by `pkg/config/settings`.

### Remote Configuration overlay

`pkg/config/remote` provides the agent-side implementation of Remote Configuration (RC):
an HTTP client that polls the Datadog backend, a TUF/Uptane verification layer, and a gRPC
service that distributes signed config files to sub-processes. `SourceRC` and
`SourceFleetPolicies` are the two `model.Source` values managed by this layer. Full details in
`docs/global/pkg/config/remote.md`.

### Reacting to config changes

Components that need to react to runtime config changes (e.g. `log_level` updates) should use
`cfg.OnUpdate`:

```go
cfg.OnUpdate(func(key string, src model.Source, old, new any, seq uint64) {
    if key == "log_level" {
        applyNewLevel(new.(string))
    }
})
```

The callback is called synchronously after each `Set`. It must not block. The `sequenceID` can
be paired with `cfg.GetSequenceID()` to detect stale reads in concurrent code.
