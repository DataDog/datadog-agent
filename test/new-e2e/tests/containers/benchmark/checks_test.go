// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package benchmark

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testStatus = `{
  "runnerStats": {
    "Checks": {
      "container": {
        "container:e8ab5e0b3a1f2d4c": {"CheckName": "container", "TotalRuns": 12, "TotalErrors": 0, "MetricSamples": 900, "TotalMetricSamples": 10500, "LastError": "", "ExecutionTimes": [12, 15, 9]}
      },
      "simple_check": {
        "simple_check:foo:1": {"TotalRuns": 3, "MetricSamples": 1},
        "simple_check:foo:2": {"TotalRuns": 0, "MetricSamples": 0}
      }
    },
    "Workers": {"Count": 4}
  },
  "version": "7.84.0"
}`

func TestParseRunnerStats(t *testing.T) {
	expected := RunnerStats{
		"container": {
			"container:e8ab5e0b3a1f2d4c": {TotalRuns: 12, MetricSamples: 900, ExecutionTimes: []int64{12, 15, 9}},
		},
		"simple_check": {
			"simple_check:foo:1": {TotalRuns: 3, MetricSamples: 1},
			"simple_check:foo:2": {},
		},
	}

	tests := []struct {
		name   string
		output string
		err    string
	}{
		{
			name:   "JSON only",
			output: testStatus,
		},
		{
			name:   "log lines around the JSON document",
			output: "2026-10-13 21:30:06 UTC | CORE | INFO | (pkg/collector/python/init.go:350 in resolvePythonHome) | Using '/opt/datadog-agent/embedded' as Python home\n" + testStatus + "\n2026-10-13 21:30:07 UTC | CORE | INFO | done\n",
		},
		{
			name:   "no JSON document",
			output: "2026-10-13 21:30:06 UTC | CORE | ERROR | unable to reach the Agent {status}",
			err:    "no JSON document in the status output",
		},
		{
			name:   "no runner stats",
			output: `{"version": "7.84.0"}`,
			err:    "no runnerStats.Checks in the status output",
		},
		{
			name:   "truncated JSON document",
			output: testStatus[:100],
			err:    "unable to parse the status output",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats, err := ParseRunnerStats(tt.output)
			if tt.err != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, expected, stats)
			assert.EqualValues(t, 901, stats.MetricSamples("container")+stats.MetricSamples("simple_check"))
			assert.Zero(t, stats.MetricSamples("kubelet"))
		})
	}
}

func TestCheckRequirementVerify(t *testing.T) {
	healthy := CheckInstanceStats{TotalRuns: 5, MetricSamples: 10}
	notRunYet := CheckInstanceStats{}
	failing := CheckInstanceStats{TotalRuns: 5, TotalErrors: 1, MetricSamples: 10, LastError: "connection refused"}

	tests := []struct {
		name        string
		requirement CheckRequirement
		stats       RunnerStats
		err         string
	}{
		{
			name:        "healthy",
			requirement: CheckRequirement{Name: "kubelet", MinRuns: 2},
			stats:       RunnerStats{"kubelet": {"kubelet:1": healthy}},
		},
		{
			name:        "not running",
			requirement: CheckRequirement{Name: "kubelet", MinRuns: 2},
			stats:       RunnerStats{"container": {"container:1": healthy}},
			err:         "check kubelet isn't running",
		},
		{
			name:        "not enough runs",
			requirement: CheckRequirement{Name: "kubelet", MinRuns: 2},
			stats:       RunnerStats{"kubelet": {"kubelet:1": {TotalRuns: 1, MetricSamples: 10}}},
			err:         "check kubelet: kubelet:1: 1 run(s), want at least 2",
		},
		{
			name:        "errors",
			requirement: CheckRequirement{Name: "kubelet", MinRuns: 2},
			stats:       RunnerStats{"kubelet": {"kubelet:1": failing}},
			err:         `check kubelet: kubelet:1: 1 error(s) in 5 run(s), last error: "connection refused"`,
		},
		{
			name:        "no metric samples",
			requirement: CheckRequirement{Name: "kubelet", MinRuns: 2},
			stats:       RunnerStats{"kubelet": {"kubelet:1": {TotalRuns: 5}}},
			err:         "check kubelet: kubelet:1: no metric sample in its last run",
		},
		{
			name:        "every instance must be healthy",
			requirement: CheckRequirement{Name: "kubelet", MinRuns: 2},
			stats:       RunnerStats{"kubelet": {"kubelet:1": healthy, "kubelet:2": notRunYet}},
			err:         "check kubelet: kubelet:2: 0 run(s), want at least 2",
		},
		{
			name:        "any healthy instance",
			requirement: CheckRequirement{Name: "simple_check", MinRuns: 1, AnyInstance: true},
			stats:       RunnerStats{"simple_check": {"simple_check:1": notRunYet, "simple_check:2": failing, "simple_check:3": healthy}},
		},
		{
			name:        "no healthy instance",
			requirement: CheckRequirement{Name: "simple_check", MinRuns: 1, AnyInstance: true},
			stats: RunnerStats{"simple_check": {
				"simple_check:1": notRunYet,
				"simple_check:2": notRunYet,
				"simple_check:3": notRunYet,
				"simple_check:4": notRunYet,
				"simple_check:5": notRunYet,
			}},
			err: "no instance of check simple_check ran 1 time(s) without error and with metric samples: simple_check:1: 0 run(s), want at least 1; simple_check:2: 0 run(s), want at least 1; simple_check:3: 0 run(s), want at least 1; and 2 more",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.requirement.Verify(tt.stats)
			if tt.err == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.err)
			}
		})
	}
}

func TestCompareMetricSamples(t *testing.T) {
	tests := []struct {
		name                 string
		baseline, comparison int64
		err                  string
	}{
		{name: "equal", baseline: 1000, comparison: 1000},
		{name: "within tolerance", baseline: 1000, comparison: 760},
		{name: "far fewer samples", baseline: 1000, comparison: 500, err: "the comparison emitted 500 metric samples per run, -50.0% from the 1000 of the baseline, beyond ±25%"},
		{name: "far more samples", baseline: 1000, comparison: 1300, err: "the comparison emitted 1300 metric samples per run, +30.0% from the 1000 of the baseline, beyond ±25%"},
		{name: "no baseline sample", baseline: 0, comparison: 10, err: "the baseline emitted 0 metric samples"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CompareMetricSamples(tt.baseline, tt.comparison, 0.25)
			if tt.err == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.err)
			}
		})
	}
}
