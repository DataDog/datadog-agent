// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package benchmark

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testReport(verdict Verdict) *Report {
	cpu := Result{
		Component:  "Node Agent",
		Metric:     "container.cpu.usage",
		Unit:       UnitNanocores,
		Thresholds: Thresholds{Relative: 0.05, Absolute: 5e6},
		Comparison: Comparison{
			BaselineMean:   100e6,
			ComparisonMean: 112.5e6,
			RelativeChange: 0.125,
			PValue:         0.0004,
			Verdict:        verdict,
			Phases: []PhaseComparison{
				{Window: testWindow, BaselineMean: 90e6, ComparisonMean: 112.5e6, LogRatio: 0.223, Blocks: 4},
				{Window: testSwappedWindow, BaselineMean: 110e6, ComparisonMean: 112.5e6, LogRatio: 0.022, Blocks: 4},
			},
		},
		ExplorerURL: "https://dddev.datadoghq.com/metric/explorer?exp_metric=container.cpu.usage",
	}
	memory := Result{
		Component:  "Cluster Agent",
		Metric:     "container.memory.rss",
		Unit:       UnitBytes,
		Thresholds: Thresholds{Relative: 0.05, Absolute: 10 << 20},
	}
	memory.Fail(errors.New("baseline: no data | at all"))
	runTime := Result{
		Component:  "Node Agent",
		Metric:     "check run time",
		Check:      "kubelet",
		Unit:       UnitMilliseconds,
		Thresholds: Thresholds{Relative: 0.1, Absolute: 5},
		Comparison: Comparison{BaselineMean: 40, ComparisonMean: 38, RelativeChange: -0.05, PValue: 0.2, Verdict: VerdictUnchanged},
	}

	return &Report{
		ClusterName: "bench-cluster",
		Phases:      [2]Window{testWindow, testSwappedWindow},
		Baseline: Version{
			Name:        "baseline",
			Commit:      "0123456789abcdef0123456789abcdef01234567",
			Images:      []Image{{Component: "Node Agent", Image: "registry/agent:7.84.0", ImageID: "registry/agent@sha256:1234"}},
			ProfilesURL: "https://dddev.datadoghq.com/profiling/explorer?query=baseline",
		},
		Comparison: Version{
			Name:   "comparison",
			Images: []Image{{Component: "Node Agent", Image: "registry/agent-qa:1234-abcdef"}},
		},
		Results: []Result{cpu, memory, runTime},
	}
}

func TestReportMarkdown(t *testing.T) {
	markdown := testReport(VerdictRegression).Markdown()

	for _, expected := range []string{
		"on the EKS cluster `bench-cluster`, in two phases: from 2026-10-13 10:00 UTC to 2026-10-13 10:20 UTC (20m0s) with the baseline images on installation a and the comparison images on installation b, each with node pools of its own, then from 2026-10-13 11:00 UTC to 2026-10-13 11:20 UTC (20m0s) with the images swapped.",
		"| Node Agent | [`container.cpu.usage`](https://dddev.datadoghq.com/metric/explorer?exp_metric=container.cpu.usage) | 100.0 millicores | 112.5 millicores | +12.5% (+12.5 millicores) | < 0.001 | **regression** |",
		`| Cluster Agent | ` + "`container.memory.rss`" + ` | | | | | **error**: baseline: no data \| at all |`,
		"Each phase is split into blocks of 5m0s, in which",
		"significant (t-test of e = 0, p < 0.05)",
		"| Node Agent | `kubelet` check run time | 40.0 ms | 38.0 ms | -5.0% (-2.0 ms) | 0.200 | unchanged |",
		"`container.cpu.usage` 5% and 5.0 millicores; `container.memory.rss` 5% and 10.0 MiB; `kubelet` check run time 10% and 5.0 ms.",
		"| baseline | Node Agent | `registry/agent:7.84.0` (`registry/agent@sha256:1234`) |",
		"| comparison | Node Agent | `registry/agent-qa:1234-abcdef` |",
		"- baseline commit: [`01234567`](https://github.com/DataDog/datadog-agent/commit/0123456789abcdef0123456789abcdef01234567)\n",
		"Profiles of the first phase: [baseline](https://dddev.datadoghq.com/profiling/explorer?query=baseline)\n",
	} {
		assert.Contains(t, markdown, expected)
	}
}

func TestReportWrite(t *testing.T) {
	tests := []struct {
		name    string
		verdict Verdict
		// failed keeps the result of testReport that couldn't be compared.
		failed   bool
		markdown bool
	}{
		{name: "regression", verdict: VerdictRegression, failed: true, markdown: true},
		{name: "improvement", verdict: VerdictImprovement, markdown: true},
		{name: "unchanged", verdict: VerdictUnchanged, markdown: false},
		{name: "unchanged with an error", verdict: VerdictUnchanged, failed: true, markdown: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "report")
			report := testReport(tt.verdict)
			if !tt.failed {
				report.Results = slices.DeleteFunc(report.Results, func(result Result) bool { return result.Error != "" })
			}
			// A Markdown report left by a previous run must not survive an unchanged one.
			require.NoError(t, os.MkdirAll(dir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, MarkdownReportFile), []byte("stale"), 0o644))

			require.NoError(t, report.Write(dir))

			markdown, err := os.ReadFile(filepath.Join(dir, MarkdownReportFile))
			if tt.markdown {
				require.NoError(t, err)
				assert.Equal(t, report.Markdown(), string(markdown))
			} else {
				assert.ErrorIs(t, err, os.ErrNotExist)
			}

			data, err := os.ReadFile(filepath.Join(dir, JSONReportFile))
			require.NoError(t, err)
			var decoded Report
			require.NoError(t, json.Unmarshal(data, &decoded))
			assert.Equal(t, report.Results, decoded.Results)
			assert.Equal(t, report.Baseline, decoded.Baseline)
			assert.Equal(t, report.Phases, decoded.Phases)
		})
	}
}

func TestReportMarkdownDifferentSignificances(t *testing.T) {
	report := testReport(VerdictRegression)
	report.Results[0].Thresholds.Significance = SignificanceLevel / 10
	markdown := report.Markdown()
	assert.Contains(t, markdown, "significant (t-test of e = 0, p below the level given with the thresholds of its metric)")
	assert.Contains(t, markdown, "(p < 0.005)")
	assert.Contains(t, markdown, "(p < 0.05)")
}

func TestURLs(t *testing.T) {
	to := testFrom.Add(30 * time.Minute)

	explorer, err := url.Parse(MetricsExplorerURL("https://dddev.datadoghq.com", "container.cpu.usage", []string{"kube_cluster_name:bench", "kube_container_name:agent"}, []string{"kube_namespace"}, testFrom, to))
	require.NoError(t, err)
	assert.Equal(t, "dddev.datadoghq.com", explorer.Host)
	assert.Equal(t, "/metric/explorer", explorer.Path)
	assert.Equal(t, url.Values{
		"exp_metric":   {"container.cpu.usage"},
		"exp_scope":    {"kube_cluster_name:bench,kube_container_name:agent"},
		"exp_group":    {"kube_namespace"},
		"exp_agg":      {"avg"},
		"exp_row_type": {"metric"},
		"from_ts":      {"1791885600000"},
		"to_ts":        {"1791887400000"},
		"live":         {"false"},
	}, explorer.Query())

	profiles, err := url.Parse(ProfilesURL("https://dddev.datadoghq.com", "kube_cluster_name:bench kube_namespace:datadog-a", testFrom, to))
	require.NoError(t, err)
	assert.Equal(t, "/profiling/explorer", profiles.Path)
	assert.Equal(t, "kube_cluster_name:bench kube_namespace:datadog-a", profiles.Query().Get("query"))
	assert.Equal(t, "1791885600000", profiles.Query().Get("from_ts"))
}
