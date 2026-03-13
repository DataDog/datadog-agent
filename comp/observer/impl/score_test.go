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
	// 5 false alarms during baseline + 1 good detection after onset.
	// Post-onset unmatched predictions are ignored, so only the 5 pre-onset
	// predictions count as FP.
	result := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{20, 30, 40, 50, 60, 102},
		GroundTruthTimestamps: []int64{100},
		Sigma:                 testSigma,
	})

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

	// Prediction 80s before onset → FP, GT unmatched → FN
	assert.Equal(t, 0.0, result.F1, "distant false alarm should score zero")
	assert.Equal(t, 1.0, result.FP)
	assert.Equal(t, 1.0, result.FN)
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
	// Prediction at 99 (before GT=100) is FP.
	// Prediction at 102 (first post-onset) matches.
	result := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{99, 102},
		GroundTruthTimestamps: []int64{100},
		Sigma:                 testSigma,
	})
	assert.Greater(t, result.F1, 0.0, "post-onset prediction should match, not the closer pre-onset one")
	assert.Greater(t, result.TP, 0.0, "should have nonzero TP from the post-onset match")
	assert.Equal(t, 1.0, result.FP, "pre-onset prediction at 99 should be FP")
}

func TestGaussianF1_MultipleFiresDuringDisruption(t *testing.T) {
	// Detector fires 3 times during disruption, 0 baseline FPs.
	// First post-onset prediction (102) matches GT. The other two (110, 120)
	// are post-onset unmatched → ignored. Should score near-perfect.
	result := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{102, 110, 120},
		GroundTruthTimestamps: []int64{100},
		Sigma:                 testSigma,
	})
	assert.InDelta(t, 1.0, result.F1, 0.1, "3 fires during disruption with 0 baseline FPs should score near-perfect")
	assert.Equal(t, 0.0, result.FP, "no FPs — extra post-onset predictions are ignored")
}

func TestGaussianF1_OneFireDuringDisruptionPlusBaselineFPs(t *testing.T) {
	// Detector fires once during disruption + 5 baseline FPs.
	// Good recall (detection matches) but precision penalized by 5 FPs.
	result := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{10, 20, 30, 40, 50, 102},
		GroundTruthTimestamps: []int64{100},
		Sigma:                 testSigma,
	})
	assert.Greater(t, result.Recall, 0.8, "should have good recall from the match")
	assert.Less(t, result.Precision, 0.5, "5 FPs should penalize precision")
	assert.Greater(t, result.F1, 0.0, "F1 should be nonzero — there is a match")
	assert.Less(t, result.F1, 0.6, "F1 should be penalized by the 5 baseline FPs")
	assert.Equal(t, 5.0, result.FP, "5 pre-onset predictions are FP")
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
	assert.Greater(t, result.F1, 0.0)
}

func TestScoreOutputFile_PostOnsetIgnored(t *testing.T) {
	// Ground truth at 100, sigma=10.
	// Predictions at 50 (baseline FP), 102 (match), 130 (post-onset ignored), 200 (post-onset ignored).
	// No cascading filter — post-onset unmatched are just ignored by the scorer.
	output := ObserverOutput{
		Metadata: ObserverMetadata{
			Scenario:      "test",
			TimelineStart: 0,
			TimelineEnd:   300,
		},
		AnomalyPeriods: []ObserverCorrelation{
			{Pattern: "baseline_fp", PeriodStart: 50},
			{Pattern: "good_detect", PeriodStart: 102},
			{Pattern: "post_onset_1", PeriodStart: 130},
			{Pattern: "post_onset_2", PeriodStart: 200},
		},
	}

	data, err := json.MarshalIndent(output, "", "  ")
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "test_output.json")
	require.NoError(t, os.WriteFile(path, data, 0644))

	result, err := ScoreOutputFile(path, []int64{100}, "", testSigma)
	require.NoError(t, err)

	// All 4 predictions pass to scorer (no cascading filter).
	assert.Equal(t, 4, result.NumPredictions, "all predictions should be passed to scorer")
	assert.Equal(t, 0, result.NumFilteredWarmup)
	assert.Equal(t, 1, result.NumGroundTruths)
	assert.Equal(t, 1, result.NumBaselineFPs, "prediction at 50 is a baseline FP")
	// FP = 1 (the prediction at 50, before onset). Post-onset 130 and 200 are ignored.
	assert.Equal(t, 1.0, result.FP, "only pre-onset predictions count as FP")
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
	scenariosDir := t.TempDir()
	scenarioDir := filepath.Join(scenariosDir, "test_scenario")
	require.NoError(t, os.MkdirAll(scenarioDir, 0755))

	metadata := `{"baseline": {"start": "1970-01-01T00:01:40Z"}, "disruption": {"start": "1970-01-01T00:03:20Z"}}`
	require.NoError(t, os.WriteFile(filepath.Join(scenarioDir, "metadata.json"), []byte(metadata), 0644))

	output := ObserverOutput{
		Metadata: ObserverMetadata{Scenario: "test_scenario"},
		AnomalyPeriods: []ObserverCorrelation{
			{Pattern: "warmup_noise_1", PeriodStart: 50},  // warmup → filtered
			{Pattern: "warmup_noise_2", PeriodStart: 90},  // warmup → filtered
			{Pattern: "baseline_fp", PeriodStart: 120},    // baseline FP → scored
			{Pattern: "good_detect", PeriodStart: 202},    // near onset → scored
			{Pattern: "post_onset", PeriodStart: 250},     // post-onset unmatched → scored but ignored by F1
		},
	}
	data, err := json.MarshalIndent(output, "", "  ")
	require.NoError(t, err)

	outputPath := filepath.Join(t.TempDir(), "output.json")
	require.NoError(t, os.WriteFile(outputPath, data, 0644))

	result, err := ScoreOutputFile(outputPath, nil, scenariosDir, testSigma)
	require.NoError(t, err)

	assert.Equal(t, 2, result.NumFilteredWarmup, "predictions before baseline.start should be filtered")
	assert.Equal(t, 3, result.NumPredictions, "baseline FP, good detect, and post-onset should all be scored")
}
