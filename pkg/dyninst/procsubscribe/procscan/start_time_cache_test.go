// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package procscan

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStartTimeCacheEvictsWhenFull(t *testing.T) {
	cache := makeStartTimeCache(2)

	cache.insert(1, ticks(10))
	cache.insert(2, ticks(20))

	// Cache is full, so inserting PID 3 is discarded.
	cache.insert(3, ticks(30))
	require.Len(t, cache.entries, 2)
	_, ok := cache.entries[3]
	require.False(t, ok)

	v1, ok := cache.getStartTime(1)
	require.True(t, ok)
	require.Equal(t, ticks(10), v1)

	v2, ok := cache.getStartTime(2)
	require.True(t, ok)
	require.Equal(t, ticks(20), v2)

	// When full, insert is a no-op even for existing PIDs.
	cache.insert(1, ticks(99))
	v1, ok = cache.getStartTime(1)
	require.True(t, ok)
	require.Equal(t, ticks(10), v1)

	v3, ok := cache.getStartTime(3)
	require.False(t, ok)
	require.Zero(t, v3)
}

func TestStartTimeCacheSweep(t *testing.T) {
	cache := makeStartTimeCache(10)

	// Insert PID 1 and clear its seen bit via sweep.
	_, ok := cache.getStartTime(1)
	require.False(t, ok)
	cache.insert(1, ticks(1))
	_, ok = cache.getStartTime(1)
	require.True(t, ok)
	cache.sweep()
	_, ok = cache.entries[1]
	require.True(t, ok)

	// PID 2 is inserted without being seen in a scan, so sweep deletes it.
	cache.entries[2] = ticks(2)
	cache.sweep()
	_, ok = cache.entries[2]
	require.False(t, ok)
}
