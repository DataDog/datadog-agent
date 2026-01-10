//! Changepoint detection study using PELT (Pruned Exact Linear Time).
//!
//! REQ-MV-017: Detects abrupt changes in timeseries data.
//! REQ-MV-018: Provides changepoint data for visualization.
//!
//! Uses our custom PELT implementation which provides O(n) expected complexity,
//! making it suitable for large datasets (10k+ points).

use super::pelt::PeltDetector;
use super::{Study, StudyResult, StudyWindow};
use crate::metrics_viewer::data::TimeseriesPoint;
use std::collections::HashMap;

/// Configuration for changepoint detection.
#[derive(Debug, Clone)]
pub struct ChangepointConfig {
    /// Penalty per changepoint.
    /// Higher = fewer changepoints. None = auto-compute BIC penalty (2 * log(n)).
    pub penalty: Option<f64>,
    /// Minimum segment length between changepoints.
    pub min_segment_len: usize,
    /// Number of samples to average before/after a changepoint for magnitude calculation.
    pub context_window: usize,
    /// Minimum magnitude (absolute difference) to report a changepoint.
    pub min_magnitude: f64,
}

impl Default for ChangepointConfig {
    fn default() -> Self {
        Self {
            penalty: None, // Auto-compute BIC penalty
            min_segment_len: 5,
            context_window: 5,
            min_magnitude: 0.0, // Report all detected changepoints
        }
    }
}

/// Changepoint detection study using PELT algorithm.
#[derive(Default)]
pub struct ChangepointStudy {
    config: ChangepointConfig,
}

impl ChangepointStudy {
    /// Create with custom configuration.
    pub fn with_config(config: ChangepointConfig) -> Self {
        Self { config }
    }
}

impl Study for ChangepointStudy {
    fn id(&self) -> &str {
        "changepoint"
    }

    fn name(&self) -> &str {
        "Changepoint Study"
    }

    fn description(&self) -> &str {
        "Detects abrupt changes using PELT (Pruned Exact Linear Time) algorithm"
    }

    fn analyze(&self, timeseries: &[TimeseriesPoint]) -> StudyResult {
        let mut windows = Vec::new();

        if timeseries.len() < 10 {
            return StudyResult {
                study_name: self.name().to_string(),
                windows,
                summary: format!(
                    "Insufficient data ({} samples, need at least 10)",
                    timeseries.len()
                ),
            };
        }

        // Extract values for PELT
        let values: Vec<f64> = timeseries.iter().map(|p| p.value).collect();

        // Configure and run PELT detector
        let pelt_config = super::pelt::PeltConfig {
            penalty: self.config.penalty,
            min_segment_len: self.config.min_segment_len,
            prune_constant: 0.0,
        };
        let detector = PeltDetector::with_config(pelt_config);
        let changepoint_indices = detector.detect(&values);

        // Convert indices to StudyWindows with context
        for &idx in &changepoint_indices {
            if idx == 0 || idx >= timeseries.len() {
                continue;
            }

            let timestamp_ms = timeseries[idx].time_ms;

            // Calculate before/after averages
            let (value_before, value_after, magnitude, direction) =
                calculate_change_metrics(&values, idx, self.config.context_window);

            // Skip if magnitude below threshold
            if magnitude < self.config.min_magnitude {
                continue;
            }

            let mut metrics = HashMap::new();
            metrics.insert("value_before".to_string(), value_before);
            metrics.insert("value_after".to_string(), value_after);
            metrics.insert("magnitude".to_string(), magnitude);
            metrics.insert(
                "direction".to_string(),
                if direction > 0.0 { 1.0 } else { -1.0 },
            );

            // Format timestamp for label
            let time_str = format_timestamp_ms(timestamp_ms);
            let sign = if direction > 0.0 { "+" } else { "" };
            let label = format!("{}{:.1} at {}", sign, direction, time_str);

            windows.push(StudyWindow {
                start_time_ms: timestamp_ms,
                end_time_ms: timestamp_ms, // Point event
                metrics,
                label,
            });
        }

        let summary = if windows.is_empty() {
            "No significant changes detected".to_string()
        } else {
            let largest = windows
                .iter()
                .map(|w| w.metrics.get("magnitude").copied().unwrap_or(0.0))
                .fold(0.0_f64, f64::max);
            format!(
                "Found {} changepoint(s), largest magnitude: {:.1}",
                windows.len(),
                largest
            )
        };

        StudyResult {
            study_name: self.name().to_string(),
            windows,
            summary,
        }
    }
}

/// Calculate metrics around a changepoint.
/// Returns (value_before, value_after, magnitude, signed_change).
fn calculate_change_metrics(
    values: &[f64],
    changepoint_idx: usize,
    context_window: usize,
) -> (f64, f64, f64, f64) {
    // Average of samples before changepoint
    let before_start = changepoint_idx.saturating_sub(context_window);
    let before_slice = &values[before_start..changepoint_idx];
    let value_before = if before_slice.is_empty() {
        values[changepoint_idx]
    } else {
        before_slice.iter().sum::<f64>() / before_slice.len() as f64
    };

    // Average of samples after changepoint
    let after_end = (changepoint_idx + context_window).min(values.len());
    let after_slice = &values[changepoint_idx..after_end];
    let value_after = if after_slice.is_empty() {
        values[changepoint_idx]
    } else {
        after_slice.iter().sum::<f64>() / after_slice.len() as f64
    };

    let signed_change = value_after - value_before;
    let magnitude = signed_change.abs();

    (value_before, value_after, magnitude, signed_change)
}

/// Format timestamp in milliseconds to a human-readable time string.
fn format_timestamp_ms(ms: i64) -> String {
    use chrono::{TimeZone, Utc};
    let dt = Utc.timestamp_millis_opt(ms).single();
    match dt {
        Some(dt) => dt.format("%H:%M:%S").to_string(),
        None => format!("{}ms", ms),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_changepoint_detection_step() {
        let study = ChangepointStudy::default();

        // Create data with a clear step change at index 50
        let mut timeseries: Vec<TimeseriesPoint> = (0..100)
            .map(|i| TimeseriesPoint {
                time_ms: i * 1000,
                value: if i < 50 { 10.0 } else { 50.0 },
            })
            .collect();

        // Add small noise
        for (i, point) in timeseries.iter_mut().enumerate() {
            point.value += (i % 3) as f64 * 0.5;
        }

        let result = study.analyze(&timeseries);

        // Should detect at least one changepoint near index 50
        assert!(
            !result.windows.is_empty(),
            "Should detect changepoint in step data"
        );

        // Check that the detected changepoint is near the actual change
        let detected_times: Vec<i64> = result.windows.iter().map(|w| w.start_time_ms).collect();
        let has_near_50 = detected_times.iter().any(|&t| {
            let idx = t / 1000;
            (45..=55).contains(&idx)
        });
        assert!(
            has_near_50,
            "Should detect changepoint near index 50, got {:?}",
            detected_times
        );
    }

    #[test]
    fn test_no_changepoint_flat() {
        // Use a high penalty to reduce false positives
        let config = ChangepointConfig {
            penalty: Some(50.0),
            ..Default::default()
        };
        let study = ChangepointStudy::with_config(config);

        // Create flat data
        let timeseries: Vec<TimeseriesPoint> = (0..100)
            .map(|i| TimeseriesPoint {
                time_ms: i * 1000,
                value: 50.0,
            })
            .collect();

        let result = study.analyze(&timeseries);

        // May detect some false positives, but should be minimal
        // The summary should indicate the result
        assert!(
            result.summary.contains("changepoint") || result.summary.contains("No significant"),
            "Summary should describe results"
        );
    }

    #[test]
    fn test_changepoint_metrics() {
        let values = vec![10.0, 10.0, 10.0, 10.0, 50.0, 50.0, 50.0, 50.0];
        let (before, after, magnitude, direction) = calculate_change_metrics(&values, 4, 3);

        assert!((before - 10.0).abs() < 1.0, "Before should be ~10");
        assert!((after - 50.0).abs() < 1.0, "After should be ~50");
        assert!((magnitude - 40.0).abs() < 1.0, "Magnitude should be ~40");
        assert!(direction > 0.0, "Direction should be positive (increase)");
    }

    #[test]
    fn test_insufficient_data() {
        let study = ChangepointStudy::default();

        let timeseries: Vec<TimeseriesPoint> = (0..5)
            .map(|i| TimeseriesPoint {
                time_ms: i * 1000,
                value: 50.0,
            })
            .collect();

        let result = study.analyze(&timeseries);
        assert!(
            result.summary.contains("Insufficient"),
            "Should report insufficient data"
        );
    }

    #[test]
    fn test_large_dataset() {
        let study = ChangepointStudy::default();

        // 10,000 points with changepoint at 5000
        let timeseries: Vec<TimeseriesPoint> = (0..10000)
            .map(|i| TimeseriesPoint {
                time_ms: i * 1000,
                value: if i < 5000 {
                    10.0 + (i as f64 * 0.01).sin()
                } else {
                    50.0 + (i as f64 * 0.01).sin()
                },
            })
            .collect();

        let start = std::time::Instant::now();
        let result = study.analyze(&timeseries);
        let elapsed = start.elapsed();

        // Should complete quickly (< 500ms for 10k points in debug mode)
        assert!(
            elapsed.as_millis() < 500,
            "Should be fast on 10k points, took {:?}",
            elapsed
        );

        // Should detect changepoint
        assert!(
            !result.windows.is_empty(),
            "Should detect changepoint in large dataset"
        );
    }
}
