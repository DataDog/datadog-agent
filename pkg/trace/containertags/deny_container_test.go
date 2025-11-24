// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package containertagsbuffer contains the logic to buffer payloads for container tags
// enrichment
package containertagsbuffer

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeniedContainers_BasicLifecycle(t *testing.T) {
	d := newDeniedContainers()
	now := time.Now()
	id := "container-123"
	require.False(t, d.shouldDeny(now, id))

	d.deny(now, id)
	require.True(t, d.shouldDeny(now, id))
}

func TestDeniedContainers_Refresh(t *testing.T) {
	d := newDeniedContainers()
	start := time.Now()
	id := "container-refresh"

	d.deny(start, id)

	tNoRefresh := start.Add(denyRefresh - time.Nanosecond)
	d.shouldDeny(tNoRefresh, id)
	assert.Equal(t, start, d.containers[id])

	tTriggerRefresh := start.Add(denyRefresh + time.Nanosecond)
	d.shouldDeny(tTriggerRefresh, id)
	assert.Equal(t, tTriggerRefresh, d.containers[id])
}

func TestDeniedContainers_Rotation(t *testing.T) {
	d := newDeniedContainers()
	now := time.Now()

	oldID := "old-container"
	oldTime := now.Add(-(denyEviction + time.Nanosecond))

	freshID := "fresh-container"
	d.containers[oldID] = oldTime
	d.containers[freshID] = now
	d.lastEviction = now.Add(-(denyEviction + time.Nanosecond))

	// trigger rotation with deny call
	d.deny(now, "trigger-id")

	_, hasOld := d.containers[oldID]
	assert.False(t, hasOld, "old container is evicted")

	_, hasFresh := d.containers[freshID]
	assert.True(t, hasFresh, "fresh container remains")

	_, hasTrigger := d.containers["trigger-id"]
	assert.True(t, hasTrigger, "trigger container should be present")

	assert.Equal(t, int64(1), d.expired.Load(), "expected 1 expired entity")
}

func TestDeniedContainers_Concurrency(t *testing.T) {
	d := newDeniedContainers()
	var wg sync.WaitGroup

	concurrentCount := 50
	wg.Add(concurrentCount)

	start := time.Now()

	for i := 0; i < concurrentCount; i++ {
		go func(i int) {
			defer wg.Done()
			now := start.Add(time.Second)
			for j := 0; j < 50; j++ {
				id := "shared-id"
				if i%2 == 0 {
					id = "unique-id"
				}
				d.deny(now, id)
				now = now.Add(time.Second)
				d.shouldDeny(now, id)
				now = now.Add(15 * time.Second)
				d.shouldDeny(now, id)
			}
		}(i)
	}

	wg.Wait()
	assert.NotEmpty(t, d.containers)
}
