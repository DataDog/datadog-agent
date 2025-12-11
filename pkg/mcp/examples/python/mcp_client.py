#!/usr/bin/env python3
# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2024-present Datadog, Inc.

"""
MCP Client Example for Datadog Agent

This script demonstrates how to connect to the Datadog Agent's MCP server
and query process information using the Model Context Protocol.

Usage:
    python mcp_client.py [--socket PATH] [--tcp HOST:PORT]

Examples:
    # Connect via Unix socket (default)
    python mcp_client.py

    # Connect via Unix socket with custom path
    python mcp_client.py --socket /var/run/datadog/mcp.sock

    # Connect via TCP
    python mcp_client.py --tcp localhost:7890
"""

import argparse
import json
import socket
import sys
from typing import Any, Optional


class MCPClient:
    """A simple MCP client for connecting to the Datadog Agent."""

    DEFAULT_SOCKET_PATH = "/var/run/datadog/mcp.sock"
    PROTOCOL_VERSION = "2024-11-05"

    def __init__(self, socket_path: Optional[str] = None, tcp_address: Optional[str] = None):
        """
        Initialize the MCP client.

        Args:
            socket_path: Path to Unix domain socket (default: /var/run/datadog/mcp.sock)
            tcp_address: TCP address in format "host:port" (alternative to Unix socket)
        """
        self.socket_path = socket_path
        self.tcp_address = tcp_address
        self.sock: Optional[socket.socket] = None
        self.request_id = 0
        self.initialized = False
        self._buffer = ""

    def connect(self) -> None:
        """Connect to the MCP server."""
        if self.tcp_address:
            # TCP connection
            host, port = self.tcp_address.rsplit(":", 1)
            self.sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            self.sock.connect((host, int(port)))
            print(f"Connected to TCP {self.tcp_address}")
        else:
            # Unix socket connection
            path = self.socket_path or self.DEFAULT_SOCKET_PATH
            self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            self.sock.connect(path)
            print(f"Connected to Unix socket {path}")

    def close(self) -> None:
        """Close the connection."""
        if self.sock:
            self.sock.close()
            self.sock = None
            self.initialized = False

    def _next_id(self) -> int:
        """Get the next request ID."""
        self.request_id += 1
        return self.request_id

    def _send(self, message: dict) -> None:
        """Send a JSON-RPC message."""
        if not self.sock:
            raise RuntimeError("Not connected")
        data = json.dumps(message) + "\n"
        self.sock.sendall(data.encode("utf-8"))

    def _recv(self) -> dict:
        """Receive a JSON-RPC message."""
        if not self.sock:
            raise RuntimeError("Not connected")

        # Read until we get a complete JSON message
        while "\n" not in self._buffer:
            chunk = self.sock.recv(4096)
            if not chunk:
                raise RuntimeError("Connection closed")
            self._buffer += chunk.decode("utf-8")

        # Extract the first complete message
        line, self._buffer = self._buffer.split("\n", 1)
        return json.loads(line)

    def _request(self, method: str, params: Optional[dict] = None) -> dict:
        """Send a request and wait for response."""
        request_id = self._next_id()
        message = {
            "jsonrpc": "2.0",
            "method": method,
            "id": request_id,
        }
        if params is not None:
            message["params"] = params

        self._send(message)
        response = self._recv()

        if response.get("id") != request_id:
            raise RuntimeError(f"Response ID mismatch: expected {request_id}, got {response.get('id')}")

        if "error" in response:
            error = response["error"]
            raise RuntimeError(f"RPC error {error.get('code')}: {error.get('message')}")

        return response.get("result", {})

    def _notify(self, method: str, params: Optional[dict] = None) -> None:
        """Send a notification (no response expected)."""
        message = {
            "jsonrpc": "2.0",
            "method": method,
        }
        if params is not None:
            message["params"] = params

        self._send(message)

    def initialize(self) -> dict:
        """
        Initialize the MCP session.

        Returns:
            Server capabilities and info
        """
        result = self._request(
            "initialize",
            {
                "protocolVersion": self.PROTOCOL_VERSION,
                "capabilities": {},
                "clientInfo": {
                    "name": "datadog-mcp-python-example",
                    "version": "1.0.0",
                },
            },
        )

        # Send initialized notification
        self._notify("notifications/initialized")
        self.initialized = True

        return result

    def ping(self) -> dict:
        """Send a ping to check server health."""
        return self._request("ping")

    def list_tools(self) -> list:
        """
        List available tools.

        Returns:
            List of tool definitions with name, description, and input schema
        """
        result = self._request("tools/list")
        return result.get("tools", [])

    def call_tool(self, name: str, arguments: Optional[dict] = None) -> Any:
        """
        Call a tool with the given arguments.

        Args:
            name: Tool name (e.g., "GetProcessSnapshot")
            arguments: Tool arguments

        Returns:
            Tool result (parsed from JSON if text content)
        """
        result = self._request(
            "tools/call",
            {
                "name": name,
                "arguments": arguments or {},
            },
        )

        # Extract content from result
        content = result.get("content", [])
        if content and len(content) > 0:
            first_content = content[0]
            if first_content.get("type") == "text":
                # Parse JSON text content
                try:
                    return json.loads(first_content.get("text", "{}"))
                except json.JSONDecodeError:
                    return first_content.get("text")
            return first_content

        return result

    def get_process_snapshot(
        self,
        pids: Optional[list] = None,
        process_names: Optional[list] = None,
        regex_filter: Optional[str] = None,
        include_stats: bool = True,
        include_io: bool = False,
        include_net: bool = False,
        limit: int = 100,
        sort_by: Optional[str] = None,
        ascending: bool = False,
    ) -> dict:
        """
        Get a snapshot of running processes.

        Args:
            pids: Filter by specific process IDs
            process_names: Filter by exact process names
            regex_filter: Regex pattern for filtering
            include_stats: Include CPU/memory statistics
            include_io: Include I/O statistics
            include_net: Include network information
            limit: Maximum number of processes to return
            sort_by: Sort field (cpu, memory, pid, name)
            ascending: Sort in ascending order

        Returns:
            Process snapshot with host info and process list
        """
        arguments = {
            "include_stats": include_stats,
            "include_io": include_io,
            "include_net": include_net,
            "limit": limit,
            "ascending": ascending,
        }

        if pids:
            arguments["pids"] = pids
        if process_names:
            arguments["process_names"] = process_names
        if regex_filter:
            arguments["regex_filter"] = regex_filter
        if sort_by:
            arguments["sort_by"] = sort_by

        return self.call_tool("GetProcessSnapshot", arguments)


def print_json(data: Any, indent: int = 2) -> None:
    """Pretty print JSON data."""
    print(json.dumps(data, indent=indent, default=str))


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(
        description="MCP Client Example for Datadog Agent",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  %(prog)s                           # Connect via default Unix socket
  %(prog)s --socket /tmp/mcp.sock    # Connect via custom Unix socket
  %(prog)s --tcp localhost:7890      # Connect via TCP
        """,
    )
    parser.add_argument(
        "--socket",
        help="Unix socket path (default: /var/run/datadog/mcp.sock)",
    )
    parser.add_argument(
        "--tcp",
        help="TCP address in format host:port",
    )
    parser.add_argument(
        "--limit",
        type=int,
        default=10,
        help="Number of processes to retrieve (default: 10)",
    )
    parser.add_argument(
        "--sort-by",
        choices=["cpu", "memory", "pid", "name"],
        default="cpu",
        help="Sort processes by field (default: cpu)",
    )
    args = parser.parse_args()

    # Create client
    client = MCPClient(socket_path=args.socket, tcp_address=args.tcp)

    try:
        # Connect
        print("=" * 60)
        print("Connecting to MCP server...")
        client.connect()

        # Initialize
        print("\n" + "=" * 60)
        print("Initializing MCP session...")
        init_result = client.initialize()
        print("Server info:")
        print_json(init_result)

        # Ping
        print("\n" + "=" * 60)
        print("Sending ping...")
        ping_result = client.ping()
        print(f"Ping result: {ping_result}")

        # List tools
        print("\n" + "=" * 60)
        print("Listing available tools...")
        tools = client.list_tools()
        print(f"Found {len(tools)} tool(s):")
        for tool in tools:
            print(f"  - {tool.get('name')}: {tool.get('description', 'No description')}")

        # Get process snapshot
        print("\n" + "=" * 60)
        print(f"Getting top {args.limit} processes by {args.sort_by}...")
        snapshot = client.get_process_snapshot(
            limit=args.limit,
            sort_by=args.sort_by,
            include_stats=True,
        )

        # Print host info
        if "host_info" in snapshot:
            print("\nHost Information:")
            print_json(snapshot["host_info"])

        # Print process summary
        if "processes" in snapshot:
            processes = snapshot["processes"]
            print(f"\nProcesses ({len(processes)} returned, {snapshot.get('total_count', '?')} total):")
            print("-" * 80)
            print(f"{'PID':>8} {'CPU%':>6} {'MEM%':>6} {'NAME':<20} {'COMMAND'}")
            print("-" * 80)
            for proc in processes:
                pid = proc.get("pid", 0)
                cpu = proc.get("cpu_percent", 0)
                mem = proc.get("memory", {}).get("percent", 0)
                name = proc.get("name", "")[:20]
                cmd = " ".join(proc.get("command_line", []))[:40]
                print(f"{pid:>8} {cpu:>6.1f} {mem:>6.2f} {name:<20} {cmd}")

        # Example: Find specific processes by name
        print("\n" + "=" * 60)
        print("Finding processes with 'agent' in name...")
        agent_snapshot = client.get_process_snapshot(
            regex_filter=".*agent.*",
            limit=5,
        )
        if "processes" in agent_snapshot:
            for proc in agent_snapshot["processes"]:
                print(f"  - PID {proc.get('pid')}: {proc.get('name')}")

        print("\n" + "=" * 60)
        print("Done!")

    except FileNotFoundError:
        print(f"Error: Socket not found. Is the MCP server running?", file=sys.stderr)
        sys.exit(1)
    except PermissionError:
        print(f"Error: Permission denied. Check socket permissions.", file=sys.stderr)
        sys.exit(1)
    except ConnectionRefusedError:
        print(f"Error: Connection refused. Is the MCP server running?", file=sys.stderr)
        sys.exit(1)
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)
    finally:
        client.close()


if __name__ == "__main__":
    main()
