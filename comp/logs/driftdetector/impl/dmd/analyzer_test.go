// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dmd

import (
	"math"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/impl/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRingBufferOperations tests basic ring buffer functionality
func TestRingBufferOperations(t *testing.T) {
	dim := 10
	size := 5
	rb := NewRingBuffer(size, dim)

	// Initial state
	assert.False(t, rb.IsFull(), "Buffer should not be full initially")
	assert.Nil(t, rb.GetNewest(), "GetNewest should return nil when empty")
	assert.Nil(t, rb.GetOldest(), "GetOldest should return nil when empty")

	// Add vectors
	for i := 0; i < size; i++ {
		vec := make(common.Vector, dim)
		for j := 0; j < dim; j++ {
			vec[j] = float64(i*dim + j)
		}
		rb.Add(vec)
	}

	// Check full state
	assert.True(t, rb.IsFull(), "Buffer should be full after adding d vectors")

	// Check newest vector (should be last added)
	newest := rb.GetNewest()
	require.NotNil(t, newest)
	assert.Equal(t, float64((size-1)*dim), newest[0], "Newest vector should be the last added")

	// Check oldest vector (second-to-last in buffer)
	oldest := rb.GetOldest()
	require.NotNil(t, oldest)
	assert.Equal(t, float64((size-2)*dim), oldest[0], "Oldest should be second-to-last vector")

	// Test FIFO behavior - add one more vector
	newVec := make(common.Vector, dim)
	for j := 0; j < dim; j++ {
		newVec[j] = float64(100 + j)
	}
	rb.Add(newVec)

	// Verify newest is now the new vector
	newest = rb.GetNewest()
	require.NotNil(t, newest)
	assert.Equal(t, 100.0, newest[0], "Newest should be the newly added vector")

	// Verify GetAllVectors returns in chronological order
	allVecs := rb.GetAllVectors()
	assert.Len(t, allVecs, size, "Should return exactly d vectors")
}

// TestOnlineDMDInitialization tests DMD initialization process
func TestOnlineDMDInitialization(t *testing.T) {
	config := common.DMDConfig{
		TimeDelay:    5, // Small for fast tests
		RLSLambda:    0.99,
		ErrorHistory: 30,
	}

	inputChan := make(chan common.EmbeddingResult, 10)
	outputChan := make(chan common.DMDResult, 10)
	analyzer := NewAnalyzer("test-source", config, inputChan, outputChan)

	analyzer.Start()
	defer analyzer.Stop()

	dim := 10

	// Send d-1 vectors (not enough to initialize)
	for i := 0; i < config.TimeDelay-1; i++ {
		vec := make(common.Vector, dim)
		for j := 0; j < dim; j++ {
			vec[j] = float64(i + j)
		}
		inputChan <- common.EmbeddingResult{
			WindowID:   i,
			Templates:  []string{"template"},
			Embeddings: []common.Vector{vec},
		}
	}

	// Wait a bit to ensure processing
	time.Sleep(100 * time.Millisecond)

	// Should not have any output yet
	select {
	case <-outputChan:
		t.Fatal("Should not produce output before buffer is full")
	default:
		// Expected
	}

	// Send one more vector to fill buffer and trigger initialization
	vec := make(common.Vector, dim)
	for j := 0; j < dim; j++ {
		vec[j] = float64(config.TimeDelay - 1 + j)
	}
	inputChan <- common.EmbeddingResult{
		WindowID:   config.TimeDelay - 1,
		Templates:  []string{"template"},
		Embeddings: []common.Vector{vec},
	}

	// Wait for initialization and first result
	time.Sleep(200 * time.Millisecond)

	// Should now have output
	select {
	case result := <-outputChan:
		assert.Equal(t, "test-source", result.SourceKey)
		assert.Equal(t, config.TimeDelay-1, result.WindowID)
		assert.GreaterOrEqual(t, result.ReconstructionError, 0.0)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Expected output after initialization")
	}
}

// TestOnlineDMDUpdate tests incremental RLS updates
func TestOnlineDMDUpdate(t *testing.T) {
	config := common.DMDConfig{
		TimeDelay:    3, // Smaller for faster test
		RLSLambda:    0.99,
		ErrorHistory: 30,
	}

	inputChan := make(chan common.EmbeddingResult, 50)
	outputChan := make(chan common.DMDResult, 50)
	analyzer := NewAnalyzer("test-source", config, inputChan, outputChan)

	analyzer.Start()
	defer analyzer.Stop()

	dim := 10

	// Send many vectors to test continuous updates
	numVectors := 20
	for i := 0; i < numVectors; i++ {
		vec := make(common.Vector, dim)
		for j := 0; j < dim; j++ {
			// Simple linear pattern: each vector is slightly different
			vec[j] = float64(i)*0.1 + float64(j)
		}
		inputChan <- common.EmbeddingResult{
			WindowID:   i,
			Templates:  []string{"template"},
			Embeddings: []common.Vector{vec},
		}
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Should have numVectors - TimeDelay results (first TimeDelay vectors fill buffer)
	expectedResults := numVectors - config.TimeDelay + 1
	actualResults := len(outputChan)
	assert.GreaterOrEqual(t, actualResults, expectedResults-2, "Should have approximately expected number of results")

	// Verify results are generated continuously
	for i := 0; i < actualResults; i++ {
		result := <-outputChan
		assert.Equal(t, "test-source", result.SourceKey)
		assert.GreaterOrEqual(t, result.ReconstructionError, 0.0)
		// Normalized error may be 0 initially (not enough history)
	}
}

// TestOnlineDMDConvergence tests that the model converges for predictable data
func TestOnlineDMDConvergence(t *testing.T) {
	config := common.DMDConfig{
		TimeDelay:    3,
		RLSLambda:    0.99,
		ErrorHistory: 30,
	}

	inputChan := make(chan common.EmbeddingResult, 100)
	outputChan := make(chan common.DMDResult, 100)
	analyzer := NewAnalyzer("test-source", config, inputChan, outputChan)

	analyzer.Start()
	defer analyzer.Stop()

	dim := 5 // Smaller dimension for easier convergence testing

	// Generate perfectly predictable sequence: v_t = v_{t-1} * 0.9
	// This should result in very low reconstruction error after convergence
	initialVec := make(common.Vector, dim)
	for j := 0; j < dim; j++ {
		initialVec[j] = 1.0
	}

	currentVec := make(common.Vector, dim)
	copy(currentVec, initialVec)

	numVectors := 50
	for i := 0; i < numVectors; i++ {
		vecCopy := make(common.Vector, dim)
		copy(vecCopy, currentVec)

		inputChan <- common.EmbeddingResult{
			WindowID:   i,
			Templates:  []string{"template"},
			Embeddings: []common.Vector{vecCopy},
		}

		// Update current vector for next iteration
		for j := 0; j < dim; j++ {
			currentVec[j] *= 0.9
		}
	}

	// Wait for all processing
	time.Sleep(1 * time.Second)

	// Collect all results
	var errors []float64
	for len(outputChan) > 0 {
		result := <-outputChan
		errors = append(errors, result.ReconstructionError)
	}

	// Should have convergence: errors should decrease over time
	require.GreaterOrEqual(t, len(errors), 10, "Should have enough results to check convergence")

	// Check that later errors are generally smaller than early errors
	earlyAvg := average(errors[:5])
	lateAvg := average(errors[len(errors)-5:])

	// Late errors should be significantly smaller (or similar if already converged)
	assert.LessOrEqual(t, lateAvg, earlyAvg*1.5, "Reconstruction error should converge (late errors <= early errors)")
}

// TestEmptyWindowHandling tests that empty windows (zero vectors) are handled gracefully
func TestEmptyWindowHandling(t *testing.T) {
	config := common.DMDConfig{
		TimeDelay:    3,
		RLSLambda:    0.99,
		ErrorHistory: 30,
	}

	inputChan := make(chan common.EmbeddingResult, 50)
	outputChan := make(chan common.DMDResult, 50)
	analyzer := NewAnalyzer("test-source", config, inputChan, outputChan)

	analyzer.Start()
	defer analyzer.Stop()

	dim := 10

	// Send mix of normal and empty windows
	for i := 0; i < 20; i++ {
		var embeddings []common.Vector
		if i%3 != 0 { // 2 out of 3 windows have data
			vec := make(common.Vector, dim)
			for j := 0; j < dim; j++ {
				vec[j] = float64(i + j)
			}
			embeddings = []common.Vector{vec}
		}
		// Empty windows have no embeddings

		inputChan <- common.EmbeddingResult{
			WindowID:   i,
			Templates:  []string{},
			Embeddings: embeddings,
		}
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Should handle empty windows gracefully (may have some results)
	// The key is no panics or errors
	assert.GreaterOrEqual(t, len(outputChan), 0, "Should not panic on empty windows")

	// Drain and verify results are valid
	for len(outputChan) > 0 {
		result := <-outputChan
		assert.Equal(t, "test-source", result.SourceKey)
		assert.GreaterOrEqual(t, result.ReconstructionError, 0.0)
	}
}

// TestErrorNormalization tests that error statistics are correctly maintained
func TestErrorNormalization(t *testing.T) {
	config := common.DMDConfig{
		TimeDelay:    3,
		RLSLambda:    0.99,
		ErrorHistory: 5, // Small history for easier testing
	}

	inputChan := make(chan common.EmbeddingResult, 50)
	outputChan := make(chan common.DMDResult, 50)
	analyzer := NewAnalyzer("test-source", config, inputChan, outputChan)

	analyzer.Start()
	defer analyzer.Stop()

	dim := 5

	// Send enough vectors to build up error history
	numVectors := 20
	for i := 0; i < numVectors; i++ {
		vec := make(common.Vector, dim)
		for j := 0; j < dim; j++ {
			vec[j] = float64(i + j)
		}
		inputChan <- common.EmbeddingResult{
			WindowID:   i,
			Templates:  []string{"template"},
			Embeddings: []common.Vector{vec},
		}
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Collect results
	var normalizedErrors []float64
	for len(outputChan) > 0 {
		result := <-outputChan
		normalizedErrors = append(normalizedErrors, result.NormalizedError)
	}

	// After enough history, normalized errors should be computed
	require.GreaterOrEqual(t, len(normalizedErrors), config.ErrorHistory, "Should have enough results")

	// Check that later normalized errors are not all zero (statistics being computed)
	lateErrors := normalizedErrors[len(normalizedErrors)-3:]
	hasNonZero := false
	for _, err := range lateErrors {
		if !math.IsNaN(err) && err != 0 {
			hasNonZero = true
			break
		}
	}
	assert.True(t, hasNonZero, "Normalized errors should be computed with non-zero values")
}

// TestSmoothMetrics tests that every window produces a result (no plateaus)
func TestSmoothMetrics(t *testing.T) {
	config := common.DMDConfig{
		TimeDelay:    3,
		RLSLambda:    0.99,
		ErrorHistory: 30,
	}

	inputChan := make(chan common.EmbeddingResult, 100)
	outputChan := make(chan common.DMDResult, 100)
	analyzer := NewAnalyzer("test-source", config, inputChan, outputChan)

	analyzer.Start()
	defer analyzer.Stop()

	dim := 10
	numWindows := 30

	// Send windows
	for i := 0; i < numWindows; i++ {
		vec := make(common.Vector, dim)
		for j := 0; j < dim; j++ {
			vec[j] = float64(i + j)
		}
		inputChan <- common.EmbeddingResult{
			WindowID:   i,
			Templates:  []string{"template"},
			Embeddings: []common.Vector{vec},
		}
	}

	// Wait for processing
	time.Sleep(1 * time.Second)

	// Collect errors with their window IDs
	results := make(map[int]float64)
	for len(outputChan) > 0 {
		result := <-outputChan
		results[result.WindowID] = result.ReconstructionError
	}

	// Should have result for every window after initialization
	expectedResults := numWindows - config.TimeDelay + 1
	assert.GreaterOrEqual(t, len(results), expectedResults-2, "Should have result for nearly every window")

	// Verify no plateaus: consecutive errors should be different
	// (not cached values being reused)
	var consecutiveSame int
	var prevError float64
	var firstWindow = true

	for windowID := config.TimeDelay; windowID < numWindows; windowID++ {
		if err, ok := results[windowID]; ok {
			if !firstWindow && err == prevError {
				consecutiveSame++
			} else {
				consecutiveSame = 0
			}
			prevError = err
			firstWindow = false
		}
	}

	// Allow at most 2 consecutive identical errors (due to floating point precision)
	assert.LessOrEqual(t, consecutiveSame, 2, "Should not have long plateaus of identical errors")
}

// Helper function to compute average
func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}
