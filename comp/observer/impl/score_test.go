// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
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

func TestGaussianF1_HalfGaussianSymmetry(t *testing.T) {
	// With both prediction and ground truth as right-sided half-Gaussians,
	// equal distances before and after produce the same overlap — both
	// distributions extend rightward, so the geometry is symmetric around d=0.
	before := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{92}, // 8s before
		GroundTruthTimestamps: []int64{100},
		Sigma:                 testSigma,
	})

	after := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{108}, // 8s after
		GroundTruthTimestamps: []int64{100},
		Sigma:                 testSigma,
	})

	assert.InDelta(t, before.F1, after.F1, 0.01,
		"equal distances should produce similar scores (before=%.3f, after=%.3f)",
		before.F1, after.F1)

	// But a prediction far before onset should score much less than one close after
	farBefore := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{60}, // 40s before (4σ)
		GroundTruthTimestamps: []int64{100},
		Sigma:                 testSigma,
	})
	assert.Less(t, farBefore.F1, after.F1,
		"far before onset (%.3f) should score less than close after (%.3f)",
		farBefore.F1, after.F1)
}

func TestScorePredictions(t *testing.T) {
	meta := &scoringMetadata{
		groundTruthTimestamps: []int64{100},
	}

	result := scorePredictions([]int64{102, 50}, meta, testSigma)

	assert.Equal(t, 2, result.NumPredictions)
	assert.Equal(t, 1, result.NumGroundTruths)
	assert.Equal(t, 0, result.NumFilteredWarmup)
	assert.Equal(t, 0, result.NumFilteredCascading)
	assert.Greater(t, result.F1, 0.0)
}

func TestScorePredictions_WindowFiltering(t *testing.T) {
	// Ground truth at 100, sigma=10 → cutoff = 100 + 2*10 = 120
	meta := &scoringMetadata{
		groundTruthTimestamps: []int64{100},
	}

	result := scorePredictions([]int64{50, 102, 130, 200}, meta, testSigma)

	// 2 predictions scored (50, 102), 2 cascading filtered (130, 200)
	assert.Equal(t, 2, result.NumPredictions, "only predictions within window should be scored")
	assert.Equal(t, 0, result.NumFilteredWarmup)
	assert.Equal(t, 2, result.NumFilteredCascading, "predictions beyond cutoff should be filtered")
	assert.Equal(t, 1, result.NumGroundTruths)

	// Prediction at exactly the cutoff (120) should be included
	result = scorePredictions([]int64{50, 102, 120, 130, 200}, meta, testSigma)
	assert.Equal(t, 3, result.NumPredictions, "prediction at cutoff should be included")
	assert.Equal(t, 2, result.NumFilteredCascading)
}

func TestScoringMetadataFromEpisode(t *testing.T) {
	info := &EpisodeInfo{
		Baseline:   &EpisodePhase{Start: "2026-03-03T12:39:35Z"},
		Disruption: &EpisodePhase{Start: "2026-03-03T12:49:35Z"},
	}

	meta, err := scoringMetadataFromEpisode(info)
	require.NoError(t, err)

	assert.Equal(t, 1, len(meta.groundTruthTimestamps))
	assert.Equal(t, int64(1772541575), meta.baselineStart)

	// Prediction exactly at disruption.start should score ~1.0
	result := scorePredictions(meta.groundTruthTimestamps, meta, 30.0)
	assert.InDelta(t, 1.0, result.F1, 0.05, "exact match should score ~1.0")
}

func TestScoringMetadataFromEpisode_NoDisruption(t *testing.T) {
	info := &EpisodeInfo{
		Baseline: &EpisodePhase{Start: "2026-03-03T12:39:35Z"},
	}

	_, err := scoringMetadataFromEpisode(info)
	assert.Error(t, err, "should fail without disruption")
}

func TestScoringMetadataFromEpisode_NilInfo(t *testing.T) {
	_, err := scoringMetadataFromEpisode(nil)
	assert.Error(t, err, "should fail with nil info")
}

func TestLoadScoringMetadataFromFile(t *testing.T) {
	scenariosDir := t.TempDir()
	scenarioDir := filepath.Join(scenariosDir, "test_scenario")
	require.NoError(t, os.MkdirAll(scenarioDir, 0755))

	episode := `{"baseline": {"start": "1970-01-01T00:01:40Z"}, "disruption": {"start": "1970-01-01T00:03:20Z"}}`
	require.NoError(t, os.WriteFile(filepath.Join(scenarioDir, "episode.json"), []byte(episode), 0644))

	meta, err := loadScoringMetadataFromFile(scenariosDir, "test_scenario")
	require.NoError(t, err)

	assert.Equal(t, []int64{200}, meta.groundTruthTimestamps)
	assert.Equal(t, int64(100), meta.baselineStart)
}

func TestScorePredictions_WarmupFiltering(t *testing.T) {
	// baseline.start at T=100, disruption at T=200
	// sigma=10 → cascading cutoff = 200 + 20 = 220
	meta := &scoringMetadata{
		groundTruthTimestamps: []int64{200},
		baselineStart:         100,
	}

	result := scorePredictions([]int64{50, 90, 120, 202, 250}, meta, testSigma)

	assert.Equal(t, 2, result.NumFilteredWarmup, "predictions before baseline.start should be filtered")
	assert.Equal(t, 1, result.NumFilteredCascading, "predictions beyond 2σ should be filtered")
	assert.Equal(t, 2, result.NumPredictions, "only baseline FP and good detect should be scored")
}
