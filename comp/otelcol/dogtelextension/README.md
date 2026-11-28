# Datadog Agent Extension for OpenTelemetry Collector

The `dogtelextension` packages Datadog Agent functionalities for use in the OpenTelemetry Collector (otel-agent) when running in **standalone mode**.

## ⚠️ Important: Standalone Mode Only

**This extension should ONLY be enabled when `DD_OTEL_STANDALONE=true`.**

- **Standalone mode**: Use this extension when otel-agent runs independently without a core Datadog Agent
- **Connected mode**: Do NOT use this extension when otel-agent runs alongside the core Datadog Agent (the core agent already provides these functionalities)

The extension will automatically disable itself if `DD_OTEL_STANDALONE` is not set to `true`.

## Features

### 1. Remote Tagger gRPC Server
Provides a minimal tagger gRPC server that allows other agents (process-agent, trace-agent, etc.) to stream entity tags.

**Key capabilities:**
- Streams entity changes to connected clients
- Fetches individual entities with tags
- Supports TLS and authentication via IPC component
- Configurable message sizes and concurrent sync limits

### 2. Kubernetes Tag Enrichment (Standalone Mode)
In standalone mode, the otel-agent runs a **local tagger** backed by workloadmeta collectors (containerd, docker, ECS, crio, podman, plus one collector for pod/K8s metadata). The `infraattributes` processor uses this local tagger to enrich spans, metrics, and logs with K8s entity tags — `kube_deployment`, `kube_namespace`, `pod_name`, `kube_replica_set`, etc. — without a core Datadog Agent on the same host.

The pod/K8s-metadata source defaults to the **`nodefilter`** collector: a workloadmeta collector that watches pods directly from the K8s API server, scoped to the local node via a `spec.nodeName` field selector. This avoids granting the broader kubelet-API RBAC that the `kubelet` collector requires, at the cost of a watch on the API server per node.
Set `use_kubelet_collector: true` in the extension config to opt in to the **`kubelet`** collector instead, which lists pods from the local kubelet API. Opting in requires more privileges: the ServiceAccount needs `get` on `nodes/proxy`, and the otel-agent container needs the kubelet's address and TLS settings (see [Using kubelet](#using-kubelet)).

#### Using the k8s API (default)

`nodefilter` reads the local node's name from the first of a list of environment variables that is set, extending the `k8sattributesprocessor`'s own `node_from_env_var` filter config, which takes a single name. The default list is `K8S_NODE_NAME,DD_KUBERNETES_KUBELET_NODENAME,OTEL_K8S_NODE_NAME`: `K8S_NODE_NAME` is only populated by the OTel Helm chart with some presets (e.g. `kubernetesAttributes` in daemonset mode), `DD_KUBERNETES_KUBELET_NODENAME` by the Datadog Helm chart/Operator, and `OTEL_K8S_NODE_NAME` is always populated by the OTel Helm chart. Set `node_from_env_var` in the extension config, as a comma-separated list or a single name, to read it from other env vars if necessary.

**Required deployment configuration, if not already set by Helm/Operator**
```yaml
env:
  - name: K8S_NODE_NAME
    valueFrom:
      fieldRef:
        fieldPath: spec.nodeName
```

**Required RBAC:** the ServiceAccount needs `get`/`list`/`watch` on `pods`:
```yaml
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list", "watch"]
```

#### Using kubelet

**Opt-in configuration**
```yaml
extensions:
  dogtel:
    use_kubelet_collector: true
```

**Required deployment configuration**
```yaml
env:
  - name: DD_KUBERNETES_KUBELET_HOST
    valueFrom:
      fieldRef:
        fieldPath: status.hostIP
  - name: DD_KUBELET_TLS_VERIFY
    value: "false"   # or configure a CA cert
```

**Required RBAC:** the ServiceAccount needs `get` on `nodes/proxy` so the kubelet collector can list pods:
```yaml
- apiGroups: [""]
  resources: ["nodes/proxy"]
  verbs: ["get"]
```

#### Hostname

When no hostname is configured and no cloud provider (GCE, Azure) gives one, the hostname is `<node name>-<cluster name>`, or the bare node name when no cluster name is found. In standalone mode, the node name is read from the same `node_from_env_var` environment variables as above. So the hostname works with what a deployment set up for the upstream OpenTelemetry Collector already provides: neither kubelet API access, which RBAC meant for its `k8sattributesprocessor` doesn't grant, nor the Datadog-specific kubelet settings (`DD_KUBERNETES_KUBELET_HOST`, `DD_KUBELET_TLS_VERIFY`), which the OpenTelemetry Helm charts don't set. It falls back to the kubelet only when `use_kubelet_collector` is `true`.

The cluster name comes from `DD_CLUSTER_NAME`, then from the cloud provider's API (GKE, AKS, EKS), then from the node's labels (only through the Cluster Agent, when `cluster_agent.enabled` is set, and never when `use_kubelet_collector` is `false`). Failing those, it is the `k8s.cluster.name` resource attribute, lowercased and with its `_` replaced by `-`. It is read from `service.telemetry.resource` in the collector config first, in either of its forms (the collector doesn't accept both at once):
```yaml
service:
  telemetry:
    resource:
      k8s.cluster.name: my-cluster
```
```yaml
service:
  telemetry:
    resource:
      attributes:
        - name: k8s.cluster.name
          value: my-cluster
```
and otherwise from `OTEL_RESOURCE_ATTRIBUTES`, which the `opentelemetry-kube-stack` chart populates from its `clusterName` value (`unknown_k8s_cluster` by default):
```yaml
env:
  - name: OTEL_RESOURCE_ATTRIBUTES
    value: k8s.cluster.name=my-cluster
```

To take precedence over the cloud provider's API, or to read it from another environment variable, set `DD_CLUSTER_NAME` instead, possibly referencing that variable:
```yaml
env:
  - name: DD_CLUSTER_NAME
    value: "$(MY_CLUSTER_NAME)"
```

### 3. Workload Detection Integration
Ensures workload metadata (via `workloadmeta.Component`) is accessible to the extension and other components.

**Note:** Workload detection itself is provided by the `workloadmetafx.Module()` already present in otel-agent.

### 4. Secrets Resolution (Conditional)
Supports secrets resolution when running in standalone mode (`DD_OTEL_STANDALONE=true`).

**Note:** Secrets are configured at the otel-agent level, not within the extension itself. See configuration section below.

### 5. Host Aliases Submission
The extension sends a host metadata payload (`/intake/`) with the otel-agent's hostname and its host aliases, computed as the core Agent computes them: the cloud provider's instance identifiers (e.g. the EC2 instance ID), the `host_aliases` setting, and `<node>-<cluster>` on Kubernetes. This ties the otel-agent's own hostname to the other names telemetry carries for the same host. For example, on EKS Auto Mode nodes, whose node name is the instance ID, the otel-agent names its host `<instance ID>-<cluster>` like the core Agent does, while the OTLP mapping names the host of telemetry with an EC2 `host.id` after the instance ID alone. Without the alias, they would be two hosts.

The payload carries nothing else: no system information, Agent version, or host tags. The backend keeps one alias set per host and replaces it with each host metadata payload, and it overwrites each metadata field with the latest value it receives, whichever agent sent it. Restricting the payload to the hostname and aliases, which every otel-agent on a node computes the same way, keeps several otel-agents on one node (e.g. a daemonset pod and a cluster or gateway collector) from overwriting each other.

- **Schedule:** like the core Agent's host metadata: at startup, then 5, 15, and every 30 minutes, adjustable with `metadata_interval` or the `host` entry of `metadata_providers`.
- **Core Agent on the same Kubernetes node:** the extension sends nothing while workloadmeta lists a running pod on the node with the `agent.datadoghq.com/component: agent` label, which the Datadog Helm chart and Operator set on node Agent pods. That Agent sends the host's aliases with its full host metadata, and a second sender would make the host's alias set flip between the two payloads. On Kubernetes, the extension also waits for workloadmeta to list the node's pods before its first payload.
- **Nothing to tie:** the extension sends nothing when no alias differs from the hostname, e.g. outside any cloud provider, or when the hostname is the instance ID itself.
- Disabled by `enable_metadata_collection: false`, in the extension config or in datadog.yaml (`DD_ENABLE_METADATA_COLLECTION=false`).

## Configuration

The extension is configured via the OpenTelemetry Collector configuration file.

**⚠️ Only include this extension when DD_OTEL_STANDALONE=true**

```yaml
# Datadog Agent config (datadog.yaml or via DD_OTEL_STANDALONE env var)
# REQUIRED: Set this to true to enable dogtelextension
otel_standalone: true

# OTel Collector config (otel-config.yaml)
extensions:
  dogtel:
    # Tagger server settings
    enable_tagger_server: true        # Enable tagger gRPC server (default: false)
    tagger_server_port: 5000          # Port to listen on, 0 = auto-assign (default: 0)
    tagger_server_addr: "localhost"   # Address to bind to (default: localhost)
    tagger_max_message_size: 4194304  # Max gRPC message size in bytes (default: 4MB)
    tagger_max_concurrent_sync: 5     # Max concurrent sync connections (default: 5)

    # Host aliases submission (see "Host Aliases Submission")
    enable_metadata_collection: true  # Sets datadog.yaml's enable_metadata_collection (default: true)
    metadata_interval: 1800           # Steady-state interval in seconds (default: the Agent's 1800)

    # K8s tag enrichment collector selection (default: nodefilter)
    use_kubelet_collector: false      # Set to true to opt in to the kubelet collector (default: false)
    # Comma-separated env vars nodefilter reads the local node's name from, the first one set winning
    # (default: K8S_NODE_NAME,DD_KUBERNETES_KUBELET_NODENAME,OTEL_K8S_NODE_NAME)
    node_from_env_var: K8S_NODE_NAME,DD_KUBERNETES_KUBELET_NODENAME,OTEL_K8S_NODE_NAME

service:
  extensions: [dogtel]  # Only include when DD_OTEL_STANDALONE=true
```

### Environment Variables

- **`DD_OTEL_STANDALONE`** (REQUIRED): Must be set to `true` to enable this extension
  - Enables standalone mode with full secrets resolution
  - Enables host aliases submission
  - Activates dogtelextension functionalities

## Deployment Modes

### Connected Mode (Default: `DD_OTEL_STANDALONE=false`)
- **dogtelextension:** ❌ **DO NOT enable** - core agent provides these functionalities
- **Secrets:** Uses no-op secrets (expects core agent for secrets)
- **Tagger:** Uses remote tagger client to connect to core agent
- **Metadata:** Handled by core agent
- **Use case:** otel-agent running alongside core Datadog agent

### Standalone Mode (`DD_OTEL_STANDALONE=true`)
- **dogtelextension:** ✅ **ENABLE THIS EXTENSION**
- **Secrets:** Full secrets resolution enabled
- **Tagger:** Runs tagger server for other agents
- **Metadata:** The extension sends the host aliases, unless a core Agent runs on the same Kubernetes node
- **Workload detection:** Provided by workloadmeta component
- **Use case:** otel-agent running independently without core agent

## Architecture

### Extension Factory Pattern
The extension uses the factory pattern to integrate with OpenTelemetry Collector:

- `NewFactory()`: Basic factory for standalone OTel builds (returns error)
- `NewFactoryForAgent()`: Factory with FX component injection for otel-agent

FX-injected components:
- `config.Component`: Core configuration
- `log.Component`: Logging
- `serializer.MetricSerializer`: Metrics serialization
- `hostnameinterface.Component`: Hostname resolution
- `workloadmeta.Component`: Workload metadata
- `tagger.Component`: Tagging service
- `ipc.Component`: IPC/authentication
- `telemetry.Component`: Telemetry
- `secrets.Component`: Secrets resolution

### Tagger Server Implementation
The tagger server is a minimal implementation of `pb.AgentSecureServer` that:
1. Wraps `comp/core/tagger/server.Server`
2. Implements only tagger methods (`TaggerStreamEntities`, `TaggerFetchEntity`)
3. Uses IPC component for authentication and TLS
4. Starts in a goroutine during `Start()`
5. Gracefully stops during `Shutdown()`

## Integration

### Registering the Extension
In `comp/otelcol/collector/impl/collector.go`:

```go
factories.Extensions[dogtelextension.Type] = dogtelextension.NewFactoryForAgent(
    reqs.Config,
    reqs.Log,
    reqs.Serializer,
    reqs.Hostname,
    reqs.Workloadmeta,
    reqs.Tagger,
    reqs.Ipc,
    reqs.Telemetry,
    reqs.Secrets,
)
```

### Conditional Secrets
In `cmd/otel-agent/subcommands/run/command.go`:

```go
// Determine standalone mode
standaloneMode := cfg.GetBool("otel_standalone") // From DD_OTEL_STANDALONE

// Choose secrets module
var secretsModule fx.Option
if standaloneMode {
    secretsModule = secretsfx.Module()  // Real secrets
} else {
    secretsModule = secretsnoopfx.Module()  // No-op
}

// Use in FX options
fxutil.Run(
    secretsModule,
    // ... other modules ...
)
```

## Development

### Building
The extension is automatically included when building otel-agent with the `otlp` build tag:

```bash
dda inv otel-agent.build
```

### Testing
```bash
# Unit tests
go test ./comp/otelcol/dogtelextension/...

# Integration test with otel-agent
./bin/otel-agent/otel-agent run --config test-config.yaml
```

### Example Standalone Mode Configuration

**Datadog Agent config (datadog.yaml):**
```yaml
api_key: ${DD_API_KEY}
hostname: my-host
otel_standalone: true  # REQUIRED for dogtelextension
enable_metadata_collection: true
```

**OTel Collector config (otel-config.yaml):**
```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: localhost:4317

exporters:
  datadog:
    api:
      key: ${env:DD_API_KEY}

extensions:
  dogtel:  # Only include in standalone mode
    enable_tagger_server: true
    tagger_server_port: 5000
    enable_metadata_collection: true

service:
  extensions: [dogtel]
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [datadog]
```

**Run command:**
```bash
DD_OTEL_STANDALONE=true ./bin/otel-agent/otel-agent \
  --core-config=datadog.yaml \
  --config=file:otel-config.yaml
```

## Limitations

### Phase 1 (Implemented)
- ✅ Tagger gRPC server
- ✅ K8s tag enrichment via local tagger (nodefilter, or kubelet as an opt-in, + container runtime collectors)
- ✅ Workload detection integration
- ✅ Conditional secrets (via otel-agent config)
- ✅ Host aliases submission
- ❌ Full host metadata and inventories (system information, Agent version, host tags)
- ❌ System metrics collection (deferred to Phase 2)

### Future Enhancements (Phase 2)
- System checks integration (CPU, memory, disk, load, uptime, io)
- Collector component with demultiplexer
- Check scheduler

## Troubleshooting

### Extension Not Starting / Disabled
**Symptom:** Logs show "dogtelextension disabled (not in standalone mode)"

**Solution:**
- Set `DD_OTEL_STANDALONE=true` environment variable
- Or set `otel_standalone: true` in datadog.yaml
- The extension requires standalone mode to function
- In connected mode (with core agent), remove this extension from your OTel config

### Tagger Server Not Starting
- **First:** Ensure `DD_OTEL_STANDALONE=true` is set
- Check that `enable_tagger_server: true` in extension config
- Verify port is not in use: `lsof -i :<port>`
- Check logs for TLS/authentication errors
- Ensure IPC component is properly initialized

### Host Aliases Not Sent
- **First:** Ensure `DD_OTEL_STANDALONE=true` is set
- Host aliases are only sent in standalone mode, and not when `enable_metadata_collection` is false
- Check the logs for "Not sending host aliases" (the reason follows) or "Unable to send host aliases"
- With debug logs, "Sending host aliases" lists the aliases sent

### Secrets Not Resolving
- **First:** Ensure `DD_OTEL_STANDALONE=true` is set
- Secrets resolution requires standalone mode
- Verify secrets backend is configured in datadog.yaml
- Check logs for secrets component errors
- In connected mode, secrets resolution is intentionally disabled (no-op)

## References

- [OTel Collector Extension Development](https://opentelemetry.io/docs/collector/extend/custom-component/extension/)
- [Datadog Agent Architecture](https://datadoghq.dev/datadog-agent/architecture/)
- [Tagger Component](../../core/tagger/README.md)
- [Workload Metadata](../../core/workloadmeta/README.md)
