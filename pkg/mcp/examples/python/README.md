# Python MCP Client Example

This directory contains a Python client example for connecting to the Datadog Agent's MCP server.

## Requirements

- Python 3.7 or later
- No external dependencies (uses Python standard library only)

## Usage

### Basic Usage

```bash
# Connect via default Unix socket (/var/run/datadog/mcp.sock)
python mcp_client.py

# Get top 20 processes by memory usage
python mcp_client.py --limit 20 --sort-by memory

# Connect via custom Unix socket
python mcp_client.py --socket /tmp/mcp.sock

# Connect via TCP
python mcp_client.py --tcp localhost:7890
```

### Command Line Options

```
usage: mcp_client.py [-h] [--socket SOCKET] [--tcp TCP] [--limit LIMIT]
                     [--sort-by {cpu,memory,pid,name}]

MCP Client Example for Datadog Agent

options:
  -h, --help            show this help message and exit
  --socket SOCKET       Unix socket path (default: /var/run/datadog/mcp.sock)
  --tcp TCP             TCP address in format host:port
  --limit LIMIT         Number of processes to retrieve (default: 10)
  --sort-by {cpu,memory,pid,name}
                        Sort processes by field (default: cpu)
```

## Example Output

```
============================================================
Connecting to MCP server...
Connected to Unix socket /var/run/datadog/mcp.sock

============================================================
Initializing MCP session...
Server info:
{
  "protocolVersion": "2024-11-05",
  "capabilities": {
    "tools": {}
  },
  "serverInfo": {
    "name": "datadog-agent-mcp",
    "version": "7.x.x"
  }
}

============================================================
Sending ping...
Ping result: {}

============================================================
Listing available tools...
Found 1 tool(s):
  - GetProcessSnapshot: Get a point-in-time snapshot of running processes

============================================================
Getting top 10 processes by cpu...

Host Information:
{
  "hostname": "my-host",
  "os": "linux",
  "platform": "ubuntu",
  "kernel_version": "5.15.0-91-generic",
  "cpu_cores": 8,
  "total_memory": 17179869184
}

Processes (10 returned, 245 total):
--------------------------------------------------------------------------------
     PID   CPU%   MEM%  NAME                 COMMAND
--------------------------------------------------------------------------------
    1234   15.2   2.45  python3              python3 /app/server.py
    5678    8.1   1.20  datadog-agent        /opt/datadog-agent/bin/agent
    9012    5.3   0.85  node                 node /app/index.js
    ...

============================================================
Done!
```

## Using as a Library

You can import the `MCPClient` class in your own Python scripts:

```python
from mcp_client import MCPClient

# Create and connect
client = MCPClient(socket_path="/var/run/datadog/mcp.sock")
client.connect()

# Initialize the session
client.initialize()

# Get process snapshot
snapshot = client.get_process_snapshot(
    limit=50,
    sort_by="memory",
    include_stats=True,
)

# Process the results
for proc in snapshot.get("processes", []):
    print(f"{proc['pid']}: {proc['name']} - {proc['cpu_percent']:.1f}% CPU")

# Clean up
client.close()
```

## API Reference

### MCPClient

#### Constructor

```python
MCPClient(socket_path=None, tcp_address=None)
```

- `socket_path`: Path to Unix domain socket (default: `/var/run/datadog/mcp.sock`)
- `tcp_address`: TCP address in format `host:port` (alternative to Unix socket)

#### Methods

##### `connect()`
Establish connection to the MCP server.

##### `close()`
Close the connection.

##### `initialize() -> dict`
Initialize the MCP session. Must be called before other operations.

##### `ping() -> dict`
Send a ping to check server health.

##### `list_tools() -> list`
List available tools with their descriptions and input schemas.

##### `call_tool(name, arguments=None) -> Any`
Call a tool by name with optional arguments.

##### `get_process_snapshot(...) -> dict`
Convenience method for calling the GetProcessSnapshot tool.

Parameters:
- `pids`: Filter by specific process IDs
- `process_names`: Filter by exact process names
- `regex_filter`: Regex pattern for filtering
- `include_stats`: Include CPU/memory statistics (default: True)
- `include_io`: Include I/O statistics (default: False)
- `include_net`: Include network information (default: False)
- `limit`: Maximum number of processes (default: 100)
- `sort_by`: Sort field (`cpu`, `memory`, `pid`, `name`)
- `ascending`: Sort in ascending order (default: False)

## Troubleshooting

### "Socket not found"

The MCP server is not running or the socket path is incorrect.

```bash
# Check if socket exists
ls -la /var/run/datadog/mcp.sock

# Check agent config
grep -A5 "mcp:" /etc/datadog-agent/datadog.yaml
```

### "Permission denied"

Your user doesn't have permission to access the socket.

```bash
# Check socket permissions
ls -la /var/run/datadog/mcp.sock

# Run with appropriate permissions
sudo python mcp_client.py
```

### "Connection refused"

The socket exists but no one is listening.

```bash
# Check if agent is running
systemctl status datadog-agent

# Check agent logs
tail -f /var/log/datadog/agent.log | grep -i mcp
```

## License

This project is licensed under the Apache License Version 2.0.
