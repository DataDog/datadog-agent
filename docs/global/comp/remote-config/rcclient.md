> **TL;DR:** Runs an in-process Remote Configuration client that polls the core agent's gRPC endpoint for config updates and dispatches changes to subscribed components at runtime without an agent restart.

# comp/remote-config/rcclient — Remote Configuration Client Component

**Team:** remote-config
**Import path (interface):** `github.com/DataDog/datadog-agent/comp/remote-config/rcclient`
**Import path (implementation):** `github.com/DataDog/datadog-agent/comp/remote-config/rcclient/rcclientimpl`
**Import path (types):** `github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types`
**Importers:** ~22 packages

## Purpose

`comp/remote-config/rcclient` runs a Remote Configuration (RC) client inside the agent. The client connects to the RC backend (via the core agent's gRPC IPC endpoint) and polls for configuration updates. When new configurations arrive for a subscribed product, the client invokes registered callbacks so that components can apply changes at runtime without restarting the agent.

Built-in behaviors handled by the client itself (no extra subscription needed):

- **`AGENT_CONFIG`** — applies `log_level` changes dynamically, respecting source precedence (CLI > RC > file).
- **`AGENT_TASK`** — dispatches one-shot tasks (e.g. flare generation, NDM device scan) to all registered task listeners, with deduplication by UUID and a 5-minute timeout.
- **`AGENT_FAILOVER`** (Multi-Region Failover) — applies `multi_region_failover.*` runtime settings when a second MRF client is configured.

## Package layout

| Package | Role |
|---|---|
| `comp/remote-config/rcclient` (root) | `Component` interface, `Params`, `NoneModule()` helper |
| `rcclientimpl` | `rcClient` struct, fx `Module()`, gRPC client construction, all built-in callbacks |
| `rcclientimpl/agent_failover.go` | MRF config parsing and `mrfUpdateCallback` |
| `types` | Shared types: `RCListener`, `RCAgentTaskListener`, `TaskListenerProvider`, `AgentTaskConfig`, `TaskType` |

## Key Elements

### Key interfaces

## Component interface

```go
type Component interface {
    // SubscribeAgentTask subscribes to AGENT_TASK product updates.
    // Call this once during agent startup to enable remote task dispatch.
    SubscribeAgentTask()

    // Subscribe registers a callback for a specific RC product.
    Subscribe(product data.Product, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)))
}
```

`Subscribe` is the generic path; `SubscribeAgentTask` is a convenience that wires the internal task dispatcher.

### Key types

## Params

```go
type Params struct {
    AgentName     string  // e.g. "core-agent", "system-probe"
    AgentVersion  string  // e.g. version.AgentVersion
    IsSystemProbe bool    // when true, AGENT_CONFIG log_level changes target sysprobeconfig
}
```

`Params` is supplied via `fx.Supply` at the call site. Both `AgentName` and `AgentVersion` are required — the component returns an error during construction if either is empty.

### Key functions

## fx group listeners — `types` package

Rather than calling `Subscribe` imperatively, components can declare their subscriptions at construction time by returning an fx group value. The client collects all group members and subscribes them automatically at startup.

### Generic product listener (`RCListener`)

```go
// types.RCListener is map[data.Product]callback
type RCListener map[data.Product]func(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))

// Return from a component constructor:
type provides struct {
    fx.Out
    Listener types.ListenerProvider `group:"rCListener"`  // use types.ListenerProvider wrapper
}
```

### Task listener (`RCAgentTaskListener`)

Used when a component wants to handle `AGENT_TASK` dispatches:

```go
type RCAgentTaskListener func(taskType TaskType, task AgentTaskConfig) (bool, error)

// Return from a component constructor:
return types.NewTaskListener(func(taskType types.TaskType, task types.AgentTaskConfig) (bool, error) {
    if taskType != types.TaskFlare { return false, nil }
    // handle flare task ...
    return true, nil
})
```

`NewTaskListener` wraps the function in a `TaskListenerProvider` (which embeds `fx.Out` with group tag `"rCAgentTaskListener"`).

### Predefined task types

| Constant | Value | Description |
|---|---|---|
| `types.TaskFlare` | `"flare"` | Request the agent to generate and send a flare |
| `types.TaskDeviceScan` | `"ndm-device-scan"` | Trigger an NDM device scan |

### Configuration and build flags

## fx wiring

```go
// Provide Params before including the module:
fx.Supply(rcclient.Params{
    AgentName:    "core-agent",
    AgentVersion: version.AgentVersion,
}),
rcclientimpl.Module(),
```

`rcclientimpl.Module()` also calls `fxutil.ProvideOptional[rcclient.Component]()`, so the component is available as `option.Option[rcclient.Component]` in addition to the concrete type.

The client is intentionally designed as a "pure leaf" component: it must not be a dependency of other components to avoid cycles. It sits at the end of the dependency graph and can interact with any other component.

The client only starts polling when Remote Configuration is enabled in the agent config (`configUtils.IsRemoteConfigEnabled`). The gRPC connection is always closed on stop.

## Disabling the component

When a binary does not need RC at all, use `rcclient.NoneModule()` to provide a disabled `option.Option[rcclient.Component]` without linking the implementation:

```go
rcclient.NoneModule(),  // provides option.None[rcclient.Component]()
```

## Usage patterns

**Wiring the client in a daemon (core agent):**

```go
// cmd/agent/subcommands/run/command.go
fx.Supply(rcclient.Params{
    AgentName:    "core-agent",
    AgentVersion: version.AgentVersion,
}),
rcclientimpl.Module(),

// In the run() function, subscribe after startup:
func run(..., rcclient rcclient.Component, ...) {
    rcclient.SubscribeAgentTask()
    rcclient.Subscribe(data.ProductAgentIntegrations, rcProvider.IntegrationScheduleCallback)
}
```

**Wiring in system-probe:**

```go
fx.Supply(rcclient.Params{
    AgentName:     "system-probe",
    AgentVersion:  version.AgentVersion,
    IsSystemProbe: true,  // routes AGENT_CONFIG log_level to sysprobeconfig
}),
rcclientimpl.Module(),
```

**Registering a component as a task listener (fx group):**

```go
// comp/core/flare/flare.go
return provides{
    Comp:       f,
    Endpoint:   api.NewAgentEndpointProvider(f.createAndReturnFlarePath, "/flare", "POST"),
    RCListener: rcclienttypes.NewTaskListener(f.onAgentTaskEvent),
}
```

**Subscribing to a product imperatively:**

```go
func run(..., rc rcclient.Component, ...) {
    rc.Subscribe(data.ProductAPMSampling, func(updates map[string]state.RawConfig, applyState func(string, state.ApplyStatus)) {
        for cfgPath, cfg := range updates {
            // parse and apply cfg.Config
            applyState(cfgPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
        }
    })
}
```

## Key dependents

- `cmd/agent/subcommands/run` — core agent; subscribes to agent tasks and integrations
- `cmd/system-probe/subcommands/run` — system-probe daemon; targets sysprobeconfig for log level
- `cmd/process-agent` — process-agent daemon
- `comp/core/flare` — handles `TaskFlare` to generate flares on demand
- `comp/syntheticstestscheduler` — subscribes to synthetic test schedule updates
- `comp/filterlist` — subscribes to filter list product
- `pkg/collector/corechecks/snmp` — subscribes to SNMP device scan tasks

## Related packages

| Package | Relationship |
|---------|-------------|
| [`comp/remote-config/rcservice`](rcservice.md) | The server-side RC component that `rcclient` connects to over gRPC IPC. `rcservice` polls the Datadog backend, verifies payloads with Uptane, and serves `ClientGetConfigs` / `CreateConfigSubscription` to downstream clients including `rcclient`. `rcclient` is the in-process wrapper; `rcservice` is the backend-facing gateway. |
| [`pkg/config/remote`](../../pkg/config/remote.md) | Underlying implementation layers consumed by both components. `rcclientimpl` constructs a `client.Client` (from `pkg/config/remote/client`) that polls `rcservice` over the gRPC IPC channel. `rcservice` itself is wired around `pkg/config/remote/service.CoreAgentService`. The `data.Product` constants used in `Subscribe` calls are defined in `pkg/config/remote/data`. |
| [`pkg/remoteconfig/state`](../../pkg/remoteconfig.md) | Client-side TUF state machine. The `client.Client` inside `rcclientimpl` delegates all TUF verification and config caching to `state.Repository`. The `state.RawConfig`, `state.ApplyStatus`, and product constants referenced in `Subscribe` callbacks come from this package. |
