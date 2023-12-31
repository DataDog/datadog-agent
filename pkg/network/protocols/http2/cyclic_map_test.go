// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http2

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	capacity   = 10
	iterations = 35
)

func TestCyclicMap(t *testing.T) {
	require.Greater(t, iterations, capacity)

	c := NewCyclicMap[int, int](capacity, nil)
	// Add elements until the list is full
	for i := 0; i < capacity; i++ {
		c.Add(i, i)
	}
	// We should have the first 10 elements in the list
	for i := 0; i < capacity; i++ {
		require.Equal(t, i, c.list[i])
	}
	// Now add more elements, and exceed the capacity. The first elements should be evicted
	// and replaced by the new ones.
	for i := capacity; i < iterations; i++ {
		c.Add(i, i)
	}

	require.Equal(t, capacity, c.Len())
	// We should have the last 10 elements in the list
	for i := iterations - capacity; i < iterations; i++ {
		require.Equal(t, i, c.values[i])
	}
}

func TestCyclicMapEviction(t *testing.T) {
	require.Greater(t, iterations, capacity)

	evictedList := make([]int, 0)
	onEvict := func(k int, v int) {
		// Ensuring we're getting the correct value and key.
		require.Equal(t, k, v)
		evictedList = append(evictedList, k)
	}

	c := NewCyclicMap[int, int](capacity, onEvict)
	// Add elements until the list is full
	for i := 0; i < iterations; i++ {
		c.Add(i, i)
	}
	require.Equal(t, capacity, c.Len())

	// We should have the last 10 elements in the list
	for i := iterations - capacity; i < iterations; i++ {
		require.Equal(t, i, c.values[i])
	}

	for i := 0; i < iterations-capacity; i++ {
		require.Equal(t, i, evictedList[i])
	}
}

func TestCyclicMapCheckLength(t *testing.T) {
	require.Greater(t, iterations, capacity)

	c := NewCyclicMap[int, int](capacity, nil)
	// Add elements until the list is full
	for i := 0; i < iterations; i++ {
		c.Add(i, i)
		if i < capacity {
			require.Equal(t, i+1, c.Len())
		} else {
			require.Equal(t, capacity, c.Len())
		}
	}
	require.Equal(t, capacity, c.Len())
}

func TestCyclicMapRemoveOldest(t *testing.T) {
	require.Greater(t, iterations, capacity)

	c := NewCyclicMap[int, int](capacity, nil)
	// Add elements until the list is full
	for i := 0; i < capacity; i++ {
		c.Add(i, i)
	}

	for i := 0; i < capacity; i++ {
		c.RemoveOldest()
		require.Equal(t, capacity-1-i, c.Len())
		_, ok := c.values[i]
		require.False(t, ok)
	}
}
