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
	"testing"
	"time"

	"github.com/samber/lo"
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
		// The results, collapsed, after the visible introduction
		"Resource usage of the comparison Agent against the baseline Agent, on the EKS cluster `bench-cluster`, in 2 phases. " +
			"Graphs and profiles of the run: [dashboard](https://dddev.datadoghq.com/dashboard/abc-def-ghi)\n\n" +
			"<details>\n\n<summary>\U0001F534 1 regression, \u26A0\uFE0F 1 metric not compared</summary>\n\n" +
			"Between the phases, installations a and b, each with node pools of its own, swap their images:\n\n" +
			"- phase 1, from 2026-10-13 10:00 UTC to 2026-10-13 10:20 UTC (20m0s): baseline images on installation a, comparison images on installation b\n" +
			"- phase 2, from 2026-10-13 11:00 UTC to 2026-10-13 11:20 UTC (20m0s): baseline images on installation b, comparison images on installation a\n\n" +
			"| Component | Metric |",
		"| Node Agent | [`container.cpu.usage`](https://dddev.datadoghq.com/metric/explorer?exp_metric=container.cpu.usage) | 100.0 millicores | 112.5 millicores | +12.5% (+12.5 millicores) | < 0.001 | \U0001F534 **regression** |",
		`| Cluster Agent | ` + "`container.memory.rss`" + ` | | | | | ` + "\u26A0\uFE0F" + ` **error**: baseline: no data \| at all |`,
		"each image runs half of the phases on the node pools of each installation",
		"Each phase is split into blocks of 5m0s, in which",
		"significant (t-test of e = 0, p < 0.05)",
		"| Node Agent | `kubelet` check run time | 40.0 ms | 38.0 ms | -5.0% (-2.0 ms) | 0.200 | \u26AA unchanged |",
		// The versions, collapsed, after the results
		"`container.cpu.usage` 5% and 5.0 millicores; `container.memory.rss` 5% and 10.0 MiB; `kubelet` check run time 10% and 5.0 ms.\n\n</details>\n\n" +
			"<details>\n\n<summary>Baseline <code>01234567</code>, comparison <code>registry/agent-qa:1234-abcdef</code></summary>\n\n" +
			"- baseline commit: [`01234567`](https://github.com/DataDog/datadog-agent/commit/0123456789abcdef0123456789abcdef01234567)\n",
		"| baseline | Node Agent | `registry/agent:7.84.0` (`registry/agent@sha256:1234`) |",
		"| comparison | Node Agent | `registry/agent-qa:1234-abcdef` |\n\n</details>\n\n" +
			"CPU profiles of the baseline (A) and of the comparison (B) over the phases: [Node Agent](https://dddev.datadoghq.com/profiling/comparison?node), [Cluster Agent](https://dddev.datadoghq.com/profiling/comparison?dca)\n",
	} {
		assert.Contains(t, markdown, expected)
	}
}

func TestReportVerdictSummary(t *testing.T) {
	results := func(verdicts ...Verdict) *Report {
		return &Report{Results: lo.Map(verdicts, func(verdict Verdict, _ int) Result {
			return Result{Comparison: Comparison{Verdict: verdict}}
		})}
	}
	for _, tt := range []struct {
		name    string
		report  *Report
		summary string
	}{
		{name: "no result", report: results(), summary: "\u26AA No significant change"},
		{name: "unchanged", report: results(VerdictUnchanged, VerdictUnchanged), summary: "\u26AA No significant change"},
		{name: "improvements", report: results(VerdictImprovement, VerdictUnchanged, VerdictImprovement), summary: "\U0001F7E2 2 improvements"},
		{name: "regression", report: results(VerdictUnchanged, VerdictRegression), summary: "\U0001F534 1 regression"},
		{
			name:    "all",
			report:  results(VerdictImprovement, VerdictError, VerdictRegression, VerdictError, VerdictRegression),
			summary: "\U0001F534 2 regressions, \U0001F7E2 1 improvement, \u26A0\uFE0F 2 metrics not compared",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.summary, tt.report.verdictSummary())
		})
	}
}

func TestVersionSummary(t *testing.T) {
	assert.Equal(t, "<code>01234567</code>", Version{Commit: "0123456789abcdef"}.summary())
	assert.Equal(t, "<code>registry/agent:7.84.0&lt;x&gt;</code>", Version{Images: []Image{{Image: "registry/agent:7.84.0<x>"}}}.summary())
	assert.Equal(t, "unknown", Version{}.summary())
}

func TestReportMarkdownFourPhases(t *testing.T) {
	report := testReport(VerdictRegression)
	report.Phases = testWindows(4, 15*time.Minute)

	assert.Contains(t, report.Markdown(),
		"Between the phases, installations a and b, each with node pools of its own, swap their images:\n\n"+
			"- phase 1, from 2026-10-13 10:00 UTC to 2026-10-13 10:15 UTC (15m0s): baseline images on installation a, comparison images on installation b\n"+
			"- phase 2, from 2026-10-13 11:00 UTC to 2026-10-13 11:15 UTC (15m0s): baseline images on installation b, comparison images on installation a\n"+
			"- phase 3, from 2026-10-13 12:00 UTC to 2026-10-13 12:15 UTC (15m0s): baseline images on installation a, comparison images on installation b\n"+
			"- phase 4, from 2026-10-13 13:00 UTC to 2026-10-13 13:15 UTC (15m0s): baseline images on installation b, comparison images on installation a\n\n")
}

func TestReportWrite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "report")
	// With a result that couldn't be compared, whose error the JSON report keeps too
	report := testReport(VerdictRegression)

	require.NoError(t, report.Write(dir))

	markdown, err := os.ReadFile(filepath.Join(dir, MarkdownReportFile))
	require.NoError(t, err)
	assert.Equal(t, report.Markdown(), string(markdown))

	data, err := os.ReadFile(filepath.Join(dir, JSONReportFile))
	require.NoError(t, err)
	var decoded Report
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, *report, decoded)
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
