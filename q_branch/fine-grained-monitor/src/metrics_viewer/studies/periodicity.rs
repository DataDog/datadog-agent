//! Periodicity detection study using autocorrelation.
//!
//! REQ-MV-006: Detects periodic patterns in timeseries data.
//! REQ-MV-007: Provides window data for visualization.

use super::{Study, StudyResult, StudyWindow};
use crate::metrics_viewer::data::TimeseriesPoint;
use std::collections::HashMap;

/// Configuration for periodicity detection.
#[derive(Debug, Clone)]
pub struct PeriodicityConfig {
    /// Window size in samples.
    pub window_size: usize,
    /// Step size between windows (overlap = window_size - step_size).
    pub step_size: usize,
    /// Minimum periodicity score (autocorrelation threshold).
    pub min_periodicity_score: f64,
    /// Minimum amplitude (peak-to-trough).
    pub min_amplitude: f64,
    /// Minimum period to detect in samples.
    pub min_period: usize,
    /// Maximum period to detect in samples.
    pub max_period: usize,
}

impl Default for PeriodicityConfig {
    fn default() -> Self {
        // REQ-MV-006: Default parameters from design.md
        Self {
            window_size: 60,
            step_size: 30, // 50% overlap
            min_periodicity_score: 0.6,
            min_amplitude: 10.0, // 10%
            min_period: 2,
            max_period: 30,
        }
    }
}

/// Periodicity detection study.
#[derive(Default)]
pub struct PeriodicityStudy {
    config: PeriodicityConfig,
}

impl PeriodicityStudy {
    /// Create with custom configuration.
    pub fn with_config(config: PeriodicityConfig) -> Self {
        Self { config }
    }
}

impl Study for PeriodicityStudy {
    fn id(&self) -> &str {
        "periodicity"
    }

    fn name(&self) -> &str {
        "Periodicity Study"
    }

    fn description(&self) -> &str {
        "Detects periodic patterns using autocorrelation"
    }

    fn analyze(&self, timeseries: &[TimeseriesPoint]) -> StudyResult {
        let mut windows = Vec::new();

        if timeseries.len() < self.config.window_size {
            return StudyResult {
                study_name: self.name().to_string(),
                windows,
                summary: format!(
                    "Insufficient data ({} samples, need {})",
                    timeseries.len(),
                    self.config.window_size
                ),
            };
        }

        // Sliding window analysis
        let mut pos = 0;
        while pos + self.config.window_size <= timeseries.len() {
            let window_data = &timeseries[pos..pos + self.config.window_size];
            let values: Vec<f64> = window_data.iter().map(|p| p.value).collect();

            if let Some(detection) = analyze_window(&values, &self.config) {
                let start_time = window_data.first().map(|p| p.time_ms).unwrap_or(0);
                let end_time = window_data.last().map(|p| p.time_ms).unwrap_or(0);

                let mut metrics = HashMap::new();
                metrics.insert("period".to_string(), detection.period);
                metrics.insert("score".to_string(), detection.periodicity_score);
                metrics.insert("amplitude".to_string(), detection.amplitude);

                windows.push(StudyWindow {
                    start_time_ms: start_time,
                    end_time_ms: end_time,
                    metrics,
                    label: format!(
                        "{:.1}s period ({:.0}% confidence)",
                        detection.period,
                        detection.periodicity_score * 100.0
                    ),
                });
            }

            pos += self.config.step_size;
        }

        let summary = if windows.is_empty() {
            "No periodic patterns detected".to_string()
        } else {
            format!("Found {} periodic windows", windows.len())
        };

        StudyResult {
            study_name: self.name().to_string(),
            windows,
            summary,
        }
    }
}

/// Detection result for a single window.
struct WindowDetection {
    period: f64,
    periodicity_score: f64,
    amplitude: f64,
}

/// Analyze a single window for periodicity.
fn analyze_window(samples: &[f64], config: &PeriodicityConfig) -> Option<WindowDetection> {
    if samples.len() < config.window_size {
        return None;
    }

    // Compute statistics
    let mean: f64 = samples.iter().sum::<f64>() / samples.len() as f64;
    let variance: f64 =
        samples.iter().map(|x| (x - mean).powi(2)).sum::<f64>() / samples.len() as f64;

    if variance == 0.0 {
        return None;
    }

    // Calculate amplitude
    let min = samples.iter().cloned().fold(f64::INFINITY, f64::min);
    let max = samples.iter().cloned().fold(f64::NEG_INFINITY, f64::max);
    let amplitude = max - min;

    // Early exit if amplitude below threshold
    if config.min_amplitude > 0.0 && amplitude < config.min_amplitude {
        return None;
    }

    // Find best autocorrelation lag
    let mut best_lag = 0;
    let mut best_corr = 0.0;

    for lag in config.min_period..=config.max_period.min(samples.len() / 2) {
        let corr = autocorrelation(samples, mean, variance, lag);
        if corr > best_corr {
            best_corr = corr;
            best_lag = lag;
        }
    }

    // Detection criteria
    if best_corr >= config.min_periodicity_score && best_lag > 0 {
        Some(WindowDetection {
            period: best_lag as f64,
            periodicity_score: best_corr,
            amplitude,
        })
    } else {
        None
    }
}

/// Compute normalized autocorrelation at a given lag.
fn autocorrelation(samples: &[f64], mean: f64, variance: f64, lag: usize) -> f64 {
    if variance == 0.0 || lag >= samples.len() {
        return 0.0;
    }

    let n = samples.len();
    let count = n - lag;
    if count == 0 {
        return 0.0;
    }

    let sum: f64 = (0..count)
        .map(|i| (samples[i] - mean) * (samples[i + lag] - mean))
        .sum();

    sum / (count as f64 * variance)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_autocorrelation_periodic() {
        // Create a sine wave with period 10
        let samples: Vec<f64> = (0..60)
            .map(|i| (2.0 * std::f64::consts::PI * i as f64 / 10.0).sin() * 50.0 + 50.0)
            .collect();

        let mean: f64 = samples.iter().sum::<f64>() / samples.len() as f64;
        let variance: f64 =
            samples.iter().map(|x| (x - mean).powi(2)).sum::<f64>() / samples.len() as f64;

        // Autocorrelation at lag 10 should be high
        let corr_10 = autocorrelation(&samples, mean, variance, 10);
        assert!(corr_10 > 0.9, "Expected high correlation at period, got {}", corr_10);

        // Autocorrelation at lag 5 (half period) should be negative
        let corr_5 = autocorrelation(&samples, mean, variance, 5);
        assert!(corr_5 < 0.0, "Expected negative correlation at half period, got {}", corr_5);
    }

    #[test]
    fn test_periodicity_detection() {
        let study = PeriodicityStudy::default();

        // Create periodic data
        let timeseries: Vec<TimeseriesPoint> = (0..100)
            .map(|i| TimeseriesPoint {
                time_ms: i * 1000,
                value: (2.0 * std::f64::consts::PI * i as f64 / 10.0).sin() * 50.0 + 50.0,
            })
            .collect();

        let result = study.analyze(&timeseries);
        assert!(!result.windows.is_empty(), "Should detect periodicity");
    }

    #[test]
    fn test_no_periodicity_flat() {
        let study = PeriodicityStudy::default();

        // Create flat data
        let timeseries: Vec<TimeseriesPoint> = (0..100)
            .map(|i| TimeseriesPoint {
                time_ms: i * 1000,
                value: 50.0,
            })
            .collect();

        let result = study.analyze(&timeseries);
        assert!(result.windows.is_empty(), "Should not detect periodicity in flat data");
    }
}
