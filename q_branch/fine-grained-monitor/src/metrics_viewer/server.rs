//! HTTP server and API handlers for metrics viewer.
//!
//! REQ-MV-001: Serves interactive timeseries chart.
//! REQ-MV-002: GET /api/metrics - list available metrics.
//! REQ-MV-003: GET /api/filters - list filter options.
//! REQ-MV-003: GET /api/containers - list containers with optional filters.
//! REQ-MV-007: GET /api/studies - list available studies.
//! REQ-MV-007: GET /api/study/:id - run study on selected containers.

use crate::metrics_viewer::data::{ContainerStats, MetricInfo, TimeseriesPoint};
use crate::metrics_viewer::lazy_data::LazyDataStore;
use crate::metrics_viewer::studies::{StudyInfo, StudyRegistry, StudyResult};
use axum::{
    extract::{Path, Query, State},
    response::{Html, IntoResponse, Json},
    routing::get,
    Router,
};
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
}

impl Default for ServerConfig {
    fn default() -> Self {
        Self {
            port: 8050,
            open_browser: true,
        }
    }
}

/// Start the HTTP server.
pub async fn run_server(data: LazyDataStore, config: ServerConfig) -> anyhow::Result<()> {
    let state = Arc::new(AppState {
        data,
        studies: StudyRegistry::new(),
    });

    let app = Router::new()
        .route("/", get(index_handler))
        .route("/api/health", get(health_handler))
        .route("/api/metrics", get(metrics_handler))
        .route("/api/filters", get(filters_handler))
        .route("/api/containers", get(containers_handler))
        .route("/api/timeseries", get(timeseries_handler))
        .route("/api/studies", get(studies_handler))
        .route("/api/study/:id", get(study_handler))
        .layer(CorsLayer::permissive())
        .with_state(state);

    let addr = SocketAddr::from(([127, 0, 0, 1], config.port));

    if config.open_browser {
        let url = format!("http://{}", addr);
        eprintln!("\nOpening browser at {}", url);
        #[cfg(target_os = "macos")]
        let _ = std::process::Command::new("open").arg(&url).spawn();
        #[cfg(target_os = "linux")]
        let _ = std::process::Command::new("xdg-open").arg(&url).spawn();
    }

    eprintln!("Server running at http://{}", addr);
    eprintln!("Press Ctrl+C to stop\n");

    let listener = tokio::net::TcpListener::bind(addr).await?;
    axum::serve(listener, app).await?;

    Ok(())
}

// --- Handlers ---

/// Serve the main HTML page.
/// REQ-MV-001: Display interactive timeseries chart.
async fn index_handler() -> Html<&'static str> {
    Html(include_str!("static/index.html"))
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

/// GET /api/metrics - list available metrics.
/// REQ-MV-002: Returns list of available metric names with sample counts.
async fn metrics_handler(State(state): State<Arc<AppState>>) -> Json<MetricsResponse> {
    Json(MetricsResponse {
        metrics: state.data.index.metrics.clone(),
    })
}

#[derive(Serialize)]
struct MetricsResponse {
    metrics: Vec<MetricInfo>,
}

/// GET /api/filters - list filter options.
/// REQ-MV-003: Returns available qos_class and namespace values.
async fn filters_handler(State(state): State<Arc<AppState>>) -> Json<FiltersResponse> {
    Json(FiltersResponse {
        qos_classes: state.data.index.qos_classes.clone(),
        namespaces: state.data.index.namespaces.clone(),
    })
}

#[derive(Serialize)]
struct FiltersResponse {
    qos_classes: Vec<String>,
    namespaces: Vec<String>,
}

/// GET /api/containers - list containers for a metric with optional filters.
/// REQ-MV-003: Filter containers by qos_class and namespace.
/// REQ-MV-004: Search containers by name.
#[derive(Deserialize)]
struct ContainersQuery {
    metric: String,
    #[serde(default)]
    qos_class: Option<String>,
    #[serde(default)]
    namespace: Option<String>,
    #[serde(default)]
    search: Option<String>,
}

async fn containers_handler(
    State(state): State<Arc<AppState>>,
    Query(query): Query<ContainersQuery>,
) -> Json<ContainersResponse> {
    // Load stats on demand
    let metric_stats = match state.data.get_container_stats(&query.metric) {
        Ok(stats) => stats,
        Err(e) => {
            eprintln!("Error loading container stats: {}", e);
            return Json(ContainersResponse { containers: vec![] });
        }
    };

    let mut containers: Vec<ContainerStats> = metric_stats
        .values()
        .filter(|c| {
            // REQ-MV-003: Filter by qos_class
            if let Some(ref qos) = query.qos_class {
                if c.info.qos_class.as_ref() != Some(qos) {
                    return false;
                }
            }
            // REQ-MV-003: Filter by namespace
            if let Some(ref ns) = query.namespace {
                if c.info.namespace.as_ref() != Some(ns) {
                    return false;
                }
            }
            // REQ-MV-004: Search by container ID or pod name
            if let Some(ref search) = query.search {
                let search_lower = search.to_lowercase();
                let matches_id = c.info.short_id.to_lowercase().contains(&search_lower);
                let matches_pod = c
                    .info
                    .pod_name
                    .as_ref()
                    .map(|p| p.to_lowercase().contains(&search_lower))
                    .unwrap_or(false);
                if !matches_id && !matches_pod {
                    return false;
                }
            }
            true
        })
        .cloned()
        .collect();

    // Sort by average value descending (for Top N selection)
    containers.sort_by(|a, b| {
        b.avg
            .partial_cmp(&a.avg)
            .unwrap_or(std::cmp::Ordering::Equal)
    });

    Json(ContainersResponse { containers })
}

#[derive(Serialize)]
struct ContainersResponse {
    containers: Vec<ContainerStats>,
}

/// GET /api/timeseries - get timeseries data for selected containers.
#[derive(Deserialize)]
struct TimeseriesQuery {
    metric: String,
    containers: String,
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
    let container_ids: Vec<&str> = query.containers.split(',').collect();

    // Load timeseries on demand
    let timeseries = match state.data.get_timeseries(&query.metric, &container_ids) {
        Ok(ts) => ts,
        Err(e) => {
            eprintln!("Error loading timeseries: {}", e);
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
/// REQ-MV-007: Returns available study types.
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
/// REQ-MV-007: Analyze timeseries for patterns.
#[derive(Deserialize)]
struct StudyQuery {
    metric: String,
    containers: String,
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

    // Load timeseries on demand
    let timeseries = match state.data.get_timeseries(&query.metric, &container_ids) {
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
