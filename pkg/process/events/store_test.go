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

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/events/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pushSync is an auxiliary function to push events to the RingStore synchronously
func pushSync(t *testing.T, r *RingStore, e *model.ProcessEvent) {
	done := make(chan bool)
	r.Push(e, done)
	ok := <-done
	require.True(t, ok)
}

func TestRingStore(t *testing.T) {
	now := time.Now()
	ctx := context.Background()
	timeout := time.Second

	cfg := config.Mock()
	cfg.Set("process_config.event_collection.store.max_items", 3)
	store, err := NewRingStore(&statsd.NoOpClient{})
	require.NoError(t, err)

	s, ok := store.(*RingStore)
	require.True(t, ok)

	s.Run()
	defer s.Stop()

	e1 := model.NewMockedProcessEvent(model.Exec, now, 23, "/usr/bin/curl", []string{"curl", "localhost:6062"})
	e2 := model.NewMockedProcessEvent(model.Exit, now, 23, "/usr/bin/curl", []string{"curl", "localhost:6062"})
	e3 := model.NewMockedProcessEvent(model.Exit, now, 23, "/usr/bin/ls", []string{"ls", "-lah"})

	// Push and pull 2 elements - no data loss
	pushSync(t, s, e1)
	require.Equal(t, 0, s.head)
	require.Equal(t, 1, s.tail)
	require.Equal(t, 1, s.size())

	pushSync(t, s, e2)
	require.Equal(t, 0, s.head)
	require.Equal(t, 2, s.tail)
	require.Equal(t, 2, s.size())

	data, err := s.Pull(ctx, timeout)
	assert.NoError(t, err)
	require.Equal(t, []*model.ProcessEvent{e1, e2}, data)
	require.Equal(t, 2, s.head)
	require.Equal(t, 2, s.tail)
	require.Equal(t, 0, s.size())

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
	data, err = s.Pull(ctx, timeout)
	assert.NoError(t, err)
	require.Equal(t, []*model.ProcessEvent{e1, e2, e3}, data)
	require.Equal(t, 2, s.head)
	require.Equal(t, 2, s.tail)
	require.Equal(t, 0, s.size())

	// Add one more element to reset head and tail
	pushSync(t, s, e1)
	require.Equal(t, 2, s.head)
	require.Equal(t, 0, s.tail)
	require.Equal(t, 1, s.size())

	data, err = s.Pull(ctx, timeout)
	assert.NoError(t, err)
	require.Equal(t, []*model.ProcessEvent{e1}, data)
	require.Equal(t, 0, s.head)
	require.Equal(t, 0, s.tail)
	require.Equal(t, 0, s.size())
}

func TestRingStoreWithDroppedData(t *testing.T) {
	now := time.Now()
	ctx := context.Background()
	timeout := time.Second

	cfg := config.Mock()
	cfg.Set("process_config.event_collection.store.max_items", 3)
	store, err := NewRingStore(&statsd.NoOpClient{})
	require.NoError(t, err)

	s, ok := store.(*RingStore)
	require.True(t, ok)

	s.Run()
	defer s.Stop()

	e1 := model.NewMockedProcessEvent(model.Exec, now, 23, "/usr/bin/curl", []string{"curl", "localhost:6062"})
	e2 := model.NewMockedProcessEvent(model.Exit, now, 23, "/usr/bin/curl", []string{"curl", "localhost:6062"})
	e3 := model.NewMockedProcessEvent(model.Exit, now, 23, "/usr/bin/ls", []string{"ls", "-lah"})
	e4 := model.NewMockedProcessEvent(model.Exit, now, 23, "/usr/bin/ls", []string{"ls", "-lah"})

	// Fill up buffer
	pushSync(t, s, e1)
	pushSync(t, s, e2)
	pushSync(t, s, e3)
	require.Equal(t, 0, s.head)
	require.Equal(t, 0, s.tail)
	require.Equal(t, 3, s.size())

	// Pushing new elements should drop old data
	s.dropHandler = func(e *model.ProcessEvent) {
		model.AssertProcessEvents(t, e1, e)
	}
	pushSync(t, s, e4) // drop e1
	require.Equal(t, 1, s.head)
	require.Equal(t, 1, s.tail)
	require.Equal(t, 3, s.size())

	s.dropHandler = func(e *model.ProcessEvent) {
		model.AssertProcessEvents(t, e2, e)
	}
	pushSync(t, s, e1) // drop e2
	require.Equal(t, 2, s.head)
	require.Equal(t, 2, s.tail)
	require.Equal(t, 3, s.size())

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

	cfg := config.Mock()
	cfg.Set("process_config.event_collection.store.max_items", 3)
	store, err := NewRingStore(&statsd.NoOpClient{})
	require.NoError(t, err)

	s, ok := store.(*RingStore)
	require.True(t, ok)

	s.Run()
	defer s.Stop()

	e1 := model.NewMockedProcessEvent(model.Exec, now, 23, "/usr/bin/curl", []string{"curl", "localhost:6062"})
	e2 := model.NewMockedProcessEvent(model.Exit, now, 23, "/usr/bin/curl", []string{"curl", "localhost:6062"})
	s.Push(e1, nil)
	s.Push(e2, nil)

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
