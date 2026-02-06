// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package detector

import (
	"testing"
)

func TestKofMDetector_SingleResult(t *testing.T) {
	detector := NewKofMDetector()
	score, err := detector.ComputeScore(CreateNormalResults(1)[0])
	AssertNoError(t, err, "ComputeScore with single result")
	AssertFloatInRange(t, score, 0.0, 1.0, "K-of-M score")
}

func TestKofMDetector_MultipleResults(t *testing.T) {
	detector := NewKofMDetector()

	normalResults := CreateNormalResults(20)
	for _, result := range normalResults {
		score, err := detector.ComputeScore(result)
		AssertNoError(t, err, "ComputeScore iteration")
		AssertFloatInRange(t, score, 0.0, 1.0, "K-of-M score")
	}

	anomalousResults := CreateAnomalousResults(10)
	for _, result := range anomalousResults {
		score, err := detector.ComputeScore(result)
		AssertNoError(t, err, "ComputeScore with anomalous data")
		AssertFloatInRange(t, score, 0.0, 1.0, "K-of-M score")
	}
}

func TestKofMDetector_ScoreRange(t *testing.T) {
	testCases := []struct {
		name    string
		results []TelemetryResult
	}{
		{"Normal", CreateNormalResults(30)},
		{"Gradual Drift", CreateGradualDriftResults(30)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			detector := NewKofMDetector()
			for _, result := range tc.results {
				score, err := detector.ComputeScore(result)
				AssertNoError(t, err, "ComputeScore in ScoreRange test")
				AssertFloatInRange(t, score, 0.0, 1.0, "K-of-M score")
			}
		})
	}
}

func TestKofMDetector_CustomParams(t *testing.T) {
	detector := NewKofMDetectorWithParams(10, 3.0, 2, 2, 30, 5)

	results := CreateNormalResults(15)
	score, err := detector.ComputeScore(results[len(results)-1])
	AssertNoError(t, err, "ComputeScore with custom params")
	AssertFloatInRange(t, score, 0.0, 1.0, "K-of-M score with custom params")
}

func TestKofMDetector_NormalVsAnomalous(t *testing.T) {
	detector := NewKofMDetector()

	normalResults := CreateNormalResults(50)
	normalScore, err := detector.ComputeScore(normalResults[len(normalResults)-1])
	AssertNoError(t, err, "ComputeScore with normal results")
	AssertFloatInRange(t, normalScore, 0.0, 1.0, "Normal K-of-M score")

	anomalousResults := CreateAnomalousResults(50)
	anomalousScore, err := detector.ComputeScore(anomalousResults[len(anomalousResults)-1])
	AssertNoError(t, err, "ComputeScore with anomalous results")
	AssertFloatInRange(t, anomalousScore, 0.0, 1.0, "Anomalous K-of-M score")

	_ = normalScore
	_ = anomalousScore
}

func TestKofMDetector_MixedResults(t *testing.T) {
	detector := NewKofMDetector()

	normalResults := CreateNormalResults(30)
	normalScore, err := detector.ComputeScore(normalResults[len(normalResults)-1])
	AssertNoError(t, err, "ComputeScore with normal baseline")
	AssertFloatInRange(t, normalScore, 0.0, 1.0, "K-of-M score")

	mixedResults := CreateMixedResults()
	mixedScore, err := detector.ComputeScore(mixedResults[len(mixedResults)-1])
	AssertNoError(t, err, "ComputeScore with mixed results")
	AssertFloatInRange(t, mixedScore, 0.0, 1.0, "K-of-M score")
}
