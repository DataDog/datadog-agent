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
In standalone mode, the otel-agent runs a **local tagger** backed by workloadmeta collectors (kubelet, containerd, docker, ECS, crio, podman). The `infraattributes` processor uses this local tagger to enrich spans, metrics, and logs with K8s entity tags — `kube_deployment`, `kube_namespace`, `pod_name`, `kube_replica_set`, etc. — without a core Datadog Agent on the same host.

**Required deployment configuration:**
```yaml
env:
  - name: DD_KUBERNETES_KUBELET_HOST
    valueFrom:
      fieldRef:
        fieldPath: status.hostIP
  - name: DD_KUBELET_TLS_VERIFY
    value: "false"   # or configure a CA cert
```

**Required RBAC:** The ServiceAccount needs `get` on `nodes/proxy` so the kubelet collector can list pods:
```yaml
- apiGroups: [""]
  resources: ["nodes/proxy"]
  verbs: ["get"]
```

### 3. Workload Detection Integration
Ensures workload metadata (via `workloadmeta.Component`) is accessible to the extension and other components.

**Note:** Workload detection itself is provided by the `workloadmetafx.Module()` already present in otel-agent.

### 4. Secrets Resolution (Conditional)
Supports secrets resolution when running in standalone mode (`DD_OTEL_STANDALONE=true`).

**Note:** Secrets are configured at the otel-agent level, not within the extension itself. See configuration section below.

### 5. Host Metadata Submission
**Status:** ✅ Implemented via FX modules in otel-agent.

Host metadata collection is enabled through FX modules added to otel-agent startup in [cmd/otel-agent/subcommands/run/command.go](../../cmd/otel-agent/subcommands/run/command.go):

```go
runnerimpl.Module(),      // Metadata scheduler and submission
hostimpl.Module(),         // Host metadata (V5 payload)
inventoryhostimpl.Module(), // Inventory host metadata
```

This provides:
- Host metadata (OS, hostname, cloud provider info, agent version)
- Inventory metadata (installed packages, system configuration)
- Scheduled submission to Datadog API (every 5 minutes by default)
- Controlled by `enable_metadata_collection` config in datadog.yaml

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

    # Metadata collection (via FX modules in otel-agent)
    enable_metadata_collection: true  # Informational flag (default: true)
    metadata_interval: 300            # Interval in seconds (default: 300)

service:
  extensions: [dogtel]  # Only include when DD_OTEL_STANDALONE=true
```

### Environment Variables

- **`DD_OTEL_STANDALONE`** (REQUIRED): Must be set to `true` to enable this extension
  - Enables standalone mode with full secrets resolution
  - Enables metadata collection via FX modules
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
- **Metadata:** Collected via FX modules (runner, host, inventoryhost)
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
- ✅ K8s tag enrichment via local tagger (kubelet + container runtime collectors)
- ✅ Workload detection integration
- ✅ Conditional secrets (via otel-agent config)
- ✅ Host metadata submission (via FX modules in otel-agent)
- ❌ System metrics collection (deferred to Phase 2)

### Future Enhancements (Phase 2)
- System checks integration (CPU, memory, disk, load, uptime, io)
- Collector component with demultiplexer
- Check scheduler
- Direct metadata collection within extension (alternative to current FX-based approach)

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

### Metadata Collection Not Working
- **First:** Ensure `DD_OTEL_STANDALONE=true` is set
- Metadata collection only runs in standalone mode
- Verify metadata modules are loaded in otel-agent startup (check for conditional FX modules)
- Check logs for "Starting metadata runner" or metadata-related errors
- Verify `enable_metadata_collection: true` in datadog.yaml
- Check that serializer and hostname components are available

### Secrets Not Resolving
- **First:** Ensure `DD_OTEL_STANDALONE=true` is set
- Secrets resolution requires standalone mode
- Verify secrets backend is configured in datadog.yaml
- Check logs for secrets component errors
- In connected mode, secrets resolution is intentionally disabled (no-op)

## References

- [OTel Collector Extension Development](https://opentelemetry.io/docs/collector/building/extension/)
- [Datadog Agent Architecture](../../docs/dev/README.md)
- [Tagger Component](../../comp/core/tagger/README.md)
- [Workload Metadata](../../comp/core/workloadmeta/README.md)
