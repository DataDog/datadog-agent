// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sources

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

func TestAddSource(t *testing.T) {
	sources := NewLogSources()
	assert.Equal(t, 0, len(sources.GetSources()))

	sources.AddSource(NewLogSource("foo", &config.LogsConfig{Type: "boo"}))
	assert.Equal(t, 1, len(sources.GetSources()))

	sources.AddSource(NewLogSource("bar", &config.LogsConfig{Type: "boo"}))
	assert.Equal(t, 2, len(sources.GetSources()))

	sources.AddSource(NewLogSource("baz", &config.LogsConfig{})) // invalid config
	assert.Equal(t, 3, len(sources.GetSources()))
}

func TestRemoveSource(t *testing.T) {
	sources := NewLogSources()
	source1 := NewLogSource("foo", &config.LogsConfig{Type: "boo"})
	source2 := NewLogSource("bar", &config.LogsConfig{Type: "boo"})

	sources.AddSource(source1)
	sources.AddSource(source2)
	assert.Equal(t, 2, len(sources.GetSources()))

	sources.RemoveSource(source1)
	assert.Equal(t, 1, len(sources.GetSources()))
	assert.Equal(t, source2, sources.GetSources()[0])

	sources.RemoveSource(source2)
	assert.Equal(t, 0, len(sources.GetSources()))
}

func TestGetSources(t *testing.T) {
	sources := NewLogSources()
	assert.Equal(t, 0, len(sources.GetSources()))

	sources.AddSource(NewLogSource("", &config.LogsConfig{Type: "boo"}))
	assert.Equal(t, 1, len(sources.GetSources()))
}

func TestGetAddedForType(t *testing.T) {
	sources := NewLogSources()
	source := NewLogSource("foo", &config.LogsConfig{Type: "foo"})

	stream1 := sources.GetAddedForType("foo", make(chan struct{}))
	assert.NotNil(t, stream1)

	stream2 := sources.GetAddedForType("foo", make(chan struct{}))
	assert.NotNil(t, stream2)

	go func() { sources.AddSource(source) }()
	s1 := <-stream1
	s2 := <-stream2
	assert.Equal(t, s1, source)
	assert.Equal(t, s2, source)
}

func TestGetAddedForTypeExistingSources(t *testing.T) {
	sources := NewLogSources()
	source1 := NewLogSource("one", &config.LogsConfig{Type: "foo"})
	source1bar := NewLogSource("one-bar", &config.LogsConfig{Type: "bar"})
	source2 := NewLogSource("two", &config.LogsConfig{Type: "foo"})
	source2bar := NewLogSource("two-bar", &config.LogsConfig{Type: "bar"})
	source3 := NewLogSource("three", &config.LogsConfig{Type: "foo"})

	go func() {
		sources.AddSource(source1bar)
		sources.AddSource(source1)
	}()

	streamA := sources.GetAddedForType("foo", make(chan struct{}))
	assert.NotNil(t, streamA)
	sa1 := <-streamA
	assert.Equal(t, sa1, source1)

	go func() {
		sources.AddSource(source2bar)
		sources.AddSource(source2)
	}()
	sa2 := <-streamA
	assert.Equal(t, sa2, source2)

	streamB := sources.GetAddedForType("foo", make(chan struct{}))
	assert.NotNil(t, streamB)
	sb1 := <-streamB
	sb2 := <-streamB
	assert.ElementsMatch(t, []*LogSource{source1, source2}, []*LogSource{sb1, sb2})

	go func() { sources.AddSource(source3) }()
	sa3 := <-streamA
	sb3 := <-streamB
	assert.Equal(t, sa3, source3)
	assert.Equal(t, sb3, source3)
}

func TestSubscribeAll(t *testing.T) {
	sources := NewLogSources()
	source1 := NewLogSource("one", &config.LogsConfig{Type: "foo"})
	source2 := NewLogSource("two-bar", &config.LogsConfig{Type: "bar"})
	source3 := NewLogSource("three", &config.LogsConfig{Type: "foo"})

	go func() { sources.AddSource(source1) }()

	addA, removeA := sources.SubscribeAll(make(chan struct{}), make(chan struct{}))
	assert.NotNil(t, addA)
	sa1 := <-addA
	assert.Equal(t, sa1, source1)
	assert.Equal(t, 0, len(removeA))

	go func() { sources.AddSource(source2) }()

	sa2 := <-addA
	assert.Equal(t, sa2, source2)
	assert.Equal(t, 0, len(removeA))

	addB, removeB := sources.SubscribeAll(make(chan struct{}), make(chan struct{}))
	assert.NotNil(t, addB)
	sb1 := <-addB
	sb2 := <-addB
	assert.ElementsMatch(t, []*LogSource{source1, source2}, []*LogSource{sb1, sb2})
	assert.Equal(t, 0, len(removeB))

	go func() { sources.AddSource(source3) }()

	sa3 := <-addA
	sb3 := <-addB
	assert.Equal(t, sa3, source3)
	assert.Equal(t, sb3, source3)

	assert.Equal(t, 0, len(removeA))
	assert.Equal(t, 0, len(removeB))

	go func() { sources.RemoveSource(source1) }()

	sa1 = <-removeA
	sb1 = <-removeB
	assert.Equal(t, sa1, source1)
	assert.Equal(t, sb1, source1)
}

func TestSubscribeForType(t *testing.T) {
	sources := NewLogSources()
	source1 := NewLogSource("one", &config.LogsConfig{Type: "foo"})
	source1bar := NewLogSource("one-bar", &config.LogsConfig{Type: "bar"})
	source2 := NewLogSource("two", &config.LogsConfig{Type: "foo"})
	source2bar := NewLogSource("two-bar", &config.LogsConfig{Type: "bar"})
	source3 := NewLogSource("three", &config.LogsConfig{Type: "foo"})

	go func() {
		sources.AddSource(source1bar)
		sources.AddSource(source1)
	}()

	addA, removeA := sources.SubscribeForType("foo", make(chan struct{}), make(chan struct{}))
	assert.NotNil(t, addA)
	sa1 := <-addA
	assert.Equal(t, sa1, source1)
	assert.Equal(t, 0, len(removeA))

	go func() {
		sources.AddSource(source2bar)
		sources.AddSource(source2)
	}()

	sa2 := <-addA
	assert.Equal(t, sa2, source2)

	addB, removeB := sources.SubscribeForType("foo", make(chan struct{}), make(chan struct{}))
	assert.NotNil(t, addB)
	sb1 := <-addB
	sb2 := <-addB
	assert.ElementsMatch(t, []*LogSource{source1, source2}, []*LogSource{sb1, sb2})
	assert.Equal(t, 0, len(removeB))

	go func() { sources.AddSource(source3) }()

	sa3 := <-addA
	sb3 := <-addB
	assert.Equal(t, sa3, source3)
	assert.Equal(t, sb3, source3)

	assert.Equal(t, 0, len(removeA))
	assert.Equal(t, 0, len(removeB))

	go func() { sources.RemoveSource(source1) }()

	sa1 = <-removeA
	sb1 = <-removeB
	assert.Equal(t, sa1, source1)
	assert.Equal(t, sb1, source1)
}

// TestPartialRestart verifies that AddSource does not block after a launcher
// stops (closes its done channel) while a new launcher is subscribed.
//
// This covers the partial pipeline restart scenario: LogSources is reused
// across restarts, so dead subscriptions from the old launcher must not
// block sends to new subscribers.
func TestPartialRestart(t *testing.T) {
	logSources := NewLogSources()
	source := NewLogSource("foo", &config.LogsConfig{Type: "foo"})

	// Simulate old launcher: subscribe but never consume, then stop.
	oldDone := make(chan struct{})
	_ = logSources.GetAddedForType("foo", oldDone)
	close(oldDone) // old launcher stopped; its channel is now dead

	// Simulate new launcher: subscribe and consume.
	newDone := make(chan struct{})
	newStream := logSources.GetAddedForType("foo", newDone)

	addSourceDone := make(chan struct{})
	go func() {
		logSources.AddSource(source)
		close(addSourceDone)
	}()

	select {
	case received := <-newStream:
		assert.Equal(t, source, received)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for new subscriber to receive source")
	}

	select {
	case <-addSourceDone:
		// AddSource completed without blocking on the dead subscription
	case <-time.After(time.Second):
		t.Fatal("AddSource blocked on dead subscription")
	}
}
