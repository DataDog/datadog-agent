// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gpu

import (
	"fmt"
	"slices"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	sectime "github.com/DataDog/datadog-agent/pkg/security/resolvers/time"
)

type StatsProcessor struct {
	key                    *model.StreamKey
	totalThreadSecondsUsed float64
	sender                 sender.Sender
	gpuMaxThreads          int
	lastCheck              time.Time
	measuredInterval       time.Duration
	timeResolver           *sectime.Resolver
	currentAllocs          []*model.MemoryAllocation
	pastAllocs             []*model.MemoryAllocation
	lastKernelEnd          time.Time
	firstKernelStart       time.Time
	sentEvents             int
	maxTimestampLastMetric time.Time
}

func (sp *StatsProcessor) processKernelSpan(span *model.KernelSpan, sendEvent bool) {
	tsStart := sp.timeResolver.ResolveMonotonicTimestamp(span.Start)
	tsEnd := sp.timeResolver.ResolveMonotonicTimestamp(span.End)

	if sp.firstKernelStart.IsZero() {
		sp.firstKernelStart = tsStart
	} else if tsStart.Before(sp.firstKernelStart) {
		sp.firstKernelStart = tsStart
	}

	realDuration := tsEnd.Sub(tsStart)
	event := event.Event{
		SourceTypeName: CheckName,
		EventType:      "gpu-kernel",
		Title:          "GPU kernel launch",
		Text:           fmt.Sprintf("Start=%s, end=%s, avgThreadSize=%d, duration=%s, pid=%d", tsStart, tsEnd, span.AvgThreadCount, realDuration, sp.key.Pid),
		Ts:             tsStart.Unix(),
	}
	fmt.Printf("spanev: %v\n", event)

	if sendEvent {
		sp.sender.Event(event)
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

func (sp *StatsProcessor) processPastData(data *model.StreamPastData) {
	for _, span := range data.Spans {
		sp.processKernelSpan(span, true)
	}

	for _, span := range data.Allocations {
		ev := event.Event{
			AlertType:      event.AlertTypeInfo,
			Priority:       event.PriorityLow,
			AggregationKey: "gpu-0",
			SourceTypeName: CheckName,
			EventType:      "gpu-memory",
			Title:          fmt.Sprintf("GPU mem alloc size %d", span.Size),
			Text:           fmt.Sprintf("Start at %d, end %d", span.Start, span.End),
			Ts:             sp.timeResolver.ResolveMonotonicTimestamp(span.Start).Unix(),
		}

		if span.IsLeaked {
			ev.Priority = event.PriorityNormal
			ev.AlertType = event.AlertTypeWarning
			ev.Title += " (leaked)"
		}

		fmt.Printf("memev: %v\n", ev)
		sp.sender.Event(ev)
	}

	sp.pastAllocs = append(sp.pastAllocs, data.Allocations...)
}

func (sp *StatsProcessor) processCurrentData(data *model.StreamCurrentData) {
	if data.Span != nil {
		sp.processKernelSpan(data.Span, false)
	}

	sp.currentAllocs = data.CurrentAllocations
}

func (sp *StatsProcessor) getTags() []string {
	return []string{
		fmt.Sprintf("pid:%d", sp.key.Pid),
	}
}

type memAllocTsPoint struct {
	ts   uint64
	size int64
}

func (sp *StatsProcessor) markInterval(now time.Time) {
	intervalSecs := sp.measuredInterval.Seconds()
	if intervalSecs > 0 {
		availableThreadSeconds := float64(sp.gpuMaxThreads) * intervalSecs
		utilization := sp.totalThreadSecondsUsed / availableThreadSeconds
		fmt.Printf("GPU utilization: %f, totalUsed %f\n", utilization, sp.totalThreadSecondsUsed)

		if sp.sentEvents == 0 {
			sp.sender.GaugeWithTimestamp("gjulian.cudapoc.utilization", utilization, "", sp.getTags(), float64(sp.firstKernelStart.Unix()))
		}
		sp.sender.GaugeWithTimestamp("gjulian.cudapoc.utilization", utilization, "", sp.getTags(), float64(sp.lastKernelEnd.Unix()))

		if sp.lastKernelEnd.After(sp.maxTimestampLastMetric) {
			sp.maxTimestampLastMetric = sp.lastKernelEnd
		}
	}

	fmt.Printf("past: %+v, current: %+v\n", sp.pastAllocs, sp.currentAllocs)

	points := make([]memAllocTsPoint, 0, len(sp.currentAllocs)+2*len(sp.pastAllocs))
	for _, alloc := range sp.currentAllocs {
		points = append(points, memAllocTsPoint{ts: alloc.Start, size: int64(alloc.Size)})
	}
	for _, alloc := range sp.pastAllocs {
		points = append(points, memAllocTsPoint{ts: alloc.Start, size: int64(alloc.Size)})
		points = append(points, memAllocTsPoint{ts: alloc.End, size: -int64(alloc.Size)})
	}

	// sort by timestamp. Stable so that allocations that start and end at the same time are processed in the order they were added
	slices.SortStableFunc(points, func(a, b memAllocTsPoint) int {
		return int(a.ts - b.ts)
	})

	currentMemory := int64(0)
	maxMemory := int64(0)
	pointsPerSecond := make([]memAllocTsPoint, 0)
	for i := range points {
		tsEpochCurrent := sp.timeResolver.ResolveMonotonicTimestamp(points[i].ts).Unix()
		if i > 0 {
			tsEpochPrev := int64(pointsPerSecond[len(pointsPerSecond)-1].ts)
			if tsEpochCurrent != tsEpochPrev {
				pointsPerSecond = append(pointsPerSecond, memAllocTsPoint{ts: uint64(tsEpochCurrent), size: 0})
			}
		} else if i == 0 {
			pointsPerSecond = append(pointsPerSecond, memAllocTsPoint{ts: uint64(tsEpochCurrent), size: 0})
		}

		pointsPerSecond[len(pointsPerSecond)-1].size += points[i].size
		currentMemory += points[i].size
		maxMemory = max(maxMemory, currentMemory)
	}

	lastCheckEpoch := sp.lastCheck.Unix()
	fmt.Printf("GPU memory: %d, max %d, lastCheckEpoch: %d, points: %+v\n", currentMemory, maxMemory, lastCheckEpoch, pointsPerSecond)

	totalMemBytes := 0
	for _, point := range pointsPerSecond {
		hadMemory := totalMemBytes > 0
		totalMemBytes += int(point.size)
		if point.ts > uint64(lastCheckEpoch) && (totalMemBytes > 0 || hadMemory) {
			fmt.Printf("gjulian.cudapoc.memory: %d, ts %d\n", totalMemBytes, point.ts)
			sp.sender.GaugeWithTimestamp("gjulian.cudapoc.memory", float64(totalMemBytes), "", sp.getTags(), float64(point.ts))
			if int64(point.ts) > sp.maxTimestampLastMetric.Unix() {
				sp.maxTimestampLastMetric = time.Unix(int64(point.ts), 0)
			}
		}
	}

	fmt.Printf("gjulian.cudapoc.max_memory: %d\n", maxMemory)
	sp.sender.GaugeWithTimestamp("gjulian.cudapoc.max_memory", float64(maxMemory), "", sp.getTags(), float64(now.Unix()))
	sp.sentEvents++
}

// finish ensures that all metrics sent by this processor are properly closed with a 0 value
func (sp *StatsProcessor) finish(now time.Time) {
	lastTs := now

	// Don't mark events as lasting more than what they should.
	if !sp.maxTimestampLastMetric.IsZero() {
		lastTs = sp.maxTimestampLastMetric.Add(time.Second)
	}

	sp.sender.GaugeWithTimestamp("gjulian.cudapoc.memory", 0, "", sp.getTags(), float64(lastTs.Unix()))
	sp.sender.GaugeWithTimestamp("gjulian.cudapoc.max_memory", 0, "", sp.getTags(), float64(lastTs.Unix()))
	sp.sender.GaugeWithTimestamp("gjulian.cudapoc.utilization", 0, "", sp.getTags(), float64(lastTs.Unix()))
}
