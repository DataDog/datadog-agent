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
