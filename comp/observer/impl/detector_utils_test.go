// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAnomalyRateLimiter_New(t *testing.T) {
	rl := NewAnomalyRateLimiter(5000)
	assert.Equal(t, int64(5000), rl.CooldownMs)
	assert.NotNil(t, rl.LastAnomalyTimesMs)
	assert.Empty(t, rl.LastAnomalyTimesMs)
}

// A brand-new key should always be allowed.
func TestAnomalyRateLimiter_FirstOccurrence(t *testing.T) {
	rl := NewAnomalyRateLimiter(5000)
	assert.True(t, rl.CanCreateAnomaly(1, 1000))
}

// A second call within the cooldown window must be rejected.
func TestAnomalyRateLimiter_BlockedDuringCooldown(t *testing.T) {
	rl := NewAnomalyRateLimiter(5000)
	rl.CanCreateAnomaly(1, 1000)
	assert.False(t, rl.CanCreateAnomaly(1, 1000+4999))
}

// A call exactly at the cooldown boundary (elapsed == cooldown) must pass.
func TestAnomalyRateLimiter_AllowedAtExactCooldownBoundary(t *testing.T) {
	rl := NewAnomalyRateLimiter(5000)
	rl.CanCreateAnomaly(1, 1000)
	assert.True(t, rl.CanCreateAnomaly(1, 1000+5000))
}

// A call one millisecond after the cooldown has elapsed must be allowed.
func TestAnomalyRateLimiter_AllowedAfterCooldown(t *testing.T) {
	rl := NewAnomalyRateLimiter(5000)
	rl.CanCreateAnomaly(1, 1000)
	assert.True(t, rl.CanCreateAnomaly(1, 1000+5001))
}

// After a successful call past cooldown, the timestamp resets and the next
// immediate call must be blocked again.
func TestAnomalyRateLimiter_TimestampUpdatedAfterCooldown(t *testing.T) {
	rl := NewAnomalyRateLimiter(5000)
	rl.CanCreateAnomaly(1, 0)
	rl.CanCreateAnomaly(1, 5001) // resets timestamp to 5001
	assert.False(t, rl.CanCreateAnomaly(1, 5001+4999))
	assert.True(t, rl.CanCreateAnomaly(1, 5001+5001))
}

// Different keys must be rate-limited independently.
func TestAnomalyRateLimiter_IndependentKeys(t *testing.T) {
	rl := NewAnomalyRateLimiter(5000)
	now := int64(1000)

	assert.True(t, rl.CanCreateAnomaly(1, now))
	assert.True(t, rl.CanCreateAnomaly(2, now))

	// key 1 is blocked, key 2 should also be blocked independently
	assert.False(t, rl.CanCreateAnomaly(1, now+100))
	assert.False(t, rl.CanCreateAnomaly(2, now+100))

	// advance only past cooldown for key 1 — key 2 behaves identically
	assert.True(t, rl.CanCreateAnomaly(1, now+5001))
	assert.True(t, rl.CanCreateAnomaly(2, now+5001))
}

// A zero cooldown should allow every call.
func TestAnomalyRateLimiter_ZeroCooldown(t *testing.T) {
	rl := NewAnomalyRateLimiter(0)
	assert.True(t, rl.CanCreateAnomaly(1, 1000))
	// elapsed (1) > cooldown (0), so allowed
	assert.True(t, rl.CanCreateAnomaly(1, 1001))
}

// Many distinct keys should all be tracked correctly without interference.
func TestAnomalyRateLimiter_ManyKeys(t *testing.T) {
	rl := NewAnomalyRateLimiter(5000)
	now := int64(0)

	const numKeys = 100
	for key := int64(0); key < numKeys; key++ {
		assert.True(t, rl.CanCreateAnomaly(key, now))
	}
	for key := int64(0); key < numKeys; key++ {
		assert.False(t, rl.CanCreateAnomaly(key, now+1))
	}
	for key := int64(0); key < numKeys; key++ {
		assert.True(t, rl.CanCreateAnomaly(key, now+5001))
	}
}
