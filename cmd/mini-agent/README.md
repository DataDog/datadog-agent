# Mini-Agent

A lightweight standalone Datadog Agent binary that provides essential agent functionalities including tagger gRPC server and workloadmeta streaming.

## Overview

The `mini-agent` is a standalone binary designed to run independently, providing core agent services like entity tagging and workload metadata streaming. It's built on top of proven Datadog Agent components and uses the standard Agent configuration.

### Features

✅ **Tagger gRPC Server** - Streams entity tags to remote clients using `comp/core/tagger/server`
✅ **Workloadmeta Streaming** - Streams workload metadata using `comp/core/workloadmeta/server`
✅ **Metric Submission** - Periodic submission of `mini_agent.running` heartbeat metric
✅ **Agent Config** - Uses standard `datadog.yaml` configuration
✅ **Component-Based** - Built with FX dependency injection and reusable components

## Building

```bash
go build -o bin/mini-agent ./cmd/mini-agent
```

Or using the invoke tasks:
```bash
dda inv agent.build --build-exclude=systemd
```

## Running

```bash
# With config file
./bin/mini-agent run -c /path/to/datadog.yaml

# With default config locations
./bin/mini-agent run

# Show version
./bin/mini-agent version
```

## Configuration

Configuration is via `datadog.yaml` (standard Datadog Agent config):

```yaml
# API key and basic config
api_key: ${DD_API_KEY}
hostname: my-host
site: datadoghq.com

# Mini-agent specific settings
mini_agent:
  # gRPC server for tagger and workloadmeta
  server:
    enabled: true              # Enable gRPC server (default: false)
    address: localhost         # Bind address (default: localhost)
    port: 5002                 # Listen port (default: 5002)
    max_message_size: 52428800 # 50MB max message size
    max_concurrent_sync: 4     # Max concurrent tagger sync clients

  # Metric submission
  submit_running_metric: true  # Enable periodic submission of mini_agent.running metric (default: true)
  metadata_interval: 60        # Heartbeat metric interval in seconds (default: 60)
```

### Environment Variables

All standard Datadog Agent environment variables are supported:
- `DD_API_KEY` - Datadog API key
- `DD_HOSTNAME` - Override hostname
- `DD_SITE` - Datadog site (datadoghq.com, datadoghq.eu, etc.)

## Architecture

The mini-agent uses FX for dependency injection and reuses existing Agent components:

```
cmd/mini-agent/
├── main.go                    # Entry point
├── command/
│   └── command.go            # Root command setup
└── subcommands/
    ├── subcommands.go        # Global parameters
    └── run/
        ├── command.go        # Run command with FX setup
        └── agent.go          # Mini-agent implementation
```

### Key Components

- **Tagger Server**: `comp/core/tagger/server` - Handles entity tag streaming
- **Workloadmeta Server**: `comp/core/workloadmeta/server` - Handles workload metadata streaming
- **Serializer**: `pkg/serializer` - Sends metrics to Datadog
- **Forwarder**: `comp/forwarder/defaultforwarder` - Forwards data to Datadog backend
- **IPC**: `comp/core/ipc` - Provides TLS and authentication for gRPC

## gRPC Services

The mini-agent exposes the `AgentSecure` gRPC service with the following methods:

### Tagger Methods
- `TaggerStreamEntities` - Subscribe to entity tag changes
- `TaggerFetchEntity` - Fetch tags for a specific entity
- `TaggerGenerateContainerIDFromOriginInfo` - Generate container ID from origin info

### Workloadmeta Methods
- `WorkloadmetaStreamEntities` - Subscribe to workload metadata changes

All gRPC endpoints support TLS and authentication via the IPC component.

## Use Cases

### Standalone Agent
Run mini-agent independently to provide tagger and workloadmeta services:
```bash
./bin/mini-agent run -c datadog.yaml
```

### Remote Tagger
Other agents can connect to mini-agent's tagger server for tag resolution:
```yaml
# In remote client agent config
cmd_port: 5002  # Connect to mini-agent's gRPC port
```

### Workload Metadata Provider
Provide workload metadata to remote collectors and agents.

## Troubleshooting

### Server Not Starting
- Check that `mini_agent.server.enabled: true` in config
- Verify port is not in use: `lsof -i :5002`
- Check logs for errors: `/var/log/datadog/mini-agent.log`

### No Metrics Being Sent
- Verify `api_key` is set correctly
- Check `DD_SITE` matches your Datadog organization
- Check forwarder logs for connection issues

### gRPC Connection Errors
- Ensure TLS certificates are properly configured
- Check IPC authentication token
- Verify firewall allows connections on the configured port

## Development

### Running Tests
```bash
# Run all tests
go test ./cmd/mini-agent/...

# Run with coverage
go test -cover ./cmd/mini-agent/...
```

### Linting
```bash
dda inv linter.go --targets=./cmd/mini-agent
```

## References

- [Tagger Component](../../comp/core/tagger/README.md)
- [Workloadmeta Component](../../comp/core/workloadmeta/README.md)
- [Datadog Agent Architecture](../../docs/dev/README.md)
- [gRPC Server Implementation](../../comp/core/tagger/server/README.md)
