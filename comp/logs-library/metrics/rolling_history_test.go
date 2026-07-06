// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// base is a minute-aligned reference time so samples fall into predictable medium-tier buckets.
var base = time.Unix(3600, 0)

// TestAllStats_StaleFineSamplesExcluded checks that fine samples older than 30m are ignored.
func TestAllStats_StaleFineSamplesExcluded(t *testing.T) {
	h := newRollingHistory()
	stale := base.Add(-31 * time.Minute)

	for i := 0; i < 5; i++ {
		h.add(stale.Add(time.Duration(i)*time.Second), 1.0)
	}

	ws := h.allStats(base)

	assert.Equal(t, time.Duration(0), ws.Saturated1m, "stale samples must not count in 1m window")
	assert.Equal(t, time.Duration(0), ws.Saturated30m, "stale samples must not count in 30m window")
	assert.False(t, ws.HasLastSaturated, "stale samples must not set LastSaturatedAt")
}

// TestAllStats_MediumPendingLastSaturatedAt checks a sample evicted into mediumPending still sets LastSaturatedAt.
func TestAllStats_MediumPendingLastSaturatedAt(t *testing.T) {
	h := newRollingHistory()

	// The 301st add evicts the saturated sample at base into mediumPending.
	h.add(base, 1.0)
	for i := 1; i <= 300; i++ {
		h.add(base.Add(time.Duration(i)*time.Millisecond), 0.0)
	}

	ws := h.allStats(base.Add(500 * time.Millisecond))

	require.True(t, ws.HasLastSaturated, "evicted saturated sample in mediumPending must set LastSaturatedAt")
	assert.Equal(t, base.UnixNano(), ws.LastSaturatedAt.UnixNano(),
		"LastSaturatedAt must point to the sample in mediumPending, not be absent")
}

// TestAllStats_MediumPendingSaturated30m checks that mediumPending's saturated count feeds Saturated30m.
func TestAllStats_MediumPendingSaturated30m(t *testing.T) {
	h := newRollingHistory()

	h.add(base, 1.0)
	for i := 1; i <= 300; i++ {
		h.add(base.Add(time.Duration(i)*time.Millisecond), 0.0)
	}

	ws := h.allStats(base.Add(500 * time.Millisecond))

	assert.Equal(t, time.Duration(1)*time.Second, ws.Saturated30m,
		"mediumPending saturated count must be included in Saturated30m")
}

// TestAllStats_CurrentlySaturatedFresh checks CurrentlySaturated decays once the newest sample ages past the window.
func TestAllStats_CurrentlySaturatedFresh(t *testing.T) {
	h := newRollingHistory()
	h.add(base, 0.95)

	assert.True(t, h.allStats(base).CurrentlySaturated,
		"a fresh saturated sample must report CurrentlySaturated")
	assert.True(t, h.allStats(base.Add(currentSaturationWindow)).CurrentlySaturated,
		"sample at exactly currentSaturationWindow must still count as fresh")
	assert.False(t, h.allStats(base.Add(currentSaturationWindow+time.Second)).CurrentlySaturated,
		"a stale saturated sample must not report CurrentlySaturated")
}

// TestAllStats_CurrentlySaturatedStickyWindow checks a single dip doesn't clear CurrentlySaturated until the window empties.
func TestAllStats_CurrentlySaturatedStickyWindow(t *testing.T) {
	h := newRollingHistory()
	h.add(base, 0.95)
	h.add(base.Add(time.Second), 0.10)

	assert.True(t, h.allStats(base.Add(time.Second)).CurrentlySaturated,
		"a single dip must not clear CurrentlySaturated while a saturated sample is still in-window")
	assert.False(t, h.allStats(base.Add(currentSaturationWindow+time.Second)).CurrentlySaturated,
		"CurrentlySaturated must clear once the last saturated sample ages out of the window")
}

// TestAllStats_CurrentlySaturatedNoFlapNearThreshold is the flip-flop regression: an EWMA oscillating around the threshold stays saturated.
func TestAllStats_CurrentlySaturatedNoFlapNearThreshold(t *testing.T) {
	h := newRollingHistory()

	for i := 0; i < 30; i++ {
		v := 0.88
		if i%2 == 0 {
			v = 0.92
		}
		now := base.Add(time.Duration(i) * time.Second)
		h.add(now, v)
		assert.True(t, h.allStats(now).CurrentlySaturated,
			"CurrentlySaturated must not flap while saturated samples keep landing within the window (i=%d)", i)
	}
}

// TestAllStats_IdleFineSamplesIn30mAverages checks fine samples retained past 5m still feed the 30m avg/max.
func TestAllStats_IdleFineSamplesIn30mAverages(t *testing.T) {
	h := newRollingHistory()

	for i := 0; i < 10; i++ {
		h.add(base.Add(time.Duration(i)*time.Second), 1.0)
	}

	// 6 minutes later every sample is older than 5m but within 30m.
	ws := h.allStats(base.Add(6 * time.Minute))

	assert.Equal(t, time.Duration(10)*time.Second, ws.Saturated30m, "burst must count as 30m saturation")
	assert.InDelta(t, 1.0, ws.Max30m, 0.0001, "30m max must reflect the saturated burst, not 0")
	assert.InDelta(t, 1.0, ws.Avg30m, 0.0001, "30m avg must reflect the saturated burst, not 0")
	assert.Equal(t, 0.0, ws.Max5m, "nothing occurred in the last 5m")
	assert.Equal(t, 0.0, ws.Avg5m, "nothing occurred in the last 5m")
}

// TestAllStats_MediumBucketStraddlesC30m checks a medium bucket straddling the 30m cutoff is still counted.
func TestAllStats_MediumBucketStraddlesC30m(t *testing.T) {
	h := newRollingHistory()

	// One fully-saturated 1-minute bucket covering base..base+60s.
	h.medium[0] = aggregateBucket{
		tsNano:            base.UnixNano(),
		ewmaSum:           60.0, // 60 samples at 1.0
		ewmaMax:           1.0,
		count:             60,
		saturatedCount:    60,
		lastSaturatedNano: base.Add(59 * time.Second).UnixNano(),
	}
	h.mediumHead = 1
	h.mediumSize = 1

	// Read so that c30m (= now-30m = base+30s) falls inside the bucket: start < c30m < end.
	ws := h.allStats(base.Add(30*time.Minute + 30*time.Second))

	assert.InDelta(t, 1.0, ws.Max30m, 0.0001, "straddling medium bucket must contribute to Max30m")
	assert.InDelta(t, 1.0, ws.Avg30m, 0.0001, "straddling medium bucket must contribute to Avg30m")
	assert.Equal(t, time.Duration(60)*time.Second, ws.Saturated30m,
		"straddling saturated bucket must count toward Saturated30m, not be dropped")
}
