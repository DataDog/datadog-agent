# MCP Metrics Viewer - Executive Summary

## Requirements Summary

LLM agents (Claude Code, AI SRE agents, future automation) need programmatic
access to container metrics analysis to assist engineers with diagnostics. The
MCP server runs in-cluster as a Kubernetes Deployment, discovering
fine-grained-monitor DaemonSet pods and routing node-targeted queries to the
correct pod.

Four tools are exposed: node discovery, metric discovery, container search, and
analytical studies. Agents receive structured findings with statistics and trend
classifications rather than raw timeseries data. The node parameter is required
on container/analysis queries, making the node-local nature of the data explicit.

The server communicates via MCP over HTTP/SSE transport, enabling both local
development (via port-forward) and direct access from AI SRE agents in backend
services.

## Technical Summary

In-cluster Deployment (`mcp-metrics`) using rmcp SDK with HTTP/SSE transport.
Discovers DaemonSet pods via Kubernetes API (list/watch with label selector),
caches node→pod mappings, and routes requests to the correct viewer pod based on
the `node` parameter.

Four tools:
- `list_nodes` - returns available nodes with pod status (call this first)
- `list_metrics` - returns metric names and study definitions
- `list_containers` - requires `node`, returns containers on that node
- `analyze_container` - requires `node`, runs study on container's metrics

No raw timeseries data is returned. No aggregation across nodes—cluster-wide
analysis is the agent's responsibility.

## Prior Implementation (v1 - Deprecated)

A laptop-based prototype exists using stdio transport and port-forward. This
will be removed once the in-cluster implementation is complete. The prototype
demonstrated the MCP tool interface but does not support:
- Node-aware routing
- AI SRE agent access
- Production deployment

## Status Summary

| Requirement | Status | Notes |
|-------------|--------|-------|
| **REQ-MCP-001:** Discover Available Metrics | Not started | Needs refactor for in-cluster |
| **REQ-MCP-002:** Find Containers by Criteria | Not started | Add `node` parameter |
| **REQ-MCP-003:** Sort Containers by Recency | Not started | Same as v1 |
| **REQ-MCP-004:** Analyze Container Behavior | Not started | Add `node` parameter |
| **REQ-MCP-005:** Identify Behavioral Trends | Not started | Same as v1 |
| **REQ-MCP-006:** Operate via HTTP/SSE | Not started | Replace stdio transport |
| **REQ-MCP-007:** Discover Cluster Nodes | Not started | New `list_nodes` tool |
| **REQ-MCP-008:** Route Requests by Node | Not started | Pod discovery + routing |

**Progress:** 0 of 8 complete

## Implementation Milestones

1. **Infrastructure:** Headless Service for DaemonSet, MCP Deployment + RBAC
2. **Pod Discovery:** kube-rs watcher, node→pod cache
3. **HTTP/SSE Transport:** Replace stdio with SSE server
4. **Node Routing:** Route tool calls to correct viewer pod
5. **Tool Updates:** Add `node` parameter, `list_nodes` tool
6. **Deprecation:** Remove laptop-based prototype
