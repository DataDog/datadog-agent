# comp/metadata/inventoryagent — Agent Inventory Payload Component

**Import path:** `github.com/DataDog/datadog-agent/comp/metadata/inventoryagent`
**Team:** agent-configuration
**Importers:** ~13 packages

## Purpose

`comp/metadata/inventoryagent` builds and periodically sends the `datadog_agent` inventory metadata payload. This payload populates the **Agent** tab in the Datadog Infrastructure inventory UI, giving operators a consolidated view of every agent's configuration, enabled features, and installation details without requiring direct access to each host.

The component aggregates information from all sub-agents running on the host:
- **Core agent** — proxy settings, feature flags (logs, remote config, container images, SBOM, Synthetics, ECS/EKS Fargate, FIPS mode)
- **Trace agent** — APM URL, APM enabled flag
- **Process agent** — process and container collection flags, language detection
- **Security agent** — CSPM compliance enabled flags
- **System probe** — CWS, NPM/USM, traceroute, dynamic instrumentation, eBPF settings
- **Tracers** — application monitoring configuration (fleet and local)
- **Installation info** — install tool, tool version, installer version
- **Configuration layers** — per-source config snapshots (file, env vars, remote config, fleet policies, …) when `inventories_configuration_enabled` is true

The component also subscribes to configuration changes (`config.OnUpdate`) and automatically schedules a refresh whenever a configuration value changes.

## Package layout

| Package | Role |
|---|---|
| `comp/metadata/inventoryagent` | Component interface (`Component`) |
| `comp/metadata/inventoryagent/inventoryagentimpl` | Implementation (`inventoryagent` struct), `Payload` type, fx `Module()` |

## Component interface

```go
type Component interface {
    // Set stores a metadata key/value pair in the agent metadata cache.
    // Triggers a refresh if the value changed.
    // The value must not be modified after being passed in.
    Set(name string, value interface{})

    // Get returns a shallow copy of the current agent metadata map.
    // Intended for status page rendering; not the serialized payload.
    Get() map[string]interface{}
}
```

## Key types

### `inventoryagentimpl.Payload`

The JSON structure sent to the backend:

```go
type Payload struct {
    Hostname  string                 `json:"hostname"`
    Timestamp int64                  `json:"timestamp"`
    Metadata  map[string]interface{} `json:"agent_metadata"`
    UUID      string                 `json:"uuid"`
}
```

`agent_metadata` is a flat string-keyed map. Notable keys include:

| Key | Source |
|---|---|
| `agent_version`, `package_version` | Agent build |
| `agent_startup_time_ms` | Startup timestamp |
| `flavor` | Agent flavor (agent, dogstatsd, …) |
| `hostname_source` | How the hostname was resolved |
| `install_method_tool`, `install_method_tool_version` | Install info file |
| `infrastructure_mode` | One of `full`, `end_user_device`, `basic`, `none` |
| `feature_logs_enabled`, `feature_apm_enabled`, … | Feature flags from each sub-agent |
| `config_dd_url`, `config_site`, … | Network/proxy configuration |
| `feature_cws_enabled`, `feature_networks_enabled`, … | System-probe features |
| `file_configuration`, `environment_variable_configuration`, `remote_configuration`, … | Config layers (when `inventories_configuration_enabled: true`) |
| `fleet_policies_applied` | List of applied fleet layers |

When `inventories_configuration_enabled` is true, scrubbed YAML snapshots of each configuration source layer are included, along with a `full_configuration` key containing the merged result.

### `util.InventoryPayload` (embedded)

The implementation embeds `comp/metadata/internal/util.InventoryPayload`, which provides:
- `Refresh()` — schedule an out-of-cycle send (respects `inventories_min_interval`)
- `GetAsJSON()` — return current payload as scrubbed JSON (used by the HTTP endpoint and flare)
- `MetadataProvider()` — returns a `runnerimpl.Provider` for the runner
- `FlareProvider()` — adds `metadata/inventory/agent.json` to flares

## fx wiring

Provided by `inventoryagentimpl.Module()`, included in `metadata.Bundle()`. Constructor outputs:

| Output | Description |
|---|---|
| `inventoryagent.Component` | The component itself |
| `runnerimpl.Provider` | Periodic collection callback registered with the runner |
| `flaretypes.Provider` | Adds `metadata/inventory/agent.json` to flares |
| `status.HeaderInformationProvider` | Contributes agent metadata to the status page |
| `api.AgentEndpointProvider` (`GET /metadata/inventory-agent`) | Returns current payload as scrubbed JSON |

## Remote sub-agent configuration fetching

When building a payload, the component attempts to fetch the live configuration of each co-located sub-agent via IPC (using the `ipc.HTTPClient` dependency). This ensures that feature flags reflect the sub-agent's actual running configuration rather than the core agent's local copy. If a sub-agent is unavailable, the component falls back to the core agent's configuration (which may be zero-valued for system-probe settings if its config was not loaded).

## Configuration

| Key | Default | Description |
|---|---|---|
| `inventories_enabled` | `true` | Master switch for all inventory metadata |
| `inventories_configuration_enabled` | `true` | Include per-source config layer snapshots in the payload |
| `inventories_min_interval` | `60s` | Minimum time between two payload submissions |
| `inventories_max_interval` | `600s` | Maximum time between two payload submissions (triggers a send even without a refresh) |
| `inventories_first_run_delay` | `60s` | Delay before the very first payload is sent after startup |

## Usage

### Reading agent metadata (status page)

```go
type dependencies struct {
    fx.In
    InventoryAgent inventoryagent.Component
}

func (c *myComp) getStatus() map[string]interface{} {
    return c.inventoryAgent.Get()
}
```

### Writing agent metadata from a component

Components that want to contribute metadata call `Set`. The key must be unique within the flat `agent_metadata` map:

```go
// From comp/logs/agent — report current transport
a.inventoryAgent.Set("logs_transport", string(status.GetCurrentTransport()))

// From comp/updater/ssistatus — report auto-instrumentation state
c.inventoryAgent.Set("feature_auto_instrumentation_enabled", enabled)
c.inventoryAgent.Set("auto_instrumentation_modes", modes)

// From comp/otelcol — report OTLP pipeline state
c.inventoryAgent.Set("otlp_enabled", on)

// From comp/connectivitychecker — report diagnostics results
c.inventoryAgent.Set("diagnostics", diagnoses)
```

`Set` is a no-op when inventory is disabled. It triggers a `Refresh()` internally if the value changed.

## Related packages

| Package | Relationship |
|---------|-------------|
| [`comp/metadata/runner`](runner.md) | Scheduling backbone. `inventoryagent` registers its `collect` callback as a `runnerimpl.Provider`. The runner drives it on the configured `inventories_max_interval`, while `Refresh()` (triggered by `Set` or `config.OnUpdate`) can schedule an earlier send subject to `inventories_min_interval`. |
| [`comp/metadata/inventorychecks`](inventorychecks.md) | Sibling inventory component targeting the **Checks** tab. Both embed `util.InventoryPayload` and follow the same `Set`/`Refresh` pattern. Where `inventoryagent` covers agent-wide feature flags and configuration layers, `inventorychecks` covers per-check-instance metadata. Components that want to expose check-specific metadata (not agent-wide) should use `inventorychecks.Set` instead. |
| [`comp/metadata/inventoryhost`](inventoryhost.md) | Sibling inventory component targeting host hardware metadata. Unlike `inventoryagent`, `inventoryhost` has no `Set` method — it derives all data from the system at collection time (gohai, DMI, cloud provider). The three inventory components share the `inventories_*` configuration namespace and are all included in `metadata.Bundle()`. |
