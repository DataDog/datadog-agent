//! HTTP server and API handlers for metrics viewer.
//!
//! REQ-MV-001: Serves interactive timeseries chart.
//! REQ-MV-002: GET /api/metrics - list available metrics.
//! REQ-MV-003: GET /api/containers - list and search containers.
//! REQ-MV-006: GET /api/studies - list available studies.
//! REQ-MV-006: GET /api/study/:id - run study on selected containers.

use crate::metrics_viewer::data::{ContainerInfo, MetricInfo, TimeRange, TimeseriesPoint};
use crate::metrics_viewer::lazy_data::LazyDataStore;
use crate::metrics_viewer::studies::{StudyInfo, StudyRegistry, StudyResult};
use arrow::array::{ArrayRef, Float64Builder, StringBuilder, TimestampMillisecondBuilder};
use arrow::datatypes::{DataType, Field, Schema, TimeUnit};
use arrow::record_batch::RecordBatch;
use axum::{
    extract::{Path, Query, State},
    http::{header, StatusCode},
    response::{Html, IntoResponse, Json, Response},
    routing::get,
    Router,
};
use parquet::arrow::ArrowWriter;
use parquet::basic::Compression;
use parquet::file::properties::WriterProperties;
use serde::{Deserialize, Serialize};
use std::net::SocketAddr;
use std::sync::Arc;
use tower_http::cors::CorsLayer;
use tower_http::services::ServeDir;

/// Application state shared across handlers.
pub struct AppState {
    pub data: LazyDataStore,
    pub studies: StudyRegistry,
}

/// Server configuration.
pub struct ServerConfig {
    pub port: u16,
    pub open_browser: bool,
    /// Interval in seconds for background sidecar refresh (0 to disable).
    pub refresh_interval_secs: u64,
}

impl Default for ServerConfig {
    fn default() -> Self {
        Self {
            port: 8050,
            open_browser: true,
            refresh_interval_secs: 30,
        }
    }
}

/// Start the HTTP server.
pub async fn run_server(data: LazyDataStore, config: ServerConfig) -> anyhow::Result<()> {
    let state = Arc::new(AppState {
        data,
        studies: StudyRegistry::new(),
    });

    // Spawn background sidecar refresh task
    if config.refresh_interval_secs > 0 {
        let refresh_state = state.clone();
        let interval_secs = config.refresh_interval_secs;
        tokio::spawn(async move {
            let mut interval = tokio::time::interval(std::time::Duration::from_secs(interval_secs));
            interval.tick().await; // Skip immediate first tick
            loop {
                interval.tick().await;
                refresh_state.data.refresh_containers_from_sidecars();
            }
        });
        eprintln!("Background sidecar refresh enabled (every {}s)", interval_secs);
    }

    // Find the static files directory
    let static_dir = find_static_dir();

    // Find testdata directory for snapshot testing (dev mode only)
    let testdata_dir = find_testdata_dir();

    let mut app = Router::new()
        .route("/", get(index_handler))
        .route("/snapshot-test", get(snapshot_test_handler))
        .route("/dashboards/:name", get(dashboard_file_handler))
        .route("/api/health", get(health_handler))
        .route("/api/instance", get(instance_handler))
        .route("/api/metrics", get(metrics_handler))
        .route("/api/filters", get(filters_handler))
        .route("/api/containers", get(containers_handler))
        .route("/api/timeseries", get(timeseries_handler))
        .route("/api/studies", get(studies_handler))
        .route("/api/study/:id", get(study_handler))
        .route("/api/export", get(export_handler))
        .nest_service("/static", ServeDir::new(&static_dir));

    // Serve testdata for snapshot testing (dev mode only)
    if let Some(ref testdata) = testdata_dir {
        eprintln!("Serving testdata from {} at /testdata/*", testdata);
        app = app.nest_service("/testdata", ServeDir::new(testdata));
    }

    let app = app
        .layer(CorsLayer::permissive())
        .with_state(state);

    // Bind to 0.0.0.0 to allow access from other pods (MCP server)
    let addr = SocketAddr::from(([0, 0, 0, 0], config.port));

    if config.open_browser {
        // Use localhost for browser URL even though we bind to 0.0.0.0
        let url = format!("http://127.0.0.1:{}", config.port);
        eprintln!("\nOpening browser at {}", url);
        #[cfg(target_os = "macos")]
        let _ = std::process::Command::new("open").arg(&url).spawn();
        #[cfg(target_os = "linux")]
        let _ = std::process::Command::new("xdg-open").arg(&url).spawn();
    }

    eprintln!("Server running at http://0.0.0.0:{}", config.port);
    eprintln!("Press Ctrl+C to stop\n");

    let listener = tokio::net::TcpListener::bind(addr).await?;
    axum::serve(listener, app).await?;

    Ok(())
}

// --- Helpers ---

/// Find the static files directory.
/// Checks in-container path first, then local dev path.
fn find_static_dir() -> String {
    let candidates = [
        "/static",                          // In-container path
        "src/metrics_viewer/static",        // Local dev path (from crate root)
    ];

    for path in candidates {
        if std::path::Path::new(path).exists() {
            return path.to_string();
        }
    }

    // Default to in-container path (ServeDir will handle missing gracefully)
    "/static".to_string()
}

/// Find the testdata directory for snapshot testing (dev mode only).
fn find_testdata_dir() -> Option<String> {
    let candidates = [
        "testdata",                         // Local dev path (from crate root)
        "q_branch/fine-grained-monitor/testdata", // From repo root
    ];

    for path in candidates {
        if std::path::Path::new(path).exists() {
            return Some(path.to_string());
        }
    }

    None
}

// --- Handlers ---

/// Default embedded HTML (fallback if external file not found).
const EMBEDDED_INDEX_HTML: &str = include_str!("static/index.html");

/// Embedded snapshot test HTML.
const EMBEDDED_SNAPSHOT_TEST_HTML: &str = include_str!("static/snapshot-test.html");

/// Serve the snapshot test page.
async fn snapshot_test_handler() -> Html<String> {
    // Check for external static file first
    let external_paths = [
        "/static/snapshot-test.html",
        "src/metrics_viewer/static/snapshot-test.html",
    ];

    for path in external_paths {
        if let Ok(content) = std::fs::read_to_string(path) {
            return Html(content);
        }
    }

    Html(EMBEDDED_SNAPSHOT_TEST_HTML.to_string())
}

/// Serve the main HTML page.
/// REQ-MV-001: Display interactive timeseries chart.
///
/// Checks for external file first (for fast iteration), falls back to embedded.
async fn index_handler() -> Html<String> {
    // Check for external static file (allows hot-reload without recompilation)
    let external_paths = [
        "/static/index.html",           // In-container path
        "src/metrics_viewer/static/index.html", // Local dev path
    ];

    for path in external_paths {
        if let Ok(content) = std::fs::read_to_string(path) {
            return Html(content);
        }
    }

    // Fall back to embedded version
    Html(EMBEDDED_INDEX_HTML.to_string())
}

/// Serve dashboard JSON files.
/// REQ-MV-033: Dashboard files are served for ?dashboard= URL parameter.
///
/// Searches in order:
/// 1. scenarios/<name>/dashboard.json (new location, dashboards nested under scenarios)
/// 2. dashboards/<name>.json (legacy location, for backwards compatibility)
async fn dashboard_file_handler(Path(name): Path<String>) -> Response {
    // Security: only allow .json files and prevent path traversal
    if !name.ends_with(".json") || name.contains("..") || name.contains('/') {
        return (StatusCode::BAD_REQUEST, "Invalid dashboard name").into_response();
    }

    // Extract scenario name (strip .json extension)
    let scenario_name = name.trim_end_matches(".json");

    // Priority 1: Look in scenarios/<name>/dashboard.json (new location)
    let scenario_candidates = [
        format!("scenarios/{}/dashboard.json", scenario_name),
        format!("q_branch/fine-grained-monitor/scenarios/{}/dashboard.json", scenario_name),
        format!("/scenarios/{}/dashboard.json", scenario_name),
    ];

    for candidate in scenario_candidates {
        let path = std::path::Path::new(&candidate);
        if path.exists() {
            match std::fs::read_to_string(path) {
                Ok(content) => {
                    return (
                        [(header::CONTENT_TYPE, "application/json")],
                        content,
                    ).into_response();
                }
                Err(e) => {
                    eprintln!("Failed to read dashboard {}: {}", path.display(), e);
                }
            }
        }
    }

    // Priority 2: Legacy location in dashboards/ directory (backwards compatibility)
    let legacy_candidates = [
        "dashboards",
        "q_branch/fine-grained-monitor/dashboards",
        "/dashboards",
    ];

    for candidate in legacy_candidates {
        let path = std::path::PathBuf::from(candidate).join(&name);
        if path.exists() {
            match std::fs::read_to_string(&path) {
                Ok(content) => {
                    return (
                        [(header::CONTENT_TYPE, "application/json")],
                        content,
                    ).into_response();
                }
                Err(e) => {
                    eprintln!("Failed to read dashboard {}: {}", path.display(), e);
                }
            }
        }
    }

    (StatusCode::NOT_FOUND, format!("Dashboard not found: {}", name)).into_response()
}

/// GET /api/health - health check endpoint for dev tooling.
async fn health_handler() -> Json<HealthResponse> {
    Json(HealthResponse {
        status: "ok".to_string(),
    })
}

#[derive(Serialize)]
struct HealthResponse {
    status: String,
}

/// GET /api/instance - instance info for in-cluster identification.
/// Returns pod name and node name from environment variables when running in Kubernetes.
async fn instance_handler() -> Json<InstanceResponse> {
    Json(InstanceResponse {
        pod_name: std::env::var("POD_NAME").ok(),
        node_name: std::env::var("NODE_NAME").ok(),
        cluster_name: std::env::var("CLUSTER_NAME").ok(),
    })
}

#[derive(Serialize)]
struct InstanceResponse {
    pod_name: Option<String>,
    node_name: Option<String>,
    cluster_name: Option<String>,
}

/// Priority metrics shown at top of list (most useful for container monitoring).
/// Order matters - these appear first in the UI.
const PRIORITY_METRICS: &[&str] = &[
    "cpu_percentage",
    "total_cpu_usage_millicores",
    "cpu_limit_millicores",
    "user_cpu_percentage",
    "kernel_cpu_percentage",
    "system_cpu_percentage",
    "smaps_rollup.pss",
    "cgroup.v2.cpu.stat.throttled_usec",
    "cgroup.v2.cpu.stat.nr_throttled",
    "cgroup.v2.cpu.pressure.some.avg10",
    "cgroup.v2.memory.current",
    "cgroup.v2.memory.max",
    "cgroup.v2.memory.peak",
    "cgroup.v2.memory.stat.anon",
    "cgroup.v2.memory.stat.file",
    "cgroup.v2.memory.events.oom_kill",
    "cgroup.v2.memory.pressure.some.avg10",
    "container.pid_count",
    "cgroup.v2.pids.current",
    "cgroup.v2.cgroup.threads",
    "cgroup.v2.io.stat.rbytes",
    "cgroup.v2.io.stat.wbytes",
    "cgroup.v2.io.pressure.some.avg10",
    "cgroup.v2.memory.swap.current",
    "cgroup.v2.memory.stat.pgmajfault",
];

/// GET /api/metrics - list available metrics.
/// REQ-MV-002: Returns list of available metric names with sample counts.
/// Priority metrics appear first, then remaining metrics alphabetically.
async fn metrics_handler(State(state): State<Arc<AppState>>) -> Json<MetricsResponse> {
    let mut metrics = state.data.get_metrics();

    // Sort: priority metrics first (in order), then alphabetically
    metrics.sort_by(|a, b| {
        let a_priority = PRIORITY_METRICS.iter().position(|&m| m == a.name);
        let b_priority = PRIORITY_METRICS.iter().position(|&m| m == b.name);

        match (a_priority, b_priority) {
            (Some(a_idx), Some(b_idx)) => a_idx.cmp(&b_idx),
            (Some(_), None) => std::cmp::Ordering::Less,
            (None, Some(_)) => std::cmp::Ordering::Greater,
            (None, None) => a.name.cmp(&b.name),
        }
    });

    Json(MetricsResponse { metrics })
}

#[derive(Serialize)]
struct MetricsResponse {
    metrics: Vec<MetricInfo>,
}

/// GET /api/filters - list filter options.
/// Returns available qos_class and namespace values for filtering.
async fn filters_handler(State(state): State<Arc<AppState>>) -> Json<FiltersResponse> {
    Json(FiltersResponse {
        qos_classes: state.data.get_qos_classes(),
        namespaces: state.data.get_namespaces(),
    })
}

#[derive(Serialize)]
struct FiltersResponse {
    qos_classes: Vec<String>,
    namespaces: Vec<String>,
}

/// GET /api/containers - list containers for a metric with optional filters.
/// REQ-MV-003: Search and select containers by name, qos_class, or namespace.
/// REQ-MV-019: Containers sorted by last_seen (most recent first) - instant response.
/// REQ-MV-032: Filter by labels (key:value pairs).
/// REQ-MV-037: Filter by time range (1h, 1d, 1w, all).
#[derive(Deserialize)]
struct ContainersQuery {
    #[allow(dead_code)]
    metric: String, // Kept for API compatibility, but not used for filtering anymore
    #[serde(default)]
    qos_class: Option<String>,
    #[serde(default)]
    namespace: Option<String>,
    #[serde(default)]
    search: Option<String>,
    #[serde(default)]
    labels: Option<String>, // REQ-MV-032: comma-separated key:value pairs
    #[serde(default)]
    range: Option<TimeRange>, // REQ-MV-037: time range filter (defaults to 1h)
}

async fn containers_handler(
    State(state): State<Arc<AppState>>,
    Query(query): Query<ContainersQuery>,
) -> Json<ContainersResponse> {
    // REQ-MV-037: Use specified time range or default to 1 hour
    let time_range = query.range.unwrap_or_default();

    // REQ-MV-019: Get containers from index sorted by last_seen (instant!)
    // REQ-MV-037: Filtered to containers with data in the specified time range
    let all_containers = state.data.get_containers_by_recency(time_range);

    // REQ-MV-032: Parse label filters if provided
    let label_filters: Vec<(&str, &str)> = query
        .labels
        .as_ref()
        .map(|labels_str| {
            labels_str
                .split(',')
                .filter_map(|kv| kv.split_once(':'))
                .collect()
        })
        .unwrap_or_default();

    let containers: Vec<ContainerInfo> = all_containers
        .into_iter()
        .filter(|c| {
            // REQ-MV-003: Filter by qos_class
            if let Some(ref qos) = query.qos_class {
                if c.qos_class.as_ref() != Some(qos) {
                    return false;
                }
            }
            // REQ-MV-003: Filter by namespace
            if let Some(ref ns) = query.namespace {
                if c.namespace.as_ref() != Some(ns) {
                    return false;
                }
            }
            // REQ-MV-003: Search by container ID, pod name, or container name
            if let Some(ref search) = query.search {
                let search_lower = search.to_lowercase();
                let matches_id = c.short_id.to_lowercase().contains(&search_lower);
                let matches_pod = c
                    .pod_name
                    .as_ref()
                    .map(|p| p.to_lowercase().contains(&search_lower))
                    .unwrap_or(false);
                let matches_container = c
                    .container_name
                    .as_ref()
                    .map(|n| n.to_lowercase().contains(&search_lower))
                    .unwrap_or(false);
                if !matches_id && !matches_pod && !matches_container {
                    return false;
                }
            }
            // REQ-MV-032: Filter by labels (all specified labels must match)
            if !label_filters.is_empty() {
                if let Some(ref container_labels) = c.labels {
                    if !label_filters.iter().all(|(k, v)| {
                        container_labels.get(*k).map(|lv| lv == *v).unwrap_or(false)
                    }) {
                        return false;
                    }
                } else {
                    // Container has no labels but filter requires labels
                    return false;
                }
            }
            true
        })
        .collect();

    // Already sorted by last_seen from get_containers_by_recency()
    Json(ContainersResponse { containers })
}

#[derive(Serialize)]
struct ContainersResponse {
    containers: Vec<ContainerInfo>,
}

/// GET /api/timeseries - get timeseries data for selected containers.
/// REQ-MV-037: Supports time range filtering.
#[derive(Deserialize)]
struct TimeseriesQuery {
    metric: String,
    containers: String,
    #[serde(default)]
    range: Option<TimeRange>, // REQ-MV-037: time range filter (defaults to 1h)
}

#[derive(Serialize)]
struct TimeseriesResponse {
    container: String,
    data: Vec<TimeseriesPoint>,
}

async fn timeseries_handler(
    State(state): State<Arc<AppState>>,
    Query(query): Query<TimeseriesQuery>,
) -> Json<Vec<TimeseriesResponse>> {
    // REQ-MV-037: Use specified time range or default to 1 hour
    let time_range = query.range.unwrap_or_default();

    let container_ids: Vec<&str> = query.containers.split(',').collect();

    // Load timeseries on demand for the specified time range
    let timeseries = match state.data.get_timeseries(&query.metric, &container_ids, time_range) {
        Ok(ts) => ts,
        Err(e) => {
            eprintln!("Error loading timeseries for metric={} containers={} range={}: {}", query.metric, query.containers, time_range, e);
            return Json(vec![]);
        }
    };

    let result: Vec<TimeseriesResponse> = timeseries
        .into_iter()
        .map(|(container, data)| TimeseriesResponse { container, data })
        .collect();

    Json(result)
}

/// GET /api/studies - list available studies.
/// REQ-MV-006: Returns available study types.
async fn studies_handler(State(state): State<Arc<AppState>>) -> Json<StudiesResponse> {
    Json(StudiesResponse {
        studies: state.studies.list(),
    })
}

#[derive(Serialize)]
struct StudiesResponse {
    studies: Vec<StudyInfo>,
}

/// GET /api/study/:id - run a study on selected containers.
/// REQ-MV-006: Analyze timeseries for patterns.
/// REQ-MV-037: Supports time range filtering.
#[derive(Deserialize)]
struct StudyQuery {
    metric: String,
    containers: String,
    #[serde(default)]
    range: Option<TimeRange>, // REQ-MV-037: time range filter (defaults to 1h)
}

#[derive(Serialize)]
struct StudyResponse {
    study: String,
    results: Vec<ContainerStudyResult>,
}

#[derive(Serialize)]
struct ContainerStudyResult {
    container: String,
    #[serde(flatten)]
    result: StudyResult,
}

async fn study_handler(
    State(state): State<Arc<AppState>>,
    Path(study_id): Path<String>,
    Query(query): Query<StudyQuery>,
) -> impl IntoResponse {
    // REQ-MV-037: Use specified time range or default to 1 hour
    let time_range = query.range.unwrap_or_default();

    let study = match state.studies.get(&study_id) {
        Some(s) => s,
        None => {
            return Json(StudyResponse {
                study: study_id,
                results: vec![],
            });
        }
    };

    let container_ids: Vec<&str> = query.containers.split(',').collect();

    // Load timeseries on demand for the specified time range
    let timeseries = match state.data.get_timeseries(&query.metric, &container_ids, time_range) {
        Ok(ts) => ts,
        Err(e) => {
            eprintln!("Error loading timeseries for study: {}", e);
            return Json(StudyResponse {
                study: study_id,
                results: vec![],
            });
        }
    };

    let mut results: Vec<ContainerStudyResult> = Vec::new();

    for (container, data) in timeseries {
        let result = study.analyze(&data);
        results.push(ContainerStudyResult { container, result });
    }

    Json(StudyResponse {
        study: study_id,
        results,
    })
}

// ============================================================================
// Export API - Generate filtered parquet file for offline viewing
// ============================================================================

/// Memory protection limits for export.
const MAX_EXPORT_ROWS: usize = 10_000_000;
const MAX_EXPORT_CONTAINERS: usize = 100;

/// GET /api/export - export filtered timeseries data as a parquet file.
/// Returns a downloadable parquet file containing timeseries data matching the filter criteria.
#[derive(Deserialize)]
struct ExportQuery {
    /// K8s namespace filter (optional)
    #[serde(default)]
    namespace: Option<String>,

    /// Comma-separated key:value label pairs for filtering (optional)
    /// Example: "app:nginx,env:prod"
    #[serde(default)]
    labels: Option<String>,

    /// Comma-separated list of metric names to include (optional, defaults to all)
    /// Example: "cpu_percentage,cgroup.v2.memory.current"
    #[serde(default)]
    metrics: Option<String>,

    /// Start time in epoch milliseconds (optional)
    #[serde(default)]
    time_from_ms: Option<i64>,

    /// End time in epoch milliseconds (optional)
    #[serde(default)]
    time_to_ms: Option<i64>,

    /// Time range shorthand (1h, 1d, 1w, all) - alternative to explicit timestamps
    #[serde(default)]
    range: Option<TimeRange>,
}

/// Export-specific errors with appropriate HTTP status codes.
enum ExportError {
    /// No containers match the filter criteria
    NoMatchingContainers,
    /// No data found for the specified time range
    NoData,
    /// Too many containers requested
    TooManyContainers(usize),
    /// Too much data requested
    TooMuchData(usize),
    /// Internal error during parquet generation
    ParquetError(String),
}

impl IntoResponse for ExportError {
    fn into_response(self) -> Response {
        let (status, message) = match self {
            ExportError::NoMatchingContainers => {
                (StatusCode::NOT_FOUND, "No containers match the specified filters".to_string())
            }
            ExportError::NoData => {
                (StatusCode::NOT_FOUND, "No data found in the specified time range".to_string())
            }
            ExportError::TooManyContainers(count) => {
                (StatusCode::BAD_REQUEST, format!(
                    "Too many containers ({}), limit is {}. Use more specific filters.",
                    count, MAX_EXPORT_CONTAINERS
                ))
            }
            ExportError::TooMuchData(rows) => {
                (StatusCode::BAD_REQUEST, format!(
                    "Too much data ({} rows), limit is {}. Use a shorter time range.",
                    rows, MAX_EXPORT_ROWS
                ))
            }
            ExportError::ParquetError(e) => {
                (StatusCode::INTERNAL_SERVER_ERROR, format!("Parquet error: {}", e))
            }
        };
        (status, message).into_response()
    }
}

/// Handler for GET /api/export
/// Returns a parquet file as a downloadable binary response with appropriate headers.
async fn export_handler(
    State(state): State<Arc<AppState>>,
    Query(query): Query<ExportQuery>,
) -> Result<Response, ExportError> {
    // Use specified time range or default to all
    let time_range = query.range.unwrap_or(TimeRange::All);

    // Get containers filtered by time range
    let all_containers = state.data.get_containers_by_recency(time_range);

    // Parse label filters if provided
    let label_filters: Vec<(&str, &str)> = query
        .labels
        .as_ref()
        .map(|labels_str| {
            labels_str
                .split(',')
                .filter_map(|kv| kv.split_once(':'))
                .collect()
        })
        .unwrap_or_default();

    // Filter containers by namespace and labels
    let filtered_containers: Vec<&ContainerInfo> = all_containers
        .iter()
        .filter(|c| {
            // Namespace filter
            if let Some(ref ns) = query.namespace {
                if c.namespace.as_ref() != Some(ns) {
                    return false;
                }
            }
            // Label filters (all specified labels must match)
            if !label_filters.is_empty() {
                if let Some(ref container_labels) = c.labels {
                    if !label_filters.iter().all(|(k, v)| {
                        container_labels.get(*k).map(|lv| lv == *v).unwrap_or(false)
                    }) {
                        return false;
                    }
                } else {
                    return false;
                }
            }
            true
        })
        .collect();

    if filtered_containers.is_empty() {
        return Err(ExportError::NoMatchingContainers);
    }

    // Check container limit
    if filtered_containers.len() > MAX_EXPORT_CONTAINERS {
        return Err(ExportError::TooManyContainers(filtered_containers.len()));
    }

    // Get container IDs
    let container_ids: Vec<&str> = filtered_containers
        .iter()
        .map(|c| c.short_id.as_str())
        .collect();

    // Get available metrics or use requested subset
    let all_metrics = state.data.get_metrics();
    let requested_metrics: Option<Vec<&str>> = query
        .metrics
        .as_ref()
        .map(|m| m.split(',').map(|s| s.trim()).collect());

    let metrics_to_export: Vec<&str> = match requested_metrics {
        Some(ref requested) => all_metrics
            .iter()
            .filter(|m| requested.contains(&m.name.as_str()))
            .map(|m| m.name.as_str())
            .collect(),
        None => all_metrics.iter().map(|m| m.name.as_str()).collect(),
    };

    // Build a lookup map from short_id to container info
    let container_map: std::collections::HashMap<&str, &ContainerInfo> = filtered_containers
        .iter()
        .map(|c| (c.short_id.as_str(), *c))
        .collect();

    // Collect all data points
    struct ExportRow {
        time_ms: i64,
        metric_name: String,
        value: f64,
        container_id: String,
        container_name: Option<String>,
        pod_name: Option<String>,
        namespace: Option<String>,
        qos_class: Option<String>,
    }

    let mut all_data: Vec<ExportRow> = Vec::new();

    for metric in &metrics_to_export {
        let timeseries = match state.data.get_timeseries(metric, &container_ids, time_range) {
            Ok(ts) => ts,
            Err(e) => {
                eprintln!("Error loading timeseries for export metric={}: {}", metric, e);
                continue;
            }
        };

        for (container_id, points) in timeseries {
            let container = container_map.get(container_id.as_str());

            for point in points {
                // Apply explicit time bounds if provided
                if let Some(from) = query.time_from_ms {
                    if point.time_ms < from {
                        continue;
                    }
                }
                if let Some(to) = query.time_to_ms {
                    if point.time_ms > to {
                        continue;
                    }
                }

                all_data.push(ExportRow {
                    time_ms: point.time_ms,
                    metric_name: metric.to_string(),
                    value: point.value,
                    container_id: container_id.clone(),
                    container_name: container.and_then(|c| c.container_name.clone()),
                    pod_name: container.and_then(|c| c.pod_name.clone()),
                    namespace: container.and_then(|c| c.namespace.clone()),
                    qos_class: container.and_then(|c| c.qos_class.clone()),
                });

                // Check row limit
                if all_data.len() > MAX_EXPORT_ROWS {
                    return Err(ExportError::TooMuchData(all_data.len()));
                }
            }
        }
    }

    if all_data.is_empty() {
        return Err(ExportError::NoData);
    }

    // Build Arrow schema
    let schema = Arc::new(Schema::new(vec![
        Field::new("time", DataType::Timestamp(TimeUnit::Millisecond, None), false),
        Field::new("metric_name", DataType::Utf8, false),
        Field::new("value_float", DataType::Float64, true),
        Field::new("l_container_id", DataType::Utf8, true),
        Field::new("l_container_name", DataType::Utf8, true),
        Field::new("l_pod_name", DataType::Utf8, true),
        Field::new("l_namespace", DataType::Utf8, true),
        Field::new("l_qos_class", DataType::Utf8, true),
    ]));

    // Build Arrow arrays
    let capacity = all_data.len();
    let mut time_builder = TimestampMillisecondBuilder::with_capacity(capacity);
    let mut metric_builder = StringBuilder::with_capacity(capacity, capacity * 30);
    let mut value_builder = Float64Builder::with_capacity(capacity);
    let mut container_id_builder = StringBuilder::with_capacity(capacity, capacity * 12);
    let mut container_name_builder = StringBuilder::with_capacity(capacity, capacity * 20);
    let mut pod_name_builder = StringBuilder::with_capacity(capacity, capacity * 30);
    let mut namespace_builder = StringBuilder::with_capacity(capacity, capacity * 15);
    let mut qos_class_builder = StringBuilder::with_capacity(capacity, capacity * 12);

    for row in &all_data {
        time_builder.append_value(row.time_ms);
        metric_builder.append_value(&row.metric_name);
        value_builder.append_value(row.value);
        container_id_builder.append_value(&row.container_id);
        container_name_builder.append_option(row.container_name.as_deref());
        pod_name_builder.append_option(row.pod_name.as_deref());
        namespace_builder.append_option(row.namespace.as_deref());
        qos_class_builder.append_option(row.qos_class.as_deref());
    }

    let arrays: Vec<ArrayRef> = vec![
        Arc::new(time_builder.finish()),
        Arc::new(metric_builder.finish()),
        Arc::new(value_builder.finish()),
        Arc::new(container_id_builder.finish()),
        Arc::new(container_name_builder.finish()),
        Arc::new(pod_name_builder.finish()),
        Arc::new(namespace_builder.finish()),
        Arc::new(qos_class_builder.finish()),
    ];

    let batch = RecordBatch::try_new(schema.clone(), arrays)
        .map_err(|e| ExportError::ParquetError(e.to_string()))?;

    // Write to in-memory buffer with ZSTD compression
    let mut buffer = Vec::new();
    let props = WriterProperties::builder()
        .set_compression(Compression::ZSTD(
            parquet::basic::ZstdLevel::try_new(3).unwrap(),
        ))
        .build();

    {
        let mut writer = ArrowWriter::try_new(&mut buffer, schema, Some(props))
            .map_err(|e| ExportError::ParquetError(e.to_string()))?;

        writer
            .write(&batch)
            .map_err(|e| ExportError::ParquetError(e.to_string()))?;

        writer
            .close()
            .map_err(|e| ExportError::ParquetError(e.to_string()))?;
    }

    // Generate filename with timestamp
    let filename = format!(
        "fgm-export-{}.parquet",
        chrono::Utc::now().format("%Y%m%dT%H%M%SZ")
    );

    eprintln!(
        "[export] Generated {} rows, {} bytes for {} containers, {} metrics",
        all_data.len(),
        buffer.len(),
        filtered_containers.len(),
        metrics_to_export.len()
    );

    // Return as downloadable binary
    Ok((
        [
            (header::CONTENT_TYPE, "application/octet-stream"),
            (
                header::CONTENT_DISPOSITION,
                &format!("attachment; filename=\"{}\"", filename),
            ),
        ],
        buffer,
    )
        .into_response())
}
