//! Study abstraction for timeseries analysis.
//!
//! REQ-MV-006: Provides extensible framework for analytical studies.
//! REQ-MV-017: Changepoint detection study.

pub mod changepoint;
pub mod periodicity;

use crate::metrics_viewer::data::TimeseriesPoint;
use std::collections::HashMap;

/// Result from analyzing a single container's timeseries.
#[derive(Debug, Clone, serde::Serialize)]
pub struct StudyWindow {
    /// Start time of this analysis window.
    pub start_time_ms: i64,
    /// End time of this analysis window.
    pub end_time_ms: i64,
    /// Study-specific metrics (e.g., period, score, amplitude).
    pub metrics: HashMap<String, f64>,
    /// Display label for this window.
    pub label: String,
}

/// Result from a study analysis.
#[derive(Debug, Clone, serde::Serialize)]
pub struct StudyResult {
    /// Name of the study.
    pub study_name: String,
    /// Detected windows with metrics.
    pub windows: Vec<StudyWindow>,
    /// Human-readable summary.
    pub summary: String,
}

/// Metadata about a study for API discovery.
#[derive(Debug, Clone, serde::Serialize)]
pub struct StudyInfo {
    /// Unique study identifier.
    pub id: String,
    /// Display name.
    pub name: String,
    /// Description of what this study detects.
    pub description: String,
}

/// Trait for implementing timeseries analysis studies.
///
/// Studies analyze timeseries data and return detected patterns/anomalies.
pub trait Study: Send + Sync {
    /// Unique identifier for this study.
    fn id(&self) -> &str;

    /// Display name for this study.
    fn name(&self) -> &str;

    /// Description of what this study analyzes.
    fn description(&self) -> &str;

    /// Analyze a timeseries and return results.
    fn analyze(&self, timeseries: &[TimeseriesPoint]) -> StudyResult;

    /// Get study info for API discovery.
    fn info(&self) -> StudyInfo {
        StudyInfo {
            id: self.id().to_string(),
            name: self.name().to_string(),
            description: self.description().to_string(),
        }
    }
}

/// Registry of available studies.
pub struct StudyRegistry {
    studies: Vec<Box<dyn Study>>,
}

impl StudyRegistry {
    /// Create a new registry with default studies.
    pub fn new() -> Self {
        let mut registry = Self {
            studies: Vec::new(),
        };
        registry.register(Box::new(periodicity::PeriodicityStudy::default()));
        registry.register(Box::new(changepoint::ChangepointStudy::default()));
        registry
    }

    /// Register a study.
    pub fn register(&mut self, study: Box<dyn Study>) {
        self.studies.push(study);
    }

    /// Get all available studies.
    pub fn list(&self) -> Vec<StudyInfo> {
        self.studies.iter().map(|s| s.info()).collect()
    }

    /// Get a study by ID.
    pub fn get(&self, id: &str) -> Option<&dyn Study> {
        self.studies
            .iter()
            .find(|s| s.id() == id)
            .map(|s| s.as_ref())
    }
}

impl Default for StudyRegistry {
    fn default() -> Self {
        Self::new()
    }
}
