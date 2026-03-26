> **TL;DR:** `pkg/trace/remoteconfighandler` bridges the Remote Configuration system to the trace agent's runtime-adjustable samplers, applying live updates to sampling TPS, rare sampler toggle, log level, and MRF failover settings without requiring an agent restart.

# pkg/trace/remoteconfighandler

## Purpose

`remoteconfighandler` bridges the Remote Configuration (RC) system and the trace-agent's
runtime-adjustable samplers. It subscribes to RC product updates and applies them to the
running sampler instances without requiring an agent restart. It also handles two additional
RC products: dynamic log-level changes for the trace-agent, and Multi-Region Failover (MRF)
APM failover configuration.

## Key elements

### Types

**`RemoteConfigHandler`** — Central struct. Holds references to the three sampler interfaces
(`prioritySampler`, `errorsSampler`, `rareSampler`) and two RC clients (standard + MRF),
the agent config, and an HTTP client used to push log-level changes to the trace-agent's
debug server.

**`prioritySampler` / `errorsSampler`** (interfaces) — Require a single method
`UpdateTargetTPS(float64)`, called whenever RC delivers a new target traces-per-second value.

**`rareSampler`** (interface) — Requires `SetEnabled(bool)`, toggled by the RC
`RareSamplerEnabled` field.

**`multiRegionFailoverConfig`** (internal) — Deserialised form of an `AGENT_FAILOVER` RC
payload. Only `failover_apm` (a `*bool`) is consumed here; `failover_logs` and
`failover_metrics` are handled by the core agent.

### Functions

| Function | Description |
|---|---|
| `New(conf, prioritySampler, rareSampler, errorsSampler)` | Creates a handler. Returns `nil` if `conf.RemoteConfigClient` is unset or if the debug server port is disabled (both are preconditions for RC to work). |
| `(*RemoteConfigHandler).Start()` | Starts both RC clients and registers the three subscription callbacks. Safe to call on a `nil` receiver (no-op). |

### RC subscriptions registered by `Start()`

| RC product | Callback | Effect |
|---|---|---|
| `APMSampling` | `onUpdate` | Parses `apmsampling.SamplerConfig`, applies `PrioritySamplerTargetTPS`, `ErrorsSamplerTargetTPS`, `RareSamplerEnabled` — first env-specific, then all-envs, then local config default. |
| `AgentConfig` | `onAgentConfigUpdate` | Merges RC agent config; if a `log_level` is present, POSTs to the trace-agent's debug server (`/config/set?log_level=<level>`). Restores the previous level when the override is removed. |
| `AgentFailover` (MRF client) | `mrfUpdateCallback` | Parses `AGENT_FAILOVER` payload and writes `failover_apm` into `agentConfig.MRFFailoverAPMRC`. |

## Usage

`RemoteConfigHandler` is created once during agent initialisation in
`pkg/trace/agent/agent.go`:

```go
agnt.RemoteConfigHandler = remoteconfighandler.New(
    conf, agnt.PrioritySampler, agnt.RareSampler, agnt.ErrorsSampler,
)
```

`Start()` is called as part of the agent's startup sequence. Because `New` returns `nil`
when RC is unavailable, every method on `RemoteConfigHandler` guards against a `nil`
receiver, so callers do not need to check the return value before calling `Start()`.

### Sampler priority resolution

For each sampler setting, `updateSamplers` applies the following precedence (highest first):

1. Env-specific value from `SamplerConfig.ByEnv` where `Env == agentConfig.DefaultEnv`
2. Global value from `SamplerConfig.AllEnvs`
3. Local config default (`agentConfig.TargetTPS` / `agentConfig.ErrorTPS` / `agentConfig.RareSamplerEnabled`)

### Log-level changes

Log-level adjustments are forwarded via an HTTP POST to the trace-agent's own debug server
(`apm_config.debug.port`, default 5012). The request is authenticated with the agent's IPC
auth token. The pre-override level is saved in `configState.FallbackLogLevel` and restored
when RC removes the override.
