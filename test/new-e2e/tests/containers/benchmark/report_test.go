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
		Phases:      []Window{testWindow, testSwappedWindow},
		Baseline: Version{
			Name:   "baseline",
			Commit: "0123456789abcdef0123456789abcdef01234567",
			Images: []Image{{Component: "Node Agent", Image: "registry/agent:7.84.0", ImageID: "registry/agent@sha256:1234"}},
		},
		Comparison: Version{
			Name:   "comparison",
			Images: []Image{{Component: "Node Agent", Image: "registry/agent-qa:1234-abcdef"}},
		},
		Results:      []Result{cpu, memory, runTime},
		DashboardURL: "https://dddev.datadoghq.com/dashboard/abc-def-ghi",
		ProfileComparisons: []ProfileComparison{
			{Component: "Node Agent", URL: "https://dddev.datadoghq.com/profiling/comparison?node"},
			{Component: "Cluster Agent", URL: "https://dddev.datadoghq.com/profiling/comparison?dca"},
		},
	}
}

func TestReportMarkdown(t *testing.T) {
	markdown := testReport(VerdictRegression).Markdown()

	for _, expected := range []string{
		"on the EKS cluster `bench-cluster`, in 2 phases, between which installations a and b, each with node pools of its own, swap their images:\n\n" +
			"- phase 1, from 2026-10-13 10:00 UTC to 2026-10-13 10:20 UTC (20m0s): baseline images on installation a, comparison images on installation b\n" +
			"- phase 2, from 2026-10-13 11:00 UTC to 2026-10-13 11:20 UTC (20m0s): baseline images on installation b, comparison images on installation a\n\n",
		"| Node Agent | [`container.cpu.usage`](https://dddev.datadoghq.com/metric/explorer?exp_metric=container.cpu.usage) | 100.0 millicores | 112.5 millicores | +12.5% (+12.5 millicores) | < 0.001 | **regression** |",
		`| Cluster Agent | ` + "`container.memory.rss`" + ` | | | | | **error**: baseline: no data \| at all |`,
		"Graphs and profiles of the run: [dashboard](https://dddev.datadoghq.com/dashboard/abc-def-ghi)\n",
		"each image runs half of the phases on the node pools of each installation",
		"Each phase is split into blocks of 5m0s, in which",
		"significant (t-test of e = 0, p < 0.05)",
		"| Node Agent | `kubelet` check run time | 40.0 ms | 38.0 ms | -5.0% (-2.0 ms) | 0.200 | unchanged |",
		"`container.cpu.usage` 5% and 5.0 millicores; `container.memory.rss` 5% and 10.0 MiB; `kubelet` check run time 10% and 5.0 ms.",
		"| baseline | Node Agent | `registry/agent:7.84.0` (`registry/agent@sha256:1234`) |",
		"| comparison | Node Agent | `registry/agent-qa:1234-abcdef` |",
		"- baseline commit: [`01234567`](https://github.com/DataDog/datadog-agent/commit/0123456789abcdef0123456789abcdef01234567)\n",
		"CPU profiles of the baseline (A) and of the comparison (B) over the phases: [Node Agent](https://dddev.datadoghq.com/profiling/comparison?node), [Cluster Agent](https://dddev.datadoghq.com/profiling/comparison?dca)\n",
	} {
		assert.Contains(t, markdown, expected)
	}
}

func TestReportMarkdownFourPhases(t *testing.T) {
	report := testReport(VerdictRegression)
	report.Phases = testWindows(4, 15*time.Minute)

	assert.Contains(t, report.Markdown(),
		"in 4 phases, between which installations a and b, each with node pools of its own, swap their images:\n\n"+
			"- phase 1, from 2026-10-13 10:00 UTC to 2026-10-13 10:15 UTC (15m0s): baseline images on installation a, comparison images on installation b\n"+
			"- phase 2, from 2026-10-13 11:00 UTC to 2026-10-13 11:15 UTC (15m0s): baseline images on installation b, comparison images on installation a\n"+
			"- phase 3, from 2026-10-13 12:00 UTC to 2026-10-13 12:15 UTC (15m0s): baseline images on installation a, comparison images on installation b\n"+
			"- phase 4, from 2026-10-13 13:00 UTC to 2026-10-13 13:15 UTC (15m0s): baseline images on installation b, comparison images on installation a\n\n")
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
			assert.Equal(t, *report, decoded)
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
	appURL := url.URL{Scheme: "https", Host: "dddev.datadoghq.com"}
	to := testFrom.Add(30 * time.Minute)

	explorer, err := url.Parse(MetricsExplorerURL(appURL, "container.cpu.usage", []string{"kube_cluster_name:bench", "kube_container_name:agent"}, []string{"kube_namespace"}, testFrom, to))
	require.NoError(t, err)
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

	profiles, err := url.Parse(ProfileComparisonURL(appURL, "kube_cluster_name:bench image_tag:1", "kube_cluster_name:bench image_tag:2", testFrom, to))
	require.NoError(t, err)
	assert.Equal(t, "/profiling/comparison", profiles.Path)
	assert.Equal(t, url.Values{
		"profile_type":      {"cpu-time"},
		"comparisonViz":     {"flamegraph"},
		"compareValuesMode": {"absolute"},
		"compare_query_A":   {"kube_cluster_name:bench image_tag:1"},
		"compare_start_A":   {"1791885600000"},
		"compare_end_A":     {"1791887400000"},
		"compare_query_B":   {"kube_cluster_name:bench image_tag:2"},
		"compare_start_B":   {"1791885600000"},
		"compare_end_B":     {"1791887400000"},
		"from_ts":           {"1791885600000"},
		"to_ts":             {"1791887400000"},
		"live":              {"false"},
	}, profiles.Query())

	assert.Equal(t, "https://dddev.datadoghq.com/dashboard/abc-def-ghi?from_ts=1791885600000&live=false&refresh_mode=paused&to_ts=1791887400000&tpl_var_kube_cluster_name%5B0%5D=bench&tpl_var_profiles_from%5B0%5D=1791885600000",
		DashboardURL(appURL, "abc-def-ghi", map[string]string{"kube_cluster_name": "bench", "profiles_from": "1791885600000"}, testFrom, to))

	// Only the scheme and the host of the app URL are kept, and the page path is escaped.
	assert.Equal(t, "https://dddev.datadoghq.com/dashboard/a%20b?from_ts=1791885600000&live=false&refresh_mode=paused&to_ts=1791887400000",
		DashboardURL(url.URL{Scheme: "https", Host: "dddev.datadoghq.com", Path: "/ignored", Fragment: "ignored"}, "a b", nil, testFrom, to))
}
