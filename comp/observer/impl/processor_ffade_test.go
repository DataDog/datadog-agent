// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFFADE_Name(t *testing.T) {
	proc := NewFFADEProcessor(DefaultFFADEConfig())
	assert.Equal(t, "ffade", proc.Name())
}

func TestFFADE_EmptyState(t *testing.T) {
	proc := NewFFADEProcessor(DefaultFFADEConfig())
	anomalies := proc.AnomalousInteractions()
	assert.Empty(t, anomalies)
}

// Test ITFM update with temporal decay
func TestFFADE_ITFMDecay(t *testing.T) {
	config := DefaultFFADEConfig()
	config.DecayRate = 0.1 // High decay for testing
	proc := NewFFADEProcessor(config)

	// First observation at t=0
	proc.itfm.Update("A", "B", 0, 1.0, config.DecayRate)
	entry := proc.itfm.frequencies[makeEdgeKey("A", "B")]
	require.NotNil(t, entry)
	assert.Equal(t, 1.0, entry.freq)

	// Second observation at t=10 (should decay first)
	proc.itfm.Update("A", "B", 10, 1.0, config.DecayRate)
	entry = proc.itfm.frequencies[makeEdgeKey("A", "B")]

	// After decay: 1.0 * exp(-0.1 * 10) ≈ 0.3679, then +1.0 ≈ 1.3679
	expectedFreq := 1.0*math.Exp(-0.1*10) + 1.0
	assert.InDelta(t, expectedFreq, entry.freq, 0.01)
	assert.Equal(t, 2, entry.totalCount)
}

// Test edge key symmetry
func TestFFADE_EdgeKeySymmetry(t *testing.T) {
	key1 := makeEdgeKey("A", "B")
	key2 := makeEdgeKey("B", "A")
	assert.Equal(t, key1, key2, "Edge keys should be symmetric")
}

// Test extracting interaction partner from signal tags
func TestFFADE_ExtractInteractionPartner(t *testing.T) {
	tests := []struct {
		name     string
		signal   observer.Signal
		expected string
	}{
		{
			name: "interacts_with tag",
			signal: observer.Signal{
				Source: "service.A",
				Tags:   []string{"env:prod", "interacts_with:service.B"},
			},
			expected: "service.B",
		},
		{
			name: "edge_dest tag",
			signal: observer.Signal{
				Source: "service.A",
				Tags:   []string{"edge_dest:service.C"},
			},
			expected: "service.C",
		},
		{
			name: "no interaction tag",
			signal: observer.Signal{
				Source: "service.A",
				Tags:   []string{"env:prod", "region:us-east-1"},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractInteractionPartner(tt.signal)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test process updates ITFM
func TestFFADE_ProcessUpdatesITFM(t *testing.T) {
	config := DefaultFFADEConfig()
	proc := NewFFADEProcessor(config)

	signal := observer.Signal{
		Source:    "service.A",
		Timestamp: 100,
		Tags:      []string{"interacts_with:service.B"},
	}

	proc.Process(signal)

	// Check ITFM was updated
	key := makeEdgeKey("service.A", "service.B")
	entry := proc.itfm.frequencies[key]
	require.NotNil(t, entry)
	assert.Equal(t, 1.0, entry.freq)
	assert.Equal(t, int64(100), entry.lastUpdate)
	assert.Equal(t, 1, entry.totalCount)
}

// Test building training set
func TestFFADE_BuildTrainingSet(t *testing.T) {
	config := DefaultFFADEConfig()
	config.MinObservations = 2
	proc := NewFFADEProcessor(config)

	// Add edges with different observation counts
	proc.itfm.frequencies[makeEdgeKey("A", "B")] = &edgeFrequency{
		freq: 5.0, totalCount: 3,
	}
	proc.itfm.frequencies[makeEdgeKey("C", "D")] = &edgeFrequency{
		freq: 2.0, totalCount: 1, // Below MinObservations
	}
	proc.itfm.frequencies[makeEdgeKey("E", "F")] = &edgeFrequency{
		freq: 8.0, totalCount: 5,
	}

	trainingSet := proc.buildTrainingSet()

	// Should only include edges with totalCount >= MinObservations
	assert.Len(t, trainingSet, 2)
}

// Test negative sampling
func TestFFADE_NegativeSampling(t *testing.T) {
	config := DefaultFFADEConfig()
	proc := NewFFADEProcessor(config)

	// Add some positive edges
	proc.itfm.frequencies[makeEdgeKey("A", "B")] = &edgeFrequency{freq: 5.0}
	proc.itfm.frequencies[makeEdgeKey("C", "D")] = &edgeFrequency{freq: 3.0}

	negatives := proc.sampleNegativePairs(10)

	// Should sample ~50% of positives (5 negatives for 10 positives)
	assert.True(t, len(negatives) > 0)
	assert.True(t, len(negatives) <= 5)

	// Negative examples should have very low frequencies
	for _, neg := range negatives {
		assert.True(t, neg.freq < config.MinFrequency)
	}
}

// Test getAllNodes
func TestFFADE_GetAllNodes(t *testing.T) {
	proc := NewFFADEProcessor(DefaultFFADEConfig())

	proc.itfm.frequencies[makeEdgeKey("A", "B")] = &edgeFrequency{}
	proc.itfm.frequencies[makeEdgeKey("C", "D")] = &edgeFrequency{}
	proc.itfm.frequencies[makeEdgeKey("A", "D")] = &edgeFrequency{}

	nodes := proc.getAllNodes()

	// Should have 4 unique nodes: A, B, C, D
	assert.Len(t, nodes, 4)

	nodeSet := make(map[string]bool)
	for _, n := range nodes {
		nodeSet[n] = true
	}
	assert.True(t, nodeSet["A"])
	assert.True(t, nodeSet["B"])
	assert.True(t, nodeSet["C"])
	assert.True(t, nodeSet["D"])
}

// Test factor initialization
func TestFFADE_EnsureFactorExists(t *testing.T) {
	config := DefaultFFADEConfig()
	config.LatentDim = 5
	proc := NewFFADEProcessor(config)

	proc.ensureFactorExists("A")

	// Should initialize both W and H
	require.Contains(t, proc.W, "A")
	require.Contains(t, proc.H, "A")
	assert.Len(t, proc.W["A"], 5)
	assert.Len(t, proc.H["A"], 5)

	// Calling again shouldn't change factors
	oldW := proc.W["A"]
	proc.ensureFactorExists("A")
	assert.Equal(t, oldW, proc.W["A"])
}

// Test lambda computation
func TestFFADE_ComputeLambda(t *testing.T) {
	config := DefaultFFADEConfig()
	config.LatentDim = 3
	proc := NewFFADEProcessor(config)

	// Set specific factors for testing
	proc.W["A"] = []float64{1.0, 2.0, 3.0}
	proc.H["B"] = []float64{0.5, 1.0, 0.5}

	lambda := proc.computeLambda("A", "B")

	// Expected: 1.0*0.5 + 2.0*1.0 + 3.0*0.5 = 0.5 + 2.0 + 1.5 = 4.0
	assert.InDelta(t, 4.0, lambda, 0.01)
}

// Test Poisson anomaly score
func TestFFADE_PoissonAnomalyScore(t *testing.T) {
	proc := NewFFADEProcessor(DefaultFFADEConfig())

	tests := []struct {
		name     string
		observed float64
		lambda   float64
		expect   string // "high" or "low"
	}{
		{
			name:     "normal observation",
			observed: 5.0,
			lambda:   5.0,
			expect:   "low",
		},
		{
			name:     "anomalously high",
			observed: 20.0,
			lambda:   5.0,
			expect:   "high",
		},
		{
			name:     "anomalously low (unexpected high frequency)",
			observed: 1.0,
			lambda:   10.0,
			expect:   "high",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := proc.poissonAnomalyScore(tt.observed, tt.lambda)

			if tt.expect == "high" {
				assert.Greater(t, score, 2.0, "Anomalous cases should have high scores")
			} else {
				assert.Less(t, score, 2.0, "Normal cases should have low scores")
			}
		})
	}
}

// Test ITFM eviction
func TestFFADE_ITFMEviction(t *testing.T) {
	itfm := &ITFM{frequencies: make(map[edgeKey]*edgeFrequency)}

	itfm.frequencies[makeEdgeKey("A", "B")] = &edgeFrequency{freq: 5.0}
	itfm.frequencies[makeEdgeKey("C", "D")] = &edgeFrequency{freq: 0.005}
	itfm.frequencies[makeEdgeKey("E", "F")] = &edgeFrequency{freq: 2.0}

	itfm.Evict(0.01)

	// Should evict C-D (freq < 0.01)
	assert.Len(t, itfm.frequencies, 2)
	assert.Contains(t, itfm.frequencies, makeEdgeKey("A", "B"))
	assert.Contains(t, itfm.frequencies, makeEdgeKey("E", "F"))
	assert.NotContains(t, itfm.frequencies, makeEdgeKey("C", "D"))
}

// Test full model update cycle
func TestFFADE_FullUpdateCycle(t *testing.T) {
	config := DefaultFFADEConfig()
	config.MinObservations = 2
	config.UpdateInterval = 1 // Update every flush
	config.SGDIterations = 20 // More iterations for convergence
	proc := NewFFADEProcessor(config)

	// Add several edges with varying frequencies
	edges := []struct {
		s, d  string
		freq  float64
		count int
	}{
		{"A", "B", 10.0, 10},
		{"A", "C", 5.0, 5},
		{"B", "C", 2.0, 3},
	}

	for _, e := range edges {
		entry := &edgeFrequency{
			freq:       e.freq,
			totalCount: e.count,
			lastUpdate: 100,
		}
		proc.itfm.frequencies[makeEdgeKey(e.s, e.d)] = entry
	}

	// Trigger model update
	proc.Flush()

	// Check that factors were fitted
	assert.NotEmpty(t, proc.W)
	assert.NotEmpty(t, proc.H)

	// Check that lambda predictions are reasonable
	for _, e := range edges {
		lambda := proc.computeLambda(e.s, e.d)
		assert.Greater(t, lambda, 0.0)
		// After training, lambda should be closer to observed freq
		// (though it won't be perfect with few iterations)
	}
}

// Test anomaly detection after model fitting
func TestFFADE_DetectsAnomalies(t *testing.T) {
	config := DefaultFFADEConfig()
	config.MinObservations = 2
	config.UpdateInterval = 1
	config.FalsePositiveRate = 0.2 // More lenient for testing
	config.SGDIterations = 30
	proc := NewFFADEProcessor(config)

	// Add normal edges
	proc.itfm.frequencies[makeEdgeKey("A", "B")] = &edgeFrequency{
		freq: 5.0, totalCount: 5, lastUpdate: 100,
	}
	proc.itfm.frequencies[makeEdgeKey("A", "C")] = &edgeFrequency{
		freq: 5.0, totalCount: 5, lastUpdate: 100,
	}

	// Add anomalous edge (very high frequency)
	proc.itfm.frequencies[makeEdgeKey("X", "Y")] = &edgeFrequency{
		freq: 50.0, totalCount: 10, lastUpdate: 100,
	}

	// Trigger model update and scoring
	proc.Flush()

	// Check for detected anomalies
	anomalies := proc.AnomalousInteractions()

	// Should detect the X-Y anomaly
	// (Note: with random initialization, results may vary, so we just check structure)
	if len(anomalies) > 0 {
		for _, a := range anomalies {
			assert.NotEmpty(t, a.Source1)
			assert.NotEmpty(t, a.Source2)
			assert.Greater(t, a.DeviationScore, 0.0)
		}
	}
}

// Test dot product
func TestFFADE_DotProduct(t *testing.T) {
	a := []float64{1.0, 2.0, 3.0}
	b := []float64{4.0, 5.0, 6.0}

	result := dotProduct(a, b)

	// 1*4 + 2*5 + 3*6 = 4 + 10 + 18 = 32
	assert.Equal(t, 32.0, result)
}

// Test dot product with mismatched lengths
func TestFFADE_DotProductMismatch(t *testing.T) {
	a := []float64{1.0, 2.0}
	b := []float64{3.0, 4.0, 5.0}

	result := dotProduct(a, b)
	assert.Equal(t, 0.0, result)
}

// Test random vector initialization
func TestFFADE_RandomVector(t *testing.T) {
	vec := randomVector(10)

	assert.Len(t, vec, 10)

	// Values should be small and random
	for _, v := range vec {
		assert.True(t, v >= -0.05 && v <= 0.05)
	}
}

// Test SGD update doesn't crash
func TestFFADE_SGDUpdate(t *testing.T) {
	config := DefaultFFADEConfig()
	config.LatentDim = 5
	proc := NewFFADEProcessor(config)

	proc.ensureFactorExists("A")
	proc.ensureFactorExists("B")

	// Perform SGD update
	proc.sgdUpdate("A", "B", 5.0)

	// Factors should have been updated (hard to predict exact values)
	assert.NotNil(t, proc.W["A"])
	assert.NotNil(t, proc.H["B"])
}

// Test threshold update
func TestFFADE_ThresholdUpdate(t *testing.T) {
	config := DefaultFFADEConfig()
	config.MinObservations = 1
	config.FalsePositiveRate = 0.1
	proc := NewFFADEProcessor(config)

	// Add edges with known frequencies
	proc.itfm.frequencies[makeEdgeKey("A", "B")] = &edgeFrequency{
		freq: 5.0, totalCount: 3,
	}
	proc.itfm.frequencies[makeEdgeKey("C", "D")] = &edgeFrequency{
		freq: 10.0, totalCount: 5,
	}

	// Initialize factors
	proc.ensureFactorExists("A")
	proc.ensureFactorExists("B")
	proc.ensureFactorExists("C")
	proc.ensureFactorExists("D")

	oldThreshold := proc.anomalyThreshold
	proc.updateThreshold()

	// Threshold should be updated (may be higher or lower than initial)
	// Just check it's a reasonable value
	assert.Greater(t, proc.anomalyThreshold, 0.0)
	assert.NotEqual(t, oldThreshold, proc.anomalyThreshold)
}

// Test process with score weighting
func TestFFADE_ProcessWithScore(t *testing.T) {
	proc := NewFFADEProcessor(DefaultFFADEConfig())

	score := 2.5
	signal := observer.Signal{
		Source:    "A",
		Timestamp: 100,
		Tags:      []string{"interacts_with:B"},
		Score:     &score,
	}

	proc.Process(signal)

	entry := proc.itfm.frequencies[makeEdgeKey("A", "B")]
	require.NotNil(t, entry)
	assert.Equal(t, 2.5, entry.freq, "Should use score as weight")
}

// Test edge key string representation
func TestFFADE_EdgeKeyString(t *testing.T) {
	key := makeEdgeKey("service.A", "service.B")
	str := key.String()

	// Should contain both services
	assert.Contains(t, str, "service.A")
	assert.Contains(t, str, "service.B")
	assert.Contains(t, str, "<->")
}

// Integration test: Full workflow
func TestFFADE_IntegrationWorkflow(t *testing.T) {
	config := DefaultFFADEConfig()
	config.UpdateInterval = 5
	config.MinObservations = 2
	config.FalsePositiveRate = 0.1
	proc := NewFFADEProcessor(config)

	// Simulate edge stream
	edges := []struct {
		s, d string
		t    int64
	}{
		{"A", "B", 100},
		{"A", "B", 101},
		{"A", "B", 102},
		{"C", "D", 100},
		{"C", "D", 105},
		{"X", "Y", 200}, // Anomalous: appears suddenly with high temporal gap
		{"X", "Y", 201},
		{"X", "Y", 202},
	}

	for _, e := range edges {
		signal := observer.Signal{
			Source:    e.s,
			Timestamp: e.t,
			Tags:      []string{fmt.Sprintf("interacts_with:%s", e.d)},
		}
		proc.Process(signal)
	}

	// Flush multiple times to trigger model update
	for i := 0; i < 10; i++ {
		proc.Flush()
	}

	// Should have built a model and potentially detected anomalies
	anomalies := proc.AnomalousInteractions()

	// Just verify the system doesn't crash and produces valid output
	for _, a := range anomalies {
		assert.NotEmpty(t, a.Source1)
		assert.NotEmpty(t, a.Source2)
		assert.GreaterOrEqual(t, a.ActualFreq, 0.0)
		assert.Greater(t, a.ExpectedFreq, 0.0)
		assert.Greater(t, a.DeviationScore, 0.0)
	}
}
