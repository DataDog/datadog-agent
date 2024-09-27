// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

	// Now we build the time series by summing the values at each point, accounting for the unit factor.
	// Multiple points can end up at the same rounded timestamp.
	tseries := make([]tsPoint, 0)
	for i := range b.points {
		// Check if we need to add a new point
		currentRoundedTime := b.points[i].time
		if i > 0 {
			prevRoundedTime := tseries[len(tseries)-1].time
			prevValue := tseries[len(tseries)-1].value

			// We advanced past the last timeseries point, so create a new one
			if currentRoundedTime != prevRoundedTime {
				tseries = append(tseries, tsPoint{time: currentRoundedTime, value: 0})

				// Update the maximum too
				maxValue = max(maxValue, prevValue)
			}
		} else if i == 0 {
			// Always add the first point
			tseries = append(tseries, tsPoint{time: uint64(currentRoundedTime), value: 0})
		}

		// Update the current value
		tseries[len(tseries)-1].value += b.points[i].value
	}

	return tseries, maxValue
}
