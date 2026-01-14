//! PELT (Pruned Exact Linear Time) changepoint detection.
//!
//! This is a custom implementation of the PELT algorithm based on:
//! Killick, R., Fearnhead, P., & Eckley, I. A. (2012). "Optimal detection of
//! changepoints with a linear computational cost." Journal of the American
//! Statistical Association, 107(500), 1590-1598.
//!
//! PELT provides exact optimal partitioning with expected O(n) complexity
//! under the assumption of a bounded number of changepoints.

/// Configuration for PELT changepoint detection.
#[derive(Debug, Clone)]
pub struct PeltConfig {
    /// Penalty per changepoint (BIC-like penalty).
    /// Higher = fewer changepoints. Default uses BIC: 2 * log(n).
    pub penalty: Option<f64>,
    /// Minimum segment length between changepoints.
    pub min_segment_len: usize,
    /// Pruning constant K for the PELT inequality.
    /// If None, uses 0 (standard PELT pruning).
    pub prune_constant: f64,
}

impl Default for PeltConfig {
    fn default() -> Self {
        Self {
            penalty: None, // Will be computed as 2 * log(n)
            min_segment_len: 2,
            prune_constant: 0.0,
        }
    }
}

/// PELT changepoint detector.
///
/// Uses dynamic programming with pruning to find optimal partitioning
/// in expected O(n) time.
pub struct PeltDetector {
    config: PeltConfig,
}

impl PeltDetector {
    /// Create a new PELT detector with default config.
    pub fn new() -> Self {
        Self {
            config: PeltConfig::default(),
        }
    }

    /// Create with custom configuration.
    pub fn with_config(config: PeltConfig) -> Self {
        Self { config }
    }

    /// Create with a specific penalty value.
    pub fn with_penalty(penalty: f64) -> Self {
        Self {
            config: PeltConfig {
                penalty: Some(penalty),
                ..Default::default()
            },
        }
    }

    /// Detect changepoints in the data, returning their indices.
    ///
    /// Returns indices where a changepoint occurs (the first index of the new segment).
    pub fn detect(&self, data: &[f64]) -> Vec<usize> {
        let n = data.len();
        if n < 2 * self.config.min_segment_len {
            return vec![];
        }

        // Compute penalty if not specified (BIC-like: 2 * log(n))
        let penalty = self
            .config
            .penalty
            .unwrap_or_else(|| 2.0 * (n as f64).ln());

        // Precompute cumulative sums for O(1) cost calculation
        let (cum_sum, cum_sum_sq) = compute_cumulative_sums(data);

        // F[t] = optimal cost for data[0:t]
        // cp[t] = last changepoint for optimal partitioning ending at t
        let mut f = vec![0.0; n + 1];
        let mut cp: Vec<usize> = vec![0; n + 1];

        // R = set of candidate changepoint positions (pruned)
        let mut candidates: Vec<usize> = vec![0];

        let min_seg = self.config.min_segment_len;
        let k = self.config.prune_constant;

        // Initialize: cost of first min_seg points as one segment
        if min_seg <= n {
            f[min_seg] = segment_cost(&cum_sum, &cum_sum_sq, 0, min_seg);
        }

        // Dynamic programming with pruning
        for t in (min_seg + 1)..=n {
            // Find optimal previous changepoint
            let mut best_cost = f64::INFINITY;
            let mut best_cp = 0;

            for &s in &candidates {
                // s is the end of previous segment, new segment starts at s
                // Check minimum segment length
                if t - s < min_seg {
                    continue;
                }

                let cost = f[s] + segment_cost(&cum_sum, &cum_sum_sq, s, t) + penalty;

                if cost < best_cost {
                    best_cost = cost;
                    best_cp = s;
                }
            }

            // Also consider no changepoint (single segment from 0 to t)
            let single_seg_cost = segment_cost(&cum_sum, &cum_sum_sq, 0, t);
            if single_seg_cost < best_cost {
                best_cost = single_seg_cost;
                best_cp = 0;
            }

            f[t] = best_cost;
            cp[t] = best_cp;

            // PELT pruning: remove candidates that can never be optimal
            // If F[s] + C(s,t) > F[t] + K, then s can be pruned
            candidates.retain(|&s| {
                if t - s < min_seg {
                    return true; // Keep, hasn't been evaluated yet
                }
                let cost_with_s = f[s] + segment_cost(&cum_sum, &cum_sum_sq, s, t);
                cost_with_s <= f[t] + k
            });

            // Add t as a candidate for future iterations
            candidates.push(t);
        }

        // Backtrack to find all changepoints
        backtrack_changepoints(&cp, n)
    }
}

impl Default for PeltDetector {
    fn default() -> Self {
        Self::new()
    }
}

/// Compute cumulative sums for O(1) segment cost calculation.
/// Returns (cumulative_sum, cumulative_sum_of_squares).
fn compute_cumulative_sums(data: &[f64]) -> (Vec<f64>, Vec<f64>) {
    let n = data.len();
    let mut cum_sum = vec![0.0; n + 1];
    let mut cum_sum_sq = vec![0.0; n + 1];

    for i in 0..n {
        cum_sum[i + 1] = cum_sum[i] + data[i];
        cum_sum_sq[i + 1] = cum_sum_sq[i] + data[i] * data[i];
    }

    (cum_sum, cum_sum_sq)
}

/// Compute cost of segment [start, end) using precomputed cumulative sums.
///
/// Cost = sum of squared deviations from segment mean (within-segment variance).
/// For normal data, this is equivalent to -2 * log-likelihood (up to constants).
fn segment_cost(cum_sum: &[f64], cum_sum_sq: &[f64], start: usize, end: usize) -> f64 {
    let n = (end - start) as f64;
    if n <= 0.0 {
        return 0.0;
    }

    let sum = cum_sum[end] - cum_sum[start];
    let sum_sq = cum_sum_sq[end] - cum_sum_sq[start];

    // Variance = E[X²] - E[X]²
    // Cost = n * variance = sum_sq - sum²/n
    let cost = sum_sq - (sum * sum) / n;

    // Ensure non-negative (numerical stability)
    cost.max(0.0)
}

/// Backtrack through changepoint array to extract all changepoints.
fn backtrack_changepoints(cp: &[usize], n: usize) -> Vec<usize> {
    let mut changepoints = Vec::new();
    let mut pos = n;

    while pos > 0 {
        let prev = cp[pos];
        if prev > 0 {
            changepoints.push(prev);
        }
        pos = prev;
    }

    changepoints.reverse();
    changepoints
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Helper to generate a step function with noise.
    fn step_data(n1: usize, n2: usize, mean1: f64, mean2: f64, noise: f64) -> Vec<f64> {
        let mut data = Vec::with_capacity(n1 + n2);

        // Simple deterministic "noise" using sine for reproducibility
        for i in 0..n1 {
            let pseudo_noise = noise * (i as f64 * 0.1).sin();
            data.push(mean1 + pseudo_noise);
        }
        for i in 0..n2 {
            let pseudo_noise = noise * ((n1 + i) as f64 * 0.1).sin();
            data.push(mean2 + pseudo_noise);
        }

        data
    }

    #[test]
    fn test_obvious_step_change() {
        let data = step_data(500, 500, 0.0, 10.0, 1.0);
        let detector = PeltDetector::new();
        let changepoints = detector.detect(&data);

        // Should detect changepoint near index 500
        assert!(!changepoints.is_empty(), "Should detect at least one changepoint");

        let has_near_500 = changepoints.iter().any(|&cp| (490..=510).contains(&cp));
        assert!(
            has_near_500,
            "Should detect changepoint near 500, got {:?}",
            changepoints
        );
    }

    #[test]
    fn test_no_changepoint_flat() {
        let data: Vec<f64> = (0..1000).map(|i| 50.0 + (i as f64 * 0.1).sin()).collect();
        let detector = PeltDetector::with_penalty(50.0); // High penalty
        let changepoints = detector.detect(&data);

        // Should detect zero or very few changepoints in flat data
        assert!(
            changepoints.len() <= 2,
            "Should detect minimal changepoints in flat data, got {:?}",
            changepoints
        );
    }

    #[test]
    fn test_multiple_changepoints() {
        // Data with three segments: [0-300] mean=0, [300-600] mean=10, [600-1000] mean=5
        let mut data = Vec::with_capacity(1000);
        for i in 0..300 {
            data.push(0.0 + (i as f64 * 0.1).sin());
        }
        for i in 300..600 {
            data.push(10.0 + (i as f64 * 0.1).sin());
        }
        for i in 600..1000 {
            data.push(5.0 + (i as f64 * 0.1).sin());
        }

        let detector = PeltDetector::new();
        let changepoints = detector.detect(&data);

        // Should detect two changepoints
        assert!(
            changepoints.len() >= 2,
            "Should detect at least 2 changepoints, got {:?}",
            changepoints
        );

        // Check they're in reasonable ranges
        let near_300 = changepoints.iter().any(|&cp| (280..=320).contains(&cp));
        let near_600 = changepoints.iter().any(|&cp| (580..=620).contains(&cp));
        assert!(near_300, "Should detect changepoint near 300");
        assert!(near_600, "Should detect changepoint near 600");
    }

    #[test]
    fn test_small_data() {
        let data = vec![1.0, 2.0, 3.0];
        let detector = PeltDetector::new();
        let changepoints = detector.detect(&data);

        // Should handle small data gracefully
        assert!(
            changepoints.len() <= 1,
            "Small data should have minimal changepoints"
        );
    }

    #[test]
    fn test_cumulative_sums() {
        let data = vec![1.0, 2.0, 3.0, 4.0, 5.0];
        let (cum_sum, cum_sum_sq) = compute_cumulative_sums(&data);

        assert_eq!(cum_sum[0], 0.0);
        assert_eq!(cum_sum[5], 15.0); // 1+2+3+4+5
        assert_eq!(cum_sum_sq[5], 55.0); // 1+4+9+16+25
    }

    #[test]
    fn test_segment_cost() {
        let data = vec![10.0, 10.0, 10.0, 10.0]; // Constant data
        let (cum_sum, cum_sum_sq) = compute_cumulative_sums(&data);

        let cost = segment_cost(&cum_sum, &cum_sum_sq, 0, 4);
        assert!(cost < 0.001, "Cost of constant segment should be ~0");
    }

    #[test]
    fn test_coal_mining_data() {
        // Classic coal mining disaster dataset
        let data: Vec<f64> = vec![
            4.0, 5.0, 4.0, 0.0, 1.0, 4.0, 3.0, 4.0, 0.0, 6.0, 3.0, 3.0, 4.0, 0.0, 2.0, 6.0, 3.0,
            3.0, 5.0, 4.0, 5.0, 3.0, 1.0, 4.0, 4.0, 1.0, 5.0, 5.0, 3.0, 4.0, 2.0, 5.0, 2.0, 2.0,
            3.0, 4.0, 2.0, 1.0, 3.0, 2.0, 2.0, 1.0, 1.0, 1.0, 1.0, 3.0, 0.0, 0.0, 1.0, 0.0, 1.0,
            1.0, 0.0, 0.0, 3.0, 1.0, 0.0, 3.0, 2.0, 2.0, 0.0, 1.0, 1.0, 1.0, 0.0, 1.0, 0.0, 1.0,
            0.0, 0.0, 0.0, 2.0, 1.0, 0.0, 0.0, 0.0, 1.0, 1.0, 0.0, 2.0, 3.0, 3.0, 1.0, 1.0, 2.0,
            1.0, 1.0, 1.0, 1.0, 2.0, 4.0, 2.0, 0.0, 0.0, 1.0, 4.0, 0.0, 0.0, 0.0, 1.0, 0.0, 0.0,
            0.0, 0.0, 0.0, 1.0, 0.0, 0.0, 1.0, 0.0, 1.0,
        ];

        let detector = PeltDetector::with_penalty(10.0);
        let changepoints = detector.detect(&data);

        // Coal mining data has a known changepoint around 1890 (index ~40)
        // which marks the transition to safer mining practices
        let has_near_40 = changepoints.iter().any(|&cp| (30..=50).contains(&cp));
        assert!(
            has_near_40,
            "Should detect changepoint near index 40 (1890), got {:?}",
            changepoints
        );
    }

    #[test]
    fn test_large_dataset_performance() {
        // 30,000 points with a changepoint at 15,000
        let mut data = Vec::with_capacity(30000);
        for i in 0..15000 {
            data.push(0.0 + (i as f64 * 0.01).sin());
        }
        for i in 15000..30000 {
            data.push(10.0 + (i as f64 * 0.01).sin());
        }

        let start = std::time::Instant::now();
        let detector = PeltDetector::new();
        let changepoints = detector.detect(&data);
        let elapsed = start.elapsed();

        // Should complete in reasonable time (< 1 second for 30k points)
        assert!(
            elapsed.as_secs() < 1,
            "PELT should be fast on 30k points, took {:?}",
            elapsed
        );

        // Should detect changepoint near 15000
        let has_near_15000 = changepoints.iter().any(|&cp| (14800..=15200).contains(&cp));
        assert!(
            has_near_15000,
            "Should detect changepoint near 15000, got {:?}",
            changepoints
        );
    }
}
