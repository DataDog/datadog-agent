// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func newTestCache() *ttlCache {
	cache := newTTLCache(1800, 3600)
	return cache
}

var dims = &Dimensions{name: "test"}

type point struct {
	startTs          uint64
	ts               uint64
	val              float64
	expectFirstPoint bool
	expectDropPoint  bool
	dropPointMessage string
	expectDx         float64
}

func TestMonotonicDiffUnknownStart(t *testing.T) {
	t.Run("diff", func(t *testing.T) {
		points := []point{
			{startTs: 0, ts: 2, val: 5, expectFirstPoint: true, expectDropPoint: false, dropPointMessage: "first point"},
			{startTs: 0, ts: 2, val: 6, expectFirstPoint: false, expectDropPoint: true, dropPointMessage: "new ts == old ts"},
			{startTs: 0, ts: 1, val: 0, expectFirstPoint: false, expectDropPoint: true, dropPointMessage: "new ts < old ts"},
			{startTs: 0, ts: 3, val: 2, expectFirstPoint: true, expectDropPoint: false, dropPointMessage: "new < old => there has been a reset: first point"},
			{startTs: 0, ts: 5, val: 6, expectFirstPoint: false, expectDropPoint: false},
		}

		prevPts := newTestCache()
		var dx float64
		var firstPoint bool
		var dropPoint bool

		for _, point := range points {
			dx, firstPoint, dropPoint = prevPts.putAndGetDiff(dims, point.startTs, point.ts, point.val, true, false)
			assert.Equal(t, point.expectFirstPoint, firstPoint)
			assert.Equal(t, point.expectDropPoint, dropPoint, point.dropPointMessage)
		}
		assert.Equal(t, 4.0, dx, "expected diff 4.0")
	})

	t.Run("rate", func(t *testing.T) {
		points := []point{
			{startTs: 0, ts: 2, val: 5, expectFirstPoint: true, expectDropPoint: false, dropPointMessage: "first point"},
			{startTs: 0, ts: 2, val: 6, expectFirstPoint: false, expectDropPoint: true, dropPointMessage: "new ts == old ts"},
			{startTs: 0, ts: 1, val: 0, expectFirstPoint: false, expectDropPoint: true, dropPointMessage: "new ts < old ts"},
			{startTs: 0, ts: 3, val: 2, expectFirstPoint: true, expectDropPoint: true, dropPointMessage: "new < old => there has been a reset: first point + drop"},
			{startTs: 0, ts: 5, val: 6, expectFirstPoint: false, expectDropPoint: false},
		}

		startTs := uint64(0) // equivalent to start being unset
		prevPts := newTestCache()
		sec := uint64(time.Second)
		var dx float64
		var firstPoint bool
		var dropPoint bool

		for _, point := range points {
			dx, firstPoint, dropPoint = prevPts.putAndGetDiff(dims, startTs, point.ts*sec, point.val, true, true)
			assert.Equal(t, point.expectFirstPoint, firstPoint)
			assert.Equal(t, point.expectDropPoint, dropPoint, point.dropPointMessage)
		}
		assert.Equal(t, 2.0, dx, "expected rate (6-2)/(5s-3s)")
	})
}

func TestDiffUnknownStart(t *testing.T) {
	points := []point{
		{startTs: 0, ts: 1, val: 5, expectFirstPoint: true, expectDropPoint: false, dropPointMessage: "first point", expectDx: 5},
		{startTs: 0, ts: 1, val: 6, expectFirstPoint: false, expectDropPoint: true, dropPointMessage: "new ts == old ts"},
		{startTs: 0, ts: 0, val: 0, expectFirstPoint: false, expectDropPoint: true, dropPointMessage: "new ts < old ts"},
		{startTs: 0, ts: 2, val: 2, expectFirstPoint: false, expectDropPoint: false, dropPointMessage: "not monotonic", expectDx: -3},
		{startTs: 0, ts: 3, val: 4, expectFirstPoint: false, expectDropPoint: false, expectDx: 2},
	}
	prevPts := newTestCache()
	for _, point := range points {
		dx, firstPoint, dropPoint := prevPts.putAndGetDiff(dims, point.startTs, point.ts, point.val, false, false)
		assert.Equal(t, point.expectFirstPoint, firstPoint)
		assert.Equal(t, point.expectDropPoint, dropPoint, point.dropPointMessage)
		if !point.expectDropPoint {
			assert.Equal(t, point.expectDx, dx)
		}
	}
}

func TestMonotonicDiffKnownStart(t *testing.T) {
	pointsAfterReset := []point{
		{startTs: 5, ts: 5, val: 8, expectFirstPoint: true, expectDropPoint: true, dropPointMessage: "first point: startTs = ts, there has been a reset at an unknown time"},
		{startTs: 5, ts: 7, val: 12, expectFirstPoint: false, expectDropPoint: false, dropPointMessage: "same startTs, old >= new"},
	}
	pointsAfterSecondReset := []point{
		{startTs: 8, ts: 9, val: 1, expectFirstPoint: true, expectDropPoint: false, dropPointMessage: "first point"},
		{startTs: 8, ts: 12, val: 10, expectFirstPoint: false, expectDropPoint: false, dropPointMessage: "same startTs, old >= new"},
	}

	t.Run("diff", func(t *testing.T) {
		initialPoints := []point{
			{startTs: 1, ts: 2, val: 5, expectFirstPoint: true, expectDropPoint: false, dropPointMessage: "first point"},
			{startTs: 1, ts: 2, val: 6, expectFirstPoint: false, expectDropPoint: true, dropPointMessage: "new ts == old ts"},
			{startTs: 1, ts: 1, val: 0, expectFirstPoint: false, expectDropPoint: true, dropPointMessage: "new ts < old ts"},
			{startTs: 1, ts: 3, val: 2, expectFirstPoint: true, expectDropPoint: false, dropPointMessage: "new < old => there has been a reset: first point"},
			{startTs: 1, ts: 4, val: 6, expectFirstPoint: false, expectDropPoint: false},
		}

		prevPts := newTestCache()
		var dx float64
		var firstPoint bool
		var dropPoint bool

		for _, point := range initialPoints {
			dx, firstPoint, dropPoint = prevPts.putAndGetDiff(dims, point.startTs, point.ts, point.val, true, false)
			assert.Equal(t, point.expectFirstPoint, firstPoint)
			assert.Equal(t, point.expectDropPoint, dropPoint, point.dropPointMessage)
		}
		assert.Equal(t, 4.0, dx, "expected diff 4.0")

		// reset
		for _, point := range pointsAfterReset {
			dx, firstPoint, dropPoint = prevPts.putAndGetDiff(dims, point.startTs, point.ts, point.val, true, false)
			assert.Equal(t, point.expectFirstPoint, firstPoint)
			assert.Equal(t, point.expectDropPoint, dropPoint, point.dropPointMessage)
		}
		assert.Equal(t, 4.0, dx, "expected diff 4.0")

		// reset
		for _, point := range pointsAfterSecondReset {
			dx, firstPoint, dropPoint = prevPts.putAndGetDiff(dims, point.startTs, point.ts, point.val, true, false)
			assert.Equal(t, point.expectFirstPoint, firstPoint)
			assert.Equal(t, point.expectDropPoint, dropPoint, point.dropPointMessage)
		}
		assert.Equal(t, 9.0, dx, "expected diff 9.0")
	})

	t.Run("rate", func(t *testing.T) {
		initialPoints := []point{
			{startTs: 1, ts: 2, val: 5, expectFirstPoint: true, expectDropPoint: false, dropPointMessage: "first point"},
			{startTs: 1, ts: 2, val: 6, expectFirstPoint: false, expectDropPoint: true, dropPointMessage: "new ts == old ts"},
			{startTs: 1, ts: 1, val: 0, expectFirstPoint: false, expectDropPoint: true, dropPointMessage: "new ts < old ts"},
			{startTs: 1, ts: 3, val: 2, expectFirstPoint: true, expectDropPoint: true, dropPointMessage: "new < old => there has been a reset: first point + drop"},
			{startTs: 1, ts: 4, val: 6, expectFirstPoint: false, expectDropPoint: false},
		}

		prevPts := newTestCache()
		sec := uint64(time.Second)
		var dx float64
		var firstPoint bool
		var dropPoint bool

		for _, point := range initialPoints {
			dx, firstPoint, dropPoint = prevPts.putAndGetDiff(dims, point.startTs*sec, point.ts*sec, point.val, true, true)
			assert.Equal(t, point.expectFirstPoint, firstPoint)
			assert.Equal(t, point.expectDropPoint, dropPoint, point.dropPointMessage)
		}
		assert.Equal(t, 4.0, dx, "expected rate (6-2)/(3s-2s)")

		// reset
		for _, point := range pointsAfterReset {
			dx, firstPoint, dropPoint = prevPts.putAndGetDiff(dims, point.startTs*sec, point.ts*sec, point.val, true, true)
			assert.Equal(t, point.expectFirstPoint, firstPoint)
			assert.Equal(t, point.expectDropPoint, dropPoint, point.dropPointMessage)
		}
		assert.Equal(t, 2.0, dx, "expected rate (12-8)/(7s-5s)")

		// rest
		for _, point := range pointsAfterSecondReset {
			dx, firstPoint, dropPoint = prevPts.putAndGetDiff(dims, point.startTs*sec, point.ts*sec, point.val, true, true)
			assert.Equal(t, point.expectFirstPoint, firstPoint)
			assert.Equal(t, point.expectDropPoint, dropPoint, point.dropPointMessage)
		}
		assert.Equal(t, 3.0, dx, "expected rate (10-1)/(12s-9s)")
	})
}

func TestDiffKnownStart(t *testing.T) {
	points := []point{
		{startTs: 1, ts: 3, val: 5, expectFirstPoint: true, expectDropPoint: false, dropPointMessage: "first point", expectDx: 5},
		{startTs: 1, ts: 3, val: 6, expectFirstPoint: false, expectDropPoint: true, dropPointMessage: "new ts == old ts"},
		{startTs: 1, ts: 2, val: 0, expectFirstPoint: false, expectDropPoint: true, dropPointMessage: "new ts < old ts"},
		{startTs: 1, ts: 4, val: 2, expectFirstPoint: false, expectDropPoint: false, dropPointMessage: "not monotonic", expectDx: -3},
		{startTs: 1, ts: 5, val: 4, expectFirstPoint: false, expectDropPoint: false, expectDx: 2},
		{startTs: 5, ts: 6, val: 8, expectFirstPoint: true, expectDropPoint: false, dropPointMessage: "first point", expectDx: 8},
		{startTs: 5, ts: 7, val: 9, expectFirstPoint: false, expectDropPoint: false, expectDx: 1},
		{startTs: 7, ts: 8, val: 1, expectFirstPoint: true, expectDropPoint: false, dropPointMessage: "first point", expectDx: 1},
		{startTs: 7, ts: 9, val: 10, expectFirstPoint: false, expectDropPoint: false, expectDx: 9},
	}
	prevPts := newTestCache()
	for _, point := range points {
		dx, firstPoint, dropPoint := prevPts.putAndGetDiff(dims, point.startTs, point.ts, point.val, false, false)
		assert.Equal(t, point.expectFirstPoint, firstPoint)
		assert.Equal(t, point.expectDropPoint, dropPoint, point.dropPointMessage)
		if !point.expectDropPoint {
			assert.Equal(t, point.expectDx, dx)
		}
	}
}

func TestPutAndGetExtrema(t *testing.T) {
	points := []struct {
		min                  float64
		resetTimeseries      bool
		assumeFromLastWindow bool
		reason               string
	}{
		{
			min:                  -10,
			assumeFromLastWindow: false,
			reason:               "there are no points in cache",
		},
		{
			min:                  -10,
			assumeFromLastWindow: false,
			reason:               "value is the same as in previous point",
		},
		{
			min:                  -11,
			assumeFromLastWindow: true,
			reason:               "value changed from previous point",
		},
		{
			min:                  -11,
			assumeFromLastWindow: false,
			reason:               "value is the same as in previous point",
		},
		{
			min:                  -9,
			assumeFromLastWindow: true,
			reason:               "minimum is bigger than the stored one so there must have been a reset",
		},
		{
			min:                  -9,
			assumeFromLastWindow: false,
			reason:               "value is the same as in previous point",
		},
		{
			min:                  -20,
			resetTimeseries:      true,
			assumeFromLastWindow: false,
			reason:               "Timeseries was reset",
		},
	}

	startTs := uint64(1)
	prevPts := newTestCache()
	minDims := dims.WithSuffix("min")
	maxDims := dims.WithSuffix("max")
	for i, points := range points {
		ts := uint64(i + 1)
		if points.resetTimeseries {
			startTs = ts
		}

		{
			// Check assertion for the minimum
			assumeMinFromLastWindow := prevPts.PutAndCheckMin(minDims, startTs, ts, points.min)
			assert.Equal(t, points.assumeFromLastWindow, assumeMinFromLastWindow,
				"Point #%d failed for min; expected %v because %q", i, points.assumeFromLastWindow, points.reason,
			)
		}

		{
			// Now do the same for the maximum; use the opposite of min to reverse comparisons.
			max := -points.min
			assumeMaxFromLastWindow := prevPts.PutAndCheckMax(maxDims, startTs, ts, max)
			assert.Equal(t, points.assumeFromLastWindow, assumeMaxFromLastWindow,
				"Point #%d failed for max; expected %v because %q", i, points.assumeFromLastWindow, points.reason,
			)
		}

	}
}
