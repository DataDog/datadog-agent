// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"hash/fnv"
	"math"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// GraphSketchConfig configures the GraphSketch anomaly detector.
// GraphSketch uses 3D tensor sketching with temporal bins for
// real-time edge anomaly detection with adaptive thresholding.
//
// Paper: "Adaptive-GraphSketch: Real-time Edge Anomaly Detection"
// (https://arxiv.org/pdf/2509.11633)
type GraphSketchConfig struct {
	// Count-Min Sketch dimensions
	Depth int // Number of hash functions (rows), default: 5
	Width int // Number of buckets per hash (columns), default: 1024

	// Temporal binning (3D tensor)
	NumTimeBins    int     // Number of temporal layers, default: 10
	TimeBinSize    int64   // Duration of each time bin in seconds, default: 60
	DecayFactor    float64 // Exponential decay for older bins, default: 0.9
	PruneThreshold float64 // Minimum frequency to keep edge, default: 0.01

	// EWMA adaptive thresholding
	EWMAAlpha       float64 // EWMA smoothing factor (α), default: 0.1
	K               float64 // Sensitivity multiplier for threshold, default: 3.0
	MinObservations int     // Minimum observations before detecting, default: 10

	// Gaussian-likelihood Bayesian inference (Algorithm 2 from paper)
	P0            float64 // Prior probability of anomaly, default: 0.01
	Delta         float64 // Threshold for anomaly decision, default: 0.5
	PriorVariance float64 // Prior variance for Bayesian inference, default: 1.0
}

// DefaultGraphSketchConfig returns a GraphSketchConfig with sensible defaults from paper.
func DefaultGraphSketchConfig() GraphSketchConfig {
	return GraphSketchConfig{
		Depth:           5,
		Width:           1024,
		NumTimeBins:     10,
		TimeBinSize:     60,
		DecayFactor:     0.9, // γ: temporal decay factor
		PruneThreshold:  0.01,
		EWMAAlpha:       0.1, // α: EWMA smoothing
		K:               3.0, // Sensitivity multiplier for threshold
		MinObservations: 10,
		P0:              0.05, // Prior probability of anomaly (paper uses 0.05)
		Delta:           0.5,  // Δ: shift for anomaly mean
		PriorVariance:   1.0,
	}
}

// GraphSketchEmitter detects edge anomalies using 3D tensor sketching.
// It implements TimeSeriesAnalysis (stateful — maintains tensor sketch across calls).
//
// Algorithm:
//  1. Extract edges from metric series
//  2. Update 3D tensor sketch [time_bin][depth][width]
//  3. Apply temporal decay to older bins
//  4. Compute expected frequency from sketch
//  5. Calculate Gaussian-likelihood Bayesian anomaly score
//  6. Prune inactive edges
//  7. Emit signals for anomalous edges
type GraphSketchEmitter struct {
	config GraphSketchConfig

	// 3D Tensor Sketch: [time_bin][depth][width]
	tensorSketch [][][]float64

	// Time bin tracking (absolute bin indices)
	currentTimeBin  int64         // Current absolute time bin index (t)
	layerToBinMap   map[int]int64 // Maps tensor layer [0..W-1] to absolute bin
	oldestActiveBin int64         // Oldest active absolute bin

	// Edge metadata for pruning and tracking
	edgeLastSeen map[string]int64 // Edge -> last timestamp seen
	edgeFirstBin map[string]int64 // Edge -> first bin where it was observed (for computing t)

	// EWMA adaptive thresholding state (Algorithm 2)
	ewmaZ             float64 // Z_t: EWMA of anomaly scores X_t
	runningMeanX      float64 // Running mean of X_t (not EWMA)
	runningM2         float64 // M2 for Welford's variance algorithm: sum of squared differences
	numScoresSeen     int     // Count of scores for running stats
	adaptiveThreshold float64 // τ = meanX + K*stdX

	// Total observations
	totalEdges int
}

// NewGraphSketchEmitter creates a new GraphSketch anomaly detector.
func NewGraphSketchEmitter(config GraphSketchConfig) *GraphSketchEmitter {
	// Initialize 3D Tensor Sketch: [time_bin][depth][width]
	tensorSketch := make([][][]float64, config.NumTimeBins)
	for t := 0; t < config.NumTimeBins; t++ {
		tensorSketch[t] = make([][]float64, config.Depth)
		for d := 0; d < config.Depth; d++ {
			tensorSketch[t][d] = make([]float64, config.Width)
		}
	}

	return &GraphSketchEmitter{
		config:            config,
		tensorSketch:      tensorSketch,
		currentTimeBin:    0,
		layerToBinMap:     make(map[int]int64),
		oldestActiveBin:   0,
		edgeLastSeen:      make(map[string]int64),
		edgeFirstBin:      make(map[string]int64),
		ewmaZ:             0.0,
		runningMeanX:      0.0,
		runningM2:         0.0,
		numScoresSeen:     0,
		adaptiveThreshold: config.K, // Initial threshold = K
		totalEdges:        0,
	}
}

// Name returns the emitter name.
func (g *GraphSketchEmitter) Name() string {
	return "graphsketch"
}

// Analyze detects edge anomalies in a time series using 3D tensor sketching.
func (g *GraphSketchEmitter) Analyze(series observer.Series) observer.TimeSeriesAnalysisResult {
	if len(series.Points) < g.config.MinObservations {
		return observer.TimeSeriesAnalysisResult{}
	}

	var anomalies []observer.AnomalyOutput

	// Extract edges from time series
	edges := g.extractEdges(series)

	for _, edge := range edges {
		// Determine time bin for this edge
		timeBin := edge.timestamp / g.config.TimeBinSize

		// Update current time bin tracking
		if timeBin > g.currentTimeBin {
			// Advance to new time bin
			g.advanceTimeBin(timeBin)
		}

		edgeKey := edge.source + "->" + edge.dest

		// Track first bin this edge was seen (for computing t)
		if _, exists := g.edgeFirstBin[edgeKey]; !exists {
			g.edgeFirstBin[edgeKey] = timeBin
		}

		// Update tensor sketch with new observation (conservative update)
		g.updateTensorSketch(edge, timeBin)

		// Update metadata
		g.edgeLastSeen[edgeKey] = edge.timestamp

		// Query â_uv(t) and ŝ_uv(t) from tensor sketch (NOT from per-edge maps)
		aHat, sHat, _ := g.queryTensorSketch(edge, timeBin)

		// Compute t: total bins elapsed since this edge was first seen
		tElapsed := timeBin - g.edgeFirstBin[edgeKey] + 1
		if tElapsed <= 0 {
			tElapsed = 1
		}

		// Compute anomaly score using Algorithm 2 (two Gaussian likelihoods with normalization)
		score := g.gaussianBayesianScore(aHat, sHat, tElapsed)

		// Update EWMA adaptive threshold
		g.updateEWMAThreshold(score)

		// Check if anomalous using Z_t vs τ (NOT individual score vs threshold)
		// Per paper: if Z_t > τ then anomaly
		if g.ewmaZ > g.adaptiveThreshold && g.totalEdges >= g.config.MinObservations {
			ewmaScore := g.ewmaZ
			anomalies = append(anomalies, observer.AnomalyOutput{
				Source:      observer.MetricName(series.Name),
				Title:       fmt.Sprintf("GraphSketch: %s", series.Name),
				Description: fmt.Sprintf("%s (score: %.2f) at timestamp %d", series.Name, ewmaScore, edge.timestamp),
				Tags:        series.Tags,
				Timestamp:   edge.timestamp,
				TimeRange: observer.TimeRange{
					Start: edge.timestamp,
					End:   edge.timestamp,
				},
				DebugInfo: &observer.AnomalyDebugInfo{
					CurrentValue:   edge.value,
					DeviationSigma: ewmaScore,
				},
			})
		}

		g.totalEdges++
	}

	// Periodic pruning of inactive edges
	if g.totalEdges%100 == 0 {
		g.pruneInactiveEdges()
	}

	return observer.TimeSeriesAnalysisResult{Anomalies: anomalies}
}

// edge represents an interaction or transition in the series.
type edge struct {
	source     string  // Source identifier
	dest       string  // Destination identifier (or value bucket)
	timestamp  int64   // When this edge was observed
	value      float64 // Metric value
	actualFreq float64 // Actual frequency of this edge
}

// extractEdges identifies edges from a time series.
// For metric series, we create edges based on value transitions or buckets.
func (g *GraphSketchEmitter) extractEdges(series observer.Series) []edge {
	if len(series.Points) < 2 {
		return nil
	}

	edges := make([]edge, 0, len(series.Points))

	// Create edges from consecutive value transitions
	// We bucket values to create discrete "destination" nodes
	for i := 0; i < len(series.Points); i++ {
		point := series.Points[i]

		// Bucket the value to create discrete edge destinations
		bucket := valueBucket(point.Value)

		edges = append(edges, edge{
			source:     series.Name,
			dest:       bucket,
			timestamp:  point.Timestamp,
			value:      point.Value,
			actualFreq: 1.0, // Each observation counts as 1
		})
	}

	return edges
}

// valueBucket converts a continuous value to a discrete bucket.
func valueBucket(value float64) string {
	// Simple bucketing: divide into ranges
	if value < 0 {
		return "neg"
	} else if value < 10 {
		return "low"
	} else if value < 100 {
		return "med"
	} else if value < 1000 {
		return "high"
	}
	return "veryhigh"
}

// advanceTimeBin advances to a new time bin with explicit layer management.
// Implements ring buffer with clearing on wrap and window pruning.
func (g *GraphSketchEmitter) advanceTimeBin(newTimeBin int64) {
	if newTimeBin <= g.currentTimeBin {
		return
	}

	// For each bin between current and new, update the ring buffer
	for b := g.currentTimeBin + 1; b <= newTimeBin; b++ {
		layer := int(b % int64(g.config.NumTimeBins))

		// Check if this layer needs to be cleared (ring wrap)
		if oldBin, exists := g.layerToBinMap[layer]; exists {
			// If b - oldBin >= W, we need to prune this old bin
			if b-oldBin >= int64(g.config.NumTimeBins) {
				// Clear the layer (set all counters to 0)
				g.clearLayer(layer)
			}
		}

		// Map this layer to the new absolute bin
		g.layerToBinMap[layer] = b
	}

	g.currentTimeBin = newTimeBin
	g.oldestActiveBin = newTimeBin - int64(g.config.NumTimeBins) + 1
	if g.oldestActiveBin < 0 {
		g.oldestActiveBin = 0
	}
}

// clearLayer zeros all counters in a tensor layer.
func (g *GraphSketchEmitter) clearLayer(layer int) {
	for d := 0; d < g.config.Depth; d++ {
		for w := 0; w < g.config.Width; w++ {
			g.tensorSketch[layer][d][w] = 0.0
		}
	}
}

// queryTensorSketch queries â_uv(t) and ŝ_uv(t) from the 3D tensor with lazy decay.
// Returns:
//   - currentBinFreq: â_uv(t) = estimated frequency in current bin
//   - cumulativeFreq: ŝ_uv(t) = cumulative frequency up to bin t (decay-weighted)
//   - variance: uncertainty estimate from hash function disagreement
func (g *GraphSketchEmitter) queryTensorSketch(e edge, currentBin int64) (float64, float64, float64) {
	edgeKey := e.source + "->" + e.dest
	gamma := g.config.DecayFactor

	// â_uv(t): Query frequency in current bin only (no decay, it's current)
	currentLayer := int(currentBin % int64(g.config.NumTimeBins))
	currentEstimates := make([]float64, g.config.Depth)
	for d := 0; d < g.config.Depth; d++ {
		idx := g.hash(edgeKey, d)
		currentEstimates[d] = g.tensorSketch[currentLayer][d][idx]
	}
	currentBinFreq := min(currentEstimates)

	// ŝ_uv(t): Query cumulative frequency with decay across all active bins
	// Decay weight computed directly from absolute bins: weight = γ^(currentBin - absoluteBin)
	var cumulativeFreq float64
	for layer := 0; layer < g.config.NumTimeBins; layer++ {
		// Check if this layer is active (maps to a bin within window)
		if absoluteBin, exists := g.layerToBinMap[layer]; exists {
			// Only sum bins up to and including current bin
			if absoluteBin <= currentBin && currentBin-absoluteBin < int64(g.config.NumTimeBins) {
				estimates := make([]float64, g.config.Depth)
				for d := 0; d < g.config.Depth; d++ {
					idx := g.hash(edgeKey, d)
					estimates[d] = g.tensorSketch[layer][d][idx]
				}

				// Compute decay weight directly from absolute bins
				age := currentBin - absoluteBin // >= 0
				decayWeight := math.Pow(gamma, float64(age))

				cumulativeFreq += min(estimates) * decayWeight
			}
		}
	}

	// Compute variance across depth (hash functions) for uncertainty
	// Apply decay to variance computation as well
	variances := make([]float64, g.config.Depth)
	for d := 0; d < g.config.Depth; d++ {
		idx := g.hash(edgeKey, d)
		// Aggregate across active bins with decay
		var freq float64
		for layer := 0; layer < g.config.NumTimeBins; layer++ {
			if absoluteBin, exists := g.layerToBinMap[layer]; exists {
				if absoluteBin <= currentBin && currentBin-absoluteBin < int64(g.config.NumTimeBins) {
					// Compute decay weight directly from absolute bins
					age := currentBin - absoluteBin // >= 0
					decayWeight := math.Pow(gamma, float64(age))
					freq += g.tensorSketch[layer][d][idx] * decayWeight
				}
			}
		}
		variances[d] = freq
	}

	variance := computeVariance(variances)
	if variance < 1e-10 {
		variance = 1e-10
	}

	return currentBinFreq, cumulativeFreq, variance
}

// updateTensorSketch updates the 3D tensor with conservative update.
func (g *GraphSketchEmitter) updateTensorSketch(e edge, timeBin int64) {
	edgeKey := e.source + "->" + e.dest

	// Determine which tensor layer (time bin) to update
	binIndex := int(timeBin % int64(g.config.NumTimeBins))

	// Query current estimates from all hash functions in this time bin
	estimates := make([]float64, g.config.Depth)
	indices := make([]int, g.config.Depth)

	for d := 0; d < g.config.Depth; d++ {
		idx := g.hash(edgeKey, d)
		indices[d] = idx
		estimates[d] = g.tensorSketch[binIndex][d][idx]
	}

	// Conservative update: only update counters with minimum value
	minEstimate := min(estimates)

	for d := 0; d < g.config.Depth; d++ {
		if g.tensorSketch[binIndex][d][indices[d]] == minEstimate {
			g.tensorSketch[binIndex][d][indices[d]] += e.actualFreq
		}
	}
}

// gaussianBayesianScore implements Algorithm 2 from the paper with proper Gaussian normalization.
// Uses exact formulas from paper with t (elapsed bins since edge first seen).
//
// Algorithm 2: Bayesian Anomaly Detection
// Input: a = â_uv(t) (current bin freq), s = ŝ_uv(t) (cumulative freq), t (time bins elapsed)
// Output: anomaly score X_t
func (g *GraphSketchEmitter) gaussianBayesianScore(a, s float64, t int64) float64 {
	if t <= 0 {
		t = 1
	}

	// Paper formulas (from Algorithm 2):
	// μ = s / t          (expected frequency per bin)
	// σ² = s / t²        (variance estimate)
	// μ_A = μ + Δ        (anomaly mean, shifted by Delta)
	// σ²_A = 4·σ²        (anomaly variance, 4× for robustness)

	mu := s / float64(t)
	sigma2 := s / float64(t*t)
	if sigma2 < 1e-10 {
		sigma2 = 1e-10
	}

	muA := mu + g.config.Delta
	sigma2A := 4.0 * sigma2

	// Two hypotheses:
	// H0: normal, a ~ N(μ, σ²)
	// H1: anomaly, a ~ N(μ_A, σ²_A)

	// Log-likelihood under H0 (with Gaussian normalization)
	// log p(a|H0) = -0.5*log(2π*σ²) - 0.5*(a-μ)²/σ²
	diff0 := a - mu
	logL0 := -0.5*math.Log(2*math.Pi*sigma2) - 0.5*(diff0*diff0)/sigma2

	// Log-likelihood under H1 (with Gaussian normalization)
	// log p(a|H1) = -0.5*log(2π*σ²_A) - 0.5*(a-μ_A)²/σ²_A
	diff1 := a - muA
	logL1 := -0.5*math.Log(2*math.Pi*sigma2A) - 0.5*(diff1*diff1)/sigma2A

	// Prior: P(H1) = p0 (default 0.05 from paper), P(H0) = 1 - p0
	p0 := g.config.P0
	logPriorH0 := math.Log(1.0 - p0)
	logPriorH1 := math.Log(p0)

	// Posterior log odds: log(P(H1|data) / P(H0|data))
	logOdds := logL1 + logPriorH1 - logL0 - logPriorH0

	// Posterior probability: P(H1|data) = 1 / (1 + exp(-logOdds))
	posteriorH1 := 1.0 / (1.0 + math.Exp(-logOdds))

	// Return posterior as anomaly score X_t
	return posteriorH1
}

// updateEWMAThreshold updates the adaptive threshold per paper specification.
// Updates:
//   - Z_t: EWMA of anomaly scores X_t
//   - meanX, stdX: running (NOT EWMA) mean and std of X_t using Welford's algorithm
//   - τ = meanX + K*stdX (threshold)
//
// Decision: if Z_t > τ then anomaly
func (g *GraphSketchEmitter) updateEWMAThreshold(score float64) {
	alpha := g.config.EWMAAlpha

	if g.numScoresSeen == 0 {
		// First observation: initialize
		g.ewmaZ = score
		g.runningMeanX = score
		g.runningM2 = 0.0
		g.numScoresSeen = 1
	} else {
		// Update EWMA Z_t: Z_t = α·X_t + (1-α)·Z_{t-1}
		g.ewmaZ = alpha*score + (1-alpha)*g.ewmaZ

		// Update running mean and M2 using Welford's algorithm (NOT EWMA)
		g.numScoresSeen++

		// Welford's online algorithm:
		// delta = x - mean_old
		// mean_new = mean_old + delta/n
		// M2_new = M2_old + delta * (x - mean_new)
		delta := score - g.runningMeanX
		g.runningMeanX += delta / float64(g.numScoresSeen)
		delta2 := score - g.runningMeanX
		g.runningM2 += delta * delta2
	}

	// Compute variance from M2: variance = M2 / n
	var variance float64
	if g.numScoresSeen > 1 {
		variance = g.runningM2 / float64(g.numScoresSeen)
	} else {
		variance = 0.0
	}

	// Ensure variance doesn't underflow
	if variance < 1e-10 {
		variance = 1e-10
	}

	// Compute adaptive threshold: τ = meanX + K·stdX
	// This is the threshold for Z_t (NOT for individual scores)
	stdDevX := math.Sqrt(variance)
	g.adaptiveThreshold = g.runningMeanX + g.config.K*stdDevX

	// Ensure threshold is always positive and reasonable
	// Minimum of 0.1 makes sense for posterior probabilities (0-1 range)
	if g.adaptiveThreshold < 0.1 {
		g.adaptiveThreshold = 0.1
	}
}

// pruneInactiveEdges removes edges that haven't been seen recently.
func (g *GraphSketchEmitter) pruneInactiveEdges() {
	currentTime := g.currentTimeBin * g.config.TimeBinSize
	maxAge := int64(g.config.NumTimeBins) * g.config.TimeBinSize

	for edgeKey, lastSeen := range g.edgeLastSeen {
		// Prune if not seen recently
		timeSinceLastSeen := currentTime - lastSeen
		if timeSinceLastSeen > maxAge {
			delete(g.edgeLastSeen, edgeKey)
			delete(g.edgeFirstBin, edgeKey)
		}
	}
}

// hash computes hash for Count-Min Sketch.
// Uses FNV-1a hash with salting for multiple hash functions.
func (g *GraphSketchEmitter) hash(key string, seed int) int {
	h := fnv.New64a()
	h.Write([]byte(key))

	// Add seed for different hash functions
	h.Write([]byte{byte(seed)})

	hashValue := h.Sum64()
	return int(hashValue % uint64(g.config.Width))
}

// min returns the minimum value from a slice.
func min(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	minVal := values[0]
	for _, v := range values[1:] {
		if v < minVal {
			minVal = v
		}
	}
	return minVal
}

// computeVariance computes the variance of a slice of values.
func computeVariance(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// Compute mean
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))

	// Compute variance
	var variance float64
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(len(values))

	return variance
}
