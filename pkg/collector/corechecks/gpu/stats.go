// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package gpu

import (
	"fmt"
	"math"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	sectime "github.com/DataDog/datadog-agent/pkg/security/resolvers/time"
)

const (
	gpuMetricsNs     = "gpu."
	metricNameMemory = gpuMetricsNs + "memory"
	metricNameUtil   = gpuMetricsNs + "utilization"
	metricNameMaxMem = gpuMetricsNs + "max_memory"
)

// StatsProcessor is responsible for processing the data from the GPU eBPF probe and generating metrics from it
type StatsProcessor struct {
	// key is the key of the stream this processor is processing
	key *model.StreamKey

	// totalThreadSecondsUsed is the total amount of thread-seconds used by the GPU in the current interval
	totalThreadSecondsUsed float64

	// sender is the sender to use to send metrics
	sender sender.Sender

	// gpuMaxThreads is the maximum number of threads the GPU can run in parallel, for utilization computations
	gpuMaxThreads int

	// lastCheck is the last time the processor was checked
	lastCheck time.Time

	// lastMemPoint is the time of the last memory timeseries point sent
	lastMemPointEpoch uint64

	// measuredInterval is the interval between the last two checks
	measuredInterval time.Duration

	// timeResolver is the time resolver to use to resolve timestamps
	timeResolver *sectime.Resolver

	// currentAllocs is the list of current memory allocations
	currentAllocs []*model.MemoryAllocation

	// pastAllocs is the list of past memory allocations
	pastAllocs []*model.MemoryAllocation

	// lastKernelEnd is the timestamp of the last kernel end
	lastKernelEnd time.Time

	// firstKernelStart is the timestamp of the first kernel start
	firstKernelStart time.Time

	// sentEvents is the number of events sent by this processor
	sentEvents int

	// maxTimestampLastMetric is the maximum timestamp of the last metric sent
	maxTimestampLastMetric time.Time

	// utilizationNormFactor is the factor to normalize the utilization by, to account for the fact that we might have more kernels enqueued than the GPU can run in parallel. This factor
	// allows distributing the utilization over all the streams that are enqueued
	utilizationNormFactor float64

	// hasPendingData is true if there is data pending to be sent
	hasPendingData bool
}

// processKernelSpan processes a kernel span
func (sp *StatsProcessor) processKernelSpan(span *model.KernelSpan) {
	tsStart := sp.timeResolver.ResolveMonotonicTimestamp(span.StartKtime)
	tsEnd := sp.timeResolver.ResolveMonotonicTimestamp(span.EndKtime)

	if sp.firstKernelStart.IsZero() || tsStart.Before(sp.firstKernelStart) {
		sp.firstKernelStart = tsStart
	}

	// we only want to consider data that was not already processed in the previous interval
	if sp.lastCheck.After(tsStart) {
		tsStart = sp.lastCheck
	}
	duration := tsEnd.Sub(tsStart)
	sp.totalThreadSecondsUsed += duration.Seconds() * float64(min(span.AvgThreadCount, uint64(sp.gpuMaxThreads))) // we can't use more threads than the GPU has
	if tsEnd.After(sp.lastKernelEnd) {
		sp.lastKernelEnd = tsEnd
	}
}

func (sp *StatsProcessor) processPastData(data *model.StreamData) {
	for _, span := range data.Spans {
		sp.processKernelSpan(span)
	}

	sp.pastAllocs = append(sp.pastAllocs, data.Allocations...)
	sp.hasPendingData = true
}

func (sp *StatsProcessor) processCurrentData(data *model.StreamData) {
	for _, span := range data.Spans {
		sp.processKernelSpan(span)
	}

	sp.currentAllocs = data.Allocations
	sp.hasPendingData = true
}

// getTags returns the tags to use for the metrics
func (sp *StatsProcessor) getTags() []string {
	return []string{
		fmt.Sprintf("pid:%d", sp.key.Pid),
	}
}

func (sp *StatsProcessor) getGPUUtilization() float64 {
	intervalSecs := sp.measuredInterval.Seconds()
	if intervalSecs > 0 {
		availableThreadSeconds := float64(sp.gpuMaxThreads) * intervalSecs
		return sp.totalThreadSecondsUsed / availableThreadSeconds
	}

	return 0
}

func (sp *StatsProcessor) setGPUUtilizationNormalizationFactor(factor float64) {
	sp.utilizationNormFactor = factor
}

func (sp *StatsProcessor) markInterval() error {
	intervalSecs := sp.measuredInterval.Seconds()
	if intervalSecs > 0 {
		utilization := sp.getGPUUtilization() / sp.utilizationNormFactor

		// if this is the first event, we need to send it with the timestamp of the first kernel start so that we have a complete series
		if sp.sentEvents == 0 {
			err := sp.sender.GaugeWithTimestamp(metricNameUtil, utilization, "", sp.getTags(), float64(sp.firstKernelStart.Unix()))
			if err != nil {
				return fmt.Errorf("cannot send metric: %w", err)
			}
		}

		// aftewards, we only need to update the utilization at the point of the last kernel end
		err := sp.sender.GaugeWithTimestamp(metricNameUtil, utilization, "", sp.getTags(), float64(sp.lastKernelEnd.Unix()))
		if err != nil {
			return fmt.Errorf("cannot send metric: %w", err)
		}

		if sp.lastKernelEnd.After(sp.maxTimestampLastMetric) {
			sp.maxTimestampLastMetric = sp.lastKernelEnd
		}
	}

	var memTsBuilder tseriesBuilder

	firstUnfreedAllocKTime := uint64(math.MaxUint64)

	for _, alloc := range sp.currentAllocs {
		firstUnfreedAllocKTime = min(firstUnfreedAllocKTime, alloc.StartKtime)
	}

	for _, alloc := range sp.pastAllocs {
		// Only build the timeseries up until the point of the first unfreed allocation. After that, the timeseries is still incomplete
		// until all those allocations are freed.
		if alloc.EndKtime < firstUnfreedAllocKTime {
			startEpoch := sp.timeResolver.ResolveMonotonicTimestamp(alloc.StartKtime).Unix()
			endEpoch := sp.timeResolver.ResolveMonotonicTimestamp(alloc.EndKtime).Unix()
			memTsBuilder.AddEvent(uint64(startEpoch), uint64(endEpoch), int64(alloc.Size))
		}
	}

	points, maxValue := memTsBuilder.Build()
	tags := sp.getTags()
	sentPoints := false

	for _, point := range points {
		// Do not send points that are before the last check, those have been already sent
		// Also do not send points that are 0, unless we have already sent some points, in which case
		// we need them to close the series
		if point.time > sp.lastMemPointEpoch && (point.value > 0 || sentPoints) {
			err := sp.sender.GaugeWithTimestamp(metricNameMemory, float64(point.value), "", tags, float64(point.time))
			if err != nil {
				return fmt.Errorf("cannot send metric: %w", err)
			}

			if int64(point.time) > sp.maxTimestampLastMetric.Unix() {
				sp.maxTimestampLastMetric = time.Unix(int64(point.time), 0)
			}

			sentPoints = true
		}
	}

	if len(points) > 0 {
		sp.lastMemPointEpoch = points[len(points)-1].time
	}

	sp.sender.Gauge(metricNameMaxMem, float64(maxValue), "", tags)
	sp.sentEvents++

	sp.hasPendingData = false

	return nil
}

// finish ensures that all metrics sent by this processor are properly closed with a 0 value
func (sp *StatsProcessor) finish(now time.Time) error {
	lastTs := now

	// Don't mark events as lasting more than what they should.
	if !sp.maxTimestampLastMetric.IsZero() {
		lastTs = sp.maxTimestampLastMetric.Add(time.Second)
	}

	err := sp.sender.GaugeWithTimestamp(metricNameMemory, 0, "", sp.getTags(), float64(lastTs.Unix()))
	if err != nil {
		return fmt.Errorf("cannot send metric: %w", err)
	}
	err = sp.sender.GaugeWithTimestamp(metricNameMaxMem, 0, "", sp.getTags(), float64(lastTs.Unix()))
	if err != nil {
		return fmt.Errorf("cannot send metric: %w", err)
	}
	err = sp.sender.GaugeWithTimestamp(metricNameUtil, 0, "", sp.getTags(), float64(lastTs.Unix()))
	if err != nil {
		return fmt.Errorf("cannot send metric: %w", err)
	}

	return nil
}
