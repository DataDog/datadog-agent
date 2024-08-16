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
}

func (sp *StatsProcessor) processPastData(data *model.StreamPastData) {
	for _, span := range data.Spans {
		event := event.Event{
			SourceTypeName: CheckName,
			EventType:      "gpu-kernel",
			Title:          "GPU kernel launch",
			Text:           fmt.Sprintf("Start=%s, end=%s, avgThreadSize=%d, duration=%ds", sp.timeResolver.ResolveMonotonicTimestamp(span.Start), sp.timeResolver.ResolveMonotonicTimestamp(span.End), span.AvgThreadCount, span.End-span.Start),
			Ts:             sp.timeResolver.ResolveMonotonicTimestamp(span.Start).Unix(),
		}
		fmt.Printf("spanev: %v\n", event)
		sp.sender.Event(event)

		durationSec := float64(span.End-span.Start) / float64(time.Second/time.Nanosecond)
		sp.totalThreadSecondsUsed += durationSec * float64(min(span.AvgThreadCount, uint64(sp.gpuMaxThreads))) // we can't use more threads than the GPU has
	}

	for _, span := range data.Allocations {
		event := event.Event{
			AlertType:      event.AlertTypeInfo,
			Priority:       event.PriorityLow,
			AggregationKey: "gpu-0",
			SourceTypeName: CheckName,
			EventType:      "gpu-memory",
			Title:          fmt.Sprintf("GPU mem alloc size %d", span.Size),
			Text:           fmt.Sprintf("Start at %d, end %d", span.Start, span.End),
			Ts:             sp.timeResolver.ResolveMonotonicTimestamp(span.Start).Unix(),
		}
		fmt.Printf("memev: %v\n", event)
		sp.sender.Event(event)
	}

	sp.pastAllocs = data.Allocations
}

func (sp *StatsProcessor) processCurrentData(data *model.StreamCurrentData) {
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

func (sp *StatsProcessor) finish() {
	intervalSecs := sp.measuredInterval.Seconds()
	if intervalSecs > 0 {
		availableThreadSeconds := float64(sp.gpuMaxThreads) * intervalSecs
		utilization := sp.totalThreadSecondsUsed / availableThreadSeconds
		fmt.Printf("GPU utilization: %f, totalUsed %f\n", utilization, sp.totalThreadSecondsUsed)

		sp.sender.Gauge("gjulian.cudapoc.utilization", utilization, "", sp.getTags())
	}

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

	fmt.Printf("GPU points: %+v\n", points)

	currentMemory := int64(0)
	maxMemory := int64(0)
	pointsPerSecond := make([]memAllocTsPoint, 0)
	for i := range points {
		tsEpochCurrent := sp.timeResolver.ResolveMonotonicTimestamp(points[i].ts).Unix()
		if i > 0 {
			tsEpochPrev := sp.timeResolver.ResolveMonotonicTimestamp(points[i-1].ts).Unix()
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

	fmt.Printf("GPU memory: %d, max %d, points: %+v\n", currentMemory, maxMemory, pointsPerSecond)

	totalMemBytes := 0
	lastCheckEpoch := sp.lastCheck.Unix()
	for _, point := range pointsPerSecond {
		totalMemBytes += int(point.size)
		if point.ts > uint64(lastCheckEpoch) {
			sp.sender.GaugeWithTimestamp("gjulian.cudapoc.memory", float64(totalMemBytes), "", sp.getTags(), float64(point.ts))
		}
	}

	sp.sender.Gauge("gjulian.cudapoc.max_memory", float64(maxMemory), "", sp.getTags())
}
