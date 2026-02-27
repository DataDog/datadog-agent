// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml

package gpu

import (
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
)

// aggregator is responsible for receiving multiple stream-level data for one process and aggregating it into metrics
// This struct is necessary as a single process might have multiple CUDA streams
type aggregator struct {
	// totalThreadSecondsUsed is the total amount of thread-seconds used by the
	// GPU in the current interval
	totalThreadSecondsUsed float64

	// lastCheckKtime is the last kernel time the processor was checked,
	// required to take into account only data from the current interval.
	lastCheckKtime uint64

	// measuredIntervalNs is the interval between the last two checks, in
	// nanoseconds, required to compute thread seconds used/available for
	// utilization percentages
	measuredIntervalNs int64

	// currentAllocs is the list of current (active) memory allocations
	currentAllocs []*memorySpan

	// pastAllocs is the list of past (freed) memory allocations
	pastAllocs []*memorySpan

	// deviceMaxThreads is the maximum number of threads the GPU can run in parallel, for utilization calculations
	deviceMaxThreads uint64

	// isActive is a flag to indicate if the aggregator has seen any activity in the last interval!
	isActive bool

	// activeIntervals stores the time intervals when kernels were active, used to compute ActiveTimePct
	activeIntervals [][2]uint64
}

func newAggregator(deviceMaxThreads uint64) *aggregator {
	return &aggregator{
		deviceMaxThreads: deviceMaxThreads,
	}
}

// mergeIntervals takes unsorted intervals and returns the total duration covered,
// handling overlapping intervals correctly. This is used to compute the percentage
// of time that any kernel was active on the GPU.
func mergeIntervals(intervals [][2]uint64) uint64 {
	if len(intervals) == 0 {
		return 0
	}

	// Sort intervals by start time
	sort.Slice(intervals, func(i, j int) bool {
		return intervals[i][0] < intervals[j][0]
	})

	// Merge overlapping intervals and compute total duration
	var totalDuration uint64
	currentStart := intervals[0][0]
	currentEnd := intervals[0][1]

	for i := 1; i < len(intervals); i++ {
		if intervals[i][0] <= currentEnd {
			// Overlapping or adjacent interval, extend the current interval
			if intervals[i][1] > currentEnd {
				currentEnd = intervals[i][1]
			}
		} else {
			// Non-overlapping interval, add duration of current interval and start a new one
			totalDuration += currentEnd - currentStart
			currentStart = intervals[i][0]
			currentEnd = intervals[i][1]
		}
	}

	// Add the last interval
	totalDuration += currentEnd - currentStart

	return totalDuration
}

// processKernelSpan takes a kernel span and computes the thread-seconds used by it during
// the interval
func (agg *aggregator) processKernelSpan(span *kernelSpan) {
	tsStart := span.startKtime
	tsEnd := span.endKtime

	// Clamp the interval to the measurement window [lastCheckKtime, lastCheckKtime + measuredIntervalNs]
	// This ensures we only count activity within the current measurement period
	if agg.lastCheckKtime > tsStart {
		tsStart = agg.lastCheckKtime
	}

	// Calculate the upper bound of the measurement window
	intervalEndKtime := agg.lastCheckKtime + uint64(agg.measuredIntervalNs)
	if tsEnd > intervalEndKtime {
		tsEnd = intervalEndKtime
	}

	durationSec := float64(tsEnd-tsStart) / float64(time.Second.Nanoseconds())

	// We can't use more threads than the GPU has. Even if we don't report
	// utilization directly, we should not count more threads than the GPU can
	// run in parallel, as the "thread count" we report is just the number of
	// threads that were enqueued.
	//
	// An example of a situation where this distinction is important: say we
	// have a kernel launch with 100 threads, but the GPU can only run 50
	// threads, and assume this kernel runs for 1 second and that we want to
	// report utilization for the last 2 seconds. If we were looking at the
	// actual GPU utilization in real-time, we'd see 100% utilization for the
	// second the span lasts, and then 0%, which would give us an average of 50%
	// utilization over the 2 seconds. However, if we didn't take into account
	// the number of threads the GPU can run in parallel, we'd report 200%
	// utilization for the first second instead, which would give us an average
	// of 100% utilization over the 2 seconds. which is not correct.
	activeThreads := min(span.avgThreadCount, agg.deviceMaxThreads)

	// weight the active threads by the time they were active
	agg.totalThreadSecondsUsed += durationSec * float64(activeThreads)

	// Track the interval for active time percentage calculation
	agg.activeIntervals = append(agg.activeIntervals, [2]uint64{tsStart, tsEnd})
}

// processPastData takes spans/allocations that have already been closed
func (agg *aggregator) processPastData(data *streamSpans) {
	for _, span := range data.kernels {
		agg.processKernelSpan(span)
	}

	agg.pastAllocs = append(agg.pastAllocs, data.allocations...)
}

// processCurrentData takes spans/allocations that are active (e.g., unfreed allocations, running kernels)
func (agg *aggregator) processCurrentData(data *streamSpans) {
	for _, span := range data.kernels {
		agg.processKernelSpan(span)
	}

	agg.currentAllocs = append(agg.currentAllocs, data.allocations...)
}

// getAverageCoreUsage returns the average core usage over the interval, in number of cores used
func (agg *aggregator) getAverageCoreUsage() float64 {
	if agg.measuredIntervalNs == 0 {
		return 0
	}

	intervalSecs := float64(agg.measuredIntervalNs) / float64(time.Second.Nanoseconds())
	return agg.totalThreadSecondsUsed / intervalSecs // Compute the average thread usage over the interval
}

// getRawStats returns the aggregated stats for the process, without any normalization
// Normalization to avoid over-reporting is done at the device level by the caller of this function.
// This function flushes the data after processing it.
func (agg *aggregator) getRawStats() model.UtilizationMetrics {
	var stats model.UtilizationMetrics
	stats.UsedCores = agg.getAverageCoreUsage()

	// Compute active time percentage
	if agg.measuredIntervalNs > 0 && len(agg.activeIntervals) > 0 {
		activeDurationNs := mergeIntervals(agg.activeIntervals)
		stats.ActiveTimePct = (float64(activeDurationNs) / float64(agg.measuredIntervalNs)) * 100.0
		// Cap at 100% to handle any edge cases
		if stats.ActiveTimePct > 100.0 {
			stats.ActiveTimePct = 100.0
		}
	}

	var memTsBuilders [memAllocTypeCount]tseriesBuilder

	for _, alloc := range agg.currentAllocs {
		memTsBuilders[alloc.allocType].AddEventStart(alloc.startKtime, int64(alloc.size))
	}

	for _, alloc := range agg.pastAllocs {
		memTsBuilders[alloc.allocType].AddEvent(alloc.startKtime, alloc.endKtime, int64(alloc.size))
	}

	for allocType := range memTsBuilders {
		lastValue, maxValue := memTsBuilders[allocType].GetLastAndMax()
		stats.Memory.CurrentBytes += uint64(lastValue)
		stats.Memory.MaxBytes += uint64(maxValue)
	}

	// Flush the data that we used
	agg.flush()

	return stats
}

func (agg *aggregator) flush() {
	agg.currentAllocs = agg.currentAllocs[:0]
	agg.pastAllocs = agg.pastAllocs[:0]
	agg.totalThreadSecondsUsed = 0
	agg.activeIntervals = agg.activeIntervals[:0]
}
