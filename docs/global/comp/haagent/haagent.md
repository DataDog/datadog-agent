> **TL;DR:** `comp/haagent` enables high-availability deployments by tracking whether the current agent is the elected leader via Remote Config, and gating HA-aware check execution on that state.

# comp/haagent/impl — High-Availability Agent Component

**Import path (interface):** `github.com/DataDog/datadog-agent/comp/haagent/def`
**Import path (implementation):** `github.com/DataDog/datadog-agent/comp/haagent/impl`
**Team:** network-device-monitoring-core
**Importers:** aggregator demultiplexer, collector, metadata bundle

## Purpose

`comp/haagent` enables multiple Datadog Agents to run side-by-side while only one — the elected leader — actively executes HA-aware checks at any given time. This prevents duplicate data collection in high-availability deployments, such as two agents monitoring the same network device.

The component tracks whether the current agent is the **active** (leader) or **standby** instance. When enabled, it registers a Remote Configuration listener for the `HA_AGENT` product. The Datadog backend pushes a small JSON payload that names the active agent. The component compares that name against the local hostname and updates its state accordingly.

## Package layout

| Path | Role |
|---|---|
| `comp/haagent/def/` | Public interface (`Component`, `State` constants) |
| `comp/haagent/impl/` | Implementation, RC payload handler, config loading |
| `comp/haagent/helpers/` | `IsEnabled` / `GetConfigID` helpers used by other packages |
| `comp/haagent/fx/` | fx module wiring |
| `comp/haagent/mock/` | Test mock |

## Key elements

### Key interfaces

```go
type Component interface {
    Enabled() bool
    GetConfigID() string
    GetState() State
    SetLeader(leaderAgentHostname string)
    IsActive() bool
}
```

### Key types

**`State`** — represents the leadership status of the current agent instance:

```go
type State string

const (
    Active  State = "active"
    Standby State = "standby"
    Unknown State = "unknown"
)
```

State starts as `Unknown` and transitions to `Active` or `Standby` once the first Remote Config update arrives. If the RC subscription delivers an empty update list the state resets to `Unknown`.

### Key functions

| Method | Description |
|---|---|
| `Enabled()` | Returns `true` when `ha_agent.enabled: true` and `config_id` is set. Logs an error once if `config_id` is missing. |
| `GetConfigID()` | Returns the `ha_agent.config_id` config value used to group agents in the same HA cluster. |
| `SetLeader(hostname)` | Compares `hostname` with the local agent hostname. Sets state to `Active` if they match, `Standby` otherwise. Logs state transitions at INFO level. |
| `IsActive()` | Returns `true` only when state is `Active`. Consumers call this before executing HA-gated work. |

### Configuration and build flags

| Key | Default | Description |
|---|---|---|
| `ha_agent.enabled` | `false` | Enables HA mode |
| `ha_agent.config_id` | `""` | Shared identifier for all agents in the same HA group |

## fx wiring

`NewComponent` (in `comp/haagent/impl`) returns a `Provides` struct with two values:

- `Comp haagent.Component` — the component itself
- `RCListener rctypes.ListenerProvider` — a Remote Config listener registered on `state.ProductHaAgent`, only populated when `Enabled()` is true

Dependencies (`Requires`): `log.Component`, `config.Component`, `hostnameinterface.Component`.

The `RCListener` uses the fx group `"rCListener"` (type `types.ListenerProvider` from `comp/remote-config/rcclient/types`). The RC client collects all group members and subscribes them at startup, so `comp/haagent` does not need to call `rcclient.Subscribe` imperatively. See [comp/remote-config/rcclient](../remote-config/rcclient.md) for the full listener mechanism.

## Usage in the codebase

### Check execution gating (`pkg/collector/worker`)

Workers call `haAgent.IsActive()` before running each check that declares HA support:

```go
if w.haAgent.Enabled() && check.IsHASupported() && !w.haAgent.IsActive() {
    checkLogger.Debug("Check is an HA integration and current agent is not leader, skipping execution...")
    continue
}
```

This is the primary enforcement point — standby agents skip HA checks entirely.

### Aggregator demultiplexer (`comp/aggregator/demultiplexer`)

The demultiplexer receives `haagent.Component` as an optional dependency. It uses the component to annotate or gate metric forwarding for HA-aware pipelines.

### Collector (`comp/collector/collector`)

The collector passes `haagent.Component` down to the runner so that the worker goroutines above can access it.

## Extending HA support to a new check

1. Implement `IsHASupported() bool` returning `true` on the check struct.
2. The worker layer will automatically skip the check when the local agent is in `Standby` state.
3. No changes to `comp/haagent` itself are required.

## Related components

| Component / Package | Doc | Relationship |
|---|---|---|
| `comp/remote-config/rcclient` | [../remote-config/rcclient.md](../remote-config/rcclient.md) | Delivers `HA_AGENT` product payloads to `comp/haagent` via the `rCListener` fx group. `haagent` registers a `types.ListenerProvider` that the RC client subscribes at startup; no explicit `Subscribe` call is needed from `haagent`. |
| `comp/aggregator/demultiplexer` | [../aggregator/demultiplexer.md](../aggregator/demultiplexer.md) | Receives `haagent.Component` as a dependency and uses it to coordinate HA-aware metric forwarding pipelines. |
| `comp/collector/collector` | [../collector/collector.md](../collector/collector.md) | Passes `haagent.Component` to the runner, which gates HA-aware check execution via `IsActive()` in each worker goroutine. |
