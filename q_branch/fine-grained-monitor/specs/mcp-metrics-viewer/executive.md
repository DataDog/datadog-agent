# MCP Metrics Viewer - Executive Summary

## Requirements Summary

LLM agents need programmatic access to container metrics analysis to assist
engineers with diagnostics. The MCP server exposes three tools: metric discovery,
container search with ranking, and analytical studies. Agents receive structured
findings with statistics and trend classifications rather than raw timeseries
data, enabling them to synthesize insights without requiring engineers to
navigate visual interfaces.

The server communicates via Model Context Protocol over stdio, matching the
standard LLM tool integration pattern. It calls the existing metrics-viewer HTTP
API, requiring only kubectl port-forward access to the cluster.

## Technical Summary

Separate binary `mcp-metrics-viewer` using rmcp SDK for MCP protocol handling.
Acts as adapter between MCP tool calls and existing HTTP API endpoints. Three
tools map to existing endpoints: `list_metrics` calls `/api/metrics` and
`/api/studies`; `list_containers` calls `/api/containers`; `analyze_container`
calls `/api/study/:id`.

Metric prefix matching (e.g., `cgroup.v2.cpu`) enables filtering and batch
analysis of related metrics without manual grouping. Trend detection uses linear
regression to classify behavior as increasing, decreasing, stable, or volatile.
No raw timeseries data is ever returned to agents.

## Status Summary

| Requirement | Status | Notes |
|-------------|--------|-------|
| **REQ-MCP-001:** Discover Available Metrics | ✅ Complete | `list_metrics` tool in `mcp/mod.rs` |
| **REQ-MCP-002:** Find Containers by Criteria | ✅ Complete | `list_containers` tool with namespace, qos_class, search filters |
| **REQ-MCP-003:** Sort Containers by Recency | 🟡 Partial | Structure in place; uses API default sort (avg) pending API changes |
| **REQ-MCP-004:** Analyze Container Behavior | ✅ Complete | `analyze_container` tool with metric prefix support |
| **REQ-MCP-005:** Identify Behavioral Trends | ✅ Complete | Linear regression trend detection in `detect_trend()` |
| **REQ-MCP-006:** Operate via Standard Protocol | ✅ Complete | rmcp 0.12 with stdio transport |

**Progress:** 5 of 6 complete (1 partial)
