# DD-Agent Toolset: LLM-Driven Agent Control via MCP

**Author**: @Valeri Pliskin
**Level of Effort**: M

## Problem

When troubleshooting production issues or managing agent features, operators need to:
- SSH into nodes
- Manually execute diagnostic commands across multiple pods
- Copy-paste output for LLM analysis
- Restart agents to enable/disable features like USM or NPM

This is slow (10-30 min per pod), doesn't scale, and prevents dynamic agent configuration. The Datadog Agent already runs on every node with elevated permissions—it could serve as a secure, pre-authorized toolkit accessible directly by LLMs.

## Solution

Expose Datadog Agent capabilities as **MCP (Model Context Protocol) tools** that Claude can invoke on-demand. This gadget explores four key experiments:

**1. End-to-End LLM Toolset via MCP**: MCP server on operator's laptop exposes 30+ agent diagnostic tools (status, health, workload-list, network_connections, process_cache, eBPF maps). Claude discovers tools, invokes them directly, and analyzes results without manual kubectl exec.

**2. Context Window Management**: Test if Claude navigates 30+ tools without context overflow. Tools organized in tiers (Discovery → Workload → Diagnostics → System-Probe). MCP tool search via keywords helps Claude discover relevant tools efficiently.
Implement tool-to-search-tools (https://www.anthropic.com/engineering/advanced-tool-use) approach to reduce the used context window by the mcps.

**3. Dynamic Feature Control**: New `module_control` tool pauses/resumes USM and NPM at runtime without agent restart. Uses eBPF bypass mechanism to instantly toggle monitoring. Tests viability for cost optimization and troubleshooting.

**4. Kubectl as Transport**: Consider a user story where the human operator wants to investigate their prod env on their laptop inside the terminal (using their favorite AI assistant: Cursor, Claude, gemini-cli...).
The LLM should be able to use the tools provided by the dd-agent running in the cloud customer's environment, this can be achieved using the `kubectl` as a transport layer for the local MCP server.
While Claude can perform the `kubectl` commands directly the choice to include the mcp in the arhitecture was made to showcase the central governance and logic layer to allow: security, audit, rate limit, and other logic rather than direct free access to LLMs into customers' environments

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           OPERATOR'S LAPTOP                                 │
│                                                                             │
│  ┌─────────────┐      ┌─────────────────────┐      ┌──────────────────┐    │
│  │             │      │                     │      │                  │    │
│  │   Human     │─────▶│   Claude Desktop    │─────▶│   MCP Server     │    │
│  │  Operator   │      │   (Claude Code)     │      │   (Python)       │    │
│  │             │◀─────│                     │◀─────│                  │    │
│  └─────────────┘      └─────────────────────┘      └────────┬─────────┘    │
│                                                              │              │
│                                                              │ kubectl exec │
│                                                              ▼              │
└──────────────────────────────────────────────────────────────┼──────────────┘
                                                               │
                          ┌────────────────────────────────────┼───────────────┐
                          │             KUBERNETES CLUSTER     │               │
                          │                                    │               │
┌─────────────────────────┼────────────────────────────────────┼───────────────┼─────────────────────────┐
│                         │                                    │               │                         │
│  ┌──────────────────────┴───────────┐    ┌───────────────────┴──────────────┴──────────┐              │
│  │           Node 1                 │    │           Node 2                            │              │
│  │                                  │    │                                             │              │
│  │  ┌────────────────────────────┐  │    │  ┌────────────────────────────┐             │              │
│  │  │     Datadog Agent Pod      │  │    │  │     Datadog Agent Pod      │◀────────────┘              │
│  │  │  ┌──────────────────────┐  │  │    │  │  ┌──────────────────────┐  │                            │
│  │  │  │  agent container     │  │  │    │  │  │  agent container     │  │                            │
│  │  │  └──────────────────────┘  │  │    │  │  └──────────────────────┘  │                            │
│  │  │  ┌──────────────────────┐  │  │    │  │  ┌──────────────────────┐  │                            │
│  │  │  │  system-probe        │  │  │    │  │  │  system-probe        │  │                            │
│  │  │  └──────────────────────┘  │  │    │  │  └──────────────────────┘  │                            │
│  │  │  ┌──────────────────────┐  │  │    │  │  ┌──────────────────────┐  │                            │
│  │  │  │  trace-agent         │  │  │    │  │  │  trace-agent         │  │                            │
│  │  │  └──────────────────────┘  │  │    │  │  └──────────────────────┘  │                            │
│  │  └────────────────────────────┘  │    │  └────────────────────────────┘             │              │
│  │                                  │    │                                             │              │
│  │  ┌────────────────────────────┐  │    │  ┌────────────────────────────┐             │              │
│  │  │   App Pod (nginx)          │  │    │  │   App Pod (redis)          │             │              │
│  │  └────────────────────────────┘  │    │  └────────────────────────────┘             │              │
│  │  ┌────────────────────────────┐  │    │  ┌────────────────────────────┐             │              │
│  │  │   App Pod (postgres)       │  │    │  │   App Pod (api-server)     │             │              │
│  │  └────────────────────────────┘  │    │  └────────────────────────────┘             │              │
│  └──────────────────────────────────┘    └─────────────────────────────────────────────┘              │
│                                                                                                       │
└───────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

## Example Scenario

```
Operator: "Redis on node-2 is experiencing connection timeouts. Investigate what's happening."

Claude: [Calls: list_agent_pods()]
        Found agent pod datadog-agent-xyz on node-2.

        [Calls: workload_list(pod="datadog-agent-xyz")]
        Identified redis container in pod redis-abc123, running on node-2.
        Container ID: containerd://7f8e9d...

        [Calls: network_connections(pod="datadog-agent-xyz")]
        Found 847 active TCP connections to redis port 6379.
        Top connection sources:
        - api-server pod (531 connections, state: ESTABLISHED)
        - nginx pod (89 connections, state: ESTABLISHED)

        [Calls: process_cache(pod="datadog-agent-xyz")]
        api-server process (PID 2847) has 531 open connections.
        Command: /usr/bin/api-server --redis-pool-size=10

Claude: Analysis: The api-server is configured with a pool size of 10 but has opened 531
        connections to redis, indicating a connection leak. This exceeds redis's max
        connections limit (default 1024), causing new connection attempts to timeout.

        Recommendation: Restart api-server pod or fix connection pooling logic.
```

## Viability Check

**This works if:**
- Claude navigates 30+ tools without context overflow
- `module_control` successfully pauses/resumes USM and NPM via eBPF bypass
- Operators prefer this workflow over manual kubectl commands or using datadog web app

**We should stop if:**
- Context window limits prevent effective tool selection with 30+ tools
- LLM is not able to discover tools via the MCP

**Being worked on?** Yes

**Requirements?** No
