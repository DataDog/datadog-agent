// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package events

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/events/model"
)

// pushSync is an auxiliary function to push events to the RingStore synchronously
func pushSync(t *testing.T, r *RingStore, e *model.ProcessEvent) {
	done := make(chan bool)
	err := r.Push(e, done)
	require.NoError(t, err)
	ok := <-done
	require.True(t, ok)
}

func TestRingStoreWithoutLoop(t *testing.T) {
	now := time.Now()
	ctx := context.Background()
	timeout := time.Second

	cfg := config.Mock(t)
	cfg.Set("process_config.event_collection.store.max_items", 4)
	store, err := NewRingStore(cfg, &statsd.NoOpClient{})
	require.NoError(t, err)

	s, ok := store.(*RingStore)
	require.True(t, ok)

	s.Run()
	defer s.Stop()

	e1 := model.NewMockedExecEvent(now, 23, "/usr/bin/curl", []string{"curl", "localhost:6062"})
	e2 := model.NewMockedExitEvent(now, 23, "/usr/bin/curl", []string{"curl", "localhost:6062"}, 0)
	e3 := model.NewMockedExecEvent(now, 23, "/usr/bin/ls", []string{"ls", "-lah"})

	// Push and pull 1 event
	pushSync(t, s, e1)
	require.Equal(t, 0, s.head)
	require.Equal(t, 1, s.tail)
	require.Equal(t, 1, s.size())

	data, err := s.Pull(ctx, timeout)
	assert.NoError(t, err)
	require.Equal(t, []*model.ProcessEvent{e1}, data)
	require.Equal(t, 1, s.head)
	require.Equal(t, 1, s.tail)
	require.Equal(t, 0, s.size())

	// Push and pull 2 more events
	pushSync(t, s, e2)
	require.Equal(t, 1, s.head)
	require.Equal(t, 2, s.tail)
	require.Equal(t, 1, s.size())

	pushSync(t, s, e3)
	require.Equal(t, 1, s.head)
	require.Equal(t, 3, s.tail)
	require.Equal(t, 2, s.size())

	data, err = s.Pull(ctx, timeout)
	assert.NoError(t, err)
	require.Equal(t, []*model.ProcessEvent{e2, e3}, data)
	require.Equal(t, 3, s.head)
	require.Equal(t, 3, s.tail)
	require.Equal(t, 0, s.size())
}

func TestRingStoreWithLoop(t *testing.T) {
	now := time.Now()
	ctx := context.Background()
	timeout := time.Second

	cfg := config.Mock(t)
	cfg.Set("process_config.event_collection.store.max_items", 3)
	store, err := NewRingStore(cfg, &statsd.NoOpClient{})
	require.NoError(t, err)

	s, ok := store.(*RingStore)
	require.True(t, ok)

	e1 := model.NewMockedExecEvent(now, 23, "/usr/bin/curl", []string{"curl", "localhost:6062"})
	e2 := model.NewMockedExitEvent(now, 23, "/usr/bin/curl", []string{"curl", "localhost:6062"}, 0)
	e3 := model.NewMockedExecEvent(now, 23, "/usr/bin/ls", []string{"ls", "-lah"})

	// Initialize store with len(buffer)-1 events that have already been consumed
	s.head = 2
	s.tail = 2
	require.Equal(t, 0, s.size())

	s.Run()
	defer s.Stop()

	// Push 1 elements to reach end of buffer - no data loss
	pushSync(t, s, e1)
	require.Equal(t, 2, s.head)
	require.Equal(t, 0, s.tail)
	require.Equal(t, 1, s.size())

	// Push 2 more elements - buffer full - no data loss
	pushSync(t, s, e2)
	require.Equal(t, 2, s.head)
	require.Equal(t, 1, s.tail)
	require.Equal(t, 2, s.size())

	pushSync(t, s, e3)
	require.Equal(t, 2, s.head)
	require.Equal(t, 2, s.tail)
	require.Equal(t, 3, s.size())

	// Retrieve all events from buffer
	data, err := s.Pull(ctx, timeout)
	assert.NoError(t, err)
	require.Equal(t, []*model.ProcessEvent{e1, e2, e3}, data)
	require.Equal(t, 2, s.head)
	require.Equal(t, 2, s.tail)
	require.Equal(t, 0, s.size())
}

func TestRingStoreWithDroppedData(t *testing.T) {
	now := time.Now()
	ctx := context.Background()
	timeout := time.Second
	expectedDrops := make([]*model.ProcessEvent, 0)
	droppedEvents := make([]*model.ProcessEvent, 0)

	cfg := config.Mock(t)
	cfg.Set("process_config.event_collection.store.max_items", 3)
	store, err := NewRingStore(cfg, &statsd.NoOpClient{})
	require.NoError(t, err)

	s, ok := store.(*RingStore)
	require.True(t, ok)
	s.dropHandler = func(e *model.ProcessEvent) {
		droppedEvents = append(droppedEvents, e)
	}

	s.Run()
	defer s.Stop()

	e1 := model.NewMockedExecEvent(now, 23, "/usr/bin/curl", []string{"curl", "localhost:6062"})
	e2 := model.NewMockedExitEvent(now, 23, "/usr/bin/curl", []string{"curl", "localhost:6062"}, 0)
	e3 := model.NewMockedExecEvent(now, 23, "/usr/bin/ls", []string{"ls", "-lah"})
	e4 := model.NewMockedExitEvent(now, 23, "/usr/bin/ls", []string{"ls", "-lah"}, 0)

	// Fill up buffer
	pushSync(t, s, e1)
	pushSync(t, s, e2)
	pushSync(t, s, e3)
	require.Equal(t, 0, s.head)
	require.Equal(t, 0, s.tail)
	require.Equal(t, 3, s.size())

	// Pushing new elements should drop old data
	expectedDrops = append(expectedDrops, e1)
	pushSync(t, s, e4)
	require.Equal(t, 1, s.head)
	require.Equal(t, 1, s.tail)
	require.Equal(t, 3, s.size())

	expectedDrops = append(expectedDrops, e2)
	pushSync(t, s, e1)
	require.Equal(t, 2, s.head)
	require.Equal(t, 2, s.tail)
	require.Equal(t, 3, s.size())

	// Assert that the expected events have been dropped
	require.Equal(t, len(expectedDrops), len(droppedEvents))
	for i := range droppedEvents {
		AssertProcessEvents(t, expectedDrops[i], droppedEvents[i])
	}

	data, err := s.Pull(ctx, timeout)
	assert.NoError(t, err)
	require.Equal(t, []*model.ProcessEvent{e3, e4, e1}, data)
	require.Equal(t, 2, s.head)
	require.Equal(t, 2, s.tail)
	require.Equal(t, 0, s.size())
}

func TestRingStoreAsynchronousPush(t *testing.T) {
	now := time.Now()
	ctx := context.Background()
	timeout := time.Second

	cfg := config.Mock(t)
	cfg.Set("process_config.event_collection.store.max_items", 3)
	store, err := NewRingStore(cfg, &statsd.NoOpClient{})
	require.NoError(t, err)

	s, ok := store.(*RingStore)
	require.True(t, ok)

	s.Run()
	defer s.Stop()

	e1 := model.NewMockedExecEvent(now, 23, "/usr/bin/curl", []string{"curl", "localhost:6062"})
	e2 := model.NewMockedExitEvent(now, 23, "/usr/bin/curl", []string{"curl", "localhost:6062"}, 0)
	err = s.Push(e1, nil)
	require.NoError(t, err)

	err = s.Push(e2, nil)
	require.NoError(t, err)

	res := make([]*model.ProcessEvent, 0, 2)
	count := 0
	assert.Eventually(t, func() bool {
		events, err := s.Pull(ctx, timeout)
		assert.NoError(t, err)
		for _, e := range events {
			count++
			res = append(res, e)
		}

		return count == 2
	}, time.Second, 100*time.Millisecond, "can't pull and events pushed to the store")

	assert.Equal(t, []*model.ProcessEvent{e1, e2}, res)
}

func TestRingStorePullErrors(t *testing.T) {
	ctx := context.Background()
	timeout := 10 * time.Millisecond // simulate timeout for all pending requests

	cfg := config.Mock(t)
	maxPulls := 2
	cfg.Set("process_config.event_collection.store.max_pending_pulls", maxPulls)
	store, err := NewRingStore(cfg, &statsd.NoOpClient{})
	require.NoError(t, err)

	s, ok := store.(*RingStore)
	require.True(t, ok)

	// Do not start the store in order to queue pull requests
	for i := 0; i < maxPulls; i++ {
		go func() {
			data, err := s.Pull(ctx, timeout)
			assert.EqualError(t, err, "pull request timed out")
			assert.Equal(t, 0, len(data))
		}()
	}

	// Wait for pull requests to pile up
	assert.Eventually(t, func() bool { return len(s.pullReq) == maxPulls }, time.Second, 100*time.Millisecond)

	// Since pending requests are never served, next request will be rejected
	data, err := store.Pull(ctx, timeout)
	assert.EqualError(t, err, "too many pending pull requests")
	assert.Equal(t, 0, len(data))
}

func TestRingStorePushErrors(t *testing.T) {
	cfg := config.Mock(t)
	maxPushes := 2
	cfg.Set("process_config.event_collection.store.max_pending_pushes", 2)
	store, err := NewRingStore(cfg, &statsd.NoOpClient{})
	require.NoError(t, err)

	s, ok := store.(*RingStore)
	require.True(t, ok)

	e := model.NewMockedExecEvent(time.Now(), 23, "/usr/bin/curl", []string{"curl", "localhost:6062"})

	// Do not start the store in order to queue push requests
	for i := 0; i < maxPushes; i++ {
		go func() {
			err := store.Push(e, nil)
			assert.NoError(t, err)
		}()
	}

	// Wait for push requests to pile up
	assert.Eventually(t, func() bool { return len(s.pushReq) == maxPushes }, time.Second, 100*time.Millisecond)

	// Next push request should return an error
	err = store.Push(e, nil)
	assert.EqualError(t, err, "too many pending push requests")
}
