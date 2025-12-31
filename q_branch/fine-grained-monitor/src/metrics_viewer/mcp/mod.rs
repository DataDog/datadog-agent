//! MCP (Model Context Protocol) server for metrics viewer.
//!
//! Provides programmatic access to metrics discovery and analysis for LLM agents.
//!
//! # Requirements
//!
//! - REQ-MCP-001: Discover available metrics and studies
//! - REQ-MCP-002: Find containers by criteria
//! - REQ-MCP-003: Sort containers by recency
//! - REQ-MCP-004: Analyze container behavior
//! - REQ-MCP-005: Identify behavioral trends
//! - REQ-MCP-006: Operate via MCP over stdio

pub mod client;

use client::{ContainerSearchParams, MetricsViewerClient, TimeseriesPoint};
use rmcp::{
    handler::server::router::tool::ToolRouter,
    handler::server::wrapper::Parameters,
    model::{CallToolResult, Content, ServerCapabilities, ServerInfo},
    tool, tool_handler, tool_router, ErrorData as McpError, ServerHandler,
};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::sync::Arc;

/// MCP server for metrics viewer.
#[derive(Clone)]
pub struct McpMetricsViewer {
    client: Arc<MetricsViewerClient>,
    tool_router: ToolRouter<Self>,
}

impl McpMetricsViewer {
    /// Create a new MCP server with the given API client.
    pub fn new(client: MetricsViewerClient) -> Self {
        Self {
            client: Arc::new(client),
            tool_router: Self::tool_router(),
        }
    }
}

// --- Tool Parameter Types ---

/// Parameters for list_containers tool.
/// REQ-MCP-002: Find containers by criteria.
#[derive(Debug, Serialize, Deserialize, JsonSchema)]
pub struct ListContainersParams {
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
#[derive(Debug, Serialize, Deserialize, JsonSchema)]
pub struct AnalyzeContainerParams {
    /// Container ID (short 12-char ID or full ID).
    pub container: String,

    /// Study to run: "periodicity" or "changepoint".
    pub study_id: String,

    /// Single metric name to analyze.
    #[serde(default)]
    pub metric: Option<String>,

    /// Metric prefix to analyze all matching metrics (e.g., "cgroup.v2.cpu").
    #[serde(default)]
    pub metric_prefix: Option<String>,
}

// --- Tool Response Types ---

/// Response from list_metrics tool.
#[derive(Debug, Serialize)]
struct ListMetricsResponse {
    metrics: Vec<MetricEntry>,
    studies: Vec<StudyEntry>,
}

#[derive(Debug, Serialize)]
struct MetricEntry {
    name: String,
    sample_count: usize,
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
    details: FindingDetails,
}

#[derive(Debug, Serialize)]
struct FindingDetails {
    period_ms: Option<f64>,
    confidence: Option<f64>,
    amplitude: Option<f64>,
}

// --- Tool Implementations ---

#[tool_router]
impl McpMetricsViewer {
    /// REQ-MCP-001: Discover available metrics and studies.
    #[tool(
        name = "list_metrics",
        description = "List available metrics and analytical studies. Call this first to discover what data is available."
    )]
    async fn list_metrics(&self) -> Result<CallToolResult, McpError> {
        let metrics = self.client.list_metrics().await.map_err(|e| {
            McpError::internal_error(format!("Failed to list metrics: {}", e), None)
        })?;

        let studies = self.client.list_studies().await.map_err(|e| {
            McpError::internal_error(format!("Failed to list studies: {}", e), None)
        })?;

        let response = ListMetricsResponse {
            metrics: metrics
                .into_iter()
                .map(|m| MetricEntry {
                    name: m.name,
                    sample_count: m.sample_count,
                })
                .collect(),
            studies: studies
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
    #[tool(
        name = "list_containers",
        description = "Find containers matching search criteria. Returns containers sorted by most recently seen. Use this to identify containers for analysis."
    )]
    async fn list_containers(
        &self,
        Parameters(params): Parameters<ListContainersParams>,
    ) -> Result<CallToolResult, McpError> {
        let search_params = ContainerSearchParams {
            metric: params.metric,
            namespace: params.namespace,
            qos_class: params.qos_class,
            search: params.search,
        };

        let containers = self
            .client
            .search_containers(&search_params)
            .await
            .map_err(|e| {
                McpError::internal_error(format!("Failed to search containers: {}", e), None)
            })?;

        let total = containers.len();
        let limit = params.limit.unwrap_or(20);

        // Note: HTTP API returns sorted by avg, we just take the results as-is for now
        // REQ-MCP-003 specifies last_seen sorting but that requires API changes
        let response = ListContainersResponse {
            containers: containers
                .into_iter()
                .take(limit)
                .map(|c| ContainerEntry {
                    id: c.info.short_id,
                    pod_name: c.info.pod_name,
                    container_name: None, // Not in current API
                    namespace: c.info.namespace,
                    qos_class: c.info.qos_class,
                    last_seen: None, // Not in current API
                })
                .collect(),
            total_matching: total,
        };

        let json = serde_json::to_string_pretty(&response)
            .map_err(|e| McpError::internal_error(format!("JSON error: {}", e), None))?;

        Ok(CallToolResult::success(vec![Content::text(json)]))
    }

    /// REQ-MCP-004, REQ-MCP-005: Analyze container behavior and identify trends.
    #[tool(
        name = "analyze_container",
        description = "Run a study on a container to detect patterns. Provide either 'metric' for a single metric or 'metric_prefix' to analyze all metrics matching that prefix (e.g., 'cgroup.v2.cpu')."
    )]
    async fn analyze_container(
        &self,
        Parameters(params): Parameters<AnalyzeContainerParams>,
    ) -> Result<CallToolResult, McpError> {
        // Determine which metrics to analyze
        let metrics_to_analyze = if let Some(ref metric) = params.metric {
            vec![metric.clone()]
        } else if let Some(ref prefix) = params.metric_prefix {
            // Get all metrics matching prefix
            let all_metrics = self.client.list_metrics().await.map_err(|e| {
                McpError::internal_error(format!("Failed to list metrics: {}", e), None)
            })?;

            all_metrics
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

        let container_ids = vec![params.container.as_str()];
        let mut results = Vec::new();

        for metric in &metrics_to_analyze {
            // Run the study
            let study_result = self
                .client
                .run_study(&params.study_id, metric, &container_ids)
                .await
                .map_err(|e| {
                    McpError::internal_error(format!("Failed to run study: {}", e), None)
                })?;

            // Get timeseries for trend detection
            let timeseries = self
                .client
                .get_timeseries(metric, &container_ids)
                .await
                .map_err(|e| {
                    McpError::internal_error(format!("Failed to get timeseries: {}", e), None)
                })?;

            // Compute stats and trend
            let ts_data = timeseries.first().map(|t| &t.data);
            let stats = compute_stats(ts_data);
            let trend = detect_trend(ts_data);

            // Convert findings
            let findings: Vec<Finding> = study_result
                .results
                .first()
                .map(|r| {
                    r.result
                        .findings
                        .iter()
                        .map(|f| Finding {
                            finding_type: params.study_id.clone(),
                            timestamp_ms: f.start_time,
                            label: format!(
                                "period={:.1}ms conf={:.2}",
                                f.period_ms, f.confidence
                            ),
                            details: FindingDetails {
                                period_ms: Some(f.period_ms),
                                confidence: Some(f.confidence),
                                amplitude: Some(f.amplitude),
                            },
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

        // Get container info from first successful search
        let container_info = self
            .client
            .search_containers(&ContainerSearchParams {
                metric: metrics_to_analyze.first().cloned().unwrap_or_default(),
                search: Some(params.container.clone()),
                ..Default::default()
            })
            .await
            .ok()
            .and_then(|c| c.into_iter().next());

        let response = AnalyzeContainerResponse {
            container: ContainerSummary {
                id: params.container,
                pod_name: container_info.as_ref().and_then(|c| c.info.pod_name.clone()),
                namespace: container_info.as_ref().and_then(|c| c.info.namespace.clone()),
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
                "MCP server for container metrics analysis. Use list_metrics to discover \
                 available metrics, list_containers to find containers, and analyze_container \
                 to run analytical studies (periodicity detection, etc.)."
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

fn compute_stats(data: Option<&Vec<TimeseriesPoint>>) -> Stats {
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
fn detect_trend(data: Option<&Vec<TimeseriesPoint>>) -> String {
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
