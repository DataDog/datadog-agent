// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package utils

import (

    "time"
    "testing"
    "github.com/stretchr/testify/require"
)


func TestUpdate(t *testing.T) {
    expire := &Expire{
        expiryDuration: 5 * time.Minute,
        lastSeen: make(map[string]time.Time),
    }
    testContainerID := "b2beae57bb2ada35e083c78029fe6a742848ff021d839107e2ede87a9ce7bf50"

    now := time.Now()
    twoMinutesAgo := now.Add(-2 * time.Minute)

    // Inserting container
    found := expire.Update(testContainerID, twoMinutesAgo)
    require.True(t, found)
    require.Len(t, expire.lastSeen, 1)
    require.Equal(t, expire.lastSeen[testContainerID], twoMinutesAgo)

    // Updating container
    found = expire.Update(testContainerID, now)
    require.False(t, found)
    require.Len(t, expire.lastSeen, 1)
    require.Equal(t, expire.lastSeen[testContainerID], now)


}

func TestExpireContainers(t *testing.T) {

    testContainerID := "b2beae57bb2ada35e083c78029fe6a742848ff021d839107e2ede87a9ce7bf50"
    testContainerID2 := "asf234ijrwada35e083c78029fe6a742848ff021d839107e2ede87a9ce7bf50"

    now := time.Now()
    twoMinutesAgo := now.Add(-2 * time.Minute)

    expire := &Expire{
        expiryDuration: 5 * time.Minute,
        lastSeen: make(map[string]time.Time),
    }

    expire.Update(testContainerID, now)
    expire.Update(testContainerID2, twoMinutesAgo)

    expirelist, err := expire.ExpireContainers()
    require.Nil(t, err)
    require.Len(t, expirelist, 0)


    // 4 minutes should NOT be enough to expire
    expire.lastSeen[testContainerID] = expire.lastSeen[testContainerID].Add(-4 * time.Minute)

    expirelist, err = expire.ExpireContainers()
    require.Nil(t, err)
    require.Len(t, expirelist, 0)

    // 6 minutes should be enough to expire
    expire.lastSeen[testContainerID2] = expire.lastSeen[testContainerID2].Add(-6 * time.Minute)
    expirelist, err = expire.ExpireContainers()
    require.Nil(t, err)
    require.Len(t, expirelist, 1)
    require.Equal(t, testContainerID2, expirelist[0])
    require.Len(t, expire.lastSeen, 1)
}
