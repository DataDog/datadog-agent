// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestRecoveryQuietWindow(t *testing.T) {
	for _, tc := range []struct{ duration, quietDuration int64 }{
		{1200, 600}, {1201, 600}, {600, 300}, {240, 120}, {501, 250},
	} {
		t.Run(fmt.Sprint(tc.duration), func(t *testing.T) {
			w := RecoveryWindow{Start: 2000, End: 2000 + tc.duration}
			start := w.End - tc.quietDuration
			r := ComputeGaussianF1(ScoreInput{
				GroundTruthTimestamps: []int64{1000}, Sigma: 30, Recovery: &w,
				// Deliberately unsorted, including both sides of each boundary.
				PredictionTimestamps: []int64{w.End, start, 1000, start - 1, w.End - 1, w.Start, w.Start - 1, 999},
			})
			if r.RecoveryQuietStart != start || r.RecoveryQuietDurationSeconds != tc.quietDuration {
				t.Fatalf("unexpected quiet window: %+v", r)
			}
			if r.NumBaselineFPs != 1 || r.NumRecoveryFPs != 2 || r.FP != 3 ||
				r.NumFilteredRecovery != 2 || r.NumFilteredOutsideScenario != 1 || r.NumFilteredCascading != 1 {
				t.Fatalf("incorrect phase classification: %+v", r)
			}
			if math.Abs(r.F1-0.4) > 0.001 {
				t.Errorf("one onset TP and three FPs: expected F1≈0.4, got %v", r.F1)
			}
		})
	}
}

func TestRecoveryCannotEarnOnsetCredit(t *testing.T) {
	r := ComputeGaussianF1(ScoreInput{
		GroundTruthTimestamps: []int64{1000}, PredictionTimestamps: []int64{2000, 2300},
		Sigma: 10000, Recovery: &RecoveryWindow{Start: 2000, End: 2600},
	})
	if r.TP != 0 || r.FN != 1 || r.FP != 1 || r.NumRecoveryFPs != 1 || r.NumFilteredRecovery != 1 {
		t.Fatalf("recovery must not match onset even with broad tolerance: %+v", r)
	}
}

func TestQuietRecoveryDoesNotRequireAnEmission(t *testing.T) {
	r := ComputeGaussianF1(ScoreInput{
		GroundTruthTimestamps: []int64{1000}, PredictionTimestamps: []int64{1000}, Sigma: 30,
		Recovery: &RecoveryWindow{Start: 2000, End: 2600},
	})
	if math.Abs(r.F1-1) > 0.001 || r.NumGroundTruths != 1 || r.NumRecoveryFPs != 0 {
		t.Fatalf("quiet recovery should leave a perfect onset score unchanged: %+v", r)
	}
}

func writeScoringFixture(t *testing.T, periods []ObserverCorrelation, cooldownStart, cooldownEnd string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	scenarioDir := filepath.Join(dir, "scenario")
	if err := os.Mkdir(scenarioDir, 0755); err != nil {
		t.Fatal(err)
	}
	stamp := func(ts int64) string { return time.Unix(ts, 0).UTC().Format(time.RFC3339) }
	meta := map[string]any{
		"baseline":   map[string]string{"start": stamp(100), "end": stamp(1000)},
		"disruption": map[string]string{"start": stamp(1000)},
		"cooldown":   map[string]string{"start": cooldownStart, "end": cooldownEnd},
	}
	output := ObserverOutput{Metadata: ObserverMetadata{Scenario: "scenario"}, AnomalyPeriods: periods}
	outputPath := filepath.Join(dir, "output.json")
	for path, value := range map[string]any{filepath.Join(scenarioDir, "episode.json"): meta, outputPath: output} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return outputPath, dir
}

func TestScoreOutputFileHighSeverityPhaseContract(t *testing.T) {
	var periods []ObserverCorrelation
	for _, ts := range []int64{50, 950, 1000, 1100, 2000, 2299, 2400, 2600} {
		end := ts + 20
		if ts == 2299 {
			// Existing episodes overlapping the quiet tail are not new FPs.
			end = 2350
		}
		periods = append(periods, ObserverCorrelation{
			Pattern: fmt.Sprintf("anomaly_scorer_high:%d", ts), PeriodStart: ts,
			PeriodEnd: end,
		})
	}
	for _, pattern := range []string{"anomaly_scorer_medium:900", "time_cluster:900", "anomaly_scorer_medium:2400", "untyped"} {
		periods = append(periods, ObserverCorrelation{Pattern: pattern, PeriodStart: 2400})
	}
	path, dir := writeScoringFixture(t, periods, "1970-01-01T00:33:20Z", "1970-01-01T00:43:20Z")
	r, err := ScoreOutputFile(path, nil, dir, 30)
	if err != nil {
		t.Fatal(err)
	}
	if r.ScoringVersion != scoringVersion || r.NumFilteredNonHigh != 4 || r.NumFilteredWarmup != 1 ||
		r.NumPredictions != 7 || r.NumBaselineFPs != 1 || r.NumRecoveryFPs != 1 || r.FP != 2 ||
		r.NumFilteredRecovery != 2 || r.NumFilteredCascading != 1 || r.NumFilteredOutsideScenario != 1 {
		t.Fatalf("incorrect output classification: %+v", r)
	}
	if math.Abs(r.F1-0.5) > 0.001 || r.BaselineDurationSeconds != 900 || math.Abs(r.Alpha-1.0/900) > 1e-10 {
		t.Fatalf("recovery FP should affect F1, but not baseline alpha: %+v", r)
	}
}

func TestScoreOutputFileValidatesRecoveryMetadata(t *testing.T) {
	for _, tc := range []struct{ name, start, end, want string }{
		{"missing", "", "", "requires cooldown.start"},
		{"bad start", "invalid", "1970-01-01T00:43:20Z", "parsing cooldown.start"},
		{"bad end", "1970-01-01T00:33:20Z", "invalid", "parsing cooldown.end"},
		{"reversed", "1970-01-01T00:43:20Z", "1970-01-01T00:33:20Z", "requires disruption.start"},
		{"empty", "1970-01-01T00:33:20Z", "1970-01-01T00:33:20Z", "requires disruption.start"},
		{"before onset", "1970-01-01T00:00:01Z", "1970-01-01T00:43:20Z", "requires disruption.start"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, dir := writeScoringFixture(t, nil, tc.start, tc.end)
			// Explicit onset must not silently bypass malformed recovery metadata.
			_, err := ScoreOutputFile(path, []int64{1000}, dir, 30)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wanted %q, got %v", tc.want, err)
			}
		})
	}
}

func TestScoreOutputFileTimestampOnly(t *testing.T) {
	path, _ := writeScoringFixture(t, []ObserverCorrelation{{Pattern: "anomaly_scorer_high:1000", PeriodStart: 1000}}, "", "")
	r, err := ScoreOutputFile(path, []int64{1000}, "", 30)
	if err != nil {
		t.Fatal(err)
	}
	if r.RecoveryQuietDurationSeconds != 0 || math.Abs(r.F1-1) > 0.001 {
		t.Fatalf("unexpected timestamp-only result: %+v", r)
	}
	for _, sigma := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := ScoreOutputFile(path, []int64{1000}, "", sigma); err == nil {
			t.Errorf("accepted invalid sigma %v", sigma)
		}
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
		{"system.cpu.usage", "svc:cpu.usage", true}, // contains match
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
