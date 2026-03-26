# comp/metadata/clusteragent

**Team:** container-platform
**Import path:** `github.com/DataDog/datadog-agent/comp/metadata/clusteragent`

## Purpose

The `clusteragent` metadata component collects and periodically sends a metadata payload for the Datadog Cluster Agent process to the Datadog inventory backend (`datadog_cluster_agent` table). It captures cluster identification, agent version, installation method, leader election state, cluster check runner and node agent counts, and the full scrubbed Cluster Agent configuration broken down by configuration source (file, environment variables, Remote Configuration, Fleet Automation Policies, etc.).

When `enable_cluster_agent_metadata_collection` is enabled the payload is submitted to the backend every ~10 minutes (governed by `inventories_max_interval`). Regardless of that flag, the payload is always available for flares (`datadog-cluster-agent.json`) and through the `/metadata/cluster-agent` HTTP endpoint.

The full implementation is guarded by the `kubeapiserver` build tag. A no-op implementation is compiled when that tag is absent so that the component can be wired unconditionally.

## Key elements

### Interface (`comp/metadata/clusteragent/def/component.go`)

```go
type Component interface {
    // WritePayloadAsJSON writes the scrubbed payload as JSON to an HTTP response.
    // Used by the cluster-agent metadata HTTP endpoint.
    WritePayloadAsJSON(w http.ResponseWriter, _ *http.Request)
}
```

### Payload (`comp/metadata/clusteragent/impl/cluster_agent.go`)

```go
type Payload struct {
    Clustername string                 `json:"clustername"`
    ClusterID   string                 `json:"cluster_id"`
    Timestamp   int64                  `json:"timestamp"`
    Metadata    map[string]interface{} `json:"datadog_cluster_agent_metadata"`
    UUID        string                 `json:"uuid"`
}
```

Notable fields inside `Metadata`:
- `agent_version`, `flavor`, `agent_startup_time_ms` — process identity
- `install_method_tool`, `install_method_tool_version`, `install_method_installer_version` — install provenance
- `is_leader`, `leader_election` — leader election status
- `cluster_check_runner_count`, `cluster_check_node_agent_count` — dispatcher health
- `cluster_id_error` — any error encountered when resolving the cluster ID
- `feature_*` — boolean/string values for every significant Cluster Agent feature flag (Admission Controller, APM instrumentation, External Metrics Provider, Cluster Checks, etc.)
- `full_configuration`, `file_configuration`, `environment_variable_configuration`, `agent_runtime_configuration`, `remote_configuration`, `fleet_policies_configuration`, `cli_configuration`, `provided_configuration` — scrubbed YAML config layers (present when `inventories_configuration_enabled` is `true`)

### fx wiring (`comp/metadata/clusteragent/fx/fx.go`)

`fx.Module()` registers `NewComponent` and makes `Component` available as an optional value. The module also `fx.Invoke`s the component so it starts even when nothing explicitly depends on it.

### Dependencies (from `Requires`)

| Dependency | Purpose |
|---|---|
| `log.Component` | Structured logging |
| `config.Component` | Read configuration flags and config layers |
| `serializer.MetricSerializer` | Submit the payload to the backend (nil when collection is disabled) |
| `hostnameinterface.Component` | Resolve the agent hostname for cluster-name lookup |

### Outputs (from `Provides`)

| Output | Purpose |
|---|---|
| `clusteragent.Component` | The component itself |
| `runnerimpl.Provider` | Hooks the component into the metadata runner's periodic flush loop |

## Usage

### Registration

The component is wired directly in the cluster-agent start command:

```
cmd/cluster-agent/subcommands/start/command.go
```

```go
dcametadatafx "github.com/DataDog/datadog-agent/comp/metadata/clusteragent/fx"
// ...
dcametadatafx.Module()
```

### HTTP endpoint

The cluster-agent API server passes the component to its router setup:

```
cmd/cluster-agent/api/server.go  — StartServer(..., dcametadataComp dcametadata.Component, ...)
cmd/cluster-agent/api/agent/agent.go
```

```go
r.HandleFunc("/metadata/cluster-agent", dcametadataComp.WritePayloadAsJSON).Methods("GET")
```

This endpoint is consumed by the Datadog backend and by `datadog-agent flare` to include the payload in support archives.

### Cloud Foundry cluster agent

The same component and registration pattern is used by the Cloud Foundry cluster agent:

```
cmd/cluster-agent-cloudfoundry/subcommands/run/command.go
```
