// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package detector

import (
	"testing"
)

func TestChangePointDetector_StableResults(t *testing.T) {
	detector := NewChangePointDetector()
	normalResults := CreateNormalResults(50)

	score, err := detector.ComputeScore(normalResults[len(normalResults)-1])
	AssertNoError(t, err, "ComputeScore with stable results")

	if score > 2.0 {
		t.Errorf("Expected stable data to have low changepoint score, got %.3f", score)
	}
}

func TestChangePointDetector_ProgressiveScoring(t *testing.T) {
	detector := NewChangePointDetector()

	results := CreateGradualDriftResults(60)

	for _, result := range results {
		score, err := detector.ComputeScore(result)
		AssertNoError(t, err, "ComputeScore in progressive scoring")

		if score < 0 {
			t.Errorf("Score should be non-negative, got %.3f", score)
		}
	}
}

func TestChangePointDetector_CUSUMMethod(t *testing.T) {
	detector := NewChangePointDetectorWithParams(
		"cusum",
		288,
		6,
		5.0,
		0.01,
		3.0,
		2,
		30,
		true,
	)

	normalResults := CreateNormalResults(50)
	normalScore, err := detector.ComputeScore(normalResults[len(normalResults)-1])
	AssertNoError(t, err, "CUSUM method with stable data")
	if normalScore < 0 {

		t.Errorf("CUSUM should produce non-negative scores, got %.3f", normalScore)
	}
}

func TestChangePointDetector_Reset(t *testing.T) {
	detector := NewChangePointDetector()

	results := CreateNormalResults(50)
	score, err := detector.ComputeScore(results[len(results)-1])
	AssertNoError(t, err, "ComputeScore before reset")

	if score < 0 {
		t.Errorf("Score should be non-negative, got %.3f", score)
	}

	detector.Reset()

	newResults := CreateNormalResults(10)
	newScore, err := detector.ComputeScore(newResults[len(newResults)-1])
	AssertNoError(t, err, "ComputeScore after reset")

	if newScore < 0 {
		t.Errorf("Score after reset should be non-negative, got %.3f", newScore)
	}
}
