// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const dummySource = "dummy"

func TestUpdate(t *testing.T) {
	// Testing the creation of the Expire object.
	expiryDuration := 5 * time.Minute
	expire, err := newExpire(dummySource, expiryDuration)
	require.Nil(t, err)

	testContainerID := "ContainerID1"

	now := time.Now()
	twoMinutesAgo := now.Add(-2 * time.Minute)

	// Checking we initialyze the map with nothing.
	require.Len(t, expire.lastSeen, 0)

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

func TestComputeExpires(t *testing.T) {
	testEntityID := "foo://bar"
	testEntityID2 := "foo://quux"

	tests := []struct {
		name       string
		lastExpire time.Duration
		entities   map[string]time.Duration
		expected   []*TagInfo
	}{
		{
			name:       "don't expire fresh entities",
			lastExpire: 15 * time.Minute,
			entities: map[string]time.Duration{
				testEntityID: time.Minute,
			},
			expected: nil,
		},
		{
			name:       "expire stale entities, keep fresh ones",
			lastExpire: 15 * time.Minute,
			entities: map[string]time.Duration{
				testEntityID:  time.Minute,
				testEntityID2: 6 * time.Minute,
			},
			expected: []*TagInfo{
				{
					Source:       dummySource,
					Entity:       testEntityID2,
					DeleteEntity: true,
				},
			},
		},
		{
			name:       "expire nothing if lastExpire is too recent",
			lastExpire: 1 * time.Minute,
			entities: map[string]time.Duration{
				testEntityID:  time.Minute,
				testEntityID2: 6 * time.Minute,
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Now()
			expire, _ := newExpire(dummySource, 5*time.Minute)
			expire.lastExpire = now.Add(-1 * tt.lastExpire)

			for id, age := range tt.entities {
				expire.Update(id, now.Add(-1*age))
			}

			expired := expire.ComputeExpires()
			assert.Equal(t, tt.expected, expired)
		})
	}
}
