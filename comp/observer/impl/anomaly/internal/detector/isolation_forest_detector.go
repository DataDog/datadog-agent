// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package detector provides anomaly detection algorithms and scoring mechanisms.
package detector

import (
	"math"
	"math/rand"
	"time"
)

// IsolationForestDetector implements unsupervised multivariate anomaly detection
type IsolationForestDetector struct {
	// Hyperparameters
	numTrees           int     // number of isolation trees
	maxSamples         int     // subsample size for each tree
	trainingWindowDays int     // size of training window in days
	retrainHours       int     // retrain every N hours
	scoreThreshold     float64 // anomaly score threshold (99.5th percentile)
	persistence        int     // must satisfy condition for N consecutive points
	suppressMinutes    int     // cooldown period after alert
	maxTreeDepth       int     // maximum depth for isolation trees
	contamination      float64 // expected fraction of anomalies in training

	// State
	trees           []*IsolationTree // ensemble of isolation trees
	trainingData    [][]float64      // historical vectors for training
	medians         []float64        // per-signal medians for normalization
	mads            []float64        // per-signal MADs for normalization
	scoreHistory    []float64        // history of anomaly scores
	lastRetrainTime time.Time        // timestamp of last retrain
	lastAlertTime   time.Time        // timestamp of last alert
	numSignals      int              // number of features
	initialized     bool
	rng             *rand.Rand // random number generator
}

// IsolationTree represents a single tree in the forest
type IsolationTree struct {
	splitFeature int     // feature index to split on
	splitValue   float64 // value to split at
	left         *IsolationTree
	right        *IsolationTree
	size         int // number of samples in this node
	depth        int // depth of this node
}

// NewIsolationForestDetector creates a new Isolation Forest detector with default hyperparameters
func NewIsolationForestDetector() *IsolationForestDetector {
	return NewIsolationForestDetectorWithParams(
		200,   // num_trees
		256,   // max_samples
		30,    // training_window_days
		6,     // retrain_hours
		2.0,   // score_threshold (will be calibrated)
		2,     // persistence
		30,    // suppress_minutes
		10,    // max_tree_depth
		0.005, // contamination
	)
}

// NewIsolationForestDetectorWithParams creates an Isolation Forest detector with custom parameters
func NewIsolationForestDetectorWithParams(numTrees, maxSamples, trainingWindowDays, retrainHours int,
	scoreThreshold float64, persistence, suppressMinutes, maxTreeDepth int, contamination float64) *IsolationForestDetector {

	return &IsolationForestDetector{
		numTrees:           numTrees,
		maxSamples:         maxSamples,
		trainingWindowDays: trainingWindowDays,
		retrainHours:       retrainHours,
		scoreThreshold:     scoreThreshold,
		persistence:        persistence,
		suppressMinutes:    suppressMinutes,
		maxTreeDepth:       maxTreeDepth,
		contamination:      contamination,
		initialized:        false,
		rng:                rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Name returns the detector name
func (d *IsolationForestDetector) Name() string {
	return "IsolationForest"
}

// HigherIsAnomalous returns true since higher scores indicate anomalies
func (d *IsolationForestDetector) HigherIsAnomalous() bool {
	return true
}

// ComputeScore processes telemetry results and returns anomaly score
func (d *IsolationForestDetector) ComputeScore(result TelemetryResult) (float64, error) {
	signals := result.ToArray()

	// Initialize on first call
	if !d.initialized {
		d.numSignals = len(signals)
		d.trainingData = make([][]float64, 0, d.trainingWindowDays*288) // 5-min intervals
		d.medians = make([]float64, d.numSignals)
		d.mads = make([]float64, d.numSignals)
		d.scoreHistory = make([]float64, 0, 10)
		d.initialized = true
	}

	// Add current point to training data
	signalsCopy := make([]float64, len(signals))
	copy(signalsCopy, signals)
	d.trainingData = append(d.trainingData, signalsCopy)

	// Trim training data if too large
	maxTrainingSize := d.trainingWindowDays * 288 // 288 five-minute intervals per day
	if len(d.trainingData) > maxTrainingSize {
		d.trainingData = d.trainingData[len(d.trainingData)-maxTrainingSize:]
	}

	// Check if we need to retrain
	now := time.Now()
	retrainInterval := time.Duration(d.retrainHours) * time.Hour
	timeSinceRetrain := now.Sub(d.lastRetrainTime)

	if d.trees == nil || timeSinceRetrain > retrainInterval {
		if len(d.trainingData) >= 100 { // minimum training size
			d.train()
			d.lastRetrainTime = now
		}
	}

	// If not enough data for scoring, return 0
	if d.trees == nil || len(d.trainingData) < 50 {
		return 0.0, nil
	}

	// Normalize current signals
	normalized := d.normalizeVector(signals)

	// Compute anomaly score
	score := d.scoreVector(normalized)

	// Store in history
	d.scoreHistory = append(d.scoreHistory, score)
	if len(d.scoreHistory) > 10 {
		d.scoreHistory = d.scoreHistory[1:]
	}

	// Check persistence
	persistenceCount := 0
	histLen := len(d.scoreHistory)
	checkLen := d.persistence
	if histLen < checkLen {
		checkLen = histLen
	}

	for i := histLen - checkLen; i < histLen; i++ {
		if d.scoreHistory[i] > d.scoreThreshold {
			persistenceCount++
		}
	}

	// Check if we should alert
	suppressDuration := time.Duration(d.suppressMinutes) * time.Minute
	timeSinceLastAlert := now.Sub(d.lastAlertTime)

	if persistenceCount >= d.persistence && timeSinceLastAlert > suppressDuration {
		d.lastAlertTime = now
		return score, nil
	}

	// Return normalized score for monitoring
	return score / 3.0, nil
}

// train builds the isolation forest
func (d *IsolationForestDetector) train() {
	// Compute robust normalization parameters
	d.computeNormalizationParams()

	// Normalize training data
	normalizedData := make([][]float64, len(d.trainingData))
	for i, vec := range d.trainingData {
		normalizedData[i] = d.normalizeVector(vec)
	}

	// Build trees
	d.trees = make([]*IsolationTree, d.numTrees)
	sampleSize := d.maxSamples
	if len(normalizedData) < sampleSize {
		sampleSize = len(normalizedData)
	}

	for i := 0; i < d.numTrees; i++ {
		// Sample data
		sample := d.sampleData(normalizedData, sampleSize)
		// Build tree
		d.trees[i] = d.buildTree(sample, 0)
	}
}

// computeNormalizationParams computes robust median and MAD for each feature
func (d *IsolationForestDetector) computeNormalizationParams() {
	for s := 0; s < d.numSignals; s++ {
		values := make([]float64, len(d.trainingData))
		for i, vec := range d.trainingData {
			values[i] = vec[s]
		}
		d.medians[s] = computeMedian(values)
		d.mads[s] = computeMAD(values, d.medians[s])
	}
}

// normalizeVector applies robust scaling to a vector
func (d *IsolationForestDetector) normalizeVector(vec []float64) []float64 {
	normalized := make([]float64, len(vec))
	for i := 0; i < len(vec); i++ {
		scale := 1.4826 * d.mads[i]
		if scale < 1e-10 {
			scale = 1e-10
		}
		normalized[i] = (vec[i] - d.medians[i]) / scale
	}
	return normalized
}

// sampleData randomly samples n points from data
func (d *IsolationForestDetector) sampleData(data [][]float64, n int) [][]float64 {
	if n >= len(data) {
		return data
	}

	sample := make([][]float64, n)
	indices := d.rng.Perm(len(data))
	for i := 0; i < n; i++ {
		sample[i] = data[indices[i]]
	}
	return sample
}

// buildTree recursively builds an isolation tree
func (d *IsolationForestDetector) buildTree(data [][]float64, depth int) *IsolationTree {
	n := len(data)
	if n == 0 {
		return nil
	}

	node := &IsolationTree{
		size:  n,
		depth: depth,
	}

	// Stop conditions: single point, max depth, or all points identical
	if n <= 1 || depth >= d.maxTreeDepth {
		return node
	}

	// Check if all points are identical (no split possible)
	allSame := true
	for f := 0; f < d.numSignals && allSame; f++ {
		first := data[0][f]
		for i := 1; i < n; i++ {
			if data[i][f] != first {
				allSame = false
				break
			}
		}
	}

	if allSame {
		return node
	}

	// Select random feature and split value
	node.splitFeature = d.rng.Intn(d.numSignals)

	// Find min and max for this feature
	minVal := data[0][node.splitFeature]
	maxVal := data[0][node.splitFeature]
	for i := 1; i < n; i++ {
		val := data[i][node.splitFeature]
		if val < minVal {
			minVal = val
		}
		if val > maxVal {
			maxVal = val
		}
	}

	// If min == max, can't split on this feature
	if minVal == maxVal {
		return node
	}

	// Random split value between min and max
	node.splitValue = minVal + d.rng.Float64()*(maxVal-minVal)

	// Partition data
	leftData := make([][]float64, 0, n/2)
	rightData := make([][]float64, 0, n/2)

	for _, point := range data {
		if point[node.splitFeature] < node.splitValue {
			leftData = append(leftData, point)
		} else {
			rightData = append(rightData, point)
		}
	}

	// Build children
	if len(leftData) > 0 {
		node.left = d.buildTree(leftData, depth+1)
	}
	if len(rightData) > 0 {
		node.right = d.buildTree(rightData, depth+1)
	}

	return node
}

// scoreVector computes anomaly score for a vector
func (d *IsolationForestDetector) scoreVector(vec []float64) float64 {
	// Average path length across all trees
	totalPathLength := 0.0
	for _, tree := range d.trees {
		pathLength := d.pathLength(tree, vec)
		totalPathLength += pathLength
	}

	avgPathLength := totalPathLength / float64(len(d.trees))

	// Normalize by expected average path length for a dataset of size n
	// E[h(x)] ≈ 2H(n-1) - 2(n-1)/n, where H is harmonic number
	n := float64(d.maxSamples)
	if n > 1 {
		c := 2.0*(math.Log(n-1)+0.5772156649) - 2.0*(n-1.0)/n // Euler-Mascheroni constant
		if c < 1e-10 {
			c = 1.0
		}
		// Anomaly score: s = 2^(-E[h(x)]/c)
		// Higher path length -> lower score (normal)
		// Lower path length -> higher score (anomalous)
		// We want higher scores for anomalies, so return inverse
		score := math.Pow(2.0, -avgPathLength/c)
		// Transform to make it more interpretable: higher = more anomalous
		return -math.Log2(1.0 - score + 1e-10)
	}

	return avgPathLength
}

// pathLength computes the path length for a point in a tree
func (d *IsolationForestDetector) pathLength(node *IsolationTree, vec []float64) float64 {
	if node == nil {
		return 0
	}

	// Leaf node: add expected path length for remaining samples
	if node.left == nil && node.right == nil {
		// Expected path length for n samples: c(n) ≈ 2H(n-1) - 2(n-1)/n
		if node.size <= 1 {
			return float64(node.depth)
		}
		n := float64(node.size)
		c := 2.0*(math.Log(n-1)+0.5772156649) - 2.0*(n-1.0)/n
		return float64(node.depth) + c
	}

	// Internal node: traverse to child
	if vec[node.splitFeature] < node.splitValue {
		if node.left != nil {
			return d.pathLength(node.left, vec)
		}
	} else {
		if node.right != nil {
			return d.pathLength(node.right, vec)
		}
	}

	return float64(node.depth)
}
