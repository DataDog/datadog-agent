#!/usr/bin/env python3
"""
Test script for the bash_execute MCP tool using simple HTTP/JSON-RPC.

Usage:
    uv run test_bash_execute.py "echo 'Hello World'"
    uv run test_bash_execute.py --url http://localhost:8080/mcp "ls -la"
    uv run test_bash_execute.py --timeout 60 "sleep 5 && echo done"
"""

import argparse
import json
import sys
import httpx


def make_jsonrpc_request(method: str, params: dict = None, request_id: int = 1):
    """Create a JSON-RPC 2.0 request."""
    request = {
        "jsonrpc": "2.0",
        "id": request_id,
        "method": method,
    }
    if params is not None:
        request["params"] = params
    return request


def main():
    parser = argparse.ArgumentParser(
        description="Execute bash commands via MCP bash_execute tool"
    )
    parser.add_argument(
        "command",
        nargs="?",
        default="echo 'Hello from MCP'",
        help="Bash command to execute (default: echo 'Hello from MCP')",
    )
    parser.add_argument(
        "--url",
        default="http://localhost:8080/mcp",
        help="MCP server URL (default: http://localhost:8080/mcp)",
    )
    parser.add_argument(
        "--timeout",
        type=int,
        default=30,
        help="Command timeout in seconds (default: 30)",
    )
    parser.add_argument(
        "--json",
        action="store_true",
        help="Output only JSON result",
    )
    parser.add_argument(
        "--debug",
        action="store_true",
        help="Enable debug output with full tracebacks",
    )

    args = parser.parse_args()

    if not args.json and args.debug:
        print(f"Connecting to MCP server at {args.url}")
        print(f"Command: {args.command}\n")

    try:
        with httpx.Client(timeout=60.0) as client:
            headers = {
                "Content-Type": "application/json",
                "Accept": "application/json, text/event-stream",
            }

            # 1. Initialize the MCP session
            init_request = make_jsonrpc_request(
                "initialize",
                {
                    "protocolVersion": "2024-11-05",
                    "capabilities": {},
                    "clientInfo": {
                        "name": "bash-execute-tester",
                        "version": "0.1.0"
                    }
                },
                request_id=1
            )

            response = client.post(args.url, json=init_request, headers=headers)
            response.raise_for_status()
            init_result = response.json()

            if "error" in init_result:
                print(f"❌ Initialize failed: {init_result['error']}", file=sys.stderr)
                sys.exit(1)

            # Get session ID from response headers (required for subsequent requests)
            session_id = response.headers.get("mcp-session-id", "")
            if not session_id:
                print(f"❌ No session ID returned by server", file=sys.stderr)
                if args.debug:
                    print(f"Response headers: {dict(response.headers)}", file=sys.stderr)
                sys.exit(1)

            headers["mcp-session-id"] = session_id

            if args.debug:
                print(f"[DEBUG] Connected to MCP server (session: {session_id})", file=sys.stderr)

            # 2. Send initialized notification (required to complete handshake)
            initialized_notification = {
                "jsonrpc": "2.0",
                "method": "notifications/initialized",
            }

            if args.debug:
                print(f"[DEBUG] Sending initialized notification with session: {session_id}", file=sys.stderr)

            response = client.post(args.url, json=initialized_notification, headers=headers)
            # Notifications don't expect a response, but we should check for errors
            response.raise_for_status()

            if args.debug:
                print(f"[DEBUG] Initialized notification response status: {response.status_code}", file=sys.stderr)
                print(f"[DEBUG] Response body: {response.text}", file=sys.stderr)

            # 3. List tools
            if args.debug:
                list_tools_request = make_jsonrpc_request("tools/list", {}, request_id=2)
                response = client.post(args.url, json=list_tools_request, headers=headers)
                response.raise_for_status()
                tools_result = response.json()

                if "result" in tools_result and "tools" in tools_result["result"]:
                    print("[DEBUG] Available tools:", file=sys.stderr)
                    for tool in tools_result["result"]["tools"]:
                        print(f"  - {tool['name']}: {tool.get('description', '')}", file=sys.stderr)
                    print(file=sys.stderr)

            # 4. Call bash_execute tool
            tool_args = {
                "command": args.command,
            }
            if args.timeout > 0:
                tool_args["timeout"] = args.timeout

            if args.debug:
                print(f"[DEBUG] Executing: {args.command}", file=sys.stderr)

            call_tool_request = make_jsonrpc_request(
                "tools/call",
                {
                    "name": "bash_execute",
                    "arguments": tool_args
                },
                request_id=3
            )

            response = client.post(args.url, json=call_tool_request, headers=headers, timeout=args.timeout + 10)
            response.raise_for_status()
            tool_result = response.json()

            if "error" in tool_result:
                print(f"❌ Tool call error: {tool_result['error']}", file=sys.stderr)
                sys.exit(1)

            # Parse the result
            result = tool_result.get("result", {})

            if result.get("isError"):
                print("❌ Tool returned error:", file=sys.stderr)
                for content in result.get("content", []):
                    if content.get("type") == "text":
                        print(content.get("text"), file=sys.stderr)
                sys.exit(1)

            # Extract and display the output
            for content in result.get("content", []):
                if content.get("type") == "text":
                    output = json.loads(content.get("text", "{}"))

                    if args.json:
                        # Machine-readable JSON only
                        print(json.dumps(output, indent=2))
                    else:
                        # Human-readable output
                        print(output.get("output", ""))
                        if output.get("error"):
                            print(f"Error: {output['error']}", file=sys.stderr)

                    if output.get("exit_code", 0) != 0:
                        sys.exit(output["exit_code"])

    except httpx.ConnectError:
        print(f"❌ Cannot connect to server at {args.url}", file=sys.stderr)
        print("   Make sure the MCP server is running: ./mcp-server", file=sys.stderr)
        sys.exit(1)
    except httpx.TimeoutException:
        print(f"❌ Request timed out", file=sys.stderr)
        sys.exit(1)
    except httpx.HTTPStatusError as e:
        print(f"❌ HTTP error {e.response.status_code}: {e.response.text}", file=sys.stderr)
        if args.debug:
            import traceback
            traceback.print_exc()
        sys.exit(1)
    except json.JSONDecodeError as e:
        print(f"❌ Failed to parse JSON response: {e}", file=sys.stderr)
        if args.debug:
            import traceback
            traceback.print_exc()
        sys.exit(1)
    except Exception as e:
        print(f"❌ Unexpected error: {e}", file=sys.stderr)
        if args.debug:
            import traceback
            traceback.print_exc()
        sys.exit(1)


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print("\n\nInterrupted by user", file=sys.stderr)
        sys.exit(130)
