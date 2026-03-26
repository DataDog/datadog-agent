> **TL;DR:** Periodically reports the High Availability Agent's enabled flag and current state (`active` or `standby`) to the Datadog inventory backend, contributing a section to the agent status page and a `ha-agent.json` file to flares.

# comp/metadata/haagent

**Team:** network-device-monitoring-core
**Import path:** `github.com/DataDog/datadog-agent/comp/metadata/haagent`

## Purpose

The `haagent` metadata component collects and periodically sends a metadata payload describing the High Availability (HA) Agent state to the Datadog inventory backend (`datadog_agent_ha_agent` table). It reports whether HA mode is enabled and, when it is, the current agent state (`active` or `standby`).

The payload is only sent when `ha_agent.enabled` is `true`. It is submitted every ~10 minutes (governed by `inventories_max_interval`), with a minimum interval of ~1 minute between updates (`inventories_min_interval`). The payload can be globally disabled with `inventories_enabled: false`.

In addition to the inventory backend, the component:
- serves the payload at the `/metadata/ha-agent` HTTP endpoint (GET)
- includes a `ha-agent.json` file in flares
- contributes a section to the agent status page (header provider, index 3, labelled "HA Agent Metadata")

## Key elements

### Key interfaces

The component interface is empty — all integration points are registered as fx output values:

```go
type Component interface{}
```

### Key types

#### Payload

```go
type Payload struct {
    Hostname  string           `json:"hostname"`
    Timestamp int64            `json:"timestamp"`
    Metadata  *haAgentMetadata `json:"ha_agent_metadata"`
}

type haAgentMetadata struct {
    Enabled bool   `json:"enabled"`
    State   string `json:"state"`
}
```

`Metadata` is `nil` (omitted from the payload) when HA Agent is disabled. `State` reflects the value returned by `haagentcomp.Component.GetState()`.

#### Status page

The component implements `status.HeaderInformationProvider` (via `status.go`). It renders the HA Agent enabled flag and current state using Go templates in `impl/status_templates/`.

### Configuration and build flags

| Key | Default | Description |
|---|---|---|
| `ha_agent.enabled` | `false` | Master switch; payload `Metadata` is `nil` and nothing is sent when `false` |
| `inventories_enabled` | `true` | Globally disables all inventory metadata when `false` |
| `inventories_min_interval` | `60s` | Minimum time between updates |
| `inventories_max_interval` | `600s` | Maximum time between updates |

#### fx wiring

`fx.Module()` registers `NewComponent` and exposes `Component` as an optional value.

#### Dependencies

| Dependency | Purpose |
|---|---|
| `log.Component` | Structured logging |
| `config.Component` | Read inventory feature flags |
| `serializer.MetricSerializer` | Submit the payload to the backend |
| `haagentcomp.Component` | Query HA Agent enabled/state at payload generation time |
| `hostnameinterface.Component` | Resolve the hostname included in the payload |

#### Outputs

| Output | Purpose |
|---|---|
| `haagent.Component` | The component itself |
| `runnerimpl.Provider` | Hooks the component into the metadata runner's periodic flush loop |
| `flaretypes.Provider` | Includes `ha-agent.json` in flare archives |
| `status.HeaderInformationProvider` | Contributes the HA Agent section to the agent status page |
| `api.AgentEndpointProvider` | Registers `GET /metadata/ha-agent` on the agent API server |

## Usage

### Registration

The component is included in the main agent metadata bundle:

```
comp/metadata/bundle.go
```

```go
haagentmetadatafx "github.com/DataDog/datadog-agent/comp/metadata/haagent/fx"
// ...
haagentmetadatafx.Module()
```

It is also imported by the main agent run command and the Windows-specific variant, as well as the flare command:

```
cmd/agent/subcommands/run/command.go
cmd/agent/subcommands/run/command_windows.go
cmd/agent/subcommands/flare/command.go
```

### Enabling HA Agent

Set `ha_agent.enabled: true` in `datadog.yaml`. When disabled, `NewComponent` logs a debug message, the payload's `Metadata` field is `nil`, and nothing is sent to the backend.

### Example payload

```json
{
    "hostname": "COMP-GQ7WQN6HYC",
    "timestamp": 1716985696922603000,
    "ha_agent_metadata": {
        "enabled": true,
        "state": "active"
    }
}
```
