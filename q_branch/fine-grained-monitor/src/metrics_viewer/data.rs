//! Core data types for the metrics viewer.
//!
//! These types are shared between the lazy loading system and the HTTP server.

use std::collections::HashMap;

/// A single timeseries data point.
#[derive(Debug, Clone, serde::Serialize)]
pub struct TimeseriesPoint {
    pub time_ms: i64,
    pub value: f64,
}

/// Container metadata extracted from labels.
#[derive(Debug, Clone, serde::Serialize)]
pub struct ContainerInfo {
    pub id: String,
    pub short_id: String,
    pub qos_class: Option<String>,
    pub namespace: Option<String>,
    pub pod_name: Option<String>,
    pub container_name: Option<String>,
    /// REQ-MV-035: When this container was first observed (epoch millis)
    #[serde(skip_serializing_if = "Option::is_none")]
    pub first_seen_ms: Option<i64>,
    /// REQ-MV-019: When this container was last observed (epoch millis)
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_seen_ms: Option<i64>,
    /// REQ-MV-032: Pod labels from Kubernetes API for filtering
    #[serde(skip_serializing_if = "Option::is_none")]
    pub labels: Option<HashMap<String, String>>,
}

/// Summary statistics for a container's metric.
#[derive(Debug, Clone, serde::Serialize)]
pub struct ContainerStats {
    pub info: ContainerInfo,
    pub avg: f64,
    pub max: f64,
}

/// Metric metadata.
#[derive(Debug, Clone, serde::Serialize)]
pub struct MetricInfo {
    pub name: String,
}
