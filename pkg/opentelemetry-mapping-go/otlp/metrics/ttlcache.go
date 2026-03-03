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
	"time"

	gocache "github.com/patrickmn/go-cache"
)

type ttlCache struct {
	cache keyHashCache
}

// numberCounter keeps the value of a number
// monotonic counter at a given point in time
type numberCounter struct {
	ts      uint64
	startTs uint64
	value   float64
}

func newTTLCache(sweepInterval int64, deltaTTL int64) *ttlCache {
	cache := gocache.New(time.Duration(deltaTTL)*time.Second, time.Duration(sweepInterval)*time.Second)
	return &ttlCache{cache: newKeyHashCache(cache)}
}

// Diff submits a new value for a given non-monotonic metric and returns the difference with the
// last submitted value (ordered by timestamp). The diff value is only valid if `ok` is true.
func (t *ttlCache) Diff(dimensions *Dimensions, startTs, ts uint64, val float64) (float64, bool) {
	return t.putAndGetDiff(dimensions, startTs, ts, val)
}

// MonotonicDiff submits a new value for a given monotonic metric and returns the difference with the
// last submitted value (ordered by timestamp). It also returns whether this is a first point and
// whether it should be dropped.
func (t *ttlCache) MonotonicDiff(dimensions *Dimensions, startTs, ts uint64, val float64) (float64, bool, bool) {
	return t.putAndGetMonotonic(dimensions, startTs, ts, val, false)
}

// MonotonicRate submits a new value for a given monotonic metric and returns the rate per second since
// the last submitted value (ordered by timestamp). It also returns whether this is a first point and
// whether it should be dropped.
func (t *ttlCache) MonotonicRate(dimensions *Dimensions, startTs, ts uint64, val float64) (float64, bool, bool) {
	return t.putAndGetMonotonic(dimensions, startTs, ts, val, true)
}

// isNotFirstPoint determines if this is NOT the first point on a cumulative series:
// https://github.com/open-telemetry/opentelemetry-specification/blob/v1.19.0/specification/metrics/data-model.md#resets-and-gaps
func isNotFirstPoint(startTs, ts, oldStartTs uint64) (isNotFirst bool) {
	if startTs == 0 {
		// We don't know the start time, assume the sequence has not been restarted.
		isNotFirst = true
	} else if startTs != ts && startTs == oldStartTs {
		// Since startTs != 0 we know the start time, thus we apply the following rules from the spec:
		//  - "When StartTimeUnixNano equals TimeUnixNano, a new unbroken sequence of observations begins with a reset at an unknown start time."
		//  - "[for cumulative series] the StartTimeUnixNano of each point matches the StartTimeUnixNano of the initial observation."
		isNotFirst = true
	}

	return
}

// putAndGetMonotonic submits a new value for a given metric and returns:
// - the difference with the last submitted value (ordered by timestamp)
// - whether the submitted point is a first point (either first point ever, or first point after reset)
// - whether the point needs to be dropped.
func (t *ttlCache) putAndGetMonotonic(
	dimensions *Dimensions,
	startTs, ts uint64,
	val float64,
	rate bool,
) (dx float64, firstPoint bool, dropPoint bool) {
	// assume it's first point before cache check.
	firstPoint = true

	key := t.cache.computeKey(dimensions.String())
	if c, found := t.cache.get(key); found {
		cnt := c.(numberCounter)
		if cnt.ts >= ts {
			// We were given a point with a timestamp older or equal to the one in the cache. This point
			// should be dropped. We keep the current point in cache.
			return 0, false, true
		}
		dx = val - cnt.value
		if rate {
			dx = dx / time.Duration(ts-cnt.ts).Seconds()
		}
		// If !isNotFirstPoint or dx < 0, there has been a reset. We cache the new value, and firstPoint is true.
		firstPoint = !isNotFirstPoint(startTs, ts, cnt.startTs) || dx < 0
	}

	t.cache.set(
		key,
		numberCounter{
			startTs: startTs,
			ts:      ts,
			value:   val,
		},
		gocache.DefaultExpiration,
	)
	return
}

// putAndGetDiff submits a new value for a given metric and returns the difference with the
// last submitted value (ordered by timestamp). The diff value is only valid if `ok` is true.
func (t *ttlCache) putAndGetDiff(
	dimensions *Dimensions,
	startTs, ts uint64,
	val float64,
) (dx float64, ok bool) {
	key := t.cache.computeKey(dimensions.String())
	if c, found := t.cache.get(key); found {
		cnt := c.(numberCounter)
		if cnt.ts > ts {
			// We were given a point older than the one in memory so we drop it
			// We keep the existing point in memory since it is the most recent
			return 0, false
		}
		dx = val - cnt.value
		ok = isNotFirstPoint(startTs, ts, cnt.startTs)
	}

	t.cache.set(
		key,
		numberCounter{
			startTs: startTs,
			ts:      ts,
			value:   val,
		},
		gocache.DefaultExpiration,
	)
	return
}

type extrema struct {
	ts            uint64
	startTs       uint64
	storedExtrema float64
}

// putAndCheckExtrema stores a new extrema for a cumulative timeseries and checks if the
// extrema is the one from the last time window. The min flag indicates whether it is a minimum extrema (true) or a maximum extrema (false).
func (t *ttlCache) putAndCheckExtrema(
	dimensions *Dimensions,
	startTs, ts uint64,
	curExtrema float64,
	min bool,
) (assumeFromLastWindow bool) {
	key := t.cache.computeKey(dimensions.String())
	if c, found := t.cache.get(key); found {
		cnt := c.(extrema)
		if cnt.ts > ts {
			// We were given a point older than the one in memory so we drop it
			// We keep the existing point in memory since it is the most recent
			// Don't use the extrema, we don't have enough information.
			return false
		}

		isNotFirst := isNotFirstPoint(startTs, ts, cnt.startTs)
		if min {
			// We assume the minimum comes from the last time window if either of the following is true:
			// - the point is NOT the first in the timeseries AND is lower than the previous one
			// - the global minimum is bigger than the stored minimum (and therefore a reset must have happened)
			assumeFromLastWindow = (isNotFirst && curExtrema < cnt.storedExtrema) || (curExtrema > cnt.storedExtrema)
		} else { // not min, therefore max
			// symmetric to the min
			assumeFromLastWindow = (isNotFirst && curExtrema > cnt.storedExtrema) || (curExtrema < cnt.storedExtrema)
		}

	}

	t.cache.set(key,
		extrema{
			startTs:       startTs,
			ts:            ts,
			storedExtrema: curExtrema,
		},
		gocache.DefaultExpiration,
	)

	return
}

// PutAndCheckMin stores a minimum and checks whether the minimum is from the last time window.
func (t *ttlCache) PutAndCheckMin(dimensions *Dimensions, startTs, ts uint64, curMin float64) (isMinFromLastTimeWindow bool) {
	return t.putAndCheckExtrema(dimensions, startTs, ts, curMin, true)
}

// PutAndCheckMax stores a maximum and checks whether the maximum is from the last time window.
func (t *ttlCache) PutAndCheckMax(dimensions *Dimensions, startTs, ts uint64, curMax float64) (isMaxFromLastTimeWindow bool) {
	return t.putAndCheckExtrema(dimensions, startTs, ts, curMax, false)
}
