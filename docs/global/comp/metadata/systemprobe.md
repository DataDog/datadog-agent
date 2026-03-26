# comp/metadata/systemprobe

**Team:** agent-configuration
**Import path:** `github.com/DataDog/datadog-agent/comp/metadata/systemprobe`

## Purpose

The `systemprobe` metadata component collects and periodically sends a metadata payload for the System Probe process to the Datadog inventory backend (`system-probe` table). It reports the agent version and the System Probe configuration broken down by configuration source (file, environment variables, Remote Configuration, Fleet Automation Policies, CLI, runtime, etc.), with all secrets scrubbed.

The payload is enabled by default and is sent every ~10 minutes (governed by `inventories_max_interval`). Configuration collection can be turned off with `inventories_configuration_enabled: false`. The payload is also available:
- at the `/metadata/system-probe` HTTP endpoint (GET)
- in flares (`system-probe.json`)

Configuration is fetched from the System Probe process via its local API using `configFetcher/sysprobe.SystemProbeConfig` and `SystemProbeConfigBySource`. The IPC component is required to ensure the auth token is created before the fetcher runs; the actual System Probe API does not use an HTTP client.

The `sysprobeconfig` dependency is optional (`option.Option[sysprobeconfig.Component]`): if System Probe is not running, metadata collection is silently skipped and only `agent_version` is reported.

## Key elements

### Interface (`comp/metadata/systemprobe/def/component.go`)

The component interface is empty — all integration points are registered as fx output values:

```go
type Component interface{}
```

### Payload (`comp/metadata/systemprobe/impl/system_probe.go`)

```go
type Payload struct {
    Hostname  string                 `json:"hostname"`
    Timestamp int64                  `json:"timestamp"`
    Metadata  map[string]interface{} `json:"system_probe_metadata"`
}
```

Fields inside `Metadata`:
- `agent_version` — always present
- `full_configuration` — complete scrubbed YAML (when `inventories_configuration_enabled` is `true` and System Probe config is available)
- `file_configuration`, `environment_variable_configuration`, `agent_runtime_configuration`, `remote_configuration`, `fleet_policies_configuration`, `cli_configuration`, `source_local_configuration`, `provided_configuration` — per-source scrubbed YAML layers

### fx wiring (`comp/metadata/systemprobe/fx/fx.go`)

`fx.Module()` registers `NewComponent` and exposes `Component` as an optional value.

### Dependencies (from `Requires`)

| Dependency | Purpose |
|---|---|
| `log.Component` | Structured logging |
| `config.Component` | Read feature flags (`inventories_configuration_enabled`) |
| `serializer.MetricSerializer` | Submit the payload to the backend |
| `ipc.Component` | Ensures the IPC auth token is created before the config fetcher is used |
| `option.Option[sysprobeconfig.Component]` | System Probe config accessor — absent when System Probe is not running |
| `hostnameinterface.Component` | Resolve the hostname included in the payload |

### Outputs (from `Provides`)

| Output | Purpose |
|---|---|
| `systemprobemetadata.Component` | The component itself |
| `runnerimpl.Provider` | Hooks the component into the metadata runner's periodic flush loop |
| `flaretypes.Provider` | Includes `system-probe.json` in flare archives |
| `api.AgentEndpointProvider` | Registers `GET /metadata/system-probe` on the agent API server |

## Usage

### Registration

The component is included in the main agent metadata bundle:

```
comp/metadata/bundle.go
```

```go
systemprobe "github.com/DataDog/datadog-agent/comp/metadata/systemprobe/fx"
// ...
systemprobe.Module()
```

It is also explicitly imported by the main agent run command:

```
cmd/agent/subcommands/run/command.go
```

### How the runner drives this component

The `runnerimpl.Provider` produced by this component is consumed by `comp/metadata/runner`. The runner spawns one goroutine per registered provider and calls this component's collection callback at the interval it returns (up to `inventories_max_interval`, default ~10 minutes). On `OnStop` the runner waits up to `metadata_provider_stop_timeout` (default 2 s) for any in-flight collection to finish. See [comp/metadata/runner](runner.md) for the scheduling contract.

### IPC dependency and config fetching

The `ipc.Component` dependency (`comp/core/ipc`) is required solely to ensure the IPC auth token is created on disk before the config fetcher runs. This is necessary because `configFetcher/sysprobe.SystemProbeConfig` and `SystemProbeConfigBySource` read the auth token file themselves. They do not use the `ipc.HTTPClient`; instead they communicate with System Probe over its own Unix socket (`system_probe_config.sysprobe_socket`). See [comp/core/ipc](../core/ipc.md) for the token creation and lifecycle model.

### `sysprobeconfig` optional dependency

`option.Option[sysprobeconfig.Component]` is provided by `sysprobeconfigimpl.Module()` (or `sysprobeconfig.NoneModule()` when System Probe is disabled). When the option is absent, the metadata dictionary contains only `agent_version`. When present, the component uses the `model.ReaderWriter` interface to check `inventories_configuration_enabled`, and the `SystemProbeConfig` and `SystemProbeConfigBySource` fetchers to collect per-source config snapshots. See [comp/core/sysprobeconfig](../core/sysprobeconfig.md) for the module's configuration model.

### How `sysprobeconfig` relates to `pkg/system-probe`

`comp/core/sysprobeconfig` wraps `pkg/system-probe/config.New()` to parse `system-probe.yaml` and populate the `pkgconfigsetup.SystemProbe()` global. The `*sysconfigtypes.Config` it exposes (via `SysProbeObject()`) enumerates enabled modules. However, the metadata component does not query module status directly — it relies on the config fetcher to call the System Probe Unix socket at collection time. See [pkg/system-probe](../../../global/pkg/system-probe.md) for the module lifecycle and client API.

### Disabling configuration collection

Set `inventories_configuration_enabled: false` in `datadog.yaml` to prevent configuration details from being included in the payload. The `agent_version` field is always sent.

If `sysprobeconfig` is not provided (i.e., System Probe is disabled or not co-located), the metadata dictionary will only contain `agent_version`.

## Related documentation

| Document | Relationship |
|---|---|
| [comp/metadata/runner](runner.md) | Scheduling backbone; `runnerimpl.Provider` produced here is consumed by the runner to drive periodic flushes |
| [comp/core/ipc](../core/ipc.md) | Provides the auth-token lifecycle; required here to ensure the token file exists before the System Probe config fetcher is called |
| [comp/core/sysprobeconfig](../core/sysprobeconfig.md) | Optional fx dependency; wraps `system-probe.yaml` and exposes `model.ReaderWriter` and `SysProbeObject()`; absent when System Probe is not running |
| [pkg/system-probe](../../../global/pkg/system-probe.md) | Lower-level System Probe infrastructure; `pkg/system-probe/config` is what `comp/core/sysprobeconfig` wraps; module lifecycle and Unix socket client are documented there |

### Example payload

```json
{
    "hostname": "my-host",
    "timestamp": 1631281754507358895,
    "system_probe_metadata": {
        "agent_version": "7.55.0",
        "full_configuration": "<entire yaml configuration for system-probe>",
        "provided_configuration": "system_probe_config:\n  sysprobe_socket: /tmp/sysprobe.sock",
        "file_configuration": "system_probe_config:\n  sysprobe_socket: /tmp/sysprobe.sock",
        "agent_runtime_configuration": "runtime_block_profile_rate: 5000",
        "environment_variable_configuration": "{}",
        "remote_configuration": "{}",
        "cli_configuration": "{}",
        "source_local_configuration": "{}"
    }
}
```
