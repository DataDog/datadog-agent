# MCP Client Examples

This directory contains example clients that demonstrate how to connect to and interact with the Datadog Agent's MCP (Model Context Protocol) server.

## Available Examples

### Python Client (`python/`)

A simple Python client that demonstrates:
- Connecting to Unix socket
- Sending MCP protocol messages (JSON-RPC 2.0)
- Calling the GetProcessSnapshot tool
- Handling responses and errors

**Requirements:**
- Python 3.7+
- No external dependencies (uses stdlib only)

**Usage:**
```bash
cd python
python mcp_client.py
```

### Go Client (`go/`)

A Go client that demonstrates:
- Connecting to Unix socket or TCP
- Full MCP protocol handshake
- Tool discovery and execution
- Error handling patterns

**Requirements:**
- Go 1.21+

**Usage:**
```bash
cd go
go run client.go
```

## Quick Start

1. Enable the MCP server in the Datadog Agent configuration:

```yaml
# /etc/datadog-agent/datadog.yaml
mcp:
  enabled: true
  server:
    address: "unix:///var/run/datadog/mcp.sock"
  tools:
    process:
      enabled: true
```

2. Restart the Datadog Agent

3. Run one of the example clients:

```bash
# Python
python python/mcp_client.py

# Go
go run go/client.go
```

## Protocol Overview

The MCP server uses JSON-RPC 2.0 over newline-delimited JSON. Messages are text-based and human-readable.

### Connection Flow

```
1. Client connects to socket
2. Client sends: initialize request
3. Server responds: initialize result (capabilities)
4. Client sends: initialized notification
5. Client can now call tools
```

### Message Format

**Request:**
```json
{"jsonrpc":"2.0","method":"tools/call","params":{"name":"GetProcessSnapshot","arguments":{"limit":10}},"id":1}
```

**Response:**
```json
{"jsonrpc":"2.0","result":{"content":[{"type":"text","text":"..."}]},"id":1}
```

## Transport Options

### Unix Socket (Recommended)

Best for local access with good security:
```
unix:///var/run/datadog/mcp.sock
```

### TCP

For network access (use with TLS in production):
```
localhost:7890
```

### stdio

For process-based integration (pipes):
```
agent mcp-server --transport=stdio
```

## Security Notes

- Unix sockets are protected by file permissions (default 0600)
- TCP connections should use TLS in production
- Always scrub sensitive data from process arguments (enabled by default)
- Consider network policies for TCP transport

## Troubleshooting

### Permission Denied
Check socket permissions and ensure your user has access:
```bash
ls -la /var/run/datadog/mcp.sock
```

### Connection Refused
Ensure the MCP server is enabled and running:
```bash
grep -A5 "mcp:" /etc/datadog-agent/datadog.yaml
```

### Invalid Response
Make sure messages end with a newline character (`\n`).

## Additional Resources

- [MCP Server Documentation](../README.md)
- [Model Context Protocol Specification](https://modelcontextprotocol.io/docs)
- [JSON-RPC 2.0 Specification](https://www.jsonrpc.org/specification)
