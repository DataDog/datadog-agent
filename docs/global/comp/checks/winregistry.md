# comp/checks/winregistry

**Package:** `github.com/DataDog/datadog-agent/comp/checks/winregistry`
**Team:** windows-products
**Platform:** Windows only (build tag `windows`)

## Purpose

`winregistry` registers the `windows_registry` agent check, which reads values from the Windows registry and:

1. Emits them as Datadog **metrics** (gauge) — useful for monitoring configuration values, feature flags, or counters stored in the registry.
2. Forwards change events as Datadog **Logs** (integration logs) — useful for auditing registry modifications.

Typical use cases include tracking software version keys, license information, OS configuration flags, or any numeric/string value stored under `HKLM`, `HKCU`, `HKU`, or `HKCR`.

## Key Elements

### Interface

```go
// def/component.go
type Component interface{}
```

Marker interface. All behaviour is the side effect of registering the check factory during startup.

### `WindowsRegistryCheck`

The core check type. Key fields:

| Field | Type | Purpose |
|-------|------|---------|
| `registryKeys` | `[]registryKey` | Parsed list of keys/values to monitor |
| `registryDelegate` | `registryDelegate` | Composite delegate that routes events to logging, metrics, and integration logs |
| `sender` | `sender.Sender` | Metrics sender |
| `logsComponent` | `agent.Component` | Optional logs pipeline |

### Configuration

Configuration is validated against a JSON Schema generated from the `checkCfg` struct at configure time.

#### Instance config (`checkCfg`)

```yaml
registry_keys:
  "HKLM\\SOFTWARE\\MyApp\\Settings":
    name: myapp.settings           # metric name prefix for this key
    registry_values:
      InstallVersion:
        name: install_version      # full metric: winregistry.install_version
        default_value: 0           # emitted when the value is missing
      FeatureFlag:
        name: feature_flag
        mapping:
          - enabled: 1.0
          - disabled: 0.0
send_on_start: true                # emit all values on first run (default: true)
```

Supported hives: `HKLM` / `HKEY_LOCAL_MACHINE`, `HKU` / `HKEY_USERS`, `HKCR` / `HKEY_CLASSES_ROOT`.

Supported registry value types: `DWORD`, `QWORD`, `SZ`, `EXPAND_SZ`. `SZ`/`EXPAND_SZ` values are parsed as `float64` if possible; otherwise the `mapping` list is consulted.

#### Init config (`checkInitCfg`)

```yaml
send_on_start: true  # applies to all instances unless overridden
```

### Delegate architecture

The check uses a delegate pattern to separate concerns. `compositeRegistryDelegate` fans events out to three implementations:

| Delegate | Output |
|----------|--------|
| `loggingRegistryDelegate` | Logs warnings/errors to the agent log |
| `metricsRegistryDelegate` | Emits gauge metrics via the sender |
| `integrationLogsRegistryDelegate` | Sends change events as Datadog Logs |

Each delegate implements the `registryDelegate` interface:

```go
type registryDelegate interface {
    onMissing(valueName string, ...)
    onAccessDenied(valueName string, ...)
    onRetrievalError(valueName string, ...)
    onSendNumber(valueName string, val float64, ...)
    onSendMappedNumber(valueName string, originalVal string, mappedVal float64, ...)
    onNoMappingFound(valueName string, val string, ...)
    onUnsupportedDataType(valueName string, valueType uint32, ...)
}
```

### Integration logs behaviour

The `integrationLogsRegistryDelegate` is muted on the first check run when `send_on_start: false`, so the initial enumeration of all existing keys does not produce a flood of "created" log events. On subsequent runs, or when `send_on_start: true`, it forwards key creation, deletion, and value-change events.

### FX wiring

`Requires`:

| Dependency | Required? | Purpose |
|------------|-----------|---------|
| `agent.Component` (logs) | Optional | Send integration logs |
| `log.Component` | Yes | Agent-level logging |
| `compdef.Lifecycle` | Yes | Register `OnStart` hook |

`NewComponent` calls `core.RegisterCheck` during `OnStart`.

## Usage

The component is wired into the Windows agent run command:

```go
// cmd/agent/subcommands/run/command_windows.go
winregistryfx.Module()
// ...
fx.Invoke(func(_ winregistry.Component) {})
```

A minimal `conf.d/windows_registry.d/conf.yaml`:

```yaml
instances:
  - registry_keys:
      "HKLM\\SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion":
        name: windows.version
        registry_values:
          CurrentBuildNumber:
            name: build_number
```

This emits `winregistry.build_number` as a gauge on each check run.

The check can be tested with:

```bash
agent check windows_registry
```
