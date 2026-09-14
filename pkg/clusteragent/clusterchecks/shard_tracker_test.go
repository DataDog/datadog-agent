// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShardTracker(t *testing.T) {
	tracker := newShardTracker()

	assert.False(t, tracker.isTracked("digest-1"))

	digests, exists := tracker.pop("digest-1")
	assert.False(t, exists)
	assert.Nil(t, digests)

	tracker.mark("digest-1", []string{"shard-1", "shard-2"})
	assert.True(t, tracker.isTracked("digest-1"))
	assert.False(t, tracker.isTracked("digest-2"), "a different config digest must not be tracked")

	popped, exists := tracker.pop("digest-1")
	assert.True(t, exists)
	assert.Equal(t, []string{"shard-1", "shard-2"}, popped)

	// pop clears tracking.
	assert.False(t, tracker.isTracked("digest-1"))
	_, exists = tracker.pop("digest-1")
	assert.False(t, exists)
}

func TestShardTracker_Reset(t *testing.T) {
	tracker := newShardTracker()
	tracker.mark("digest-1", []string{"shard-1"})
	tracker.mark("digest-2", []string{"shard-2"})

	tracker.reset()

	assert.False(t, tracker.isTracked("digest-1"))
	assert.False(t, tracker.isTracked("digest-2"))
}
