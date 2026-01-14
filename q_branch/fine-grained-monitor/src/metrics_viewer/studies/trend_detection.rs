//! Trend detection study using linear regression and Mann-Kendall test.
//!
//! Detects monotonic trends (increasing or decreasing) in timeseries data.
//! Useful for identifying memory leaks, resource accumulation, or gradual degradation.

use super::{Study, StudyResult, StudyWindow};
use crate::metrics_viewer::data::TimeseriesPoint;
use std::collections::HashMap;

/// Configuration for trend detection.
#[derive(Debug, Clone)]
pub struct TrendConfig {
    /// Minimum R² value to consider a trend significant (0.0 to 1.0).
    /// Higher = stricter (only strong linear trends reported).
    pub min_r_squared: f64,
    /// Minimum absolute slope to report (units per hour).
    pub min_slope_per_hour: f64,
    /// Whether to calculate Mann-Kendall statistical significance.
    pub use_mann_kendall: bool,
    /// P-value threshold for Mann-Kendall test (typically 0.05).
    pub mann_kendall_p_value: f64,
}

impl Default for TrendConfig {
    fn default() -> Self {
        Self {
            min_r_squared: 0.6, // Moderate linear fit required
            min_slope_per_hour: 0.0, // Report all slopes by default
            use_mann_kendall: true,
            mann_kendall_p_value: 0.05, // 95% confidence
        }
    }
}

/// Trend direction classification.
#[derive(Debug, Clone, Copy, PartialEq)]
pub enum TrendDirection {
    Increasing,
    Decreasing,
    Stable,
}

impl TrendDirection {
    fn as_str(&self) -> &str {
        match self {
            TrendDirection::Increasing => "increasing",
            TrendDirection::Decreasing => "decreasing",
            TrendDirection::Stable => "stable",
        }
    }
}

/// Trend detection study using linear regression.
#[derive(Default)]
pub struct TrendDetectionStudy {
    config: TrendConfig,
}

impl TrendDetectionStudy {
    /// Create with custom configuration.
    pub fn with_config(config: TrendConfig) -> Self {
        Self { config }
    }
}

impl Study for TrendDetectionStudy {
    fn id(&self) -> &str {
        "trend"
    }

    fn name(&self) -> &str {
        "Trend Detection"
    }

    fn description(&self) -> &str {
        "Detects monotonic trends using linear regression and Mann-Kendall test"
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

        // Calculate linear regression over entire timeseries
        let regression = calculate_linear_regression(timeseries);

        // Determine trend direction
        let direction = if regression.slope.abs() < self.config.min_slope_per_hour {
            TrendDirection::Stable
        } else if regression.slope > 0.0 {
            TrendDirection::Increasing
        } else {
            TrendDirection::Decreasing
        };

        // Check if trend meets significance criteria
        let is_significant = regression.r_squared >= self.config.min_r_squared;

        // Optionally check Mann-Kendall significance
        let mann_kendall_significant = if self.config.use_mann_kendall {
            let mk_result = mann_kendall_test(timeseries);
            mk_result.p_value <= self.config.mann_kendall_p_value
        } else {
            true // Skip test
        };

        if !is_significant || !mann_kendall_significant {
            return StudyResult {
                study_name: self.name().to_string(),
                windows,
                summary: format!(
                    "No significant trend detected (R²={:.3}, direction={})",
                    regression.r_squared,
                    direction.as_str()
                ),
            };
        }

        // Calculate projected limit breach time (for increasing trends only)
        let projected_breach = if direction == TrendDirection::Increasing {
            // Assume a typical memory limit (we don't have the actual limit here)
            // This would ideally come from container metadata
            // For now, project when we'll reach 10x the current max value
            let current_max = timeseries.iter().map(|p| p.value).fold(f64::NEG_INFINITY, f64::max);
            let projected_limit = current_max * 10.0;

            if regression.slope > 0.0 {
                let time_to_limit_ms = ((projected_limit - regression.intercept) / regression.slope) as i64;
                Some(time_to_limit_ms)
            } else {
                None
            }
        } else {
            None
        };

        // Create study window for the entire analyzed range
        let start_time_ms = timeseries.first().unwrap().time_ms;
        let end_time_ms = timeseries.last().unwrap().time_ms;

        let mut metrics = HashMap::new();
        metrics.insert("slope".to_string(), regression.slope);
        metrics.insert("r_squared".to_string(), regression.r_squared);
        metrics.insert("start_value".to_string(), regression.start_value);
        metrics.insert("end_value".to_string(), regression.end_value);
        metrics.insert("direction".to_string(), match direction {
            TrendDirection::Increasing => 1.0,
            TrendDirection::Decreasing => -1.0,
            TrendDirection::Stable => 0.0,
        });

        if let Some(breach_ms) = projected_breach {
            metrics.insert("projected_breach_ms".to_string(), breach_ms as f64);
        }

        // Format label
        let sign = if regression.slope > 0.0 { "+" } else { "" };
        let label = format!(
            "{}{:.2}/hr (R²={:.2}, {})",
            sign,
            regression.slope,
            regression.r_squared,
            direction.as_str()
        );

        windows.push(StudyWindow {
            start_time_ms,
            end_time_ms,
            metrics,
            label,
        });

        // Generate summary
        let summary = format!(
            "{} trend detected: {}{:.2} units/hour (R²={:.2})",
            match direction {
                TrendDirection::Increasing => "Increasing",
                TrendDirection::Decreasing => "Decreasing",
                TrendDirection::Stable => "Stable",
            },
            if regression.slope > 0.0 { "+" } else { "" },
            regression.slope,
            regression.r_squared
        );

        StudyResult {
            study_name: self.name().to_string(),
            windows,
            summary,
        }
    }
}

/// Linear regression result.
#[derive(Debug)]
struct LinearRegression {
    slope: f64,          // Units per hour
    intercept: f64,      // Y-intercept
    r_squared: f64,      // Coefficient of determination (0 to 1)
    start_value: f64,    // Fitted value at start time
    end_value: f64,      // Fitted value at end time
}

/// Calculate linear regression on timeseries data.
/// Slope is in units per hour (converted from units per millisecond).
fn calculate_linear_regression(timeseries: &[TimeseriesPoint]) -> LinearRegression {
    let n = timeseries.len() as f64;

    // Use time in hours from start for better numerical stability
    let start_time_ms = timeseries[0].time_ms;
    let time_hours: Vec<f64> = timeseries
        .iter()
        .map(|p| (p.time_ms - start_time_ms) as f64 / 3_600_000.0) // ms to hours
        .collect();
    let values: Vec<f64> = timeseries.iter().map(|p| p.value).collect();

    // Calculate means
    let mean_time: f64 = time_hours.iter().sum::<f64>() / n;
    let mean_value: f64 = values.iter().sum::<f64>() / n;

    // Calculate slope and intercept
    let mut numerator = 0.0;
    let mut denominator = 0.0;
    for i in 0..timeseries.len() {
        let time_diff = time_hours[i] - mean_time;
        let value_diff = values[i] - mean_value;
        numerator += time_diff * value_diff;
        denominator += time_diff * time_diff;
    }

    let slope = if denominator.abs() > 1e-10 {
        numerator / denominator
    } else {
        0.0
    };
    let intercept = mean_value - slope * mean_time;

    // Calculate R²
    let mut ss_tot = 0.0; // Total sum of squares
    let mut ss_res = 0.0; // Residual sum of squares
    for i in 0..timeseries.len() {
        let fitted = slope * time_hours[i] + intercept;
        ss_res += (values[i] - fitted).powi(2);
        ss_tot += (values[i] - mean_value).powi(2);
    }

    let r_squared = if ss_tot > 1e-10 {
        1.0 - (ss_res / ss_tot)
    } else {
        0.0
    };

    // Calculate fitted values at start and end
    let start_value = intercept; // At time 0
    let end_time_hours = time_hours.last().copied().unwrap_or(0.0);
    let end_value = slope * end_time_hours + intercept;

    LinearRegression {
        slope,
        intercept,
        r_squared,
        start_value,
        end_value,
    }
}

/// Mann-Kendall test result.
#[derive(Debug)]
struct MannKendallResult {
    #[allow(dead_code)]
    statistic: f64,
    p_value: f64,
}

/// Perform Mann-Kendall trend test.
/// Returns statistical significance of monotonic trend.
fn mann_kendall_test(timeseries: &[TimeseriesPoint]) -> MannKendallResult {
    let n = timeseries.len();
    let values: Vec<f64> = timeseries.iter().map(|p| p.value).collect();

    // Calculate S statistic (sum of signs of all pairwise differences)
    let mut s = 0i64;
    for i in 0..n {
        for j in (i + 1)..n {
            let diff = values[j] - values[i];
            if diff > 0.0 {
                s += 1;
            } else if diff < 0.0 {
                s -= 1;
            }
        }
    }

    // Calculate variance (assuming no ties for simplicity)
    let n_f64 = n as f64;
    let var_s = (n_f64 * (n_f64 - 1.0) * (2.0 * n_f64 + 5.0)) / 18.0;

    // Calculate Z-score
    let z = if s > 0 {
        (s as f64 - 1.0) / var_s.sqrt()
    } else if s < 0 {
        (s as f64 + 1.0) / var_s.sqrt()
    } else {
        0.0
    };

    // Approximate p-value using standard normal distribution
    // This is a simplified approximation; for production, use a proper stats library
    let p_value = 2.0 * (1.0 - normal_cdf(z.abs()));

    MannKendallResult {
        statistic: s as f64,
        p_value,
    }
}

/// Approximate cumulative distribution function for standard normal distribution.
/// Uses error function approximation.
fn normal_cdf(x: f64) -> f64 {
    0.5 * (1.0 + erf(x / std::f64::consts::SQRT_2))
}

/// Error function approximation (Abramowitz and Stegun).
fn erf(x: f64) -> f64 {
    let sign = if x >= 0.0 { 1.0 } else { -1.0 };
    let x = x.abs();

    let a1 = 0.254829592;
    let a2 = -0.284496736;
    let a3 = 1.421413741;
    let a4 = -1.453152027;
    let a5 = 1.061405429;
    let p = 0.3275911;

    let t = 1.0 / (1.0 + p * x);
    let t2 = t * t;
    let t3 = t2 * t;
    let t4 = t3 * t;
    let t5 = t4 * t;

    let erf_val = 1.0 - (a1 * t + a2 * t2 + a3 * t3 + a4 * t4 + a5 * t5) * (-x * x).exp();

    sign * erf_val
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_trend_detection_increasing() {
        let study = TrendDetectionStudy::default();

        // Create data with clear upward trend
        let timeseries: Vec<TimeseriesPoint> = (0..100)
            .map(|i| TimeseriesPoint {
                time_ms: i * 1000,
                value: 10.0 + i as f64 * 0.5, // Slope of 0.5 per second
            })
            .collect();

        let result = study.analyze(&timeseries);

        assert!(!result.windows.is_empty(), "Should detect increasing trend");
        assert!(result.summary.contains("Increasing"), "Should report increasing trend");

        let window = &result.windows[0];
        let slope = window.metrics.get("slope").unwrap();
        assert!(*slope > 0.0, "Slope should be positive");
    }

    #[test]
    fn test_trend_detection_decreasing() {
        let study = TrendDetectionStudy::default();

        // Create data with clear downward trend
        let timeseries: Vec<TimeseriesPoint> = (0..100)
            .map(|i| TimeseriesPoint {
                time_ms: i * 1000,
                value: 100.0 - i as f64 * 0.3, // Negative slope
            })
            .collect();

        let result = study.analyze(&timeseries);

        assert!(!result.windows.is_empty(), "Should detect decreasing trend");
        assert!(result.summary.contains("Decreasing"), "Should report decreasing trend");

        let window = &result.windows[0];
        let slope = window.metrics.get("slope").unwrap();
        assert!(*slope < 0.0, "Slope should be negative");
    }

    #[test]
    fn test_trend_detection_stable() {
        let config = TrendConfig {
            min_r_squared: 0.3, // Lower threshold for this test
            min_slope_per_hour: 1.0, // Require at least 1 unit/hr
            ..Default::default()
        };
        let study = TrendDetectionStudy::with_config(config);

        // Create flat data with small noise
        let timeseries: Vec<TimeseriesPoint> = (0..100)
            .map(|i| TimeseriesPoint {
                time_ms: i * 1000,
                value: 50.0 + (i % 5) as f64 * 0.1, // Minimal variation
            })
            .collect();

        let result = study.analyze(&timeseries);

        // Should either report stable or no significant trend
        assert!(
            result.summary.contains("stable") || result.summary.contains("No significant"),
            "Should report stable or no trend for flat data"
        );
    }

    #[test]
    fn test_linear_regression() {
        // Perfect linear data: y = 2x + 5
        let timeseries: Vec<TimeseriesPoint> = (0..10)
            .map(|i| TimeseriesPoint {
                time_ms: i * 3_600_000, // 1 hour intervals
                value: 2.0 * i as f64 + 5.0,
            })
            .collect();

        let regression = calculate_linear_regression(&timeseries);

        assert!((regression.slope - 2.0).abs() < 0.01, "Slope should be ~2");
        assert!((regression.intercept - 5.0).abs() < 0.01, "Intercept should be ~5");
        assert!(regression.r_squared > 0.99, "R² should be near 1.0 for perfect fit");
    }

    #[test]
    fn test_insufficient_data() {
        let study = TrendDetectionStudy::default();

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
    fn test_mann_kendall() {
        // Increasing trend
        let timeseries: Vec<TimeseriesPoint> = (0..30)
            .map(|i| TimeseriesPoint {
                time_ms: i * 1000,
                value: i as f64,
            })
            .collect();

        let mk_result = mann_kendall_test(&timeseries);
        assert!(mk_result.statistic > 0.0, "Should detect positive trend");
        assert!(mk_result.p_value < 0.05, "Should be statistically significant");
    }
}
