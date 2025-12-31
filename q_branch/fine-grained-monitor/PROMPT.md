# Task: MVP MCP Server for Fine-Grained Metrics Viewer

## Objective

Build an MCP (Model Context Protocol) server that enables LLM agents to:
1. Programmatically access Studies (periodicity detection, changepoint detection)
2. Query container metrics with high-level insights (no raw timeseries)

## Plan Document

Full implementation plan is at:
```
~/.claude/plans/sparkling-forging-crayon.md
```

## Quick Summary

**Architecture:**
- Separate binary (`mcp-metrics-viewer`) using stdio transport
- Runs locally, calls existing HTTP API via port-forward
- Uses official rmcp SDK v0.8

**3 MCP Tools:**
1. `list_metrics` - Discover metrics, groups, and available studies
2. `list_containers` - Find containers with summary statistics for ranking/selection
3. `analyze_container` - Run study on container, returns insights + summaries (NO raw data)

**Key Design Principles:**
- No raw timeseries (LLMs can't interpret thousands of points)
- Predefined studies only (no ad-hoc query DSL)
- Single container analysis (LLM searches first, then analyzes)
- Metric groups (cpu, memory, io) for diagnostic triage

## Files to Create

| File | Description |
|------|-------------|
| `Cargo.toml` | Add rmcp, reqwest deps; add new binary |
| `src/bin/mcp_metrics_viewer.rs` | Binary entry point |
| `src/metrics_viewer/mcp/mod.rs` | Module exports |
| `src/metrics_viewer/mcp/client.rs` | HTTP client for API |
| `src/metrics_viewer/mcp/tools.rs` | Tool implementations |
| `src/metrics_viewer/mcp/groups.rs` | Metric group definitions |

## Getting Started

1. Read the full plan: `cat ~/.claude/plans/sparkling-forging-crayon.md`
2. Start with Step 1: Add dependencies to Cargo.toml
3. Implement tools incrementally, testing against the existing HTTP API
