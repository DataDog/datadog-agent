// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSigma = 10.0

func TestGaussianF1_PerfectDetection(t *testing.T) {
	result := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{100},
		GroundTruthTimestamps: []int64{100},
		Sigma:                 testSigma,
	})

	assert.InDelta(t, 1.0, result.F1, 0.05, "perfect detection should score ~1.0")
	assert.InDelta(t, 1.0, result.Precision, 0.05)
	assert.InDelta(t, 1.0, result.Recall, 0.05)
}

func TestGaussianF1_SlightlyLate(t *testing.T) {
	result := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{108},
		GroundTruthTimestamps: []int64{100},
		Sigma:                 testSigma,
	})

	// Detection 8s late with σ=10 should have meaningful but reduced score
	assert.Greater(t, result.F1, 0.3, "slightly late detection should score reasonably")
	assert.Less(t, result.F1, 0.9, "slightly late detection shouldn't score perfectly")
}

func TestGaussianF1_LateDetection(t *testing.T) {
	result := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{130},
		GroundTruthTimestamps: []int64{100},
		Sigma:                 testSigma,
	})

	// Detection 30s late (3σ) should score poorly
	assert.Less(t, result.F1, 0.3, "late detection (3σ) should score poorly")
	assert.Greater(t, result.F1, 0.0, "late detection should still be nonzero")
}

func TestGaussianF1_NoisyButCorrect(t *testing.T) {
	result := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{20, 30, 40, 50, 60, 102},
		GroundTruthTimestamps: []int64{100},
		Sigma:                 testSigma,
	})

	// 5 false alarms during baseline + 1 good detection
	assert.Less(t, result.F1, 0.5, "noisy detector should be penalized")
	assert.Greater(t, result.F1, 0.0, "correct detection should keep score above zero")
	assert.Less(t, result.Precision, 0.3, "many FPs should hurt precision")
}

func TestGaussianF1_MissedEntirely(t *testing.T) {
	result := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{},
		GroundTruthTimestamps: []int64{100},
		Sigma:                 testSigma,
	})

	assert.Equal(t, 0.0, result.F1, "missed detection should score 0")
	assert.Equal(t, 0.0, result.Precision)
	assert.Equal(t, 0.0, result.Recall)
	assert.Equal(t, 1.0, result.FN)
}

func TestGaussianF1_FalseAlarmOnly(t *testing.T) {
	result := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{20},
		GroundTruthTimestamps: []int64{100},
		Sigma:                 testSigma,
	})

	// Prediction 80s away (8σ) — virtually no overlap
	assert.Less(t, result.F1, 0.05, "distant false alarm should score near zero")
}

func TestGaussianF1_NoPredictionsNoGroundTruth(t *testing.T) {
	result := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{},
		GroundTruthTimestamps: []int64{},
		Sigma:                 testSigma,
	})

	assert.Equal(t, 1.0, result.F1, "empty scenario should be trivially perfect")
}

func TestGaussianF1_NoGroundTruth(t *testing.T) {
	result := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{100, 200},
		GroundTruthTimestamps: []int64{},
		Sigma:                 testSigma,
	})

	assert.Equal(t, 0.0, result.F1, "predictions with no ground truth should score 0")
	assert.Equal(t, 2.0, result.FP)
}

func TestGaussianF1_MultipleGroundTruths(t *testing.T) {
	result := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{102, 502},
		GroundTruthTimestamps: []int64{100, 500},
		Sigma:                 testSigma,
	})

	// Both detections are very close to their ground truths
	assert.Greater(t, result.F1, 0.7, "two good detections should score well")
	assert.Equal(t, 2, result.NumPredictions)
	assert.Equal(t, 2, result.NumGroundTruths)
}

func TestGaussianF1_BeforeOnsetIsZero(t *testing.T) {
	// Predictions before ground truth onset get zero overlap — no credit for early alarms.
	before := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{92}, // 8s before
		GroundTruthTimestamps: []int64{100},
		Sigma:                 testSigma,
	})
	assert.Equal(t, 0.0, before.F1, "prediction before onset should score 0")
	assert.Equal(t, 1.0, before.FP, "prediction before onset is a full FP")
	assert.Equal(t, 1.0, before.FN, "ground truth is unmatched → full FN")

	after := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{108}, // 8s after
		GroundTruthTimestamps: []int64{100},
		Sigma:                 testSigma,
	})
	assert.Greater(t, after.F1, 0.3, "prediction after onset should score well")
}

func TestGaussianF1_BeforeOnsetDoesNotStealMatch(t *testing.T) {
	// A baseline FP 1s before GT must not steal the match from a real detection after GT.
	result := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{99, 102}, // 99 is closer by distance but before GT
		GroundTruthTimestamps: []int64{100},
		Sigma:                 testSigma,
	})
	assert.Greater(t, result.F1, 0.0, "post-onset prediction should match, not the closer pre-onset one")
	assert.Greater(t, result.TP, 0.0, "should have nonzero TP from the post-onset match")
}

func TestScoreOutputFile(t *testing.T) {
	output := ObserverOutput{
		Metadata: ObserverMetadata{
			Scenario:      "test",
			TimelineStart: 0,
			TimelineEnd:   300,
		},
		AnomalyPeriods: []ObserverCorrelation{
			{Pattern: "cluster_1", PeriodStart: 102},
			{Pattern: "cluster_2", PeriodStart: 50},
		},
	}

	data, err := json.MarshalIndent(output, "", "  ")
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "test_output.json")
	require.NoError(t, os.WriteFile(path, data, 0644))

	result, err := ScoreOutputFile(path, []int64{100}, "", testSigma)
	require.NoError(t, err)

	assert.Equal(t, 2, result.NumPredictions)
	assert.Equal(t, 1, result.NumGroundTruths)
	assert.Equal(t, 0, result.NumFilteredWarmup)
	assert.Equal(t, 0, result.NumFilteredCascading)
	assert.Greater(t, result.F1, 0.0)
}

func TestScoreOutputFile_PredictionWindowFiltering(t *testing.T) {
	// Ground truth at 100, sigma=10 → cutoff = 100 + 2*10 = 120
	// Predictions at 50 (baseline FP), 102 (good), 130 (cascading, filtered), 200 (cascading, filtered)
	output := ObserverOutput{
		Metadata: ObserverMetadata{
			Scenario:      "test",
			TimelineStart: 0,
			TimelineEnd:   300,
		},
		AnomalyPeriods: []ObserverCorrelation{
			{Pattern: "baseline_fp", PeriodStart: 50},
			{Pattern: "good_detect", PeriodStart: 102},
			{Pattern: "cascade_1", PeriodStart: 130},
			{Pattern: "cascade_2", PeriodStart: 200},
		},
	}

	data, err := json.MarshalIndent(output, "", "  ")
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "test_output.json")
	require.NoError(t, os.WriteFile(path, data, 0644))

	result, err := ScoreOutputFile(path, []int64{100}, "", testSigma)
	require.NoError(t, err)

	// 2 predictions scored (50, 102), 2 cascading filtered (130, 200)
	assert.Equal(t, 2, result.NumPredictions, "only predictions within window should be scored")
	assert.Equal(t, 0, result.NumFilteredWarmup)
	assert.Equal(t, 2, result.NumFilteredCascading, "predictions beyond cutoff should be filtered")
	assert.Equal(t, 1, result.NumGroundTruths)

	// Prediction at exactly the cutoff (120) should be included
	output.AnomalyPeriods = append(output.AnomalyPeriods, ObserverCorrelation{
		Pattern: "at_cutoff", PeriodStart: 120,
	})
	data, err = json.MarshalIndent(output, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0644))

	result, err = ScoreOutputFile(path, []int64{100}, "", testSigma)
	require.NoError(t, err)

	assert.Equal(t, 3, result.NumPredictions, "prediction at cutoff should be included")
	assert.Equal(t, 2, result.NumFilteredCascading)
}

func TestScoreOutputFile_MetadataInference(t *testing.T) {
	// Set up a fake scenario dir with metadata.json
	scenariosDir := t.TempDir()
	scenarioDir := filepath.Join(scenariosDir, "test_scenario")
	require.NoError(t, os.MkdirAll(scenarioDir, 0755))

	metadata := `{"baseline": {"start": "2026-03-03T12:39:35Z"}, "disruption": {"start": "2026-03-03T12:49:35Z"}}`
	require.NoError(t, os.WriteFile(filepath.Join(scenarioDir, "metadata.json"), []byte(metadata), 0644))

	// Output JSON references "test_scenario"
	output := ObserverOutput{
		Metadata: ObserverMetadata{
			Scenario: "test_scenario",
		},
		AnomalyPeriods: []ObserverCorrelation{
			{Pattern: "cluster_1", PeriodStart: 1772542175}, // exact match with disruption.start
		},
	}
	data, err := json.MarshalIndent(output, "", "  ")
	require.NoError(t, err)

	outputPath := filepath.Join(t.TempDir(), "output.json")
	require.NoError(t, os.WriteFile(outputPath, data, 0644))

	// Score with nil ground truth — should infer from metadata
	result, err := ScoreOutputFile(outputPath, nil, scenariosDir, 30.0)
	require.NoError(t, err)

	assert.Equal(t, 1, result.NumGroundTruths)
	assert.InDelta(t, 1.0, result.F1, 0.05, "exact match should score ~1.0")
}

func TestScoreOutputFile_ExplicitOverridesMetadata(t *testing.T) {
	// Set up metadata — disruption.start is at 12:49:35, but we override GT to 12:45:00
	scenariosDir := t.TempDir()
	scenarioDir := filepath.Join(scenariosDir, "test_scenario")
	require.NoError(t, os.MkdirAll(scenarioDir, 0755))

	metadata := `{"baseline": {"start": "2026-03-03T12:39:35Z"}, "disruption": {"start": "2026-03-03T12:49:35Z"}}`
	require.NoError(t, os.WriteFile(filepath.Join(scenarioDir, "metadata.json"), []byte(metadata), 0644))

	// Prediction matches our explicit GT (12:45:00 = 1772541900), not metadata's disruption.start
	explicitGT := int64(1772541900)
	output := ObserverOutput{
		Metadata: ObserverMetadata{Scenario: "test_scenario"},
		AnomalyPeriods: []ObserverCorrelation{
			{Pattern: "cluster_1", PeriodStart: explicitGT},
		},
	}
	data, err := json.MarshalIndent(output, "", "  ")
	require.NoError(t, err)

	outputPath := filepath.Join(t.TempDir(), "output.json")
	require.NoError(t, os.WriteFile(outputPath, data, 0644))

	// Explicit ground truth should override metadata's disruption.start
	result, err := ScoreOutputFile(outputPath, []int64{explicitGT}, scenariosDir, 30.0)
	require.NoError(t, err)

	assert.InDelta(t, 1.0, result.F1, 0.05, "explicit GT should be used, not metadata")
}

func TestScoreOutputFile_WarmupFiltering(t *testing.T) {
	// baseline.start at T=100 (warmup ends), disruption.start at T=200
	// sigma=10 → cascading cutoff = 200 + 20 = 220
	scenariosDir := t.TempDir()
	scenarioDir := filepath.Join(scenariosDir, "test_scenario")
	require.NoError(t, os.MkdirAll(scenarioDir, 0755))

	metadata := `{"baseline": {"start": "1970-01-01T00:01:40Z"}, "disruption": {"start": "1970-01-01T00:03:20Z"}}`
	require.NoError(t, os.WriteFile(filepath.Join(scenarioDir, "metadata.json"), []byte(metadata), 0644))

	output := ObserverOutput{
		Metadata: ObserverMetadata{Scenario: "test_scenario"},
		AnomalyPeriods: []ObserverCorrelation{
			{Pattern: "warmup_noise_1", PeriodStart: 50}, // warmup → filtered
			{Pattern: "warmup_noise_2", PeriodStart: 90}, // warmup → filtered
			{Pattern: "baseline_fp", PeriodStart: 120},   // baseline FP → scored
			{Pattern: "good_detect", PeriodStart: 202},   // near onset → scored
			{Pattern: "cascade", PeriodStart: 250},       // cascading → filtered
		},
	}
	data, err := json.MarshalIndent(output, "", "  ")
	require.NoError(t, err)

	outputPath := filepath.Join(t.TempDir(), "output.json")
	require.NoError(t, os.WriteFile(outputPath, data, 0644))

	result, err := ScoreOutputFile(outputPath, nil, scenariosDir, testSigma)
	require.NoError(t, err)

	assert.Equal(t, 2, result.NumFilteredWarmup, "predictions before baseline.start should be filtered")
	assert.Equal(t, 1, result.NumFilteredCascading, "predictions beyond 2σ should be filtered")
	assert.Equal(t, 2, result.NumPredictions, "only baseline FP and good detect should be scored")
}
