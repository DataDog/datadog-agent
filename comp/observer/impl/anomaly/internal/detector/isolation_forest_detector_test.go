// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package detector

import (
	"testing"
)

func TestIsolationForestDetector_NormalResults(t *testing.T) {
	detector := NewIsolationForestDetector()
	normalResults := CreateNormalResults(100)

	score, err := detector.ComputeScore(normalResults[len(normalResults)-1])
	AssertNoError(t, err, "ComputeScore with normal results")
	AssertFloatInRange(t, score, 0.0, 1.0, "Isolation Forest anomaly score")
}

func TestIsolationForestDetector_IncrementalScoring(t *testing.T) {
	detector := NewIsolationForestDetector()

	results := CreateGradualDriftResults(50)

	for _, result := range results {
		score, err := detector.ComputeScore(result)
		AssertNoError(t, err, "ComputeScore in incremental scoring")
		AssertFloatInRange(t, score, 0.0, 1.0, "Isolation Forest score")
	}
}

func TestIsolationForestDetector_CustomParams(t *testing.T) {
	detector := NewIsolationForestDetectorWithParams(
		50,
		50,
		10,
		6,
		2.0,
		2,
		30,
		8,
		0.01,
	)

	results := CreateNormalResults(100)
	score, err := detector.ComputeScore(results[len(results)-1])
	AssertNoError(t, err, "ComputeScore with custom params")

	if score != score {
		t.Error("Detector produced NaN score")
	}
}

func TestIsolationForestDetector_WindowManagement(t *testing.T) {
	detector := NewIsolationForestDetectorWithParams(
		10,
		10,
		1,
		1,
		2.0,
		2,
		30,
		5,
		0.01,
	)

	results := CreateNormalResults(300)
	score, err := detector.ComputeScore(results[len(results)-1])
	AssertNoError(t, err, "ComputeScore with window overflow")

	if score != score {
		t.Error("Detector produced NaN score after window overflow")
	}
}

func TestIsolationForestDetector_AllAnomalous(t *testing.T) {
	detector := NewIsolationForestDetector()

	anomalousResults := CreateAnomalousResults(100)
	score, err := detector.ComputeScore(anomalousResults[len(anomalousResults)-1])
	AssertNoError(t, err, "ComputeScore with all anomalous data")
	AssertFloatInRange(t, score, 0.0, 1.0, "All anomalous score")
}
