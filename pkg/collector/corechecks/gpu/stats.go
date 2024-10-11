// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package gpu

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
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

	// Metadata associated to this stream
	metadata *model.StreamMetadata
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

	for _, span := range data.Allocations {
		// Send events on memory leak
		if span.IsLeaked {
			start := sp.timeResolver.ResolveMonotonicTimestamp(span.StartKtime)
			end := sp.timeResolver.ResolveMonotonicTimestamp(span.EndKtime)

			ev := event.Event{
				AlertType:      event.AlertTypeWarning,
				Priority:       event.PriorityNormal,
				SourceTypeName: CheckName,
				EventType:      "gpu-memory-leak",
				Title:          "Leaked GPU memory allocation",
				Text:           fmt.Sprintf("PID %d leaked %d bytes of memory, allocated at time %s", sp.key.Pid, span.Size, start),
				Ts:             end.Unix(),
			}

			sp.sender.Event(ev)
		}
	}

	sp.pastAllocs = append(sp.pastAllocs, data.Allocations...)
}

func (sp *StatsProcessor) processCurrentData(data *model.StreamData) {
	for _, span := range data.Spans {
		sp.processKernelSpan(span)
	}

	sp.currentAllocs = data.Allocations
}

// getTags returns the tags to use for the metrics
func (sp *StatsProcessor) getTags() []string {
	return []string{
		fmt.Sprintf("pid:%d", sp.key.Pid),
		fmt.Sprintf("container_id:%s", sp.metadata.ContainerID),
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

		if sp.sentEvents == 0 {
			err := sp.sender.GaugeWithTimestamp(metricNameUtil, utilization, "", sp.getTags(), float64(sp.firstKernelStart.Unix()))
			if err != nil {
				return fmt.Errorf("cannot send metric: %w", err)
			}
		}
		err := sp.sender.GaugeWithTimestamp(metricNameUtil, utilization, "", sp.getTags(), float64(sp.lastKernelEnd.Unix()))
		if err != nil {
			return fmt.Errorf("cannot send metric: %w", err)
		}

		if sp.lastKernelEnd.After(sp.maxTimestampLastMetric) {
			sp.maxTimestampLastMetric = sp.lastKernelEnd
		}
	}

	builders := make(map[model.MemAllocType]*tseriesBuilder)
	getBuilder := func(allocType model.MemAllocType) *tseriesBuilder {
		if _, ok := builders[allocType]; !ok {
			builders[allocType] = &tseriesBuilder{}
		}
		return builders[allocType]
	}

	for _, alloc := range sp.currentAllocs {
		startEpoch := uint64(sp.timeResolver.ResolveMonotonicTimestamp(alloc.StartKtime).Unix())
		getBuilder(alloc.Type).AddEventStart(startEpoch, int64(alloc.Size))
	}
	for _, alloc := range sp.pastAllocs {
		startEpoch := uint64(sp.timeResolver.ResolveMonotonicTimestamp(alloc.StartKtime).Unix())
		endEpoch := uint64(sp.timeResolver.ResolveMonotonicTimestamp(alloc.EndKtime).Unix())
		getBuilder(alloc.Type).AddEvent(startEpoch, endEpoch, int64(alloc.Size))
	}

	lastCheckEpoch := sp.lastCheck.Unix()

	for allocType, builder := range builders {
		tags := append(sp.getTags(), fmt.Sprintf("memory_type:%s", allocType))
		sentPoints := false
		points, maxMemory := builder.Build()
		for _, point := range points {
			// Do not send points that are before the last check, those have been already sent
			// Also do not send points that are 0, unless we have already sent some points, in which case
			// we need them to close the series
			if int64(point.time) > lastCheckEpoch && (point.value > 0 || sentPoints) {
				err := sp.sender.GaugeWithTimestamp(metricNameMemory, float64(point.value), "", tags, float64(point.time))
				if err != nil {
					return fmt.Errorf("cannot send metric: %w", err)
				}
			}

			if int64(point.time) > sp.maxTimestampLastMetric.Unix() {
				sp.maxTimestampLastMetric = time.Unix(int64(point.time), 0)
			}

			sentPoints = true
		}

		sp.sender.Gauge(metricNameMaxMem, float64(maxMemory), "", tags)
	}

	sp.sentEvents++

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
