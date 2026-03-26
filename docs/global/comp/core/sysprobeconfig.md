> **TL;DR:** `comp/core/sysprobeconfig` is the fx-injectable wrapper around the `system-probe.yaml` configuration store, exposing both a key-level `model.ReaderWriter` API and a strongly-typed `*sysconfigtypes.Config` struct for system-probe module settings.

# comp/core/sysprobeconfig — System-Probe Configuration Component

**Team:** ebpf-platform
**Import path (interface):** `github.com/DataDog/datadog-agent/comp/core/sysprobeconfig`
**Import path (implementation):** `github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl`
**Importers:** ~45 packages

## Purpose

`comp/core/sysprobeconfig` is the fx-injectable wrapper around the system-probe configuration store. It mirrors the role that `comp/core/config` plays for the main agent, but targets the separate `system-probe.yaml` configuration file and the global `pkg/config.SystemProbe` Viper instance.

The component does two things at startup:

1. Calls `sysconfig.New(sysProbeConfFilePath, fleetPoliciesDirPath)` to parse `system-probe.yaml`, apply Fleet Policy files, and populate the `pkgconfigsetup.SystemProbe()` global store.
2. Exposes the resulting configuration as both a `model.ReaderWriter` (for key-level access) and a `*sysconfigtypes.Config` struct (for typed access to system-probe module toggles and settings).

It is listed as a `CoreConfig config.Component` dependency in its own constructor, which ensures the main agent config is fully loaded before the system-probe config is initialised.

## Package layout

| Package | Role |
|---|---|
| `comp/core/sysprobeconfig` (root) | `Component` interface, `NoneModule()` helper |
| `sysprobeconfigimpl` | `cfg` struct, `newConfig` constructor, fx `Module()`, `MockModule()` |
| `sysprobeconfigimpl/params.go` | `Params` type and functional option constructors |
| `sysprobeconfigimpl/config_mock.go` | Test helpers (`MockModule`, `NewMock`) |
| `sysprobeconfigimpl/mock_params.go` | `MockParams` (functional options + key override map for tests) |

## Key elements

### Key interfaces

#### Component interface

```go
type Component interface {
    model.ReaderWriter  // full read + write access to config keys (same API as comp/core/config)

    // Warnings returns any warnings generated while parsing the config.
    Warnings() *model.Warnings

    // SysProbeObject returns the strongly-typed system-probe config struct.
    SysProbeObject() *sysconfigtypes.Config
}
```

`model.ReaderWriter` provides the same typed getters as `comp/core/config` (`GetBool`, `GetString`, `GetInt`, `GetStringSlice`, etc.) and setters (`Set`, `SetWithoutSource`, `UnsetForSource`). See the `comp/core/config` docs for the full API.

`SysProbeObject()` returns `*sysconfigtypes.Config`, which exposes per-module enable flags and typed settings (e.g. `HealthPort`, `MaxTrackedConnections`, module-specific `Enabled` booleans). Use this when you need to read system-probe module configuration directly rather than via a key string.

### Key types

#### Params

```go
// sysprobeconfigimpl.NewParams(options ...func(*Params)) Params
sysprobeconfigimpl.NewParams(
    sysprobeconfigimpl.WithSysProbeConfFilePath("/etc/datadog-agent/system-probe.yaml"),
    sysprobeconfigimpl.WithFleetPoliciesDirPath("/etc/datadog-agent/fleet"),
)
```

| Option | Description |
|---|---|
| `WithSysProbeConfFilePath(path)` | Path to `system-probe.yaml`; usually from `--sysprobecfgpath` CLI flag |
| `WithFleetPoliciesDirPath(dir)` | Directory containing Fleet Policy YAML files from Remote Configuration |

### Key functions

#### fx wiring

`sysprobeconfigimpl.Module()` is typically included as part of `core.Bundle()` in the main agent (via `core.BundleParams.SysprobeConfigParams`), and directly in the system-probe daemon:

```go
// Main agent (cmd/agent/subcommands/run/command.go)
fx.Supply(core.BundleParams{
    SysprobeConfigParams: sysprobeconfigimpl.NewParams(
        sysprobeconfigimpl.WithSysProbeConfFilePath(globalParams.SysProbeConfFilePath),
        sysprobeconfigimpl.WithFleetPoliciesDirPath(cliParams.FleetPoliciesDirPath),
    ),
    // ...
}),
core.Bundle(),

// System-probe daemon (cmd/system-probe/subcommands/run/command.go)
sysprobeconfigimpl.Module(),
```

`Module()` also calls `fxutil.ProvideOptional[sysprobeconfig.Component]()`, making the component available as `option.Option[sysprobeconfig.Component]` in addition to the concrete type. This is used by `rcclientimpl`, which accepts an optional sysprobeconfig to route `AGENT_CONFIG` log-level changes to the correct config store when running as system-probe.

### Configuration and build flags

#### Disabling the component

When a binary does not need system-probe configuration, use `sysprobeconfig.NoneModule()` to provide a disabled optional without linking the implementation:

```go
sysprobeconfig.NoneModule()  // provides option.None[sysprobeconfig.Component]()
```

#### Mock

`sysprobeconfigimpl.MockModule()` (build tag `test`) provides an in-memory configuration backed by `mock.NewSystemProbe(t)`. It strips all `DD_` environment variables for the duration of the test and restores them in a `t.Cleanup` hook.

```go
// In a test:
sysprobeComp := sysprobeconfigimpl.NewMock(t)
sysprobeComp.Set("system_probe_config.enabled", true, model.SourceDefault)
```

`MockParams.Overrides` accepts a `map[string]interface{}` of key overrides applied on top of defaults:

```go
fxutil.Test[MyComp](t, fx.Options(
    sysprobeconfigimpl.MockModule(),
    fx.Supply(sysprobeconfigimpl.MockParams{
        Overrides: map[string]interface{}{
            "network_config.enabled": true,
        },
    }),
    fx.Provide(newMyComp),
))
```

## Usage patterns

**Reading a typed setting via `SysProbeObject()`:**

```go
// cmd/system-probe/subcommands/run/command.go
fx.Provide(func(sysprobeconfig sysprobeconfig.Component) healthprobe.Options {
    return healthprobe.Options{
        Port: sysprobeconfig.SysProbeObject().HealthPort,
    }
})
```

**Reading a config key via the `ReaderWriter` interface:**

```go
func NewMyComp(deps struct {
    fx.In
    SysprobeConfig sysprobeconfig.Component
}) MyComp {
    enabled := deps.SysprobeConfig.GetBool("network_config.enabled")
    // ...
}
```

**Providing sysprobeconfig as a `settings.Params` config target (system-probe only):**

```go
fx.Provide(func(sysprobeconfig sysprobeconfig.Component) settings.Params {
    return settings.Params{
        Settings: map[string]settings.RuntimeSetting{"log_level": commonsettings.NewLogLevelRuntimeSetting()},
        Config:   sysprobeconfig,  // satisfies model.ReaderWriter
    }
})
```

**As a status information provider:**

```go
fx.Provide(func(sysprobeconfig sysprobeconfig.Component) status.InformationProvider {
    return status.NewInformationProvider(systemprobeStatus.GetProvider(sysprobeconfig))
})
```

## Key dependents

- `cmd/system-probe/subcommands/run` — primary consumer; uses both `GetBool`/`GetString` and `SysProbeObject()`
- `cmd/agent/subcommands/run` — core agent reads system-probe module status via `SysProbeObject()`
- `comp/remote-config/rcclient/rcclientimpl` — applies RC log-level changes to this component when `IsSystemProbe: true`
- `comp/core/workloadmeta/collectors/internal/process` — reads process-agent related settings
- `comp/metadata/systemprobe` — collects system-probe metadata for inventory payloads
- `comp/checks/agentcrashdetect` — reads crash detection config keys
- `pkg/network/sender` — reads network monitoring settings on Linux
- `pkg/system-probe/api/module` — receives sysprobeconfig via `FactoryDependencies` for module initialisation

## Related packages and components

| Package / Component | Doc | Relationship |
|---|---|---|
| `comp/core/config` | [config.md](config.md) | The main agent counterpart to this component. Both implement `model.ReaderWriter` and are provided through `core.Bundle()`, but target different config files (`datadog.yaml` vs `system-probe.yaml`) and different global Viper instances. The `sysprobeconfig` constructor declares `CoreConfig config.Component` as a dependency, ensuring `datadog.yaml` is fully loaded before `system-probe.yaml` parsing begins. |
| `pkg/system-probe` | [../../pkg/system-probe.md](../../pkg/system-probe.md) | Lower-level package that this component wraps. `sysconfig.New()` (from `pkg/system-probe/config`) is called inside `newConfig` to parse `system-probe.yaml` and populate the `pkgconfigsetup.SystemProbe()` global. The `*types.Config` returned by that call is surfaced as `SysProbeObject()`. Module factories in `pkg/system-probe/api/module` receive this component via `FactoryDependencies.SysprobeConfig`. |
| `pkg/network/config` | [../../pkg/network/config.md](../../pkg/network/config.md) | Reads the `system_probe_config`, `network_config`, and `service_monitoring_config` namespaces from the `pkgconfigsetup.SystemProbe()` store that this component populates. `pkg/network/config.New()` must be called after the sysprobeconfig component has initialised the global store. The `Config.NPMEnabled` and related fields it exposes are derived from keys readable via `sysprobeconfig.Component.GetBool`. |
