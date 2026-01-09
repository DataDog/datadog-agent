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
use axum::{
    extract::{Path, Query, State},
    http::{header, StatusCode},
    response::{Html, IntoResponse, Json, Response},
    routing::get,
    Router,
};
use tower_http::services::ServeDir;
use serde::{Deserialize, Serialize};
use std::net::SocketAddr;
use std::sync::Arc;
use tower_http::cors::CorsLayer;

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

    let app = Router::new()
        .route("/", get(index_handler))
        .route("/dashboards/:name", get(dashboard_file_handler))
        .route("/api/health", get(health_handler))
        .route("/api/instance", get(instance_handler))
        .route("/api/metrics", get(metrics_handler))
        .route("/api/filters", get(filters_handler))
        .route("/api/containers", get(containers_handler))
        .route("/api/timeseries", get(timeseries_handler))
        .route("/api/studies", get(studies_handler))
        .route("/api/study/:id", get(study_handler))
        .nest_service("/static", ServeDir::new(&static_dir))
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

// --- Handlers ---

/// Default embedded HTML (fallback if external file not found).
const EMBEDDED_INDEX_HTML: &str = include_str!("static/index.html");

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

/// Serve dashboard JSON files from the dashboards/ directory.
/// REQ-MV-033: Dashboard files are served for ?dashboard= URL parameter.
async fn dashboard_file_handler(Path(name): Path<String>) -> Response {
    // Security: only allow .json files and prevent path traversal
    if !name.ends_with(".json") || name.contains("..") || name.contains('/') {
        return (StatusCode::BAD_REQUEST, "Invalid dashboard name").into_response();
    }

    // Look for dashboards in parent of static dir (crate root) or common locations
    let candidates = [
        "dashboards",                                           // From crate root
        "q_branch/fine-grained-monitor/dashboards",             // From repo root
        "/dashboards",                                          // In-container
    ];

    for candidate in candidates {
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
