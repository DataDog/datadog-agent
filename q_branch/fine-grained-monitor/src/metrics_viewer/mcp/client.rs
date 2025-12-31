//! HTTP client wrapper for metrics-viewer API.
//!
//! REQ-MCP-001: Calls /api/metrics and /api/studies for discovery.
//! REQ-MCP-002: Calls /api/containers for container search.
//! REQ-MCP-004: Calls /api/study/:id for analysis.

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use std::time::Duration;

/// HTTP client for the metrics-viewer API.
#[derive(Clone)]
pub struct MetricsViewerClient {
    base_url: String,
    client: reqwest::Client,
}

impl MetricsViewerClient {
    /// Create a new client with the given base URL.
    pub fn new(base_url: &str) -> Result<Self> {
        let client = reqwest::Client::builder()
            .timeout(Duration::from_secs(30))
            .build()
            .context("Failed to create HTTP client")?;

        Ok(Self {
            base_url: base_url.trim_end_matches('/').to_string(),
            client,
        })
    }

    /// REQ-MCP-001: List available metrics.
    pub async fn list_metrics(&self) -> Result<Vec<MetricInfo>> {
        let url = format!("{}/api/metrics", self.base_url);
        let resp: MetricsResponse = self
            .client
            .get(&url)
            .send()
            .await
            .context("Failed to fetch metrics")?
            .json()
            .await
            .context("Failed to parse metrics response")?;
        Ok(resp.metrics)
    }

    /// REQ-MCP-001: List available studies.
    pub async fn list_studies(&self) -> Result<Vec<StudyInfo>> {
        let url = format!("{}/api/studies", self.base_url);
        let resp: StudiesResponse = self
            .client
            .get(&url)
            .send()
            .await
            .context("Failed to fetch studies")?
            .json()
            .await
            .context("Failed to parse studies response")?;
        Ok(resp.studies)
    }

    /// REQ-MCP-002: List available filter options (namespaces, qos_classes).
    pub async fn list_filters(&self) -> Result<FiltersResponse> {
        let url = format!("{}/api/filters", self.base_url);
        self.client
            .get(&url)
            .send()
            .await
            .context("Failed to fetch filters")?
            .json()
            .await
            .context("Failed to parse filters response")
    }

    /// REQ-MCP-002, REQ-MCP-003: Search containers by criteria.
    pub async fn search_containers(&self, params: &ContainerSearchParams) -> Result<Vec<ContainerInfo>> {
        let mut url = format!("{}/api/containers?metric={}", self.base_url, params.metric);

        if let Some(ref ns) = params.namespace {
            url.push_str(&format!("&namespace={}", ns));
        }
        if let Some(ref qos) = params.qos_class {
            url.push_str(&format!("&qos_class={}", qos));
        }
        if let Some(ref search) = params.search {
            url.push_str(&format!("&search={}", search));
        }

        let resp: ContainersResponse = self
            .client
            .get(&url)
            .send()
            .await
            .context("Failed to fetch containers")?
            .json()
            .await
            .context("Failed to parse containers response")?;

        Ok(resp.containers)
    }

    /// REQ-MCP-004: Run a study on containers.
    pub async fn run_study(
        &self,
        study_id: &str,
        metric: &str,
        container_ids: &[&str],
    ) -> Result<StudyResponse> {
        let containers = container_ids.join(",");
        let url = format!(
            "{}/api/study/{}?metric={}&containers={}",
            self.base_url, study_id, metric, containers
        );

        self.client
            .get(&url)
            .send()
            .await
            .context("Failed to run study")?
            .json()
            .await
            .context("Failed to parse study response")
    }

    /// Get timeseries data for trend detection (REQ-MCP-005).
    pub async fn get_timeseries(
        &self,
        metric: &str,
        container_ids: &[&str],
    ) -> Result<Vec<TimeseriesResponse>> {
        let containers = container_ids.join(",");
        let url = format!(
            "{}/api/timeseries?metric={}&containers={}",
            self.base_url, metric, containers
        );

        self.client
            .get(&url)
            .send()
            .await
            .context("Failed to fetch timeseries")?
            .json()
            .await
            .context("Failed to parse timeseries response")
    }
}

// --- API Response Types ---

#[derive(Debug, Deserialize)]
struct MetricsResponse {
    metrics: Vec<MetricInfo>,
}

/// Metric metadata from /api/metrics.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MetricInfo {
    pub name: String,
}

#[derive(Debug, Deserialize)]
struct StudiesResponse {
    studies: Vec<StudyInfo>,
}

/// Study metadata from /api/studies.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StudyInfo {
    pub id: String,
    pub name: String,
    pub description: String,
}

/// Filter options from /api/filters.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FiltersResponse {
    pub qos_classes: Vec<String>,
    pub namespaces: Vec<String>,
}

/// Parameters for container search.
#[derive(Debug, Default)]
pub struct ContainerSearchParams {
    pub metric: String,
    pub namespace: Option<String>,
    pub qos_class: Option<String>,
    pub search: Option<String>,
}

#[derive(Debug, Deserialize)]
struct ContainersResponse {
    containers: Vec<ContainerInfo>,
}

/// Container metadata from /api/containers.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ContainerInfo {
    pub id: String,
    pub short_id: String,
    pub qos_class: Option<String>,
    pub namespace: Option<String>,
    pub pod_name: Option<String>,
    pub container_name: Option<String>,
    pub last_seen_ms: Option<i64>,
}

/// Study result from /api/study/:id.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StudyResponse {
    pub study: String,
    pub results: Vec<ContainerStudyResult>,
}

/// Per-container study result.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ContainerStudyResult {
    pub container: String,
    #[serde(flatten)]
    pub result: StudyResult,
}

/// Study analysis result.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StudyResult {
    pub window_count: usize,
    pub findings: Vec<StudyFinding>,
}

/// Individual finding from a study.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StudyFinding {
    pub start_time: i64,
    pub end_time: i64,
    pub period_ms: f64,
    pub confidence: f64,
    pub amplitude: f64,
}

/// Timeseries response from /api/timeseries.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TimeseriesResponse {
    pub container: String,
    pub data: Vec<TimeseriesPoint>,
}

/// Single timeseries data point.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TimeseriesPoint {
    pub time_ms: i64,
    pub value: f64,
}
