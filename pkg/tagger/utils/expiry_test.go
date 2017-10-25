// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package utils

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestUpdate(t *testing.T) {
	expire := &Expire{
		expiryDuration: 5 * time.Minute,
		lastSeen:       make(map[string]time.Time),
	}
	testContainerID := "b2beae57bb2ada35e083c78029fe6a742848ff021d839107e2ede87a9ce7bf50"

	now := time.Now()
	twoMinutesAgo := now.Add(-2 * time.Minute)

	// Inserting container, we expect True as it's not in the lastseen map.
	found := expire.Update(testContainerID, twoMinutesAgo)
	require.True(t, found)
	require.Len(t, expire.lastSeen, 1)
	require.Equal(t, expire.lastSeen[testContainerID], twoMinutesAgo)

	// Updating container, we expect false as it already exists. We also expect the timestamp to be correct.
	found = expire.Update(testContainerID, now)
	require.False(t, found)
	require.Len(t, expire.lastSeen, 1)
	require.Equal(t, expire.lastSeen[testContainerID], now)

}

func TestExpireContainers(t *testing.T) {

	testContainerID := "b2beae57bb2ada35e083c78029fe6a742848ff021d839107e2ede87a9ce7bf50"
	testContainerID2 := "asf234ijrwada35e083c78029fesfs789s7w0sx99ffs107e2ede87a9ce7bf50"

	now := time.Now()
	twoMinutesAgo := now.Add(-2 * time.Minute)

	expire := &Expire{
		expiryDuration: 5 * time.Minute,
		lastSeen:       make(map[string]time.Time),
	}

	// We add the two containers to the lastseen list with different timestamps.
	expire.Update(testContainerID, now)
	expire.Update(testContainerID2, twoMinutesAgo)

	// First we check that given the passed timestamps (inferior to the expire threshold)
	// the list of expired containers is empty
	expirelist, err := expire.ExpireContainers()
	require.Nil(t, err)
	require.Len(t, expirelist, 0)

	// We update one container's timestamp, 4 minutes should NOT be enough to expire
	expire.lastSeen[testContainerID] = expire.lastSeen[testContainerID].Add(-4 * time.Minute)

	expirelist, err = expire.ExpireContainers()
	require.Nil(t, err)
	require.Len(t, expirelist, 0)

	// We update the other container's timestamp, 6 minutes should be enough to expire
	expire.lastSeen[testContainerID2] = expire.lastSeen[testContainerID2].Add(-6 * time.Minute)
	expirelist, err = expire.ExpireContainers()
	require.Nil(t, err)
	require.Len(t, expirelist, 1)
	require.Equal(t, testContainerID2, expirelist[0])
	require.Len(t, expire.lastSeen, 1)
}
