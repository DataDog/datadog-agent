// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	recorderimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/impl"
	"github.com/stretchr/testify/require"
)

// evalScenario defines a benchmark scenario with its hardcoded ground truth.
type evalScenario struct {
	name          string
	groundTruthTS int64 // disruption onset, unix seconds
	baselineStart int64 // warmup ends / baseline begins, unix seconds
}

// Benchmark scenarios with verified ground truths.
// Disruption timestamps from gensim episode results (2026-03-03 runs).
var evalScenarios = []evalScenario{
	{
		name:          "213_pagerduty",
		groundTruthTS: 1772542175, // 2026-03-03T12:49:35Z — Cassandra repair onset
		baselineStart: 1772541575, // 2026-03-03T12:39:35Z
	},
	{
		name:          "353_postmark",
		groundTruthTS: 1772542515, // 2026-03-03T12:55:15Z — DNS upstream outage onset
		baselineStart: 1772541639, // 2026-03-03T12:40:39Z
	},
	{
		name:          "food_delivery_redis",
		groundTruthTS: 1772542488, // 2026-03-03T12:54:48Z — Redis CPU saturation onset
		baselineStart: 1772541888, // 2026-03-03T12:44:48Z
	},
}

// scenariosDir returns the absolute path to the scenarios directory.
func scenariosDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	dir := filepath.Join(repoRoot, "scenarios")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skipf("scenarios dir not found at %s — skipping eval", dir)
	}
	return dir
}

func TestEval(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping eval in short mode")
	}

	sDir := scenariosDir(t)
	recorder := recorderimpl.NewReadOnlyRecorder()
	sigma := 30.0

	type result struct {
		name  string
		score *ScoreResult
	}
	var results []result

	for _, sc := range evalScenarios {
		t.Run(sc.name, func(t *testing.T) {
			parquetDir := filepath.Join(sDir, sc.name, "parquet")
			if _, err := os.Stat(parquetDir); os.IsNotExist(err) {
				t.Skipf("parquet data not found for %s — skipping", sc.name)
			}

			tb, err := NewTestBench(TestBenchConfig{
				ScenariosDir: sDir,
				Recorder:     recorder,
			})
			require.NoError(t, err)

			outputPath := filepath.Join(t.TempDir(), "output.json")
			err = tb.RunHeadless(sc.name, outputPath, false)
			require.NoError(t, err)

			score, err := ScoreOutputFile(outputPath, []int64{sc.groundTruthTS}, sDir, sigma)
			require.NoError(t, err)

			results = append(results, result{name: sc.name, score: score})

			t.Logf("F1=%.4f  P=%.4f  R=%.4f  TP=%.2f  FP=%.2f  scored=%d  warmup(excl)=%d  cascading(excl)=%d",
				score.F1, score.Precision, score.Recall, score.TP, score.FP,
				score.NumPredictions, score.NumFilteredWarmup, score.NumFilteredCascading)
		})
	}

	// Print summary table
	if len(results) > 0 {
		fmt.Println()
		fmt.Printf("%-25s  %6s  %9s  %6s  %4s  %4s  %6s  %18s  %20s\n",
			"Scenario", "F1", "Precision", "Recall", "TP", "FP", "Scored", "Warmup (excluded)", "Cascading (excluded)")
		fmt.Printf("%-25s  %6s  %9s  %6s  %4s  %4s  %6s  %18s  %20s\n",
			"─────────────────────────", "──────", "─────────", "──────", "────", "────", "──────",
			"──────────────────", "────────────────────")
		for _, r := range results {
			fmt.Printf("%-25s  %6.4f  %9.4f  %6.4f  %4.2f  %4.2f  %6d  %18d  %20d\n",
				r.name, r.score.F1, r.score.Precision, r.score.Recall,
				r.score.TP, r.score.FP,
				r.score.NumPredictions, r.score.NumFilteredWarmup, r.score.NumFilteredCascading)
		}
		fmt.Println()
	}
}
