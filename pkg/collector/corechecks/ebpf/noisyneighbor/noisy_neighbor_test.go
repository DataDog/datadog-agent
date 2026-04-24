// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package noisyneighbor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWatchlistChanged(t *testing.T) {
	check := &NoisyNeighborCheck{}

	// nil → empty: no change
	assert.False(t, check.watchlistChanged(nil), "nil → nil should not be a change")

	// nil → non-empty: change
	assert.True(t, check.watchlistChanged([]uint64{1, 2, 3}), "nil → {1,2,3} should be a change")

	// same set: no change
	assert.False(t, check.watchlistChanged([]uint64{1, 2, 3}), "{1,2,3} → {1,2,3} should not be a change")

	// same set, different order: no change
	assert.False(t, check.watchlistChanged([]uint64{3, 1, 2}), "{1,2,3} → {3,1,2} should not be a change")

	// subset: change
	assert.True(t, check.watchlistChanged([]uint64{1, 2}), "{1,2,3} → {1,2} should be a change")

	// superset: change
	assert.True(t, check.watchlistChanged([]uint64{1, 2, 4}), "{1,2} → {1,2,4} should be a change")

	// disjoint: change
	assert.True(t, check.watchlistChanged([]uint64{5, 6}), "{1,2,4} → {5,6} should be a change")

	// non-empty → empty: change
	assert.True(t, check.watchlistChanged(nil), "{5,6} → nil should be a change")

	// empty → empty: no change
	assert.False(t, check.watchlistChanged(nil), "nil → nil should not be a change")
}

func TestParseConfigDefaults(t *testing.T) {
	c := &NoisyNeighborConfig{}
	err := c.Parse([]byte("{}"))
	assert.NoError(t, err)
	assert.Equal(t, defaultPSIThreshold, c.PSIThreshold)
	assert.Equal(t, defaultThrottleRatio, c.ThrottleRatio)
	assert.Equal(t, defaultStealThreshold, c.StealThreshold)
	assert.Equal(t, defaultMaxWatchlistSize, c.MaxWatchlistSize)
	assert.Equal(t, defaultMaxTopNPreemptors, c.MaxTopNPreemptors)
	assert.Equal(t, defaultMaxNonContainerCgroups, c.MaxNonContainerCgroups)
	assert.Equal(t, uint64(defaultMinForeignPreemptionsImpact), c.MinForeignPreemptionsImpact)
}

func TestParseConfigClampsWatchlistSize(t *testing.T) {
	c := &NoisyNeighborConfig{}
	err := c.Parse([]byte("max_watchlist_size: 500"))
	assert.NoError(t, err)
	assert.Equal(t, hardMaxWatchlistSize, c.MaxWatchlistSize)
}
