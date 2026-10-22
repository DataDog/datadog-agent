// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package benchmark holds the cloud-independent logic of the benchmarkeks performance
// non-regression test: it verifies the checks the Agents run, compares the resource usage
// of the baseline and comparison versions, and renders the result as a report.
package benchmark

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
)

// maxReportedInstanceFailures caps the instances listed by CheckRequirement.Verify, as a check
// scheduled on churning pods can have dozens of them.
const maxReportedInstanceFailures = 3

// CheckInstanceStats is the subset of the stats of a check instance listed under
// runnerStats.Checks.<check>.<instance> by `agent status --json` the benchmark relies on.
type CheckInstanceStats struct {
	TotalRuns   uint64
	TotalErrors uint64
	// MetricSamples is the number of metric samples of the last run.
	MetricSamples int64
	LastError     string
	// ExecutionTimes is a ring buffer of the durations of the last runs, in milliseconds:
	// the run number n (from 0) is at index n % len(ExecutionTimes).
	ExecutionTimes []int64
}

// RunnerStats are the stats of the check instances of an Agent, by check name, then by
// instance ID.
type RunnerStats map[string]map[string]CheckInstanceStats

// ParseRunnerStats extracts the runner stats from the output of `agent status --json`. The
// log lines the Agent may print before the JSON document are skipped.
func ParseRunnerStats(output string) (RunnerStats, error) {
	start := 0
	if !strings.HasPrefix(output, "{") {
		start = strings.Index(output, "\n{") + 1
		if start == 0 {
			return nil, errors.New("no JSON document in the status output")
		}
	}

	var status struct {
		RunnerStats struct {
			Checks RunnerStats `json:"Checks"`
		} `json:"runnerStats"`
	}
	// Decode rather than Unmarshal, so that trailing log lines are ignored too.
	if err := json.NewDecoder(strings.NewReader(output[start:])).Decode(&status); err != nil {
		return nil, fmt.Errorf("unable to parse the status output: %w", err)
	}
	if status.RunnerStats.Checks == nil {
		return nil, errors.New("no runnerStats.Checks in the status output")
	}
	return status.RunnerStats.Checks, nil
}

// MetricSamples returns the number of metric samples the instances of check emitted in
// their last run.
func (s RunnerStats) MetricSamples(check string) int64 {
	var samples int64
	for _, instance := range s[check] {
		samples += instance.MetricSamples
	}
	return samples
}

// CheckRequirement is what the instances of a check must meet in the runner stats of an
// Agent: to have run at least MinRuns times, without any error, and to have emitted metric
// samples in their last run.
type CheckRequirement struct {
	Name    string
	MinRuns uint64
	// AnyInstance only requires one instance of the check to meet the requirement instead of
	// all of them, for a check scheduled on churning pods: its instances come and go, and
	// the newest ones may not have run yet.
	AnyInstance bool
}

// Verify returns an error if the check doesn't meet the requirement in stats.
func (r CheckRequirement) Verify(stats RunnerStats) error {
	instances := stats[r.Name]
	if len(instances) == 0 {
		return fmt.Errorf("check %s isn't running", r.Name)
	}

	var failures []string
	for _, id := range slices.Sorted(maps.Keys(instances)) {
		err := r.verifyInstance(instances[id])
		if err == nil && r.AnyInstance {
			return nil
		}
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", id, err))
		}
	}
	if len(failures) == 0 {
		return nil
	}

	if len(failures) > maxReportedInstanceFailures {
		failures = append(failures[:maxReportedInstanceFailures], fmt.Sprintf("and %d more", len(failures)-maxReportedInstanceFailures))
	}
	if r.AnyInstance {
		return fmt.Errorf("no instance of check %s ran %d time(s) without error and with metric samples: %s", r.Name, r.MinRuns, strings.Join(failures, "; "))
	}
	return fmt.Errorf("check %s: %s", r.Name, strings.Join(failures, "; "))
}

func (r CheckRequirement) verifyInstance(instance CheckInstanceStats) error {
	switch {
	case instance.TotalErrors > 0:
		return fmt.Errorf("%d error(s) in %d run(s), last error: %q", instance.TotalErrors, instance.TotalRuns, instance.LastError)
	case instance.TotalRuns < r.MinRuns:
		return fmt.Errorf("%d run(s), want at least %d", instance.TotalRuns, r.MinRuns)
	case instance.MetricSamples <= 0:
		return errors.New("no metric sample in its last run")
	}
	return nil
}

// CompareMetricSamples returns an error if the metric samples per run of a check of the
// comparison differ from the baseline's by more than tolerance, relative to the baseline.
// A comparison emitting far fewer samples than the baseline most likely means the
// comparison broke the check, which would otherwise pass for a performance improvement.
func CompareMetricSamples(baseline, comparison int64, tolerance float64) error {
	if baseline <= 0 {
		return fmt.Errorf("the baseline emitted %d metric samples", baseline)
	}
	change := float64(comparison-baseline) / float64(baseline)
	if math.Abs(change) > tolerance {
		return fmt.Errorf("the comparison emitted %d metric samples per run, %+.1f%% from the %d of the baseline, beyond ±%.0f%%", comparison, 100*change, baseline, 100*tolerance)
	}
	return nil
}
