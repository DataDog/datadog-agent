// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package detector

import (
	"testing"
)

func TestWeightedDetector_EmptyResults(t *testing.T) {
	detector := NewWeightedDetector()
	score, err := detector.ComputeScore(TelemetryResult{})
	AssertNoError(t, err, "ComputeScore with empty results")
	if score != 0 {
		t.Errorf("Expected score 0 for empty results, got %.3f", score)
	}
}

func TestWeightedDetector_PerfectScore(t *testing.T) {
	detector := NewWeightedDetector()
	result := TelemetryResult{
		CPU:                1.0,
		Mem:                1.0,
		Err:                1.0,
		ClientSentByClient: 1.0,
		ClientSentByServer: 1.0,
		ServerSentByClient: 1.0,
		ServerSentByServer: 1.0,
		TraceP50:           1.0,
		TraceP95:           1.0,
		TraceP99:           1.0,
		Metrics:            1.0,
	}
	score, err := detector.ComputeScore(result)
	AssertNoError(t, err, "ComputeScore with perfect results")
	if score != 1.0 {
		t.Errorf("Expected score 1.0 for perfect results, got %.3f", score)
	}
}

func TestWeightedDetector_ZeroScore(t *testing.T) {
	detector := NewWeightedDetector()
	result := TelemetryResult{
		CPU:                0.0,
		Mem:                0.0,
		Err:                0.0,
		ClientSentByClient: 0.0,
		ClientSentByServer: 0.0,
		ServerSentByClient: 0.0,
		ServerSentByServer: 0.0,
		TraceP50:           0.0,
		TraceP95:           0.0,
		TraceP99:           0.0,
		Metrics:            0.0,
	}
	score, err := detector.ComputeScore(result)
	AssertNoError(t, err, "ComputeScore with zero results")
	if score != 0.0 {
		t.Errorf("Expected score 0.0 for zero results, got %.3f", score)
	}
}

func TestWeightedDetector_MixedScores(t *testing.T) {
	detector := NewWeightedDetector()
	result := TelemetryResult{
		CPU:                0.5,
		Mem:                0.5,
		Err:                0.5,
		ClientSentByClient: 0.5,
		ClientSentByServer: 0.5,
		ServerSentByClient: 0.5,
		ServerSentByServer: 0.5,
		TraceP50:           0.5,
		TraceP95:           0.5,
		TraceP99:           0.5,
		Metrics:            0.5,
	}
	score, err := detector.ComputeScore(result)
	AssertNoError(t, err, "ComputeScore with mixed results")
	if score != 0.5 {
		t.Errorf("Expected score 0.5 for mixed results, got %.3f", score)
	}
}

func TestWeightedDetector_UsesLastResult(t *testing.T) {
	detector := NewWeightedDetector()
	// First result is normal
	normalResult := CreateNormalResults(1)[0]
	_, err := detector.ComputeScore(normalResult)
	AssertNoError(t, err, "ComputeScore with normal result")

	// Last result is anomalous
	anomalousResult := CreateAnomalousResults(1)[0]
	score, err := detector.ComputeScore(anomalousResult)
	AssertNoError(t, err, "ComputeScore with anomalous result")

	// Score should reflect the last (anomalous) result, which should be low
	if score > 0.5 {
		t.Errorf("Expected low score (anomalous), got %.3f", score)
	}
}

func TestWeightedDetector_ScoreRange(t *testing.T) {
	detector := NewWeightedDetector()

	testCases := []struct {
		name    string
		results []TelemetryResult
	}{
		{"Normal", CreateNormalResults(5)},
		{"Anomalous", CreateAnomalousResults(5)},
		{"Mixed", CreateMixedResults()},
		{"Gradual Drift", CreateGradualDriftResults(10)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			score, err := detector.ComputeScore(tc.results[len(tc.results)-1])
			AssertNoError(t, err, "ComputeScore")
			AssertFloatInRange(t, score, 0.0, 1.0, "Score")
		})
	}
}
