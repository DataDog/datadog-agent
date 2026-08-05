// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"math"
	"testing"
)

// ── Gaussian overlap ──────────────────────────────────────────────────────────

func TestNumericalOverlapHalfHalf_Exact(t *testing.T) {
	// When prediction == ground truth (d=0), overlap should be ≈ 1.
	got := numericalOverlapHalfHalf(0, 30)
	if math.Abs(got-1.0) > 0.01 {
		t.Errorf("d=0: expected overlap ≈ 1, got %.4f", got)
	}
}

func TestNumericalOverlapHalfHalf_FarAway(t *testing.T) {
	// When prediction is 10σ late, overlap should be negligible.
	got := numericalOverlapHalfHalf(300, 30) // d = 10σ
	if got > 0.01 {
		t.Errorf("d=10σ: expected overlap ≈ 0, got %.4f", got)
	}
}

func TestNumericalOverlapHalfHalf_Monotone(t *testing.T) {
	// Overlap must decrease as d increases (later = worse).
	sigma := 30.0
	prev := numericalOverlapHalfHalf(0, sigma)
	for _, d := range []float64{15, 30, 60, 120} {
		curr := numericalOverlapHalfHalf(d, sigma)
		if curr >= prev {
			t.Errorf("overlap not monotone decreasing: d=%.0f → %.4f, prev=%.4f", d, curr, prev)
		}
		prev = curr
	}
}

func TestNumericalOverlapHalfHalf_BeforeGT(t *testing.T) {
	// Negative d means prediction is BEFORE ground truth — halfGaussianOverlap
	// is called with predTS < gtTS so d < 0. The right-sided half-Gaussian of
	// the prediction starts at d, while the GT half-Gaussian starts at 0.
	// For d << 0 the overlap should drop toward 0.
	got := numericalOverlapHalfHalf(-300, 30) // prediction 10σ before GT
	if got > 0.01 {
		t.Errorf("d=-10σ: expected overlap ≈ 0, got %.4f", got)
	}
}

// ── ComputeGaussianF1 ─────────────────────────────────────────────────────────

func TestComputeGaussianF1_BothEmpty(t *testing.T) {
	r := ComputeGaussianF1(ScoreInput{Sigma: 30})
	if r.F1 != 1 || r.Precision != 1 || r.Recall != 1 {
		t.Errorf("both empty: expected F1=P=R=1, got F1=%.4f P=%.4f R=%.4f", r.F1, r.Precision, r.Recall)
	}
}

func TestComputeGaussianF1_NoPredictions(t *testing.T) {
	r := ComputeGaussianF1(ScoreInput{
		GroundTruthTimestamps: []int64{1000},
		Sigma:                 30,
	})
	if r.F1 != 0 || r.Recall != 0 {
		t.Errorf("no predictions: expected F1=R=0, got F1=%.4f R=%.4f", r.F1, r.Recall)
	}
	if r.FN != 1 {
		t.Errorf("no predictions: expected FN=1, got %.4f", r.FN)
	}
}

func TestComputeGaussianF1_NoGroundTruth(t *testing.T) {
	r := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps: []int64{1000, 2000},
		Sigma:                30,
	})
	if r.F1 != 0 || r.Precision != 0 {
		t.Errorf("no GT: expected F1=P=0, got F1=%.4f P=%.4f", r.F1, r.Precision)
	}
	if r.FP != 2 {
		t.Errorf("no GT: expected FP=2, got %.4f", r.FP)
	}
}

func TestComputeGaussianF1_PerfectDetection(t *testing.T) {
	// Prediction exactly at disruption onset → F1 ≈ 1.
	gt := int64(1000)
	r := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{gt},
		GroundTruthTimestamps: []int64{gt},
		Sigma:                 30,
	})
	if math.Abs(r.F1-1.0) > 0.01 {
		t.Errorf("perfect detection: expected F1 ≈ 1, got %.4f", r.F1)
	}
}

func TestComputeGaussianF1_LateDetection(t *testing.T) {
	// Detection 3σ late — F1 should be significantly below 1 but above 0.
	gt := int64(1000)
	sigma := 30.0
	r := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{gt + 90}, // +3σ
		GroundTruthTimestamps: []int64{gt},
		Sigma:                 sigma,
	})
	if r.F1 >= 1.0 || r.F1 <= 0 {
		t.Errorf("late detection: expected 0 < F1 < 1, got %.4f", r.F1)
	}
}

func TestComputeGaussianF1_BaselineFPCounting(t *testing.T) {
	// Predictions before the GT onset count as baseline FP and drag down precision.
	gt := int64(1000)
	r := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{gt - 200, gt}, // one FP before onset, one exact
		GroundTruthTimestamps: []int64{gt},
		Sigma:                 30,
	})
	if r.FP == 0 {
		t.Errorf("expected FP > 0 for pre-onset prediction, got %.4f", r.FP)
	}
	if r.F1 >= 1.0 {
		t.Errorf("FP should depress F1 below 1, got %.4f", r.F1)
	}
}

func TestComputeGaussianF1_PostOnsetCascadingIgnored(t *testing.T) {
	// Extra post-onset predictions (after the first match) should be ignored,
	// not counted as FP.
	gt := int64(1000)
	r := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{gt, gt + 60, gt + 120}, // first matches, rest ignored
		GroundTruthTimestamps: []int64{gt},
		Sigma:                 30,
	})
	if r.NumFilteredCascading != 2 {
		t.Errorf("expected 2 cascading ignored, got %d", r.NumFilteredCascading)
	}
	// Precision should still be high — ignored predictions don't count as FP.
	if r.Precision < 0.9 {
		t.Errorf("cascading should not penalise precision, got %.4f", r.Precision)
	}
}

func TestComputeGaussianF1_MultipleGT(t *testing.T) {
	// Two GT events each matched by a distinct prediction — recall should be full.
	gt1, gt2 := int64(1000), int64(2000)
	r := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  []int64{gt1, gt2},
		GroundTruthTimestamps: []int64{gt1, gt2},
		Sigma:                 30,
	})
	if math.Abs(r.F1-1.0) > 0.01 {
		t.Errorf("two perfect matches: expected F1 ≈ 1, got %.4f", r.F1)
	}
}

// ── ScoreMetrics ──────────────────────────────────────────────────────────────

func makeOutput(sources ...string) *ObserverOutput {
	out := &ObserverOutput{}
	for _, src := range sources {
		out.AnomalyPeriods = append(out.AnomalyPeriods, ObserverCorrelation{
			PeriodStart: 1000,
			Anomalies:   []ObserverAnomaly{{Source: src}},
		})
	}
	return out
}

func makeGT(tps, fps []string) *MetricGroundTruth {
	gt := &MetricGroundTruth{}
	if len(tps) > 0 {
		gt.TruePositives = []MetricGroundTruthEntry{{Service: "svc", Metrics: tps}}
	}
	if len(fps) > 0 {
		gt.FalsePositives = []MetricGroundTruthEntry{{Service: "svc", Metrics: fps}}
	}
	return gt
}

func TestScoreMetrics_AllTP(t *testing.T) {
	out := makeOutput("cpu.usage", "mem.usage")
	gt := makeGT([]string{"cpu.usage", "mem.usage"}, nil)
	r := ScoreMetrics(out, gt, 0)
	if r.TPCount != 2 || r.FPCount != 0 || r.UnknownCount != 0 {
		t.Errorf("all TP: expected TP=2 FP=0 Unk=0, got TP=%d FP=%d Unk=%d", r.TPCount, r.FPCount, r.UnknownCount)
	}
	if math.Abs(r.MetricPrecision-1.0) > 0.001 || math.Abs(r.MetricRecall-1.0) > 0.001 {
		t.Errorf("all TP: expected P=R=1, got P=%.4f R=%.4f", r.MetricPrecision, r.MetricRecall)
	}
}

func TestScoreMetrics_AllFP(t *testing.T) {
	out := makeOutput("bad.metric")
	gt := makeGT([]string{"cpu.usage"}, []string{"bad.metric"})
	r := ScoreMetrics(out, gt, 0)
	if r.FPCount != 1 || r.TPCount != 0 {
		t.Errorf("all FP: expected FP=1 TP=0, got FP=%d TP=%d", r.FPCount, r.TPCount)
	}
	if r.MetricPrecision != 0 {
		t.Errorf("all FP: expected precision=0, got %.4f", r.MetricPrecision)
	}
}

func TestScoreMetrics_Unknown(t *testing.T) {
	out := makeOutput("some.unlabeled.metric")
	gt := makeGT([]string{"cpu.usage"}, nil)
	r := ScoreMetrics(out, gt, 0)
	if r.UnknownCount != 1 || r.TPCount != 0 || r.FPCount != 0 {
		t.Errorf("unknown: expected Unk=1 TP=0 FP=0, got Unk=%d TP=%d FP=%d", r.UnknownCount, r.TPCount, r.FPCount)
	}
}

func TestScoreMetrics_MissedTP(t *testing.T) {
	// Prediction for only one of two TP metrics → recall = 0.5.
	out := makeOutput("cpu.usage")
	gt := makeGT([]string{"cpu.usage", "mem.usage"}, nil)
	r := ScoreMetrics(out, gt, 0)
	if math.Abs(r.MetricRecall-0.5) > 0.001 {
		t.Errorf("missed TP: expected recall=0.5, got %.4f", r.MetricRecall)
	}
	if len(r.TPMetricsMissed) != 1 {
		t.Errorf("missed TP: expected 1 missed metric, got %v", r.TPMetricsMissed)
	}
}

// ── metricMatches ─────────────────────────────────────────────────────────────

func TestMetricMatches(t *testing.T) {
	cases := []struct {
		source string
		key    string
		want   bool
	}{
		{"cpu.usage", "svc:cpu.usage", true},
		{"system.cpu.usage", "svc:cpu.usage", true},  // contains match
		{"mem.rss", "svc:cpu.usage", false},
		{"", "svc:cpu.usage", false},
		{"cpu.usage", "bad-key-no-colon", false}, // malformed key
	}
	for _, tc := range cases {
		got := metricMatches(tc.source, tc.key)
		if got != tc.want {
			t.Errorf("metricMatches(%q, %q) = %v, want %v", tc.source, tc.key, got, tc.want)
		}
	}
}
