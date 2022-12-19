// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCounterWithOriginCache(t *testing.T) {
	counter := NewCounter("counter_with_origin", "test", []string{"message_type", "state", "origin"}, "test metric")
	cache := NewMetricCounterWithOriginCache(counter)

	for i := 0; i < 250; i += 1 {
		cache.Get(fmt.Sprintf("hello-%d", i))
	}

	// cache is limiting the amount of entries

	require := require.New(t)
	require.Len(cache.cachedOrder, maxOriginCounters)
	require.Len(cache.cachedCountersWithOrigin, maxOriginCounters)

	// order in the cache is correct

	for i := 0; i < 50; i += 1 {
		require.NotContains(cache.cachedCountersWithOrigin, fmt.Sprintf("hello-%d", i))
	}
	for i := 50; i < 250; i += 1 {
		require.Contains(cache.cachedCountersWithOrigin, fmt.Sprintf("hello-%d", i))
	}

	// always the same entry is returned

	okCnt1, errCnt1 := cache.Get("hello-5")
	okCnt2, errCnt2 := cache.Get("hello-5")
	require.Equal(okCnt1, okCnt2)
	require.Equal(errCnt1, errCnt2)

	// but all cache entries are not equal

	okCnt3, errCnt3 := cache.Get("hello-10")
	require.NotEqual(okCnt1, okCnt3)
	require.NotEqual(errCnt1, errCnt3)

	// reset the cache

	cache = NewMetricCounterWithOriginCache(counter)

	cache.Get("container_id://abcdef")
	require.Len(cache.cachedOrder, 1)
	require.Equal(cache.cachedOrder[0].origin, "container_id://abcdef")
	require.Equal(cache.cachedOrder[0].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "container_id://abcdef"})
	require.Equal(cache.cachedOrder[0].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "container_id://abcdef"})

	cache.Get("container_id://abcdef")
	require.Len(cache.cachedOrder, 1)
	require.Equal(cache.cachedOrder[0].origin, "container_id://abcdef")
	require.Equal(cache.cachedOrder[0].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "container_id://abcdef"})
	require.Equal(cache.cachedOrder[0].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "container_id://abcdef"})

	cache.Get("container_id://xyz")
	require.Len(cache.cachedOrder, 2)
	require.Equal(cache.cachedOrder[0].origin, "container_id://abcdef")
	require.Equal(cache.cachedOrder[0].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "container_id://abcdef"})
	require.Equal(cache.cachedOrder[0].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "container_id://abcdef"})
	require.Equal(cache.cachedOrder[1].origin, "container_id://xyz")
	require.Equal(cache.cachedOrder[1].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "container_id://xyz"})
	require.Equal(cache.cachedOrder[1].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "container_id://xyz"})
}

// This test is proving that no data race occurred on the cache map.
// It should not fail since `cachedCountersWithOrigin` and `cachedOrder` should be
// properly protected from multiple accesses by the mutex.
// The main purpose of this test is to detect early if a future code change is
// introducing a data race.
func TestNoRaceCounterWithOriginCache(t *testing.T) {
	const N = 100
	counter := NewCounter("counter_with_origin", "another_test", []string{"message_type", "state", "origin"}, "test metric")
	cache := NewMetricCounterWithOriginCache(counter)
	sync := make(chan struct{})
	done := make(chan struct{}, N)
	for i := 0; i < N; i++ {
		id := fmt.Sprintf("%d", i)
		go func() {
			defer func() { done <- struct{}{} }()
			<-sync
			cache.Get(id)
		}()
	}
	close(sync)
	for i := 0; i < N; i++ {
		<-done
	}
}
