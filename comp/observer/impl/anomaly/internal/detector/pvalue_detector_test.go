// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package detector

import (
	"testing"
)

func TestPValueDetector_ProgressiveScoring(t *testing.T) {
	detector := NewPValueDetector()

	// Feed increasing amounts of data
	normalResults := CreateNormalResults(30)
	for _, result := range normalResults {
		score, err := detector.ComputeScore(result)
		AssertNoError(t, err, "ComputeScore in progressive scoring")

		// PValue detector uses Z-scores, should be in reasonable range
		AssertFloatInRange(t, score, -10.0, 10.0, "PValue Z-score")
	}
}

func TestPValueDetector_AnomalousDetection(t *testing.T) {
	detector := NewPValueDetector()

	// Build up normal baseline
	normalResults := CreateNormalResults(50)
	for _, result := range normalResults {
		_, err := detector.ComputeScore(result)
		AssertNoError(t, err, "ComputeScore with normal baseline")
	}

	// Add anomalous results
	anomalousResults := CreateAnomalousResults(10)
	for _, result := range anomalousResults {
		_, err := detector.ComputeScore(result)
		AssertNoError(t, err, "ComputeScore with anomalous results")
	}
}

func TestPValueDetector_NormalResults(t *testing.T) {
	detector := NewPValueDetector()

	normalResults := CreateNormalResults(100)
	score, err := detector.ComputeScore(normalResults[len(normalResults)-1])
	AssertNoError(t, err, "ComputeScore with normal results")

	// PValue uses Z-scores, normal data should have reasonable Z-score
	AssertFloatInRange(t, score, -5.0, 5.0, "Normal data Z-score")
}

func TestPValueDetector_GradualDrift(t *testing.T) {
	detector := NewPValueDetector()

	// Establish baseline
	normalResults := CreateNormalResults(50)
	for _, result := range normalResults {
		_, err := detector.ComputeScore(result)
		AssertNoError(t, err, "ComputeScore with normal baseline")
	}

	// Process gradual drift
	driftResults := CreateGradualDriftResults(50)
	for _, result := range driftResults {
		_, err := detector.ComputeScore(result)
		AssertNoError(t, err, "ComputeScore with gradual drift")
	}
}
