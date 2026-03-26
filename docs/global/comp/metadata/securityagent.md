# comp/metadata/securityagent

**Team:** agent-configuration
**Import path:** `github.com/DataDog/datadog-agent/comp/metadata/securityagent`

## Purpose

The `securityagent` metadata component collects and periodically sends a metadata payload for the Security Agent process to the Datadog inventory backend (`security-agent` table). It reports the agent version and the Security Agent configuration broken down by configuration source (file, environment variables, Remote Configuration, Fleet Automation Policies, CLI, runtime, etc.), with all secrets scrubbed.

The payload is enabled by default and is sent every ~10 minutes (governed by `inventories_max_interval`). Configuration collection can be turned off with `inventories_configuration_enabled: false`. The payload is also available:
- at the `/metadata/security-agent` HTTP endpoint (GET)
- in flares (`security-agent.json`)

Configuration is fetched from the Security Agent process over IPC using `configFetcher.SecurityAgentConfig` and `SecurityAgentConfigBySource`.

## Key elements

### Interface (`comp/metadata/securityagent/def/component.go`)

The component interface is empty — the component contributes its behaviour entirely through the values it provides to fx:

```go
type Component interface{}
```

All integration points (HTTP endpoint, flare, metadata runner) are registered as fx output values.

### Payload (`comp/metadata/securityagent/impl/security_agent.go`)

```go
type Payload struct {
    Hostname  string                 `json:"hostname"`
    Timestamp int64                  `json:"timestamp"`
    Metadata  map[string]interface{} `json:"security_agent_metadata"`
}
```

Fields inside `Metadata`:
- `agent_version` — always present
- `full_configuration` — complete scrubbed YAML (when `inventories_configuration_enabled` is `true`)
- `file_configuration`, `environment_variable_configuration`, `agent_runtime_configuration`, `remote_configuration`, `fleet_policies_configuration`, `cli_configuration`, `source_local_configuration`, `provided_configuration` — per-source scrubbed YAML layers

### fx wiring (`comp/metadata/securityagent/fx/fx.go`)

`fx.Module()` registers `NewComponent` and exposes `Component` as an optional value.

### Dependencies (from `Requires`)

| Dependency | Purpose |
|---|---|
| `log.Component` | Structured logging |
| `config.Component` | Read feature flags (`inventories_configuration_enabled`) |
| `serializer.MetricSerializer` | Submit the payload to the backend |
| `hostnameinterface.Component` | Resolve the hostname included in the payload |
| `ipc.HTTPClient` | Fetch the Security Agent configuration over the IPC socket |

### Outputs (from `Provides`)

| Output | Purpose |
|---|---|
| `securityagent.Component` | The component itself |
| `runnerimpl.Provider` | Hooks the component into the metadata runner's periodic flush loop |
| `flaretypes.Provider` | Includes `security-agent.json` in flare archives |
| `api.AgentEndpointProvider` | Registers `GET /metadata/security-agent` on the agent API server |

## Usage

### Registration

The component is included in the main agent metadata bundle:

```
comp/metadata/bundle.go
```

```go
securityagent "github.com/DataDog/datadog-agent/comp/metadata/securityagent/fx"
// ...
securityagent.Module()
```

It is also explicitly imported by the main agent run command:

```
cmd/agent/subcommands/run/command.go
```

### How the runner drives this component

The `runnerimpl.Provider` produced by this component is consumed by `comp/metadata/runner`. The runner spawns a dedicated goroutine that calls this component's collection callback at the interval it returns (up to `inventories_max_interval`, default ~10 minutes). On `OnStop` the runner waits up to `metadata_provider_stop_timeout` (default 2 s) for any in-flight collection to finish before exiting. See [comp/metadata/runner](runner.md) for the scheduling contract and configuration keys.

### IPC transport and config fetching

The `ipc.HTTPClient` dependency is obtained from `comp/core/ipc` (using `fx.ModuleReadWrite()` in the security-agent daemon). The client uses mutual-TLS and a bearer-token `Authorization` header to reach the Security Agent's CMD API server. The component calls `configFetcher.SecurityAgentConfig` and `SecurityAgentConfigBySource` over that HTTP connection to collect the full and per-source configuration snapshots that populate the payload. See [comp/core/ipc](../core/ipc.md) for the IPC authentication model.

### Relationship to the Security Agent process

The Security Agent binary (`cmd/security-agent`) registers `ipc.ModuleReadWrite()` in its fx graph, which makes an `ipc.HTTPClient` available for this metadata component to call. The `RuntimeSecurityAgent` struct in `pkg/security/agent` (the CWS runtime component) is entirely separate from this metadata payload — it handles gRPC event streaming, not configuration inventory. See [pkg/security/agent](../../pkg/security/agent.md) for the runtime agent lifecycle.

### Disabling configuration collection

Set `inventories_configuration_enabled: false` in `datadog.yaml` to prevent configuration details from being included in the payload. The `agent_version` field is always sent.

## Related documentation

| Document | Relationship |
|---|---|
| [comp/metadata/runner](runner.md) | Scheduling backbone that drives the periodic flush for this component; `runnerimpl.Provider` produced here is consumed by the runner |
| [comp/core/ipc](../core/ipc.md) | Provides the `HTTPClient` and bearer-token/TLS infrastructure used to call the Security Agent CMD API for config fetching |
| [pkg/security/agent](../../pkg/security/agent.md) | The runtime CWS agent (`RuntimeSecurityAgent`); shares the security-agent process but handles gRPC event streaming, not configuration inventory |

### Example payload

```json
{
    "hostname": "my-host",
    "timestamp": 1631281754507358895,
    "security_agent_metadata": {
        "agent_version": "7.55.0",
        "full_configuration": "<entire yaml configuration for security-agent>",
        "provided_configuration": "runtime_security_config:\n  socket: /opt/datadog-agent/run/runtime-security.sock",
        "file_configuration": "runtime_security_config:\n  socket: /opt/datadog-agent/run/runtime-security.sock",
        "agent_runtime_configuration": "runtime_block_profile_rate: 5000",
        "environment_variable_configuration": "{}",
        "remote_configuration": "{}",
        "cli_configuration": "{}",
        "source_local_configuration": "{}"
    }
}
```
