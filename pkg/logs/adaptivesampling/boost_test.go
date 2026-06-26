// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package adaptivesampling

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSamplingBoostStoreExactPatternBeatsContainerWide(t *testing.T) {
	store := NewSamplingBoostStore()
	now := time.Unix(100, 0)
	store.Set(SamplingBoost{
		ContainerID:     "container-a",
		ExpiresAt:       now.Add(time.Minute),
		RateMultiplier:  2,
		BurstMultiplier: 2,
	})
	exact := store.Set(SamplingBoost{
		ContainerID:     "container-a",
		PatternHash:     "abc",
		ExpiresAt:       now.Add(time.Minute),
		RateMultiplier:  10,
		BurstMultiplier: 10,
	})

	got, ok := store.Lookup("container-a", "abc", now)
	require.True(t, ok)
	assert.Equal(t, exact.ID, got.ID)
	assert.Equal(t, 10.0, got.RateMultiplier)
}

func TestSamplingBoostStoreFallsBackToContainerWide(t *testing.T) {
	store := NewSamplingBoostStore()
	now := time.Unix(100, 0)
	boost := store.Set(SamplingBoost{
		ContainerID:    "container-a",
		ExpiresAt:      now.Add(time.Minute),
		RateMultiplier: 3,
	})

	got, ok := store.Lookup("container-a", "different", now)
	require.True(t, ok)
	assert.Equal(t, boost.ID, got.ID)
	assert.Equal(t, 1.0, got.BurstMultiplier)
}

func TestSamplingBoostStoreExpiresBoosts(t *testing.T) {
	store := NewSamplingBoostStore()
	now := time.Unix(100, 0)
	store.Set(SamplingBoost{
		ContainerID: "container-a",
		ExpiresAt:   now,
	})

	_, ok := store.Lookup("container-a", "", now)
	assert.False(t, ok)
}
