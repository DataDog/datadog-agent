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

// base is a fixed reference time aligned to a minute boundary so all test
// samples fall into predictable medium-tier buckets.
var base = time.Unix(3600, 0) // exactly 60 minutes from the Unix epoch

// TestAllStats_StaleFineSamplesExcluded verifies that fine-tier samples older
// than 30 minutes are ignored by allStats. The fine ring evicts by count rather
// than by time, so an idle component can retain ancient samples; the c30m break
// added to the fine-tier loop prevents those samples from inflating satFine or
// setting LastSaturatedAt.
func TestAllStats_StaleFineSamplesExcluded(t *testing.T) {
	h := newRollingHistory()
	stale := base.Add(-31 * time.Minute)

	for i := 0; i < 5; i++ {
		h.add(stale.Add(time.Duration(i)*time.Second), 1.0) // fully saturated but stale
	}

	ws := h.allStats(base)

	assert.Equal(t, time.Duration(0), ws.Saturated1m, "stale samples must not count in 1m window")
	assert.Equal(t, time.Duration(0), ws.Saturated30m, "stale samples must not count in 30m window")
	assert.False(t, ws.HasLastSaturated, "stale samples must not set LastSaturatedAt")
}

// TestAllStats_MediumPendingLastSaturatedAt verifies that a saturated sample
// that has been evicted from the fine ring into the in-progress mediumPending
// bucket still appears in LastSaturatedAt. Without the mediumPending check in
// allStats, LastSaturatedAt could jump backward when the fine ring rolls over.
func TestAllStats_MediumPendingLastSaturatedAt(t *testing.T) {
	h := newRollingHistory()

	// Add one saturated sample at base, then fill the fine ring with
	// 300 non-saturated samples in the same minute. The 301st add evicts
	// the first (saturated) sample into mediumPending.
	h.add(base, 1.0) // saturated — will be evicted to mediumPending
	for i := 1; i <= 300; i++ {
		h.add(base.Add(time.Duration(i)*time.Millisecond), 0.0)
	}

	// Fine ring now holds only the 300 non-saturated samples.
	// mediumPending holds the saturated sample at base.
	ws := h.allStats(base.Add(500 * time.Millisecond))

	require.True(t, ws.HasLastSaturated, "evicted saturated sample in mediumPending must set LastSaturatedAt")
	assert.Equal(t, base.UnixNano(), ws.LastSaturatedAt.UnixNano(),
		"LastSaturatedAt must point to the sample in mediumPending, not be absent")
}

// TestAllStats_MediumPendingSaturated30m verifies that the saturated-seconds
// count stored in mediumPending is included in Saturated30m even though the
// sample is no longer in the fine ring.
func TestAllStats_MediumPendingSaturated30m(t *testing.T) {
	h := newRollingHistory()

	h.add(base, 1.0) // saturated — will be evicted
	for i := 1; i <= 300; i++ {
		h.add(base.Add(time.Duration(i)*time.Millisecond), 0.0)
	}

	ws := h.allStats(base.Add(500 * time.Millisecond))

	// Fine ring contributes 0 saturated seconds; mediumPending contributes 1.
	assert.Equal(t, time.Duration(1)*time.Second, ws.Saturated30m,
		"mediumPending saturated count must be included in Saturated30m")
}

// TestAllStats_CurrentlySaturatedFresh verifies that a recent saturated sample sets
// CurrentlySaturated, and that re-reading allStats against a later clock (no new samples,
// as happens when the component goes idle) clears it once the newest sample ages past
// currentSaturationWindow. This is the core of the stale-EWMA regression.
func TestAllStats_CurrentlySaturatedFresh(t *testing.T) {
	h := newRollingHistory()
	h.add(base, 0.95) // saturated, newest sample

	// Read immediately: newest sample is fresh and saturated.
	assert.True(t, h.allStats(base).CurrentlySaturated,
		"a fresh saturated sample must report CurrentlySaturated")

	// Read just inside the freshness window: still saturated.
	assert.True(t, h.allStats(base.Add(currentSaturationWindow)).CurrentlySaturated,
		"sample at exactly currentSaturationWindow must still count as fresh")

	// Read past the freshness window with no new samples (idle component): must decay.
	assert.False(t, h.allStats(base.Add(currentSaturationWindow+time.Second)).CurrentlySaturated,
		"a stale saturated sample must not report CurrentlySaturated")
}

// TestAllStats_CurrentlySaturatedBelowThreshold verifies a fresh but sub-threshold newest
// sample does not report CurrentlySaturated even if older samples were saturated.
func TestAllStats_CurrentlySaturatedBelowThreshold(t *testing.T) {
	h := newRollingHistory()
	h.add(base, 0.95)                  // saturated
	h.add(base.Add(time.Second), 0.10) // newest: well below threshold

	assert.False(t, h.allStats(base.Add(time.Second)).CurrentlySaturated,
		"a fresh sub-threshold newest sample must clear CurrentlySaturated")
}

// TestAllStats_IdleFineSamplesIn30mAverages verifies that fine samples retained past the
// 5m window (because the ring evicts by count, not time) still feed the 30m avg/max — not
// just Saturated30m. A short saturated burst followed by >5m of idle must not report
// Saturated30m > 0 while showing 30m avg/max as 0, which would hide the actual peak.
func TestAllStats_IdleFineSamplesIn30mAverages(t *testing.T) {
	h := newRollingHistory()

	// 10 saturated samples in a short burst, then the component goes idle (no more adds).
	for i := 0; i < 10; i++ {
		h.add(base.Add(time.Duration(i)*time.Second), 1.0)
	}

	// Read 6 minutes later: every sample is now older than 5m but within 30m.
	ws := h.allStats(base.Add(6 * time.Minute))

	assert.Equal(t, time.Duration(10)*time.Second, ws.Saturated30m, "burst must count as 30m saturation")
	assert.InDelta(t, 1.0, ws.Max30m, 0.0001, "30m max must reflect the saturated burst, not 0")
	assert.InDelta(t, 1.0, ws.Avg30m, 0.0001, "30m avg must reflect the saturated burst, not 0")
	assert.Equal(t, 0.0, ws.Max5m, "nothing occurred in the last 5m")
	assert.Equal(t, 0.0, ws.Avg5m, "nothing occurred in the last 5m")
}
