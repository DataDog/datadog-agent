// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
)

const nsecPerSec = 1e9

// aggregator is responsible for processing the data from the streams and generating the metrics
type aggregator struct {
	// totalThreadSecondsUsed is the total amount of thread-seconds used by the GPU in the current interval
	totalThreadSecondsUsed float64

	// lastCheckKtime is the last kernel time the processor was checked
	lastCheckKtime int64

	// measuredIntervalNs is the interval between the last two checks, in nanoseconds
	measuredIntervalNs int64

	// currentAllocs is the list of current memory allocations
	currentAllocs []*model.MemoryAllocation

	// pastAllocs is the list of past memory allocations
	pastAllocs []*model.MemoryAllocation

	// lastKernelEndKtime is the timestamp of the last kernel end
	lastKernelEndKtime int64

	// firstKernelStart is the timestamp of the first kernel start
	firstKernelStartKtime int64

	// utilizationNormFactor is the factor to normalize the utilization by, to account for the fact that we might have more kernels enqueued than the GPU can run in parallel. This factor
	// allows distributing the utilization over all the streams that are enqueued
	utilizationNormFactor float64

	// hasPendingData is true if there is data pending to be sent
	hasPendingData bool

	// processEnded is true if the process has ended and this aggregator should be deleted
	processEnded bool

	// sysCtx is the system context with global GPU-system data
	sysCtx *systemContext
}

func newAggregator(sysCtx *systemContext) *aggregator {
	return &aggregator{
		sysCtx: sysCtx,
	}
}

// processKernelSpan processes a kernel span
func (agg *aggregator) processKernelSpan(span *model.KernelSpan) {
	tsStart := int64(span.StartKtime)
	tsEnd := int64(span.EndKtime)

	if agg.firstKernelStartKtime == 0 || tsStart < agg.firstKernelStartKtime {
		agg.firstKernelStartKtime = int64(span.EndKtime)
	}

	// we only want to consider data that was not already processed in the previous interval
	if agg.lastCheckKtime > tsStart {
		tsStart = agg.lastCheckKtime
	}

	durationSec := float64(tsEnd-tsStart) / nsecPerSec
	maxThreads := uint64(agg.sysCtx.maxGpuThreadsPerDevice[0])                                // TODO: MultiGPU support not enabled yet
	agg.totalThreadSecondsUsed += durationSec * float64(min(span.AvgThreadCount, maxThreads)) // we can't use more threads than the GPU has

	if tsEnd > agg.lastKernelEndKtime {
		agg.lastKernelEndKtime = tsEnd
	}
}

func (agg *aggregator) processPastData(data *model.StreamData) {
	for _, span := range data.Spans {
		agg.processKernelSpan(span)
	}

	agg.pastAllocs = append(agg.pastAllocs, data.Allocations...)
	agg.hasPendingData = true
}

func (agg *aggregator) processCurrentData(data *model.StreamData) {
	for _, span := range data.Spans {
		agg.processKernelSpan(span)
	}

	agg.currentAllocs = append(agg.currentAllocs, data.Allocations...)
	agg.hasPendingData = true
}

func (agg *aggregator) getGPUUtilization() float64 {
	intervalSecs := float64(agg.measuredIntervalNs) / nsecPerSec
	if intervalSecs > 0 {
		// TODO: MultiGPU support not enabled yet
		availableThreadSeconds := float64(agg.sysCtx.maxGpuThreadsPerDevice[0]) * intervalSecs
		return agg.totalThreadSecondsUsed / availableThreadSeconds
	}

	return 0
}

func (agg *aggregator) setGPUUtilizationNormalizationFactor(factor float64) {
	agg.utilizationNormFactor = factor
}

func (agg *aggregator) getStats() model.PIDStats {
	var stats model.PIDStats

	if agg.measuredIntervalNs > 0 {
		stats.UtilizationPercentage = agg.getGPUUtilization() / agg.utilizationNormFactor
	}

	var memTsBuilder tseriesBuilder

	for _, alloc := range agg.currentAllocs {
		memTsBuilder.AddEventStart(alloc.StartKtime, int64(alloc.Size))
	}

	for _, alloc := range agg.pastAllocs {
		memTsBuilder.AddEvent(alloc.StartKtime, alloc.EndKtime, int64(alloc.Size))
	}

	lastValue, maxValue := memTsBuilder.GetLastAndMax()
	stats.CurrentMemoryBytes = uint64(lastValue)
	stats.MaxMemoryBytes = uint64(maxValue)

	// Flush the data that we used
	agg.flushProcessedStats()

	return stats
}

func (agg *aggregator) flushProcessedStats() {
	agg.currentAllocs = agg.currentAllocs[:0]
	agg.pastAllocs = agg.pastAllocs[:0]
	agg.totalThreadSecondsUsed = 0
	agg.hasPendingData = false
}
