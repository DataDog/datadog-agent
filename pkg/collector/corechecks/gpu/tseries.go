// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package gpu

import (
	"slices"
)

// tseriesBuilder is a helper to build a time series of events with duration
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

// buildTseries builds the time series, returning a slice of points and the max value in the interval
func (b *tseriesBuilder) Build() ([]tsPoint, int64) {
	// sort by timestamp. Stable so that events that start and end at the same time are processed in the order they were added
	slices.SortStableFunc(b.points, func(p1, p2 tsPoint) int {
		return int(p1.time - p2.time)
	})

	maxValue := int64(0)
	currentValue := int64(0)

	// Now we build the time series by doing a cumulative sum of the values at each point, accounting for the unit factor.
	// Multiple points can end up at the same rounded timestamp.
	tseries := make([]tsPoint, 0)
	for i := range b.points {
		// Check if we need to add a new point
		currTime := b.points[i].time
		if i > 0 {
			prevTime := tseries[len(tseries)-1].time

			// We advanced past the last timeseries point, so create a new one
			if currTime != prevTime {
				tseries = append(tseries, tsPoint{time: currTime, value: 0})
			}
		} else if i == 0 {
			// Always add the first point
			tseries = append(tseries, tsPoint{time: uint64(currTime), value: 0})
		}

		// Update the current value for this point
		currentValue += b.points[i].value

		// assign it to the current point and update the maximum
		tseries[len(tseries)-1].value = currentValue
		maxValue = max(maxValue, currentValue)
	}

	return tseries, maxValue
}
