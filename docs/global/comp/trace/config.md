# comp/trace/config

**Team:** agent-apm
**Import path:** `github.com/DataDog/datadog-agent/comp/trace/config/def`
**fx module:** `github.com/DataDog/datadog-agent/comp/trace/config/fx`

## Purpose

`comp/trace/config` translates the core agent configuration into a
`pkg/trace/config.AgentConfig` object that the trace pipeline understands. It
is a bridge between the unified `comp/core/config` system and the
`pkg/trace/config` package, which predates the component architecture.

Responsibilities:

- Builds `AgentConfig` from `datadog.yaml` (`apm_config.*` keys), environment
  variables, and defaults.
- Applies overrides for endpoints, proxy, obfuscation rules, sampling
  parameters, OTLP receiver settings, watchdog limits, etc.
- Subscribes to `comp/core/config` live updates to rotate the API key without
  a restart.
- Connects a remote-configuration gRPC client (`ProductAPMSampling`,
  `ProductAgentConfig`) when RC is enabled.
- Exposes authenticated HTTP handlers (`/config`, `/config/set`) so the debug
  server and `datadog-agent config` CLI can inspect or change runtime
  settings.
- Sets `MaxCPU` / `MaxMemory` to zero in containerized environments where the
  container runtime is responsible for resource limits.

## Key elements

### Interface (`comp/trace/config/def`)

```go
type Component interface {
    // Warnings returns config warnings collected during setup (missing keys,
    // deprecated options, etc.).
    Warnings() *model.Warnings

    // SetHandler returns an authenticated HTTP handler that accepts POST
    // requests to change runtime settings (currently: log_level).
    SetHandler() http.Handler

    // GetConfigHandler returns an authenticated HTTP handler that responds
    // to GET requests with the scrubbed runtime YAML configuration.
    GetConfigHandler() http.Handler

    // SetMaxMemCPU adjusts watchdog thresholds; call with isContainerized=true
    // to disable them (limits are managed by the container runtime instead).
    SetMaxMemCPU(isContainerized bool)

    // Object returns the underlying *pkg/trace/config.AgentConfig.
    // Callers that need raw config fields use this accessor.
    Object() *traceconfig.AgentConfig

    // OnUpdateAPIKey registers a callback that fires when the core API key
    // changes at runtime. Only one callback is supported at a time.
    OnUpdateAPIKey(func(oldKey, newKey string))
}
```

### Params (`comp/trace/config/def`)

```go
type Params struct {
    // FailIfAPIKeyMissing controls whether NewComponent returns an error
    // when no API key is present. Set to false by the main Agent process
    // (which can start without APM active) and true for standalone
    // trace-agent.
    FailIfAPIKeyMissing bool
}
```

### Dependencies (`comp/trace/config/impl`)

| Dep | Purpose |
|---|---|
| `comp/core/config.Component` | Source of all `apm_config.*` / top-level settings. |
| `comp/core/tagger/def.Component` | Provides container tag lookup used to enrich spans with `ContainerTags`. |
| `comp/core/ipc/def.Component` | Supplies the auth token and TLS config for IPC / debug server endpoints. |

### Important `pkg/trace/config.AgentConfig` fields set by this component

| Field | Config key |
|---|---|
| `Endpoints` | `api_key`, `site`, `apm_config.additional_endpoints` |
| `ReceiverHost/Port` | `apm_config.receiver_port`, `bind_host` |
| `TargetTPS` | `apm_config.target_traces_per_second` |
| `OTLPReceiver` | `otlp_config.traces.*` |
| `Obfuscation` | `apm_config.obfuscation.*` |
| `RemoteConfigClient` | auto-created when RC is enabled |
| `MaxCPU` / `MaxMemory` | `apm_config.max_cpu_percent` / `apm_config.max_memory` |

### Remote configuration

When `remote_configuration.apm_sampling.enabled` is true, the component
creates a gRPC RC client subscribed to `ProductAPMSampling` and
`ProductAgentConfig`. A separate MRF client (`ProductAgentFailover`) is
created when `multi_region_failover.enabled` is true.

### fx wiring

```
comp/trace/config/fx.Module()
  └─ fxutil.ProvideComponentConstructor(configimpl.NewComponent)
     fx.Supply(traceconfig.Params{FailIfAPIKeyMissing: true})
```

For tests, `comp/trace/config/mock/mock.go` provides a mock with
`MockNewComponent`.

## Usage

### Standalone trace-agent

`cmd/trace-agent/subcommands/run/command.go` uses
`traceconfigimpl.NewComponent` directly (bypassing the `fx` module) so it can
supply custom `Params`:

```go
fx.Provide(func() traceconfigdef.Params {
    return traceconfigdef.Params{FailIfAPIKeyMissing: true}
}),
fx.Provide(traceconfigimpl.NewComponent),
```

### Core agent (embedded trace pipeline)

`cmd/agent/subcommands/run/command_windows.go` includes the config component
inside the main agent's fx app.

### OTel agent / host-profiler

`cmd/otel-agent` and `cmd/host-profiler` include this component to configure
the trace pipeline they embed.

### Accessing raw config

Most consumers call `cfg.Object()` to read `AgentConfig` fields that are not
exposed on the interface, for example:

```go
tracecfg := deps.Config.Object()
if tracecfg.Enabled { ... }
```

### Runtime configuration endpoint

The `comp/trace/agent/impl` registers the handlers on the debug server:

```go
ag.Agent.DebugServer.AddRoute("/config", ag.config.GetConfigHandler())
ag.Agent.DebugServer.AddRoute("/config/set", ag.config.SetHandler())
```

---

## Related packages and components

| Package / Component | Doc | Relationship |
|---|---|---|
| `pkg/trace/config` | [../../pkg/trace/config.md](../../pkg/trace/config.md) | Defines `AgentConfig` — the struct populated by this component. `cfg.Object()` returns a `*pkg/trace/config.AgentConfig`. All `apm_config.*` YAML keys, sampler parameters, writer tuning, obfuscation rules, and OTLP receiver settings are documented there. `ObfuscationConfig.Export()` converts the nested config to an `obfuscate.Config` consumed by `pkg/obfuscate`. |
| `comp/trace/agent` | [agent.md](agent.md) | Consumer. `comp/trace/agent/impl` holds a reference to this component to call `OnUpdateAPIKey` for key rotation and to read `cfg.Object()` when constructing `pkg/trace/agent.NewAgent`. The debug server routes (`/config`, `/config/set`) are registered by `comp/trace/agent/impl` using handlers returned by this component. |
| `comp/remote-config/rcclient` | [../remote-config/rcclient.md](../remote-config/rcclient.md) | RC integration. This component creates gRPC RC clients subscribing to `ProductAPMSampling` and `ProductAgentConfig` (and `ProductAgentFailover` when MRF is enabled). The resulting `RemoteConfigClient` interface is stored on `AgentConfig.RemoteConfigClient` and consumed by `pkg/trace/remoteconfighandler` to apply live sampling-rate and failover updates. |
