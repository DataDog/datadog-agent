// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package benchmark

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testRunTimeSnapshots returns the snapshots of a pod taken at the boundaries of the blocks
// of window, whose kubelet check runs 10 times per block, each lasting runTime(block, run)
// milliseconds.
func testRunTimeSnapshots(window Window, runTime func(block, run int) int64) []Snapshot {
	ring := make([]int64, 32)
	var totalRuns uint64
	var snapshots []Snapshot
	for block := 0; ; block++ {
		at := window.From.Add(time.Duration(block) * window.Block)
		stats := CheckInstanceStats{TotalRuns: totalRuns, ExecutionTimes: slices.Clone(ring)}
		snapshots = append(snapshots, Snapshot{Time: at, Stats: map[string]RunnerStats{"pod": {"kubelet": {"kubelet:1": stats}}}})
		if !at.Before(window.To) {
			return snapshots
		}
		for run := range 10 {
			ring[totalRuns%32] = runTime(block, run)
			totalRuns++
		}
	}
}

func TestRunTimeWindow(t *testing.T) {
	first, swapped := RunTimeWindow(testWindow), RunTimeWindow(testSwappedWindow)
	assert.Equal(t, Window{From: testWindow.From, To: testWindow.To, Block: 150 * time.Second}, first)

	// The comparison makes the check 15% faster, on node pools making it 20% slower on the
	// comparison installation in the first phase.
	runTime := func(milliseconds float64, seed int) func(block, run int) int64 {
		return func(block, run int) int64 {
			return int64(milliseconds * (1 + 0.05*math.Sin(float64(10*block+run+seed))))
		}
	}
	points := func(window Window, runTime func(block, run int) int64) []Point {
		points, err := RunTimePoints("kubelet", testRunTimeSnapshots(window, runTime))
		require.NoError(t, err)
		return points
	}
	c, err := Compare(
		Phase{Window: first, Baseline: points(first, runTime(40, 0)), Comparison: points(first, runTime(1.2*34, 1))},
		Phase{Window: swapped, Baseline: points(swapped, runTime(1.2*40, 2)), Comparison: points(swapped, runTime(34, 3))},
		Thresholds{Relative: 0.1, Absolute: 5},
	)
	require.NoError(t, err)
	assert.Equal(t, VerdictImprovement, c.Verdict)
	assert.InDelta(t, -0.15, c.RelativeChange, 0.02)
	// A point per block of the run times, twice as many as the blocks of the container metrics
	assert.Equal(t, 8, c.Phases[0].Blocks)
	assert.Equal(t, 8, c.Phases[1].Blocks)
}

// staleRunTime fills the entries of the ring buffer of older runs, which must never be read.
const staleRunTime = 999

// instanceStats returns the stats of an instance whose runs from the run number first lasted
// durations, as the Agent stores them in its ring buffer of 32 entries.
func instanceStats(first uint64, durations ...int64) CheckInstanceStats {
	stats := CheckInstanceStats{TotalRuns: first + uint64(len(durations)), ExecutionTimes: make([]int64, 32)}
	for i := range stats.ExecutionTimes {
		stats.ExecutionTimes[i] = staleRunTime
	}
	for i, duration := range durations {
		stats.ExecutionTimes[(first+uint64(i))%32] = duration
	}
	return stats
}

func TestRunTimes(t *testing.T) {
	tests := []struct {
		name          string
		before, after CheckInstanceStats
		times         []int64
		err           string
	}{
		{
			name:   "no run",
			before: instanceStats(0, 10, 20),
			after:  instanceStats(0, 10, 20),
			times:  []int64{},
		},
		{
			name:   "runs",
			before: instanceStats(0, 10, 20),
			after:  instanceStats(0, 10, 20, 30, 40),
			times:  []int64{30, 40},
		},
		{
			name:   "wrap-around",
			before: CheckInstanceStats{TotalRuns: 30},
			after:  instanceStats(30, 1, 2, 3, 4, 5),
			times:  []int64{1, 2, 3, 4, 5},
		},
		{
			name:   "new instance",
			before: CheckInstanceStats{},
			after:  instanceStats(0, 7, 8),
			times:  []int64{7, 8},
		},
		{
			name:   "as many runs as the ring buffer holds",
			before: CheckInstanceStats{TotalRuns: 40},
			after:  instanceStats(40, make([]int64, 32)...),
			times:  make([]int64, 32),
		},
		{
			name:   "more runs than the ring buffer holds",
			before: CheckInstanceStats{TotalRuns: 40},
			after:  instanceStats(40, make([]int64, 33)...),
			err:    "33 runs between the snapshots, but the Agent keeps the durations of the last 32 only",
		},
		{
			name:   "restart",
			before: CheckInstanceStats{TotalRuns: 40},
			after:  instanceStats(0, 1, 2),
			err:    "2 run(s), down from 40: the Agent restarted",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			times, err := runTimes(tt.before, tt.after)
			if tt.err != "" {
				assert.EqualError(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.times, times)
		})
	}
}

func TestMeanRunTime(t *testing.T) {
	at := time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)
	snapshot := func(stats map[string]RunnerStats) Snapshot {
		return Snapshot{Time: at, Stats: stats}
	}
	before := snapshot(map[string]RunnerStats{
		"pod-a": {
			"kubelet":      {"kubelet:a": instanceStats(0, 50)},
			"simple_check": {"simple_check:a": instanceStats(0, 1)},
		},
		"pod-b": {"kubelet": {"kubelet:b": instanceStats(30, 60, 60)}},
	})

	tests := []struct {
		name  string
		after Snapshot
		mean  float64
		err   string
	}{
		{
			name: "runs pooled over pods and instances",
			after: snapshot(map[string]RunnerStats{
				// 2 runs on pod-a, 1 run of the existing instance and 2 of a new one on pod-b
				"pod-a": {"kubelet": {"kubelet:a": instanceStats(0, 50, 10, 20)}},
				"pod-b": {"kubelet": {
					"kubelet:b":   instanceStats(30, 60, 60, 30),
					"kubelet:new": instanceStats(0, 40, 50),
				}},
			}),
			mean: (10 + 20 + 30 + 40 + 50) / 5.0,
		},
		{
			name: "pod replaced",
			after: snapshot(map[string]RunnerStats{
				"pod-a": {"kubelet": {"kubelet:a": instanceStats(0, 50, 10)}},
				"pod-c": {"kubelet": {"kubelet:c": instanceStats(0, 10)}},
			}),
			err: "the pods changed between the snapshots: [pod-a pod-b], then [pod-a pod-c]",
		},
		{
			name: "instance stopped",
			after: snapshot(map[string]RunnerStats{
				"pod-a": {"kubelet": {}},
				"pod-b": {"kubelet": {"kubelet:b": instanceStats(30, 60, 60, 30)}},
			}),
			err: "pod pod-a: instance kubelet:a of check kubelet stopped between the snapshots",
		},
		{
			name: "restart",
			after: snapshot(map[string]RunnerStats{
				"pod-a": {"kubelet": {"kubelet:a": instanceStats(0, 50, 10)}},
				"pod-b": {"kubelet": {"kubelet:b": instanceStats(0, 10)}},
			}),
			err: "pod pod-b: instance kubelet:b: 1 run(s), down from 32: the Agent restarted",
		},
		{
			name: "no run",
			after: snapshot(map[string]RunnerStats{
				"pod-a": {"kubelet": {"kubelet:a": instanceStats(0, 50)}},
				"pod-b": {"kubelet": {"kubelet:b": instanceStats(30, 60, 60)}},
			}),
			err: "check kubelet didn't run between 2026-10-13T10:00:00Z and 2026-10-13T10:00:00Z",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mean, err := meanRunTime("kubelet", before, tt.after)
			if tt.err != "" {
				assert.EqualError(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			assert.InDelta(t, tt.mean, mean, 1e-9)
		})
	}
}

func TestRunTimePoints(t *testing.T) {
	at := time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)
	snapshots := []Snapshot{
		{Time: at, Stats: map[string]RunnerStats{"pod": {"kubelet": {"kubelet:1": instanceStats(0, 5)}}}},
		{Time: at.Add(5 * time.Minute), Stats: map[string]RunnerStats{"pod": {"kubelet": {"kubelet:1": instanceStats(0, 5, 10, 20)}}}},
		{Time: at.Add(10 * time.Minute), Stats: map[string]RunnerStats{"pod": {"kubelet": {"kubelet:1": instanceStats(0, 5, 10, 20, 30)}}}},
	}

	points, err := RunTimePoints("kubelet", snapshots)
	require.NoError(t, err)
	assert.Equal(t, []Point{
		{Timestamp: at, Value: 15},
		{Timestamp: at.Add(5 * time.Minute), Value: 30},
	}, points)

	snapshots[2].Stats["pod"]["kubelet"]["kubelet:1"] = instanceStats(0, 1)
	_, err = RunTimePoints("kubelet", snapshots)
	assert.EqualError(t, err, "between 2026-10-13T10:05:00Z and 2026-10-13T10:10:00Z: pod pod: instance kubelet:1: 1 run(s), down from 3: the Agent restarted")
}
