# MCP Metrics Viewer - Technical Design

## Architecture Overview

The MCP server is a separate binary (`mcp-metrics-viewer`) that communicates with
LLM agents via stdio using the Model Context Protocol. It acts as a thin adapter
layer between MCP tool calls and the existing metrics-viewer HTTP API.

```
Claude Code (laptop)
    |
    v [stdio/JSON-RPC]
mcp-metrics-viewer binary (laptop)
    |
    v [HTTP via port-forward]
metrics-viewer HTTP API (k8s cluster)
    |
    v
Parquet data files
```

This architecture reuses the existing HTTP API without modification. The MCP
binary can be distributed independently and requires only network access to the
metrics-viewer service.

## Data Flow

### REQ-MCP-001, REQ-MCP-006: Tool Registration and Discovery

At startup, the MCP server registers three tools with the MCP runtime:
- `list_metrics` - calls `/api/metrics` and `/api/studies`
- `list_containers` - calls `/api/containers` and `/api/filters`
- `analyze_container` - calls `/api/study/:id` and `/api/timeseries`

The rmcp SDK handles schema generation from Rust types automatically.

### REQ-MCP-002, REQ-MCP-003: Container Search Flow

1. Agent calls `list_containers` with optional filters
2. MCP server calls `/api/containers?metric=X&namespace=Y&search=Z`
3. HTTP API returns containers with stats (avg, max, sample_count)
4. Returns sorted results to agent (no raw timeseries)

### REQ-MCP-004, REQ-MCP-005: Analysis Flow

1. Agent calls `analyze_container` with container ID and study type
2. MCP server calls `/api/study/:id?metric=X&containers=Y`
3. HTTP API runs study, returns findings with timestamps
4. MCP server computes trend from stats
5. Returns structured result with findings and trend (no raw timeseries)

## Tool Schemas

### list_metrics

**Input:** None

**Output:**
```
{
  metrics: [{ name, sample_count }],
  studies: [{ id, name, description }]
}
```

### list_containers

**Input:**
- `metric` (required): Metric name to filter containers (only returns containers with data for this metric)
- `namespace` (optional): Kubernetes namespace filter
- `qos_class` (optional): QoS class filter
- `search` (optional): Text search in pod/container names
- `limit` (optional): Max results (default 20)

**Output:**
```
{
  containers: [{
    id, pod_name, container_name, namespace, qos_class, last_seen
  }],
  total_matching: 47
}
```

Results sorted by `last_seen` descending (most recent first).

### analyze_container

**Input:**
- `container` (required): Container ID (short or full)
- `study_id` (required): "periodicity" or "changepoint"
- `metric` (optional): Single metric name
- `metric_prefix` (optional): Metric prefix to analyze all matching metrics

**Output:**
```
{
  container: { id, pod_name, namespace },
  study: "periodicity",
  results: [{
    metric,
    stats: { avg, max, min, stddev, trend, sample_count, time_range },
    findings: [{ type, timestamp, label, details }]
  }]
}
```

## Metric Prefix Matching

REQ-MCP-004 supports analyzing multiple metrics via prefix matching. The MCP
server queries `/api/metrics` and filters locally by prefix. Examples:

- `cgroup.v2.cpu` matches `cgroup.v2.cpu.stat.usage_usec`, `cgroup.v2.cpu.stat.throttled_usec`, etc.
- `cgroup.v2.memory` matches `cgroup.v2.memory.current`, `cgroup.v2.memory.max`, etc.
- `cgroup.v2.io` matches `cgroup.v2.io.stat.rbytes`, `cgroup.v2.io.stat.wbytes`, etc.

Each matching metric is analyzed separately and included in the results array.

## Trend Detection (REQ-MCP-005)

Trend classification uses linear regression on the timeseries values:

1. Compute slope via least-squares fit
2. Normalize slope by mean value to get relative change rate
3. Classify based on thresholds:
   - `increasing`: relative slope > 1% per sample
   - `decreasing`: relative slope < -1% per sample
   - `volatile`: stddev/mean > 30%
   - `stable`: otherwise

## HTTP Client

The MCP server uses reqwest to call the existing HTTP API:

```
MetricsViewerClient
  - base_url: String (e.g., "http://localhost:8050")
  - client: reqwest::Client

Methods:
  - list_metrics() -> Vec<MetricInfo>
  - list_filters() -> FiltersResponse
  - search_containers(params) -> Vec<ContainerStats>
  - run_study(study_id, metric, containers) -> StudyResponse
```

## Configuration

The binary accepts configuration via CLI args:

- `--api-url` (required): Base URL of metrics-viewer API
- `--timeout` (optional): HTTP request timeout (default 30s)

Example: `mcp-metrics-viewer --api-url http://localhost:8050`

## File Locations

| File | Purpose |
|------|---------|
| `Cargo.toml` | rmcp 0.12, reqwest, schemars dependencies; mcp-metrics-viewer binary |
| `src/bin/mcp_metrics_viewer.rs` | Binary entry point with CLI args |
| `src/metrics_viewer/mcp/mod.rs` | MCP server, tool implementations, trend detection |
| `src/metrics_viewer/mcp/client.rs` | HTTP client wrapper for metrics-viewer API |

## Error Handling

- HTTP errors: Return MCP error response with status code and message
- Timeout: Return MCP error with timeout indication
- Invalid container ID: Return empty results (not an error)
- Invalid study ID: Return MCP error listing valid study IDs
- Missing required params: rmcp SDK validates automatically

## Dependencies

| Crate | Version | Purpose |
|-------|---------|---------|
| rmcp | 0.8 | Official Rust MCP SDK |
| reqwest | 0.12 | HTTP client |
| tokio | 1.x | Async runtime (existing) |
| serde | 1.x | Serialization (existing) |
| clap | 4.x | CLI argument parsing (existing) |
