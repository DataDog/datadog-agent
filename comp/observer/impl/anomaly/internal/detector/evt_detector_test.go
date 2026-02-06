// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package detector

import (
	"testing"
)

func TestEVTDetector_NormalResults(t *testing.T) {
	detector := NewEVTDetector()
	normalResults := CreateNormalResults(100)

	score, err := detector.ComputeScore(normalResults[len(normalResults)-1])
	AssertNoError(t, err, "ComputeScore with normal results")
	AssertFloatInRange(t, score, -5.0, 5.0, "Normal data combined Z-score")
}

func TestEVTDetector_ProgressiveScoring(t *testing.T) {
	detector := NewEVTDetector()

	results := CreateGradualDriftResults(100)

	for _, result := range results {
		score, err := detector.ComputeScore(result)
		AssertNoError(t, err, "ComputeScore in progressive scoring")
		AssertFloatInRange(t, score, -10.0, 10.0, "EVT Z-score")
	}
}

func TestEVTDetector_FisherMethod(t *testing.T) {
	detector := NewEVTDetectorWithParams(
		30*288,
		0.975,
		50,
		1,
		10.0,
		2,
		30,
		false,
	)

	normalResults := CreateNormalResults(100)
	score, err := detector.ComputeScore(normalResults[len(normalResults)-1])
	AssertNoError(t, err, "ComputeScore with Fisher method")

	if score < 0 {
		t.Errorf("Fisher method should produce non-negative scores, got %.3f", score)
	}
}

func TestEVTDetector_WindowManagement(t *testing.T) {
	detector := NewEVTDetectorWithParams(
		10,
		0.975,
		2,
		1,
		3.3,
		2,
		30,
		true,
	)

	results := CreateNormalResults(20)
	score, err := detector.ComputeScore(results[len(results)-1])
	AssertNoError(t, err, "ComputeScore with window overflow")

	if score != score {
		t.Error("Detector produced NaN score after window overflow")
	}
}
