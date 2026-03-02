// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testEvent string

const (
	eventA testEvent = "eventA"
	eventB testEvent = "eventB"
)

func TestNewNotifier(t *testing.T) {
	n := NewNotifier[testEvent, string]()
	require.NotNil(t, n)
	require.NotNil(t, n.listeners)
}

func TestNotifier_RegisterListener(t *testing.T) {
	n := NewNotifier[testEvent, string]()

	var called bool
	listener := func(obj string) {
		called = true
	}

	err := n.RegisterListener(eventA, listener)
	require.NoError(t, err)

	// Trigger the event
	n.NotifyListeners(eventA, "test")
	assert.True(t, called)
}

func TestNotifier_RegisterListener_Uninitialized(t *testing.T) {
	n := &Notifier[testEvent, string]{}

	listener := func(obj string) {}

	err := n.RegisterListener(eventA, listener)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "before initialization")
}

func TestNotifier_MultipleListeners(t *testing.T) {
	n := NewNotifier[testEvent, int]()

	var sum int32

	listener1 := func(obj int) {
		atomic.AddInt32(&sum, int32(obj))
	}
	listener2 := func(obj int) {
		atomic.AddInt32(&sum, int32(obj*2))
	}

	err := n.RegisterListener(eventA, listener1)
	require.NoError(t, err)
	err = n.RegisterListener(eventA, listener2)
	require.NoError(t, err)

	n.NotifyListeners(eventA, 5)

	// listener1 adds 5, listener2 adds 10
	assert.Equal(t, int32(15), atomic.LoadInt32(&sum))
}

func TestNotifier_DifferentEvents(t *testing.T) {
	n := NewNotifier[testEvent, string]()

	var eventAReceived, eventBReceived string

	listenerA := func(obj string) {
		eventAReceived = obj
	}
	listenerB := func(obj string) {
		eventBReceived = obj
	}

	err := n.RegisterListener(eventA, listenerA)
	require.NoError(t, err)
	err = n.RegisterListener(eventB, listenerB)
	require.NoError(t, err)

	n.NotifyListeners(eventA, "hello")
	assert.Equal(t, "hello", eventAReceived)
	assert.Empty(t, eventBReceived)

	n.NotifyListeners(eventB, "world")
	assert.Equal(t, "hello", eventAReceived)
	assert.Equal(t, "world", eventBReceived)
}

func TestNotifier_NoListeners(t *testing.T) {
	n := NewNotifier[testEvent, string]()

	// Should not panic when no listeners are registered
	n.NotifyListeners(eventA, "test")
}

func TestNotifier_ConcurrentAccess(t *testing.T) {
	n := NewNotifier[testEvent, int]()

	var count int32
	listener := func(obj int) {
		atomic.AddInt32(&count, 1)
	}

	err := n.RegisterListener(eventA, listener)
	require.NoError(t, err)

	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent notifications
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n.NotifyListeners(eventA, 1)
		}()
	}

	wg.Wait()
	assert.Equal(t, int32(numGoroutines), atomic.LoadInt32(&count))
}

func TestNotifier_ConcurrentRegisterAndNotify(t *testing.T) {
	n := NewNotifier[testEvent, int]()

	var wg sync.WaitGroup

	// Register listeners concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			listener := func(obj int) {}
			_ = n.RegisterListener(eventA, listener)
		}()
	}

	// Notify concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n.NotifyListeners(eventA, 1)
		}()
	}

	wg.Wait()
	// Test passes if no race conditions or deadlocks occur
}
