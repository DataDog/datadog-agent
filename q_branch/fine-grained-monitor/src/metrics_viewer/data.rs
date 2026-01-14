//! Core data types for the metrics viewer.
//!
//! These types are shared between the lazy loading system and the HTTP server.

use chrono::Duration;
use serde::{Deserialize, Deserializer};
use std::collections::HashMap;

// ============================================================================
// REQ-MV-037: Time Range Selection
// ============================================================================

/// Time range for queries.
/// Uses short format for API: 1h, 1d, 1w, all
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Default)]
pub enum TimeRange {
    #[default]
    Hour1,
    Day1,
    Week1,
    All,
}

impl TimeRange {
    /// Convert to chrono Duration. Returns None for All (unbounded).
    pub fn to_duration(&self) -> Option<Duration> {
        match self {
            TimeRange::Hour1 => Some(Duration::hours(1)),
            TimeRange::Day1 => Some(Duration::days(1)),
            TimeRange::Week1 => Some(Duration::weeks(1)),
            TimeRange::All => None,
        }
    }

    /// Get the cutoff timestamp in milliseconds for this time range.
    /// Returns None for All (no cutoff).
    pub fn cutoff_ms(&self) -> Option<i64> {
        self.to_duration().map(|d| {
            let now = chrono::Utc::now();
            (now - d).timestamp_millis()
        })
    }

    /// Short string representation for API.
    pub fn as_str(&self) -> &'static str {
        match self {
            TimeRange::Hour1 => "1h",
            TimeRange::Day1 => "1d",
            TimeRange::Week1 => "1w",
            TimeRange::All => "all",
        }
    }

    /// Convert lookback hours to TimeRange (for backwards compatibility).
    pub fn from_hours(hours: i64) -> Self {
        match hours {
            h if h <= 1 => TimeRange::Hour1,
            h if h <= 24 => TimeRange::Day1,
            h if h <= 168 => TimeRange::Week1,
            _ => TimeRange::All,
        }
    }
}

impl std::fmt::Display for TimeRange {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.as_str())
    }
}

/// Custom deserialize for short format: "1h", "1d", "1w", "all"
impl<'de> Deserialize<'de> for TimeRange {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        let s = String::deserialize(deserializer)?;
        match s.as_str() {
            "1h" => Ok(TimeRange::Hour1),
            "1d" => Ok(TimeRange::Day1),
            "1w" => Ok(TimeRange::Week1),
            "all" => Ok(TimeRange::All),
            _ => Err(serde::de::Error::custom(format!(
                "unknown time range: {}, expected 1h, 1d, 1w, or all",
                s
            ))),
        }
    }
}

impl serde::Serialize for TimeRange {
    fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        serializer.serialize_str(self.as_str())
    }
}

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
    /// When this container first appeared in data files (epoch millis, from file timestamps)
    #[serde(skip_serializing_if = "Option::is_none")]
    pub first_seen_ms: Option<i64>,
    /// When this container last appeared in data files (epoch millis, from file timestamps)
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_seen_ms: Option<i64>,
    /// REQ-MV-032: Pod labels from Kubernetes API for filtering
    #[serde(skip_serializing_if = "Option::is_none")]
    pub labels: Option<HashMap<String, String>>,
}

/// Metric metadata.
#[derive(Debug, Clone, serde::Serialize)]
pub struct MetricInfo {
    pub name: String,
}
