// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"slices"
)

// tseriesBuilder is a helper to get the last value and the max value of a timeseries, without actually having
// to store the entire timeseries.
type tseriesBuilder struct {
	points []tsPoint
}

type tsPoint struct {
	time  uint64
	value int64
}

func (b *tseriesBuilder) AddEvent(startTime, endTime uint64, value int64) {
	b.points = append(b.points, tsPoint{time: startTime, value: value})
	b.points = append(b.points, tsPoint{time: endTime, value: -value})
}

func (b *tseriesBuilder) AddEventStart(startTime uint64, value int64) {
	b.points = append(b.points, tsPoint{time: startTime, value: value})
}

// GetLastAndMax returns the last value of the timeseries and the max seen in the interval
func (b *tseriesBuilder) GetLastAndMax() (int64, int64) {
	// sort by timestamp. Stable so that events that start and end at the same time are processed in the order they were added
	slices.SortStableFunc(b.points, func(p1, p2 tsPoint) int {
		return int(p1.time - p2.time)
	})

	currentValue, maxValue := int64(0), int64(0)
	for _, point := range b.points {
		currentValue += point.value
		maxValue = max(maxValue, currentValue)
	}

	return currentValue, maxValue
}
