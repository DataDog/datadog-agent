// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package comparator provides telemetry comparison and anomaly detection logic.
package comparator

import (
	"math"
	"sort"
	"sync"
)

// NetworkDetector implements Mahalanobis distance-based anomaly detection for network metrics
type NetworkDetector struct {
	mu sync.RWMutex

	// Historical feature vectors (rolling window)
	history [][]float64 // Each inner slice is a 4D feature vector

	// Robust statistics
	robustMean []float64   // 4D mean vector (μ)
	robustCov  [][]float64 // 4x4 covariance matrix (Σ)
	covInverse [][]float64 // Inverse of covariance matrix
	covValid   bool        // Whether covariance matrix is valid

	// Recent anomaly scores for thresholding
	recentScores []float64

	// Configuration
	maxHistory    int     // Maximum history window size (e.g., 288 = 24 hours)
	maxScores     int     // Maximum scores to keep for percentile calculation
	percentile    float64 // Percentile threshold (e.g., 0.999 for 99.9%)
	minDataPoints int     // Minimum data points needed before computing statistics
}

// NewNetworkDetector creates a new network anomaly detector
func NewNetworkDetector(historySize, scoreSize int, percentile float64) *NetworkDetector {
	return &NetworkDetector{
		history:       make([][]float64, 0, historySize),
		robustMean:    make([]float64, 4),
		robustCov:     make([][]float64, 4),
		covInverse:    make([][]float64, 4),
		recentScores:  make([]float64, 0, scoreSize),
		maxHistory:    historySize,
		maxScores:     scoreSize,
		percentile:    percentile,
		minDataPoints: 10, // Need at least 10 points for meaningful statistics
		covValid:      false,
	}
}

// buildFeatureVector creates a 4D feature vector with log transforms
func (d *NetworkDetector) buildFeatureVector(bytesSentByClient, bytesSentByServer, packetsSentByClient, packetsSentByServer int64) []float64 {
	return []float64{
		math.Log(1 + float64(bytesSentByClient)),
		math.Log(1 + float64(bytesSentByServer)),
		math.Log(1 + float64(packetsSentByClient)),
		math.Log(1 + float64(packetsSentByServer)),
	}
}

// ComputeAnomalyScore computes the Mahalanobis distance for network metrics
// Returns: (mahalanobis_distance, is_anomalous)
func (d *NetworkDetector) ComputeAnomalyScore(bytesSentByClient, bytesSentByServer, packetsSentByClient, packetsSentByServer int64) (float64, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Build feature vector
	x := d.buildFeatureVector(bytesSentByClient, bytesSentByServer, packetsSentByClient, packetsSentByServer)

	// Add to history
	d.history = append(d.history, x)
	if len(d.history) > d.maxHistory {
		d.history = d.history[1:]
	}

	// Need minimum data points before we can detect anomalies
	if len(d.history) < d.minDataPoints {
		return 0.0, false
	}

	// Update robust statistics
	d.updateRobustStatistics()

	// If covariance is not valid, cannot compute Mahalanobis distance
	if !d.covValid {
		return 0.0, false
	}

	// Compute squared Mahalanobis distance
	d2 := d.mahalanobisDistance(x)

	// Add to recent scores
	d.recentScores = append(d.recentScores, d2)
	if len(d.recentScores) > d.maxScores {
		d.recentScores = d.recentScores[1:]
	}

	// Determine if anomalous using empirical threshold
	isAnomalous := d.isAnomalous(d2)

	return d2, isAnomalous
}

// updateRobustStatistics computes robust mean and covariance from history
func (d *NetworkDetector) updateRobustStatistics() {
	n := len(d.history)
	if n < d.minDataPoints {
		return
	}

	// Compute robust mean (component-wise median)
	for i := 0; i < 4; i++ {
		values := make([]float64, n)
		for j := 0; j < n; j++ {
			values[j] = d.history[j][i]
		}
		d.robustMean[i] = median(values)
	}

	// Compute robust covariance (simple approach: covariance after removing outliers)
	// Remove top/bottom 5% per component
	filtered := d.filterOutliers(0.05)

	if len(filtered) < 4 {
		// Not enough data after filtering
		d.covValid = false
		return
	}

	// Compute covariance matrix from filtered data
	d.robustCov = d.computeCovariance(filtered)

	// Compute inverse of covariance matrix
	inv, ok := d.invertMatrix4x4(d.robustCov)
	if !ok {
		d.covValid = false
		return
	}

	d.covInverse = inv
	d.covValid = true
}

// filterOutliers removes top/bottom p percent of values per component
func (d *NetworkDetector) filterOutliers(p float64) [][]float64 {
	n := len(d.history)
	removeCount := int(float64(n) * p)

	// For each component, find the values to exclude
	exclude := make(map[int]bool)

	for dim := 0; dim < 4; dim++ {
		// Create sorted index array for this dimension
		type indexValue struct {
			index int
			value float64
		}
		indexed := make([]indexValue, n)
		for i := 0; i < n; i++ {
			indexed[i] = indexValue{i, d.history[i][dim]}
		}
		sort.Slice(indexed, func(i, j int) bool {
			return indexed[i].value < indexed[j].value
		})

		// Mark bottom removeCount indices
		for i := 0; i < removeCount && i < n; i++ {
			exclude[indexed[i].index] = true
		}
		// Mark top removeCount indices
		for i := n - removeCount; i < n; i++ {
			exclude[indexed[i].index] = true
		}
	}

	// Build filtered dataset
	var filtered [][]float64
	for i := 0; i < n; i++ {
		if !exclude[i] {
			filtered = append(filtered, d.history[i])
		}
	}

	return filtered
}

// computeCovariance computes the 4x4 covariance matrix from data
func (d *NetworkDetector) computeCovariance(data [][]float64) [][]float64 {
	n := len(data)
	if n == 0 {
		return nil
	}

	// Compute mean of filtered data
	mean := make([]float64, 4)
	for i := 0; i < n; i++ {
		for j := 0; j < 4; j++ {
			mean[j] += data[i][j]
		}
	}
	for j := 0; j < 4; j++ {
		mean[j] /= float64(n)
	}

	// Compute covariance matrix
	cov := make([][]float64, 4)
	for i := 0; i < 4; i++ {
		cov[i] = make([]float64, 4)
	}

	for _, point := range data {
		for i := 0; i < 4; i++ {
			for j := 0; j < 4; j++ {
				cov[i][j] += (point[i] - mean[i]) * (point[j] - mean[j])
			}
		}
	}

	// Normalize by n-1 for sample covariance
	divisor := float64(n - 1)
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			cov[i][j] /= divisor
		}
	}

	// Add small regularization to diagonal to avoid singular matrix
	epsilon := 1e-6
	for i := 0; i < 4; i++ {
		cov[i][i] += epsilon
	}

	return cov
}

// mahalanobisDistance computes squared Mahalanobis distance
// d² = (x - μ)ᵀ Σ⁻¹ (x - μ)
func (d *NetworkDetector) mahalanobisDistance(x []float64) float64 {
	// Compute (x - μ)
	diff := make([]float64, 4)
	for i := 0; i < 4; i++ {
		diff[i] = x[i] - d.robustMean[i]
	}

	// Compute Σ⁻¹ (x - μ)
	temp := make([]float64, 4)
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			temp[i] += d.covInverse[i][j] * diff[j]
		}
	}

	// Compute (x - μ)ᵀ temp
	d2 := 0.0
	for i := 0; i < 4; i++ {
		d2 += diff[i] * temp[i]
	}

	return d2
}

// invertMatrix4x4 inverts a 4x4 matrix using Gauss-Jordan elimination
func (d *NetworkDetector) invertMatrix4x4(m [][]float64) ([][]float64, bool) {
	// Create augmented matrix [M | I]
	aug := make([][]float64, 4)
	for i := 0; i < 4; i++ {
		aug[i] = make([]float64, 8)
		for j := 0; j < 4; j++ {
			aug[i][j] = m[i][j]
		}
		aug[i][i+4] = 1.0
	}

	// Gauss-Jordan elimination
	for i := 0; i < 4; i++ {
		// Find pivot
		maxRow := i
		for k := i + 1; k < 4; k++ {
			if math.Abs(aug[k][i]) > math.Abs(aug[maxRow][i]) {
				maxRow = k
			}
		}

		// Swap rows
		aug[i], aug[maxRow] = aug[maxRow], aug[i]

		// Check for singular matrix
		if math.Abs(aug[i][i]) < 1e-10 {
			return nil, false
		}

		// Scale pivot row
		pivot := aug[i][i]
		for j := 0; j < 8; j++ {
			aug[i][j] /= pivot
		}

		// Eliminate column
		for k := 0; k < 4; k++ {
			if k != i {
				factor := aug[k][i]
				for j := 0; j < 8; j++ {
					aug[k][j] -= factor * aug[i][j]
				}
			}
		}
	}

	// Extract inverse from right half
	inv := make([][]float64, 4)
	for i := 0; i < 4; i++ {
		inv[i] = make([]float64, 4)
		for j := 0; j < 4; j++ {
			inv[i][j] = aug[i][j+4]
		}
	}

	return inv, true
}

// isAnomalous determines if a score is anomalous using empirical percentile
func (d *NetworkDetector) isAnomalous(score float64) bool {
	if len(d.recentScores) < 20 {
		// Not enough scores for meaningful threshold
		return false
	}

	// Compute percentile threshold
	threshold := d.computePercentile(d.recentScores, d.percentile)
	return score > threshold
}

// computePercentile computes the p-th percentile of values
func (d *NetworkDetector) computePercentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0.0
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	index := p * float64(len(sorted)-1)
	lower := int(index)
	upper := lower + 1

	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}

	// Linear interpolation
	weight := index - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

// median computes the median of a slice of float64 values
func median(values []float64) float64 {
	if len(values) == 0 {
		return 0.0
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2.0
	}
	return sorted[n/2]
}

// Reset clears the detector state
func (d *NetworkDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.history = make([][]float64, 0, d.maxHistory)
	d.recentScores = make([]float64, 0, d.maxScores)
	d.covValid = false
}
