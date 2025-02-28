// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package blacklist

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl/common"
)

var config = Config{
	Capacity:      10,
	TTL:           10 * time.Minute,
	CleanInterval: 5 * time.Minute,
}

var mockTime = time.Now()

func mockTimeNow() time.Time {
	return mockTime
}

var hashA = common.PathtestHash(123)
var hashB = common.PathtestHash(456)

func TestContains(t *testing.T) {
	cache := NewCache(config, mockTimeNow)

	require.False(t, cache.Contains(hashA))
	ok := cache.Add(hashA)
	require.True(t, ok)
	require.True(t, cache.Contains(hashA))
}

func TestExpire(t *testing.T) {
	cache := NewCache(config, mockTimeNow)

	ok := cache.Add(hashA)
	require.True(t, ok)

	// revert out time mocking later
	t.Cleanup(func() { mockTime = time.Now() })

	mockTime = mockTime.Add(config.TTL / 2)
	cache.cleanupExpired()

	// we spent half the TTL, so it should still be there
	require.True(t, cache.Contains(hashA))

	mockTime = mockTime.Add(config.TTL)
	cache.cleanupExpired()

	// now it's 1.5x the TTL, so it should be gone
	require.False(t, cache.Contains(hashA))
}

func TestFull(t *testing.T) {
	// make a copy to modify config
	smallConfig := config
	smallConfig.Capacity = 1
	cache := NewCache(smallConfig, mockTimeNow)

	ok := cache.Add(hashA)
	require.True(t, ok)
	require.True(t, cache.Contains(hashA))

	// now it's full, so this one should fail to add
	ok = cache.Add(hashB)
	require.False(t, ok)
	require.False(t, cache.Contains(hashB))
}
