# MCP (Model Context Protocol) Server for Datadog Agent

This package implements an MCP server for the Datadog Agent, enabling external systems to query agent telemetry data through a standardized tool interface based on the [Model Context Protocol specification](https://modelcontextprotocol.io/docs).

## Overview

The MCP server provides a plugin-based architecture for exposing agent capabilities as MCP tools. Tools can be registered dynamically and configured through the agent's configuration system. The implementation uses JSON-RPC 2.0 over various transports (Unix socket, stdio, TCP).

### Key Features

- **Standardized Protocol**: Full MCP protocol support with JSON-RPC 2.0
- **Multiple Transports**: Unix domain sockets (recommended), stdio, and TCP
- **Extensible Tools**: Plugin architecture for adding new tools
- **Security**: Command-line argument scrubbing, TLS support, connection limits
- **Performance**: Efficient process data collection with configurable limits

## Architecture

```
┌─────────────────┐     JSON-RPC 2.0     ┌──────────────────┐
│  MCP Client     │◄───────────────────►│  Transport       │
│  (Claude, etc.) │     (Unix/TCP/stdio) │  (Unix/TCP/stdio)│
└─────────────────┘                      └────────┬─────────┘
                                                  │
                                                  ▼
                                         ┌──────────────────┐
                                         │  JSON-RPC        │
                                         │  Handler         │
                                         └────────┬─────────┘
                                                  │
                                                  ▼
                                         ┌──────────────────┐
                                         │  MCP Server      │
                                         │  (Tool Registry) │
                                         └────────┬─────────┘
                                                  │
                                                  ▼
                                         ┌──────────────────┐
                                         │  Tool Handlers   │
                                         │  (Process, etc.) │
                                         └──────────────────┘
```

### Package Structure

```
pkg/mcp/
├── server/          # Core MCP server implementation
│   ├── server.go    # Main server logic with tool registration
│   ├── handler.go   # Tool handler interface definitions
│   └── config.go    # Configuration handling
├── tools/           # Tool implementations
│   ├── registry.go  # Tool registry for managing handlers
│   └── process/     # Process monitoring tool
│       ├── handler.go  # Process snapshot handler
│       └── types.go    # Request/Response types
├── transport/       # Transport layer implementations
│   ├── transport.go # Transport interface definitions
│   ├── jsonrpc.go   # JSON-RPC 2.0 protocol handler
│   ├── unix.go      # Unix domain socket transport
│   └── stdio.go     # Standard I/O transport
└── types/           # Shared type definitions
    └── types.go     # Core request/response types
```

## Configuration

### Basic Configuration

Add to `datadog.yaml`:

```yaml
# MCP Server Configuration
mcp:
  # Enable/disable MCP server
  enabled: true
  
  # Server configuration
  server:
    # Address to listen on (Unix socket recommended for security)
    address: "unix:///var/run/datadog/mcp.sock"
    
    # Request limits
    max_request_size: 10485760  # 10MB
    request_timeout: 30s
    
    # Connection limits
    max_connections: 100
    
  # Tool-specific configuration
  tools:
    # Process monitoring tool
    process:
      enabled: true
      
      # Data scrubbing (recommended)
      scrub_args: true
      
      # Performance limits
      max_processes_per_request: 1000
      
      # Include additional metadata
      include_container_metadata: true
```

### TLS Configuration (for TCP transport)

```yaml
mcp:
  enabled: true
  server:
    address: "localhost:7890"
    tls:
      enabled: true
      cert_file: "/etc/datadog-agent/certs/mcp-cert.pem"
      key_file: "/etc/datadog-agent/certs/mcp-key.pem"
      ca_file: "/etc/datadog-agent/certs/mcp-ca.pem"
```

### Environment Variables

All configuration options can be set via environment variables with the `DD_` prefix:

```bash
DD_MCP_ENABLED=true
DD_MCP_SERVER_ADDRESS="unix:///var/run/datadog/mcp.sock"
DD_MCP_TOOLS_PROCESS_ENABLED=true
DD_MCP_TOOLS_PROCESS_SCRUB_ARGS=true
DD_MCP_TOOLS_PROCESS_MAX_PROCESSES_PER_REQUEST=500
```

## Available Tools

### GetProcessSnapshot

Retrieves a point-in-time snapshot of running processes with optional filtering, sorting, and enrichment.

#### Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `pids` | `[]int32` | `[]` | Filter by specific process IDs |
| `process_names` | `[]string` | `[]` | Filter by exact process names |
| `regex_filter` | `string` | `""` | Regex pattern for filtering process names or command lines |
| `include_stats` | `bool` | `true` | Include CPU/memory statistics |
| `include_io` | `bool` | `false` | Include I/O statistics |
| `include_net` | `bool` | `false` | Include network information (requires system-probe) |
| `limit` | `int` | `100` | Maximum number of processes to return |
| `sort_by` | `string` | `""` | Sort field: `cpu`, `memory`, `pid`, `name` |
| `ascending` | `bool` | `false` | Sort in ascending order |

#### Response

```json
{
  "timestamp": 1699459200,
  "timestamp_str": "2024-11-08T12:00:00Z",
  "host_info": {
    "hostname": "my-host",
    "os": "linux",
    "platform": "ubuntu",
    "kernel_version": "5.15.0",
    "cpu_cores": 8,
    "total_memory": 17179869184
  },
  "processes": [
    {
      "pid": 1234,
      "ppid": 1,
      "name": "python",
      "executable": "/usr/bin/python3",
      "command_line": ["python3", "app.py"],
      "username": "app",
      "uid": 1000,
      "gid": 1000,
      "create_time": 1699459000,
      "status": "S",
      "cpu_percent": 2.5,
      "cpu_time": {
        "user": 125.5,
        "system": 45.2
      },
      "memory": {
        "rss": 104857600,
        "vms": 209715200,
        "percent": 0.61
      },
      "io": {
        "read_bytes": 1048576,
        "write_bytes": 524288
      },
      "open_files": 25,
      "num_threads": 4,
      "tcp_ports": [8080, 8443],
      "service": {
        "name": "my-app",
        "language": "python"
      },
      "apm_injected": true
    }
  ],
  "total_count": 150,
  "filtered_count": 10
}
```

#### Example Usage

**Get top 10 processes by CPU usage:**
```json
{
  "include_stats": true,
  "limit": 10,
  "sort_by": "cpu",
  "ascending": false
}
```

**Find all Python processes:**
```json
{
  "regex_filter": "^python.*",
  "include_stats": true
}
```

**Get specific processes by PID:**
```json
{
  "pids": [1234, 5678, 9012]
}
```

## MCP Protocol

The server implements the [Model Context Protocol](https://modelcontextprotocol.io/docs) specification using JSON-RPC 2.0.

### Supported Methods

| Method | Description |
|--------|-------------|
| `initialize` | Initialize the MCP session |
| `notifications/initialized` | Client acknowledgment after initialization |
| `ping` | Health check |
| `tools/list` | List available tools |
| `tools/call` | Execute a tool |
| `resources/list` | List available resources (currently empty) |
| `prompts/list` | List available prompts (currently empty) |

### Example Session

```json
// Client → Server: Initialize
{"jsonrpc": "2.0", "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "my-client", "version": "1.0.0"}}, "id": 1}

// Server → Client: Initialize response
{"jsonrpc": "2.0", "result": {"protocolVersion": "2024-11-05", "capabilities": {"tools": {}}, "serverInfo": {"name": "datadog-agent-mcp", "version": "7.x.x"}}, "id": 1}

// Client → Server: Initialized notification
{"jsonrpc": "2.0", "method": "notifications/initialized"}

// Client → Server: List tools
{"jsonrpc": "2.0", "method": "tools/list", "id": 2}

// Server → Client: Tools list
{"jsonrpc": "2.0", "result": {"tools": [{"name": "GetProcessSnapshot", "description": "Get snapshot of running processes", "inputSchema": {...}}]}, "id": 2}

// Client → Server: Call tool
{"jsonrpc": "2.0", "method": "tools/call", "params": {"name": "GetProcessSnapshot", "arguments": {"limit": 10, "sort_by": "cpu"}}, "id": 3}

// Server → Client: Tool result
{"jsonrpc": "2.0", "result": {"content": [{"type": "text", "text": "{...}"}]}, "id": 3}
```

## Security Considerations

### Data Privacy

1. **Command-Line Scrubbing**: By default, sensitive command-line arguments are scrubbed (passwords, API keys, etc.). This is controlled by `mcp.tools.process.scrub_args`.

2. **Access Control**: 
   - Unix sockets: Control access via file permissions (default: 0600)
   - TCP: Use TLS with client certificate authentication
   - Consider network policies to restrict access

3. **Audit Logging**: All MCP requests are logged at debug level. Enable debug logging for audit trails.

### Network Security

1. **Unix Sockets (Recommended)**: Use Unix domain sockets for local-only access:
   ```yaml
   mcp:
     server:
       address: "unix:///var/run/datadog/mcp.sock"
   ```

2. **TLS for TCP**: Always enable TLS when using TCP transport:
   ```yaml
   mcp:
     server:
       address: "localhost:7890"
       tls:
         enabled: true
         # ... certificate configuration
   ```

3. **Connection Limits**: Configure appropriate connection limits to prevent resource exhaustion:
   ```yaml
   mcp:
     server:
       max_connections: 100
       max_request_size: 10485760  # 10MB
       request_timeout: 30s
   ```

### Resource Protection

1. **Request Limits**: Configure `max_processes_per_request` to prevent expensive queries
2. **Timeouts**: Set appropriate `request_timeout` values
3. **Rate Limiting**: Connection limits help prevent DoS attacks

## Troubleshooting

### Common Issues

#### MCP server not starting

1. Check if MCP is enabled:
   ```bash
   grep -A 5 "mcp:" /etc/datadog-agent/datadog.yaml
   ```

2. Verify socket path permissions:
   ```bash
   ls -la /var/run/datadog/
   ```

3. Check agent logs:
   ```bash
   tail -f /var/log/datadog/agent.log | grep -i mcp
   ```

#### Connection refused

1. Verify the socket exists:
   ```bash
   ls -la /var/run/datadog/mcp.sock
   ```

2. Check socket permissions match your client's user/group

3. For TCP, verify the port is open:
   ```bash
   netstat -tlnp | grep 7890
   ```

#### Tool execution errors

1. Enable debug logging:
   ```yaml
   log_level: debug
   ```

2. Check for permission issues (process reading requires appropriate privileges)

3. Verify the process agent is running if process data is empty

### Debug Mode

Enable verbose logging for MCP:

```yaml
log_level: debug
```

Or via environment:
```bash
DD_LOG_LEVEL=debug
```

### Health Check

Test the MCP server with a simple ping:

```bash
echo '{"jsonrpc":"2.0","method":"ping","id":1}' | nc -U /var/run/datadog/mcp.sock
```

Expected response:
```json
{"jsonrpc":"2.0","result":{},"id":1}
```

## Performance

### Benchmarks

Typical performance characteristics (8-core, 16GB RAM system):

| Operation | Latency (p50) | Latency (p99) |
|-----------|---------------|---------------|
| Process snapshot (100 procs) | ~5ms | ~15ms |
| Process snapshot (1000 procs) | ~25ms | ~50ms |
| Filtered snapshot (regex) | ~10ms | ~30ms |

### Optimization Tips

1. **Limit Results**: Always use `limit` parameter to restrict result size
2. **Filter Early**: Use `pids` or `process_names` instead of `regex_filter` when possible
3. **Disable Unused Stats**: Set `include_io: false` if I/O stats aren't needed
4. **Connection Pooling**: Reuse MCP connections instead of creating new ones

## Extending MCP

### Creating a New Tool

1. Create a new handler in `pkg/mcp/tools/`:

```go
package mytool

import (
    "context"
    "github.com/DataDog/datadog-agent/pkg/mcp/types"
)

type MyToolHandler struct {
    // dependencies
}

func NewMyToolHandler() *MyToolHandler {
    return &MyToolHandler{}
}

func (h *MyToolHandler) Handle(ctx context.Context, req *types.ToolRequest) (*types.ToolResponse, error) {
    // Parse parameters from req.Parameters
    // Execute tool logic
    // Return response
    return &types.ToolResponse{
        ToolName:  req.ToolName,
        Result:    myResult,
        RequestID: req.RequestID,
    }, nil
}
```

2. Register the tool with the server:

```go
server.RegisterTool("MyTool", mytool.NewMyToolHandler())
```

### Tool Input Schema

Define JSON Schema for tool parameters:

```go
var MyToolSchema = map[string]interface{}{
    "type": "object",
    "properties": map[string]interface{}{
        "param1": map[string]interface{}{
            "type": "string",
            "description": "First parameter",
        },
        "param2": map[string]interface{}{
            "type": "integer",
            "default": 10,
        },
    },
    "required": []string{"param1"},
}
```

## API Reference

See the [GoDoc documentation](https://pkg.go.dev/github.com/DataDog/datadog-agent/pkg/mcp) for complete API details.

## License

This project is licensed under the Apache License Version 2.0 - see the LICENSE file for details.
