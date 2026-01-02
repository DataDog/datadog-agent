//! MCP (Model Context Protocol) server for metrics viewer.
//!
//! Provides programmatic access to metrics discovery and analysis for LLM agents.
//! Runs as an in-cluster Deployment, discovering DaemonSet pods and routing
//! node-targeted queries to the correct pod.
//!
//! # Requirements
//!
//! - REQ-MCP-001: Discover available metrics and studies
//! - REQ-MCP-002: Find containers by criteria
//! - REQ-MCP-003: Sort containers by recency
//! - REQ-MCP-004: Analyze container behavior
//! - REQ-MCP-005: Identify behavioral trends
//! - REQ-MCP-006: Operate via MCP over HTTP/SSE
//! - REQ-MCP-007: Discover cluster nodes
//! - REQ-MCP-008: Route requests by node

pub mod pod_watcher;
pub mod router;

use std::sync::Arc;

use pod_watcher::{NodeInfo, PodWatcher};
use rmcp::{
    handler::server::router::tool::ToolRouter,
    handler::server::wrapper::Parameters,
    model::{CallToolResult, Content, ServerCapabilities, ServerInfo},
    tool, tool_handler, tool_router, ErrorData as McpError, ServerHandler,
};
use router::{NodeRouter, RouterError};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

/// MCP server for metrics viewer with node-aware routing.
#[derive(Clone)]
pub struct McpMetricsViewer {
    pod_watcher: Arc<PodWatcher>,
    router: Arc<NodeRouter>,
    tool_router: ToolRouter<Self>,
}

impl McpMetricsViewer {
    /// Create a new MCP server with the given pod watcher.
    pub fn new(pod_watcher: Arc<PodWatcher>) -> Self {
        let router = Arc::new(NodeRouter::new(pod_watcher.clone()));
        Self {
            pod_watcher,
            router,
            tool_router: Self::tool_router(),
        }
    }
}

// --- Tool Parameter Types ---

/// Parameters for list_containers tool.
/// REQ-MCP-002: Find containers by criteria.
/// REQ-MCP-008: Requires node parameter.
#[derive(Debug, Serialize, Deserialize, JsonSchema)]
pub struct ListContainersParams {
    /// Node name to query (required).
    pub node: Option<String>,

    /// Metric name to filter containers (only returns containers with data for this metric).
    pub metric: String,

    /// Kubernetes namespace filter.
    #[serde(default)]
    pub namespace: Option<String>,

    /// QoS class filter (e.g., "Guaranteed", "Burstable", "BestEffort").
    #[serde(default)]
    pub qos_class: Option<String>,

    /// Text search in pod/container names.
    #[serde(default)]
    pub search: Option<String>,

    /// Maximum number of results (default: 20).
    #[serde(default)]
    pub limit: Option<usize>,
}

/// Parameters for analyze_container tool.
/// REQ-MCP-004: Analyze container behavior.
/// REQ-MCP-008: Requires node parameter.
#[derive(Debug, Serialize, Deserialize, JsonSchema)]
pub struct AnalyzeContainerParams {
    /// Node name to query (required).
    pub node: Option<String>,

    /// Container ID (short 12-char ID or full ID).
    pub container: String,

    /// Study to run: "periodicity" or "changepoint".
    pub study_id: String,

    /// Metric name to analyze (required).
    pub metric: Option<String>,

    /// Metric prefix to analyze all matching metrics (e.g., "cgroup.v2.cpu").
    #[serde(default)]
    pub metric_prefix: Option<String>,
}

// --- Tool Response Types ---

/// Response from list_nodes tool.
#[derive(Debug, Serialize)]
struct ListNodesResponse {
    watcher_stale: bool,
    last_sync_ms: i64,
    nodes: Vec<NodeInfo>,
}

/// Response from list_metrics tool.
#[derive(Debug, Serialize)]
struct ListMetricsResponse {
    node: String,
    metrics: Vec<MetricEntry>,
    studies: Vec<StudyEntry>,
}

#[derive(Debug, Serialize)]
struct MetricEntry {
    name: String,
}

#[derive(Debug, Serialize)]
struct StudyEntry {
    id: String,
    name: String,
    description: String,
}

/// Response from list_containers tool.
#[derive(Debug, Serialize)]
struct ListContainersResponse {
    node: String,
    containers: Vec<ContainerEntry>,
    total_matching: usize,
}

#[derive(Debug, Serialize)]
struct ContainerEntry {
    id: String,
    pod_name: Option<String>,
    container_name: Option<String>,
    namespace: Option<String>,
    qos_class: Option<String>,
    last_seen: Option<i64>,
}

/// Response from analyze_container tool.
#[derive(Debug, Serialize)]
struct AnalyzeContainerResponse {
    node: String,
    container: ContainerSummary,
    study: String,
    results: Vec<MetricAnalysisResult>,
}

#[derive(Debug, Serialize)]
struct ContainerSummary {
    id: String,
    pod_name: Option<String>,
    namespace: Option<String>,
}

#[derive(Debug, Serialize)]
struct MetricAnalysisResult {
    metric: String,
    stats: MetricStats,
    findings: Vec<Finding>,
}

#[derive(Debug, Serialize)]
struct MetricStats {
    avg: f64,
    max: f64,
    min: f64,
    stddev: f64,
    trend: String,
    sample_count: usize,
    time_range: Option<TimeRange>,
}

#[derive(Debug, Serialize)]
struct TimeRange {
    start_ms: i64,
    end_ms: i64,
}

#[derive(Debug, Serialize)]
struct Finding {
    #[serde(rename = "type")]
    finding_type: String,
    timestamp_ms: i64,
    label: String,
    /// Study-specific metrics (pass-through from study output)
    metrics: std::collections::HashMap<String, f64>,
}

// --- API Response Types (from viewer HTTP API) ---
// These structs mirror the viewer's API responses. Some fields are deserialized
// but not used in the MCP response transformations.

#[allow(dead_code)]
#[derive(Debug, Deserialize)]
struct ViewerMetricsResponse {
    metrics: Vec<ViewerMetricInfo>,
}

#[derive(Debug, Deserialize)]
struct ViewerMetricInfo {
    name: String,
}

#[derive(Debug, Deserialize)]
struct ViewerStudiesResponse {
    studies: Vec<ViewerStudyInfo>,
}

#[derive(Debug, Deserialize)]
struct ViewerStudyInfo {
    id: String,
    name: String,
    description: String,
}

#[derive(Debug, Deserialize)]
struct ViewerContainersResponse {
    containers: Vec<ViewerContainerInfo>,
}

#[allow(dead_code)]
#[derive(Debug, Deserialize)]
struct ViewerContainerInfo {
    id: String,
    short_id: String,
    qos_class: Option<String>,
    namespace: Option<String>,
    pod_name: Option<String>,
    container_name: Option<String>,
    last_seen_ms: Option<i64>,
}

#[allow(dead_code)]
#[derive(Debug, Deserialize)]
struct ViewerStudyResponse {
    study: String,
    results: Vec<ViewerContainerStudyResult>,
}

#[allow(dead_code)]
#[derive(Debug, Deserialize)]
struct ViewerContainerStudyResult {
    container: String,
    #[serde(flatten)]
    result: ViewerStudyResult,
}

#[allow(dead_code)]
#[derive(Debug, Deserialize)]
struct ViewerStudyResult {
    study_name: String,
    windows: Vec<ViewerStudyWindow>,
    summary: String,
}

#[allow(dead_code)]
#[derive(Debug, Deserialize)]
struct ViewerStudyWindow {
    start_time_ms: i64,
    end_time_ms: i64,
    metrics: std::collections::HashMap<String, f64>,
    label: String,
}

#[allow(dead_code)]
#[derive(Debug, Deserialize)]
struct ViewerTimeseriesResponse {
    container: String,
    data: Vec<ViewerTimeseriesPoint>,
}

#[derive(Debug, Deserialize)]
struct ViewerTimeseriesPoint {
    time_ms: i64,
    value: f64,
}

// --- Helper to convert router errors to MCP errors ---

fn router_error_to_mcp(e: RouterError) -> McpError {
    match e {
        RouterError::WatcherStale => {
            McpError::internal_error("watcher is stale (Kubernetes API may be unreachable)", None)
        }
        RouterError::NodeNotFound(node) => {
            McpError::invalid_params(format!("node '{}' not found", node), None)
        }
        RouterError::NodeNotReady(node) => {
            McpError::invalid_params(format!("node '{}' is not ready", node), None)
        }
        RouterError::NodeStale(node, secs) => {
            McpError::invalid_params(format!("node '{}' is stale (last seen {}s ago)", node, secs), None)
        }
        RouterError::Timeout(node) => {
            McpError::internal_error(format!("request to node '{}' timed out", node), None)
        }
        RouterError::RequestFailed(node, msg) => {
            McpError::internal_error(format!("request to node '{}' failed: {}", node, msg), None)
        }
        RouterError::ViewerError(node, status, body) => {
            McpError::internal_error(format!("viewer error on node '{}': {} {}", node, status, body), None)
        }
    }
}

// --- Tool Implementations ---

#[tool_router]
impl McpMetricsViewer {
    /// REQ-MCP-007: Discover cluster nodes.
    #[tool(
        name = "list_nodes",
        description = "List available cluster nodes running fine-grained-monitor. Call this first to discover nodes before querying containers or running analysis."
    )]
    async fn list_nodes(&self) -> Result<CallToolResult, McpError> {
        let watcher_stale = self.pod_watcher.is_watcher_stale().await;
        let last_sync_ms = self.pod_watcher.last_sync_ms().await;
        let nodes = self.pod_watcher.list_nodes().await;

        let response = ListNodesResponse {
            watcher_stale,
            last_sync_ms,
            nodes,
        };

        let json = serde_json::to_string_pretty(&response)
            .map_err(|e| McpError::internal_error(format!("JSON error: {}", e), None))?;

        Ok(CallToolResult::success(vec![Content::text(json)]))
    }

    /// REQ-MCP-001: Discover available metrics and studies.
    /// REQ-MCP-008: Requires node parameter.
    #[tool(
        name = "list_metrics",
        description = "List available metrics and analytical studies. Call this first to discover what data is available."
    )]
    async fn list_metrics(&self) -> Result<CallToolResult, McpError> {
        // For list_metrics without node, pick any available node
        let nodes = self.pod_watcher.list_nodes().await;
        let node = nodes
            .iter()
            .find(|n| n.ready && !n.stale)
            .ok_or_else(|| McpError::internal_error("no ready nodes available", None))?;

        let node_name = node.name.clone();

        // Fetch metrics
        let metrics_body = self
            .router
            .get(&node_name, "/api/metrics", NodeRouter::timeout_list_metrics())
            .await
            .map_err(router_error_to_mcp)?;

        let metrics_resp: ViewerMetricsResponse = serde_json::from_str(&metrics_body)
            .map_err(|e| McpError::internal_error(format!("Failed to parse metrics: {}", e), None))?;

        // Fetch studies
        let studies_body = self
            .router
            .get(&node_name, "/api/studies", NodeRouter::timeout_list_metrics())
            .await
            .map_err(router_error_to_mcp)?;

        let studies_resp: ViewerStudiesResponse = serde_json::from_str(&studies_body)
            .map_err(|e| McpError::internal_error(format!("Failed to parse studies: {}", e), None))?;

        let response = ListMetricsResponse {
            node: node_name,
            metrics: metrics_resp
                .metrics
                .into_iter()
                .map(|m| MetricEntry { name: m.name })
                .collect(),
            studies: studies_resp
                .studies
                .into_iter()
                .map(|s| StudyEntry {
                    id: s.id,
                    name: s.name,
                    description: s.description,
                })
                .collect(),
        };

        let json = serde_json::to_string_pretty(&response)
            .map_err(|e| McpError::internal_error(format!("JSON error: {}", e), None))?;

        Ok(CallToolResult::success(vec![Content::text(json)]))
    }

    /// REQ-MCP-002, REQ-MCP-003: Find containers by criteria, sorted by recency.
    /// REQ-MCP-008: Requires node parameter.
    #[tool(
        name = "list_containers",
        description = "Find containers matching search criteria. Returns containers sorted by most recently seen. Use this to identify containers for analysis."
    )]
    async fn list_containers(
        &self,
        Parameters(params): Parameters<ListContainersParams>,
    ) -> Result<CallToolResult, McpError> {
        let node = params
            .node
            .ok_or_else(|| McpError::invalid_params("node parameter is required", None))?;

        // Build query string
        let mut path = format!("/api/containers?metric={}", urlencoding::encode(&params.metric));

        if let Some(ref ns) = params.namespace {
            path.push_str(&format!("&namespace={}", urlencoding::encode(ns)));
        }
        if let Some(ref qos) = params.qos_class {
            path.push_str(&format!("&qos_class={}", urlencoding::encode(qos)));
        }
        if let Some(ref search) = params.search {
            path.push_str(&format!("&search={}", urlencoding::encode(search)));
        }

        let body = self
            .router
            .get(&node, &path, NodeRouter::timeout_list_containers())
            .await
            .map_err(router_error_to_mcp)?;

        let viewer_resp: ViewerContainersResponse = serde_json::from_str(&body)
            .map_err(|e| McpError::internal_error(format!("Failed to parse containers: {}", e), None))?;

        let total = viewer_resp.containers.len();
        let limit = params.limit.unwrap_or(20);

        let response = ListContainersResponse {
            node: node.clone(),
            containers: viewer_resp
                .containers
                .into_iter()
                .take(limit)
                .map(|c| ContainerEntry {
                    id: c.short_id,
                    pod_name: c.pod_name,
                    container_name: c.container_name,
                    namespace: c.namespace,
                    qos_class: c.qos_class,
                    last_seen: c.last_seen_ms,
                })
                .collect(),
            total_matching: total,
        };

        let json = serde_json::to_string_pretty(&response)
            .map_err(|e| McpError::internal_error(format!("JSON error: {}", e), None))?;

        Ok(CallToolResult::success(vec![Content::text(json)]))
    }

    /// REQ-MCP-004, REQ-MCP-005: Analyze container behavior and identify trends.
    /// REQ-MCP-008: Requires node parameter.
    #[tool(
        name = "analyze_container",
        description = "Run a study on a container to detect patterns. Provide either 'metric' for a single metric or 'metric_prefix' to analyze all metrics matching that prefix (e.g., 'cgroup.v2.cpu')."
    )]
    async fn analyze_container(
        &self,
        Parameters(params): Parameters<AnalyzeContainerParams>,
    ) -> Result<CallToolResult, McpError> {
        let node = params
            .node
            .ok_or_else(|| McpError::invalid_params("node parameter is required", None))?;

        // Determine which metrics to analyze
        let metrics_to_analyze = if let Some(ref metric) = params.metric {
            vec![metric.clone()]
        } else if let Some(ref prefix) = params.metric_prefix {
            // Get all metrics matching prefix from this node
            let metrics_body = self
                .router
                .get(&node, "/api/metrics", NodeRouter::timeout_list_metrics())
                .await
                .map_err(router_error_to_mcp)?;

            let metrics_resp: ViewerMetricsResponse = serde_json::from_str(&metrics_body)
                .map_err(|e| McpError::internal_error(format!("Failed to parse metrics: {}", e), None))?;

            metrics_resp
                .metrics
                .into_iter()
                .filter(|m| m.name.starts_with(prefix))
                .map(|m| m.name)
                .collect()
        } else {
            return Err(McpError::invalid_params(
                "Either 'metric' or 'metric_prefix' must be provided",
                None,
            ));
        };

        if metrics_to_analyze.is_empty() {
            return Err(McpError::invalid_params(
                "No metrics found matching the criteria",
                None,
            ));
        }

        let mut results = Vec::new();

        for metric in &metrics_to_analyze {
            // Run the study
            let study_path = format!(
                "/api/study/{}?metric={}&containers={}",
                urlencoding::encode(&params.study_id),
                urlencoding::encode(metric),
                urlencoding::encode(&params.container)
            );

            let study_body = self
                .router
                .get(&node, &study_path, NodeRouter::timeout_analyze())
                .await
                .map_err(router_error_to_mcp)?;

            let study_resp: ViewerStudyResponse = serde_json::from_str(&study_body)
                .map_err(|e| McpError::internal_error(format!("Failed to parse study: {}", e), None))?;

            // Get timeseries for trend detection
            let ts_path = format!(
                "/api/timeseries?metric={}&containers={}",
                urlencoding::encode(metric),
                urlencoding::encode(&params.container)
            );

            let ts_body = self
                .router
                .get(&node, &ts_path, NodeRouter::timeout_analyze())
                .await
                .map_err(router_error_to_mcp)?;

            let ts_resp: Vec<ViewerTimeseriesResponse> = serde_json::from_str(&ts_body)
                .map_err(|e| McpError::internal_error(format!("Failed to parse timeseries: {}", e), None))?;

            // Compute stats and trend
            let ts_data = ts_resp.first().map(|t| &t.data);
            let stats = compute_stats(ts_data);
            let trend = detect_trend(ts_data);

            // Convert study windows to findings (pass through metrics directly)
            let findings: Vec<Finding> = study_resp
                .results
                .first()
                .map(|r| {
                    r.result
                        .windows
                        .iter()
                        .map(|w| Finding {
                            finding_type: params.study_id.clone(),
                            timestamp_ms: w.start_time_ms,
                            label: w.label.clone(),
                            metrics: w.metrics.clone(),
                        })
                        .collect()
                })
                .unwrap_or_default();

            results.push(MetricAnalysisResult {
                metric: metric.clone(),
                stats: MetricStats {
                    avg: stats.avg,
                    max: stats.max,
                    min: stats.min,
                    stddev: stats.stddev,
                    trend,
                    sample_count: stats.count,
                    time_range: stats.time_range,
                },
                findings,
            });
        }

        // Get container info
        let containers_path = format!(
            "/api/containers?metric={}&search={}",
            urlencoding::encode(metrics_to_analyze.first().unwrap_or(&String::new())),
            urlencoding::encode(&params.container)
        );

        let container_info = self
            .router
            .get(&node, &containers_path, NodeRouter::timeout_list_containers())
            .await
            .ok()
            .and_then(|body| serde_json::from_str::<ViewerContainersResponse>(&body).ok())
            .and_then(|resp| resp.containers.into_iter().next());

        let response = AnalyzeContainerResponse {
            node: node.clone(),
            container: ContainerSummary {
                id: params.container,
                pod_name: container_info.as_ref().and_then(|c| c.pod_name.clone()),
                namespace: container_info.as_ref().and_then(|c| c.namespace.clone()),
            },
            study: params.study_id,
            results,
        };

        let json = serde_json::to_string_pretty(&response)
            .map_err(|e| McpError::internal_error(format!("JSON error: {}", e), None))?;

        Ok(CallToolResult::success(vec![Content::text(json)]))
    }
}

#[tool_handler]
impl ServerHandler for McpMetricsViewer {
    fn get_info(&self) -> ServerInfo {
        ServerInfo {
            server_info: rmcp::model::Implementation {
                name: "mcp-metrics-viewer".into(),
                version: env!("CARGO_PKG_VERSION").into(),
                title: Some("MCP Metrics Viewer".into()),
                icons: None,
                website_url: None,
            },
            instructions: Some(
                "MCP server for container metrics analysis. Use list_nodes to discover \
                 available nodes, list_metrics to discover metrics, list_containers to find \
                 containers, and analyze_container to run analytical studies (periodicity \
                 detection, etc.)."
                    .into(),
            ),
            capabilities: ServerCapabilities::builder().enable_tools().build(),
            ..Default::default()
        }
    }
}

// --- Helper Functions ---

struct Stats {
    avg: f64,
    max: f64,
    min: f64,
    stddev: f64,
    count: usize,
    time_range: Option<TimeRange>,
}

fn compute_stats(data: Option<&Vec<ViewerTimeseriesPoint>>) -> Stats {
    let Some(points) = data else {
        return Stats {
            avg: 0.0,
            max: 0.0,
            min: 0.0,
            stddev: 0.0,
            count: 0,
            time_range: None,
        };
    };

    if points.is_empty() {
        return Stats {
            avg: 0.0,
            max: 0.0,
            min: 0.0,
            stddev: 0.0,
            count: 0,
            time_range: None,
        };
    }

    let values: Vec<f64> = points.iter().map(|p| p.value).collect();
    let n = values.len() as f64;
    let sum: f64 = values.iter().sum();
    let avg = sum / n;

    let max = values.iter().cloned().fold(f64::NEG_INFINITY, f64::max);
    let min = values.iter().cloned().fold(f64::INFINITY, f64::min);

    let variance: f64 = values.iter().map(|v| (v - avg).powi(2)).sum::<f64>() / n;
    let stddev = variance.sqrt();

    let time_range = if let (Some(first), Some(last)) = (points.first(), points.last()) {
        Some(TimeRange {
            start_ms: first.time_ms,
            end_ms: last.time_ms,
        })
    } else {
        None
    };

    Stats {
        avg,
        max,
        min,
        stddev,
        count: points.len(),
        time_range,
    }
}

/// REQ-MCP-005: Detect behavioral trend using linear regression.
fn detect_trend(data: Option<&Vec<ViewerTimeseriesPoint>>) -> String {
    let Some(points) = data else {
        return "unknown".into();
    };

    if points.len() < 2 {
        return "unknown".into();
    }

    let values: Vec<f64> = points.iter().map(|p| p.value).collect();
    let n = values.len() as f64;

    // Compute mean
    let sum_y: f64 = values.iter().sum();
    let mean = sum_y / n;

    // Compute slope via least-squares
    let sum_x: f64 = (0..values.len()).map(|i| i as f64).sum();
    let sum_xy: f64 = values
        .iter()
        .enumerate()
        .map(|(i, v)| i as f64 * v)
        .sum();
    let sum_x2: f64 = (0..values.len()).map(|i| (i * i) as f64).sum();

    let denominator = n * sum_x2 - sum_x * sum_x;
    if denominator.abs() < f64::EPSILON {
        return "stable".into();
    }

    let slope = (n * sum_xy - sum_x * sum_y) / denominator;

    // Normalize slope by mean to get relative change rate
    if mean.abs() < f64::EPSILON {
        return "stable".into();
    }

    let relative_slope = (slope / mean) * 100.0; // % change per sample

    // Compute coefficient of variation for volatility
    let variance: f64 = values.iter().map(|v| (v - mean).powi(2)).sum::<f64>() / n;
    let stddev = variance.sqrt();
    let cv = stddev / mean.abs();

    // Classify based on thresholds
    if cv > 0.3 {
        "volatile".into()
    } else if relative_slope > 1.0 {
        "increasing".into()
    } else if relative_slope < -1.0 {
        "decreasing".into()
    } else {
        "stable".into()
    }
}
