// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package benchmark

import (
	"fmt"
	"maps"
	"slices"
	"time"
)

// Snapshot holds the runner stats of the pods of an Agent workload at a time, by pod name.
type Snapshot struct {
	Time  time.Time
	Stats map[string]RunnerStats
}

// runTimes returns the durations, in milliseconds, of the runs a check instance did between
// two snapshots of its stats. The Agent only keeps the durations of the last runs, so the
// snapshots must be close enough.
func runTimes(before, after CheckInstanceStats) ([]int64, error) {
	if after.TotalRuns < before.TotalRuns {
		return nil, fmt.Errorf("%d run(s), down from %d: the Agent restarted", after.TotalRuns, before.TotalRuns)
	}
	runs := after.TotalRuns - before.TotalRuns
	ring := uint64(len(after.ExecutionTimes))
	if runs > ring {
		return nil, fmt.Errorf("%d runs between the snapshots, but the Agent keeps the durations of the last %d only", runs, ring)
	}

	times := make([]int64, 0, runs)
	for run := before.TotalRuns; run < after.TotalRuns; run++ {
		times = append(times, after.ExecutionTimes[run%ring])
	}
	return times, nil
}

// meanRunTime returns the mean duration, in milliseconds, of the runs of check between two
// snapshots of the same pods, pooled over their instances of the check. An instance missing
// from the first snapshot is new: all its runs are counted.
func meanRunTime(check string, before, after Snapshot) (float64, error) {
	beforePods, afterPods := slices.Sorted(maps.Keys(before.Stats)), slices.Sorted(maps.Keys(after.Stats))
	if !slices.Equal(beforePods, afterPods) {
		return 0, fmt.Errorf("the pods changed between the snapshots: %v, then %v", beforePods, afterPods)
	}

	var total int64
	var runs int
	for _, pod := range afterPods {
		beforeInstances, afterInstances := before.Stats[pod][check], after.Stats[pod][check]
		for _, id := range slices.Sorted(maps.Keys(beforeInstances)) {
			if _, found := afterInstances[id]; !found {
				return 0, fmt.Errorf("pod %s: instance %s of check %s stopped between the snapshots", pod, id, check)
			}
		}
		for _, id := range slices.Sorted(maps.Keys(afterInstances)) {
			times, err := runTimes(beforeInstances[id], afterInstances[id])
			if err != nil {
				return 0, fmt.Errorf("pod %s: instance %s: %w", pod, id, err)
			}
			for _, t := range times {
				total += t
			}
			runs += len(times)
		}
	}
	if runs == 0 {
		return 0, fmt.Errorf("check %s didn't run between %s and %s", check, before.Time.UTC().Format(time.RFC3339), after.Time.UTC().Format(time.RFC3339))
	}
	return float64(total) / float64(runs), nil
}

// RunTimePoints returns a point per pair of consecutive snapshots: the mean duration, in
// milliseconds, of the runs of check between them, timestamped at the earlier one. Taking the
// snapshots at the start of the blocks of a window, and at its end, makes a point per block.
func RunTimePoints(check string, snapshots []Snapshot) ([]Point, error) {
	points := make([]Point, 0, len(snapshots))
	for i := 1; i < len(snapshots); i++ {
		mean, err := meanRunTime(check, snapshots[i-1], snapshots[i])
		if err != nil {
			return nil, fmt.Errorf("between %s and %s: %w", snapshots[i-1].Time.UTC().Format(time.RFC3339), snapshots[i].Time.UTC().Format(time.RFC3339), err)
		}
		points = append(points, Point{Timestamp: snapshots[i-1].Time, Value: mean})
	}
	return points, nil
}
