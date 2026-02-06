// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package comparator

import (
	"math"
	"sort"
	"sync"
)

// CosineDistanceDetector maintains state for anomaly detection using cosine distance
type CosineDistanceDetector struct {
	mu sync.RWMutex

	// Baseline profile: map[key]weight where sum of weights = 1
	baselineProfileMap map[string]float64

	// Rolling window of recent anomaly scores for statistical analysis
	recentScores []float64

	// Configuration
	alpha              float64 // Exponential smoothing parameter: 2/(N+1)
	maxRecentScores    int     // Maximum number of recent scores to keep (e.g., 288 for 24 hours at 5-min intervals)
	anomalyThreshold   float64 // Deviation threshold to consider anomalous (e.g., 3.0 for 3 standard deviations)
	verySmallNumber    float64 // Small epsilon to avoid division by zero
	madScalingConstant float64 // 1.4826 - converts MAD to standard deviation equivalent
}

// NewCosineDistanceDetector creates a new detector with default parameters
func NewCosineDistanceDetector(windowSize int, alpha float64) *CosineDistanceDetector {
	return &CosineDistanceDetector{
		baselineProfileMap: make(map[string]float64),
		recentScores:       make([]float64, 0, windowSize),
		alpha:              alpha,
		maxRecentScores:    windowSize,
		anomalyThreshold:   3.0,
		verySmallNumber:    1e-10,
		madScalingConstant: 1.4826,
	}
}

// ProfileMap represents a profile as a map of keys to normalized weights
type ProfileMap map[string]float64

// ComputeAnomalyScore computes the anomaly score for a current profile
// Returns: (anomaly_score, deviation, is_anomalous)
func (d *CosineDistanceDetector) ComputeAnomalyScore(profileValues map[string]float64) (float64, float64, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Convert to ProfileMap and normalize
	currentProfileMap := d.normalizeProfileMap(ProfileMap(profileValues))

	// Initialize baseline if this is the first profile
	if len(d.baselineProfileMap) == 0 {
		d.baselineProfileMap = d.copyProfileMap(currentProfileMap)
		// First profile is not anomalous
		return 0.0, 0.0, false
	}

	// Step 2: Compute anomaly score (cosine distance)
	anomalyScore := d.computeCosineDistance(currentProfileMap, d.baselineProfileMap)

	// Add to recent scores
	d.recentScores = append(d.recentScores, anomalyScore)
	if len(d.recentScores) > d.maxRecentScores {
		d.recentScores = d.recentScores[1:]
	}

	// Step 3: Decide if this score is anomalous
	isAnomalous, deviation := d.isAnomalous(anomalyScore)

	// Step 1: Update baseline if not anomalous
	if !isAnomalous {
		d.updateBaseline(currentProfileMap)
	}

	return anomalyScore, deviation, isAnomalous
}

// computeCosineDistance computes the cosine distance between two profiles
// Returns: 1 - cosine_similarity, where result ∈ [0, 1]
func (d *CosineDistanceDetector) computeCosineDistance(current, baseline ProfileMap) float64 {
	// 2.1 Compute dot product
	dotProduct := 0.0
	for key := range current {
		if baselineVal, exists := baseline[key]; exists {
			dotProduct += current[key] * baselineVal
		}
	}

	// 2.2 Compute magnitudes
	magnitudeCurrent := 0.0
	for _, val := range current {
		magnitudeCurrent += val * val
	}
	magnitudeCurrent = math.Sqrt(magnitudeCurrent)

	magnitudeBaseline := 0.0
	for _, val := range baseline {
		magnitudeBaseline += val * val
	}
	magnitudeBaseline = math.Sqrt(magnitudeBaseline)

	// 2.3 Compute cosine distance
	if magnitudeCurrent == 0 || magnitudeBaseline == 0 {
		// One or both profiles are empty
		if magnitudeCurrent == 0 && magnitudeBaseline == 0 {
			return 0.0 // Both empty, consider similar
		}
		return 1.0 // One empty, one not - completely different
	}

	similarity := dotProduct / (magnitudeCurrent * magnitudeBaseline)
	// Clamp similarity to [0, 1] to handle floating point errors
	similarity = math.Max(0.0, math.Min(1.0, similarity))

	return 1.0 - similarity
}

// isAnomalous determines if the current anomaly score is anomalous
// using median absolute deviation
func (d *CosineDistanceDetector) isAnomalous(anomalyScore float64) (bool, float64) {
	// Need at least 3 data points for meaningful statistics
	if len(d.recentScores) < 3 {
		return false, 0.0
	}

	// Compute median score
	medianScore := d.median(d.recentScores)

	// Compute median absolute deviation
	absoluteDeviations := make([]float64, len(d.recentScores))
	for i, score := range d.recentScores {
		absoluteDeviations[i] = math.Abs(score - medianScore)
	}
	mad := d.median(absoluteDeviations)

	// Convert to robust deviation value
	deviation := (anomalyScore - medianScore) / (d.madScalingConstant*mad + d.verySmallNumber)

	// Check if deviation exceeds threshold
	isAnomalous := deviation > d.anomalyThreshold

	return isAnomalous, deviation
}

// updateBaseline updates the baseline profile using exponential smoothing
func (d *CosineDistanceDetector) updateBaseline(currentProfileMap ProfileMap) {
	// Get all unique keys from both profiles
	allKeys := make(map[string]struct{})
	for key := range d.baselineProfileMap {
		allKeys[key] = struct{}{}
	}
	for key := range currentProfileMap {
		allKeys[key] = struct{}{}
	}

	// Update baseline for all keys
	newBaseline := make(map[string]float64)
	for key := range allKeys {
		baselineVal := d.baselineProfileMap[key] // 0 if not present
		currentVal := currentProfileMap[key]     // 0 if not present
		newBaseline[key] = (1-d.alpha)*baselineVal + d.alpha*currentVal
	}

	// Normalize to ensure sum = 1
	d.baselineProfileMap = d.normalizeProfileMap(newBaseline)
}

// normalizeProfileMap ensures all values sum to 1
func (d *CosineDistanceDetector) normalizeProfileMap(profile ProfileMap) ProfileMap {
	sum := 0.0
	for _, val := range profile {
		sum += val
	}

	if sum == 0 {
		return profile
	}

	normalized := make(ProfileMap)
	for key, val := range profile {
		normalized[key] = val / sum
	}
	return normalized
}

// copyProfileMap creates a deep copy of a profile
func (d *CosineDistanceDetector) copyProfileMap(profile ProfileMap) ProfileMap {
	copy := make(ProfileMap)
	for key, val := range profile {
		copy[key] = val
	}
	return copy
}

// median computes the median of a slice of float64 values
func (d *CosineDistanceDetector) median(values []float64) float64 {
	if len(values) == 0 {
		return 0.0
	}

	// Create a copy to avoid modifying the original
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	n := len(sorted)
	if n%2 == 0 {
		// Even number of elements: average of two middle values
		return (sorted[n/2-1] + sorted[n/2]) / 2.0
	}
	// Odd number of elements: middle value
	return sorted[n/2]
}

// Reset clears the detector state (useful for testing or reinitialization)
func (d *CosineDistanceDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.baselineProfileMap = make(map[string]float64)
	d.recentScores = make([]float64, 0, d.maxRecentScores)
}

// GetBaseline returns a copy of the current baseline profile (for inspection/debugging)
func (d *CosineDistanceDetector) GetBaseline() ProfileMap {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.copyProfileMap(d.baselineProfileMap)
}

// GetRecentScores returns a copy of recent anomaly scores (for inspection/debugging)
func (d *CosineDistanceDetector) GetRecentScores() []float64 {
	d.mu.RLock()
	defer d.mu.RUnlock()

	scores := make([]float64, len(d.recentScores))
	copy(scores, d.recentScores)
	return scores
}
