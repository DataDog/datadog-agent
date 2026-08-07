// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package noisyneighbor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWatchlistRotationIsFair(t *testing.T) {
	live := make([]uint64, 500)
	for i := range live {
		live[i] = uint64(i + 1)
	}
	rotator := watchlistRotator{}
	seen := make(map[uint64]struct{})
	for range 8 {
		selected := rotator.selectNext(live, 64)
		require.LessOrEqual(t, len(selected), 64)
		for _, id := range selected {
			seen[id] = struct{}{}
		}
	}
	require.Len(t, seen, 500)
}

func TestWatchlistRotationHandlesChurnAndDuplicates(t *testing.T) {
	rotator := watchlistRotator{}
	require.Equal(t, []uint64{1, 2}, rotator.selectNext([]uint64{3, 2, 1, 2}, 2))

	selected := rotator.selectNext([]uint64{2, 3, 4, 4}, 2)
	require.Equal(t, []uint64{3, 4}, selected)
	require.NotContains(t, rotator.lastSampled, uint64(1))
}
