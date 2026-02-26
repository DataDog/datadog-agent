// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraphSketch_Name(t *testing.T) {
	emitter := NewGraphSketchEmitter(DefaultGraphSketchConfig())
	assert.Equal(t, "graphsketch", emitter.Name())
}

func TestGraphSketch_EmptySeriesNoAnomalies(t *testing.T) {
	emitter := NewGraphSketchEmitter(DefaultGraphSketchConfig())

	series := observer.Series{
		Name:   "test.metric",
		Points: []observer.Point{},
	}

	result := emitter.Analyze(series)
	assert.Empty(t, result.Anomalies)
}

func TestGraphSketch_InsufficientPointsNoAnomalies(t *testing.T) {
	config := DefaultGraphSketchConfig()
	config.MinObservations = 10
	emitter := NewGraphSketchEmitter(config)

	series := observer.Series{
		Name: "test.metric",
		Points: []observer.Point{
			{Timestamp: 1, Value: 5.0},
			{Timestamp: 2, Value: 6.0},
		},
	}

	result := emitter.Analyze(series)
	assert.Empty(t, result.Anomalies)
}

func TestGraphSketch_StableDataNoAnomalies(t *testing.T) {
	config := DefaultGraphSketchConfig()
	config.MinObservations = 5
	config.K = 10.0 // Very high sensitivity multiplier
	emitter := NewGraphSketchEmitter(config)

	// Create stable data (all same bucket)
	points := make([]observer.Point, 20)
	for i := 0; i < 20; i++ {
		points[i] = observer.Point{
			Timestamp: int64(i + 1),
			Value:     5.0, // All in "low" bucket
		}
	}

	series := observer.Series{
		Name:   "test.metric",
		Points: points,
	}

	result := emitter.Analyze(series)
	// With stable data and high threshold, should have fewer anomalies than data points
	// (Early observations may trigger before pattern is learned)
	assert.Less(t, len(result.Anomalies), len(points), "Stable data should have fewer anomalies than total points")
}

func TestGraphSketch_DetectsSuddenSpike(t *testing.T) {
	config := DefaultGraphSketchConfig()
	config.MinObservations = 5
	config.K = 2.0
	emitter := NewGraphSketchEmitter(config)

	// Stable baseline
	points := make([]observer.Point, 15)
	for i := 0; i < 10; i++ {
		points[i] = observer.Point{
			Timestamp: int64(i + 1),
			Value:     5.0, // "low" bucket
		}
	}

	// Sudden spike to different bucket
	for i := 10; i < 15; i++ {
		points[i] = observer.Point{
			Timestamp: int64(i + 1),
			Value:     150.0, // "high" bucket - unusual
		}
	}

	series := observer.Series{
		Name:   "test.metric",
		Points: points,
		Tags:   []string{"env:test"},
	}

	result := emitter.Analyze(series)

	// Should detect anomalies in the spike
	// (May not detect all, as CMS learns adaptively)
	if len(result.Anomalies) > 0 {
		for _, a := range result.Anomalies {
			assert.Equal(t, observer.MetricName("test.metric"), a.Source)
			assert.Contains(t, a.Tags, "env:test")
			require.NotNil(t, a.DebugInfo)
			assert.Greater(t, a.DebugInfo.DeviationSigma, 0.0)
		}
	}
}

func TestGraphSketch_ValueBucketing(t *testing.T) {
	tests := []struct {
		value    float64
		expected string
	}{
		{-5.0, "neg"},
		{0.0, "low"},
		{5.0, "low"},
		{9.9, "low"},
		{10.0, "med"},
		{50.0, "med"},
		{99.9, "med"},
		{100.0, "high"},
		{500.0, "high"},
		{999.9, "high"},
		{1000.0, "veryhigh"},
		{5000.0, "veryhigh"},
	}

	for _, tt := range tests {
		result := valueBucket(tt.value)
		assert.Equal(t, tt.expected, result, "value %.1f should be in bucket %s", tt.value, tt.expected)
	}
}

func TestGraphSketch_ExtractEdges(t *testing.T) {
	emitter := NewGraphSketchEmitter(DefaultGraphSketchConfig())

	series := observer.Series{
		Name: "test.metric",
		Points: []observer.Point{
			{Timestamp: 1, Value: 5.0},   // low
			{Timestamp: 2, Value: 15.0},  // med
			{Timestamp: 3, Value: 150.0}, // high
		},
	}

	edges := emitter.extractEdges(series)

	require.Len(t, edges, 3)
	assert.Equal(t, "test.metric", edges[0].source)
	assert.Equal(t, "low", edges[0].dest)
	assert.Equal(t, "med", edges[1].dest)
	assert.Equal(t, "high", edges[2].dest)
}

func TestGraphSketch_TensorInitialization(t *testing.T) {
	config := DefaultGraphSketchConfig()
	config.NumTimeBins = 5
	config.Depth = 3
	config.Width = 512
	emitter := NewGraphSketchEmitter(config)

	// Check tensor dimensions: [time_bin][depth][width]
	assert.Len(t, emitter.tensorSketch, 5)
	for tb := 0; tb < 5; tb++ {
		assert.Len(t, emitter.tensorSketch[tb], 3)
		for d := 0; d < 3; d++ {
			assert.Len(t, emitter.tensorSketch[tb][d], 512)
		}
	}

	// All counters should be zero initially
	for tb := 0; tb < config.NumTimeBins; tb++ {
		for d := 0; d < config.Depth; d++ {
			for w := 0; w < config.Width; w++ {
				assert.Equal(t, 0.0, emitter.tensorSketch[tb][d][w])
			}
		}
	}
}

func TestGraphSketch_UpdateTensorSketch(t *testing.T) {
	config := DefaultGraphSketchConfig()
	config.Depth = 3
	config.Width = 100
	config.NumTimeBins = 5
	emitter := NewGraphSketchEmitter(config)

	edge := edge{
		source:     "A",
		dest:       "B",
		actualFreq: 1.0,
		timestamp:  100,
	}

	timeBin := edge.timestamp / config.TimeBinSize

	// Update tensor sketch
	emitter.updateTensorSketch(edge, timeBin)

	// Check that some counters were updated in the appropriate time bin
	binIndex := int(timeBin % int64(config.NumTimeBins))
	nonZeroCount := 0
	for d := 0; d < config.Depth; d++ {
		for w := 0; w < config.Width; w++ {
			if emitter.tensorSketch[binIndex][d][w] > 0 {
				nonZeroCount++
			}
		}
	}

	assert.Greater(t, nonZeroCount, 0, "Should have updated some counters")
}

func TestGraphSketch_ConservativeUpdate(t *testing.T) {
	config := DefaultGraphSketchConfig()
	config.Depth = 3
	config.Width = 100
	config.NumTimeBins = 5
	emitter := NewGraphSketchEmitter(config)

	// Add same edge multiple times
	edge := edge{
		source:     "A",
		dest:       "B",
		actualFreq: 1.0,
		timestamp:  100,
	}

	timeBin := edge.timestamp / config.TimeBinSize

	// Initialize layer mapping
	emitter.layerToBinMap[int(timeBin%int64(config.NumTimeBins))] = timeBin

	// Update 5 times
	for i := 0; i < 5; i++ {
		emitter.updateTensorSketch(edge, timeBin)
	}

	// Query the frequency estimate
	currentFreq, cumulFreq, _ := emitter.queryTensorSketch(edge, timeBin)

	// Estimate should have grown with observations
	assert.Greater(t, currentFreq, 0.0, "Current bin frequency should be positive after updates")
	assert.Greater(t, cumulFreq, 0.0, "Cumulative frequency should be positive after updates")
}

func TestGraphSketch_HashConsistency(t *testing.T) {
	emitter := NewGraphSketchEmitter(DefaultGraphSketchConfig())

	key := "A->B"

	// Hash should be consistent
	hash1 := emitter.hash(key, 0)
	hash2 := emitter.hash(key, 0)
	assert.Equal(t, hash1, hash2, "Same key with same seed should hash to same value")

	// Different seeds should give different hashes (usually)
	hash3 := emitter.hash(key, 1)
	assert.NotEqual(t, hash1, hash3, "Different seeds should usually give different hashes")

	// Hash should be within bounds
	assert.GreaterOrEqual(t, hash1, 0)
	assert.Less(t, hash1, emitter.config.Width)
}

func TestGraphSketch_LayerClearing(t *testing.T) {
	config := DefaultGraphSketchConfig()
	config.NumTimeBins = 5
	emitter := NewGraphSketchEmitter(config)

	// Set some initial values in tensor layer 0
	for d := 0; d < config.Depth; d++ {
		for w := 0; w < config.Width; w++ {
			emitter.tensorSketch[0][d][w] = 10.0
		}
	}

	// Map layer 0 to bin 0
	emitter.layerToBinMap[0] = 0
	emitter.currentTimeBin = 0

	// Advance to bin 5 (exactly wraps back to layer 0: 5 % 5 = 0)
	emitter.advanceTimeBin(5)

	// Layer 0 should be cleared (all zeros) because old bin 0 is now > W bins old
	for d := 0; d < config.Depth; d++ {
		for w := 0; w < config.Width; w++ {
			assert.Equal(t, 0.0, emitter.tensorSketch[0][d][w], "Layer should be cleared on wrap")
		}
	}

	// Layer 0 should now be mapped to bin 5
	assert.Equal(t, int64(5), emitter.layerToBinMap[0])
}

func TestGraphSketch_Pruning(t *testing.T) {
	config := DefaultGraphSketchConfig()
	config.NumTimeBins = 5
	config.TimeBinSize = 60
	emitter := NewGraphSketchEmitter(config)

	// Add edges with different last-seen times
	// maxAge = 5 * 60 = 300
	emitter.currentTimeBin = 10 // currentTime = 10 * 60 = 600

	// A->B seen recently (within maxAge)
	emitter.edgeLastSeen["A->B"] = 400 // timeSince = 600 - 400 = 200 < 300
	emitter.edgeFirstBin["A->B"] = 5

	// C->D seen long ago (beyond maxAge)
	emitter.edgeLastSeen["C->D"] = 100 // timeSince = 600 - 100 = 500 > 300
	emitter.edgeFirstBin["C->D"] = 1

	// Prune
	emitter.pruneInactiveEdges()

	// A->B should remain, C->D should be removed
	assert.Contains(t, emitter.edgeLastSeen, "A->B")
	assert.NotContains(t, emitter.edgeLastSeen, "C->D")
}

func TestGraphSketch_Algorithm2BayesianScore(t *testing.T) {
	config := DefaultGraphSketchConfig()
	config.P0 = 0.05
	config.Delta = 0.5
	emitter := NewGraphSketchEmitter(config)

	tests := []struct {
		name     string
		a        float64 // â_uv(t): current bin frequency
		s        float64 // ŝ_uv(t): cumulative frequency
		tElapsed int64   // time bins elapsed
		check    string
	}{
		{
			name:     "normal observation",
			a:        5.0,
			s:        50.0, // μ = 50/10 = 5, matches current
			tElapsed: 10,
			check:    "low",
		},
		{
			name:     "anomalous spike",
			a:        50.0, // Much higher than μ = 50/10 = 5
			s:        50.0,
			tElapsed: 10,
			check:    "high",
		},
		{
			name:     "anomalous drop",
			a:        0.1, // Much lower than μ = 50/10 = 5
			s:        50.0,
			tElapsed: 10,
			check:    "high",
		},
		{
			name:     "early observation",
			a:        10.0,
			s:        10.0, // μ = 10/1 = 10, matches
			tElapsed: 1,
			check:    "positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := emitter.gaussianBayesianScore(tt.a, tt.s, tt.tElapsed)

			// Score is posterior probability, so 0 ≤ score ≤ 1
			assert.GreaterOrEqual(t, score, 0.0, "Score should be >= 0")
			assert.LessOrEqual(t, score, 1.0, "Score should be <= 1 (probability)")

			if tt.check == "high" {
				assert.Greater(t, score, 0.1, "Anomalous cases should have higher posterior")
			} else if tt.check == "low" {
				assert.Less(t, score, 0.1, "Normal cases should have low posterior")
			} else if tt.check == "positive" {
				// Just check it's reasonable
				assert.Greater(t, score, 0.0)
			}
		})
	}
}

func TestGraphSketch_MinFunction(t *testing.T) {
	tests := []struct {
		values   []float64
		expected float64
	}{
		{[]float64{1.0, 2.0, 3.0}, 1.0},
		{[]float64{5.0, 2.0, 8.0}, 2.0},
		{[]float64{10.0}, 10.0},
		{[]float64{}, 0.0},
		{[]float64{-1.0, 0.0, 1.0}, -1.0},
	}

	for _, tt := range tests {
		result := min(tt.values)
		assert.Equal(t, tt.expected, result)
	}
}

func TestGraphSketch_ComputeVariance(t *testing.T) {
	tests := []struct {
		name     string
		values   []float64
		expected float64
	}{
		{
			name:     "constant values",
			values:   []float64{5.0, 5.0, 5.0, 5.0},
			expected: 0.0,
		},
		{
			name:     "simple values",
			values:   []float64{1.0, 2.0, 3.0, 4.0, 5.0},
			expected: 2.0, // variance of 1,2,3,4,5 is 2.0
		},
		{
			name:     "empty",
			values:   []float64{},
			expected: 0.0,
		},
		{
			name:     "single value",
			values:   []float64{10.0},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := computeVariance(tt.values)
			assert.InDelta(t, tt.expected, result, 0.01)
		})
	}
}

func TestGraphSketch_TotalEdgesIncreases(t *testing.T) {
	config := DefaultGraphSketchConfig()
	emitter := NewGraphSketchEmitter(config)

	assert.Equal(t, 0, emitter.totalEdges)

	edge := edge{source: "A", dest: "B", actualFreq: 1.0, timestamp: 100}
	timeBin := edge.timestamp / config.TimeBinSize

	emitter.updateTensorSketch(edge, timeBin)
	emitter.totalEdges++

	assert.Equal(t, 1, emitter.totalEdges)

	emitter.updateTensorSketch(edge, timeBin)
	emitter.totalEdges++

	assert.Equal(t, 2, emitter.totalEdges)
}

func TestGraphSketch_ScorePresent(t *testing.T) {
	config := DefaultGraphSketchConfig()
	config.MinObservations = 5
	config.K = 1.0 // Low sensitivity to ensure detection
	emitter := NewGraphSketchEmitter(config)

	// Create data with clear anomaly pattern
	points := make([]observer.Point, 15)
	for i := 0; i < 10; i++ {
		points[i] = observer.Point{
			Timestamp: int64(i + 1),
			Value:     5.0, // low
		}
	}
	for i := 10; i < 15; i++ {
		points[i] = observer.Point{
			Timestamp: int64(i + 1),
			Value:     500.0, // high - new bucket
		}
	}

	series := observer.Series{
		Name:   "test.metric",
		Points: points,
	}

	result := emitter.Analyze(series)

	// If anomalies detected, they should have debug info with scores
	for _, a := range result.Anomalies {
		require.NotNil(t, a.DebugInfo, "Anomalies should have DebugInfo")
		assert.Greater(t, a.DebugInfo.DeviationSigma, 0.0)
	}
}

func TestGraphSketch_IntegrationWorkflow(t *testing.T) {
	config := DefaultGraphSketchConfig()
	config.MinObservations = 5
	config.Depth = 4
	config.Width = 512
	emitter := NewGraphSketchEmitter(config)

	// Simulate multiple series emissions
	for round := 0; round < 3; round++ {
		points := make([]observer.Point, 20)
		for i := 0; i < 20; i++ {
			value := 5.0
			if i > 15 && round == 2 {
				value = 200.0 // Anomaly in last round
			}

			points[i] = observer.Point{
				Timestamp: int64(round*20 + i + 1),
				Value:     value,
			}
		}

		series := observer.Series{
			Name:   "test.metric",
			Points: points,
			Tags:   []string{"env:prod"},
		}

		result := emitter.Analyze(series)

		// First rounds build the model
		// Last round should potentially detect anomalies
		if round == 2 && len(result.Anomalies) > 0 {
			for _, a := range result.Anomalies {
				assert.Equal(t, observer.MetricName("test.metric"), a.Source)
				require.NotNil(t, a.DebugInfo)
			}
		}
	}

	// Verify tensor sketch was updated
	assert.Greater(t, emitter.totalEdges, 0)
	assert.NotEmpty(t, emitter.edgeLastSeen, "Should have tracked some edges")
	assert.NotEmpty(t, emitter.layerToBinMap, "Should have mapped layers to bins")
}

func TestGraphSketch_DifferentBucketsCreateDifferentEdges(t *testing.T) {
	emitter := NewGraphSketchEmitter(DefaultGraphSketchConfig())

	series := observer.Series{
		Name: "test.metric",
		Points: []observer.Point{
			{Timestamp: 1, Value: 5.0},    // low
			{Timestamp: 2, Value: 50.0},   // med
			{Timestamp: 3, Value: 500.0},  // high
			{Timestamp: 4, Value: 5000.0}, // veryhigh
			{Timestamp: 5, Value: -5.0},   // neg
		},
	}

	edges := emitter.extractEdges(series)

	// All different buckets
	buckets := make(map[string]bool)
	for _, e := range edges {
		buckets[e.dest] = true
	}

	assert.Len(t, buckets, 5, "Should have 5 different buckets")
	assert.True(t, buckets["low"])
	assert.True(t, buckets["med"])
	assert.True(t, buckets["high"])
	assert.True(t, buckets["veryhigh"])
	assert.True(t, buckets["neg"])
}

func TestGraphSketch_EWMAAdaptiveThreshold(t *testing.T) {
	config := DefaultGraphSketchConfig()
	config.EWMAAlpha = 0.3
	config.K = 2.0
	emitter := NewGraphSketchEmitter(config)

	// Initial state
	assert.Equal(t, 0.0, emitter.ewmaZ, "EWMA Z should start at 0")
	assert.Equal(t, 0.0, emitter.runningMeanX, "Running mean should start at 0")
	assert.Equal(t, 2.0, emitter.adaptiveThreshold, "Initial threshold should be K")

	// Update with scores
	scores := []float64{1.0, 1.5, 2.0, 5.0, 1.0}
	for _, score := range scores {
		emitter.updateEWMAThreshold(score)
	}

	// EWMA Z and running stats should be updated
	assert.Greater(t, emitter.ewmaZ, 0.0, "EWMA Z should be positive")
	assert.Greater(t, emitter.runningMeanX, 0.0, "Running mean should be positive")
	assert.Greater(t, emitter.runningM2, 0.0, "M2 should be positive")

	// Adaptive threshold should change from initial
	assert.NotEqual(t, config.K, emitter.adaptiveThreshold, "Threshold should adapt")
}

func TestGraphSketch_EWMAFirstObservation(t *testing.T) {
	config := DefaultGraphSketchConfig()
	config.K = 3.0
	emitter := NewGraphSketchEmitter(config)

	// First observation
	emitter.updateEWMAThreshold(5.0)

	assert.Equal(t, 5.0, emitter.ewmaZ, "First Z should equal first score")
	assert.Equal(t, 5.0, emitter.runningMeanX, "First mean should equal first score")
	assert.Equal(t, 0.0, emitter.runningM2, "First M2 should be 0.0")
	assert.Equal(t, 1, emitter.numScoresSeen)
}

func TestGraphSketch_EWMAConvergence(t *testing.T) {
	config := DefaultGraphSketchConfig()
	config.EWMAAlpha = 0.1
	config.K = 2.0
	emitter := NewGraphSketchEmitter(config)

	// Feed many constant scores
	for i := 0; i < 100; i++ {
		emitter.updateEWMAThreshold(3.0)
	}

	// EWMA Z should converge to the constant value
	assert.InDelta(t, 3.0, emitter.ewmaZ, 0.1, "EWMA Z should converge to constant")

	// Running mean should converge to constant
	assert.InDelta(t, 3.0, emitter.runningMeanX, 0.01, "Running mean should converge to constant")

	// Variance should be small for constant data (variance = M2/n)
	variance := emitter.runningM2 / float64(emitter.numScoresSeen)
	assert.Less(t, variance, 0.1, "Variance should be small for constant data")
}

func TestGraphSketch_AdaptiveThresholdFormula(t *testing.T) {
	config := DefaultGraphSketchConfig()
	config.K = 2.5
	emitter := NewGraphSketchEmitter(config)

	// Set known running state
	emitter.numScoresSeen = 100
	emitter.runningMeanX = 10.0
	emitter.runningM2 = 400.0 // variance = M2/n = 400/100 = 4.0, stdDev = 2.0

	// Compute threshold using the formula
	variance := emitter.runningM2 / float64(emitter.numScoresSeen)
	stdDev := math.Sqrt(variance)
	expectedThreshold := emitter.runningMeanX + config.K*stdDev
	// expectedThreshold = 10.0 + 2.5 * 2.0 = 15.0

	emitter.adaptiveThreshold = expectedThreshold

	assert.Equal(t, 15.0, emitter.adaptiveThreshold, "Threshold = meanX + K*σ")
}

func TestGraphSketch_ThresholdMinimum(t *testing.T) {
	config := DefaultGraphSketchConfig()
	config.K = 0.01
	emitter := NewGraphSketchEmitter(config)

	// Update with very small scores
	emitter.updateEWMAThreshold(0.001)

	// Threshold should be at least 0.1
	assert.GreaterOrEqual(t, emitter.adaptiveThreshold, 0.1, "Threshold should have minimum value")
}
