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

func TestShardTracker_SiblingsAndParents(t *testing.T) {
	tracker := newShardTracker()
	tracker.mark("parent-1", []string{"shard-a", "shard-b", "shard-c"})
	tracker.mark("parent-2", []string{"shard-d"})

	assert.ElementsMatch(t, []string{"shard-b", "shard-c"}, tracker.siblings("shard-a"))
	assert.Empty(t, tracker.siblings("shard-d"), "a lone shard has no siblings")
	assert.Nil(t, tracker.siblings("not-a-shard"))
	assert.Equal(t, map[string]string{"shard-a": "parent-1", "shard-d": "parent-2"}, tracker.parentsOf([]string{"shard-a", "shard-d", "not-a-shard"}))

	// Removing a parent's shards also forgets that they are related.
	tracker.pop("parent-1")
	assert.Nil(t, tracker.siblings("shard-b"))
	assert.Empty(t, tracker.parentsOf([]string{"shard-a"}))

	tracker.reset()
	assert.Empty(t, tracker.parentsOf([]string{"shard-d"}))
}

func TestShardTracker_NilTrackerHasNoShards(t *testing.T) {
	var tracker *shardTracker
	assert.Nil(t, tracker.siblings("shard-a"))
	assert.Nil(t, tracker.parentsOf([]string{"shard-a"}))
}
