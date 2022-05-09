// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddSource(t *testing.T) {
	sources := NewLogSources()
	assert.Equal(t, 0, len(sources.GetSources()))

	sources.AddSource(NewLogSource("foo", &LogsConfig{Type: "boo"}))
	assert.Equal(t, 1, len(sources.GetSources()))

	sources.AddSource(NewLogSource("bar", &LogsConfig{Type: "boo"}))
	assert.Equal(t, 2, len(sources.GetSources()))

	sources.AddSource(NewLogSource("baz", &LogsConfig{})) // invalid config
	assert.Equal(t, 3, len(sources.GetSources()))
}

func TestRemoveSource(t *testing.T) {
	sources := NewLogSources()
	source1 := NewLogSource("foo", &LogsConfig{Type: "boo"})
	source2 := NewLogSource("bar", &LogsConfig{Type: "boo"})

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

	sources.AddSource(NewLogSource("", &LogsConfig{Type: "boo"}))
	assert.Equal(t, 1, len(sources.GetSources()))
}

func TestGetAddedForType(t *testing.T) {
	sources := NewLogSources()
	source := NewLogSource("foo", &LogsConfig{Type: "foo"})

	stream1 := sources.GetAddedForType("foo")
	assert.NotNil(t, stream1)

	stream2 := sources.GetAddedForType("foo")
	assert.NotNil(t, stream2)

	go func() { sources.AddSource(source) }()
	s1 := <-stream1
	s2 := <-stream2
	assert.Equal(t, s1, source)
	assert.Equal(t, s2, source)
}

func TestGetAddedForTypeExistingSources(t *testing.T) {
	sources := NewLogSources()
	source1 := NewLogSource("one", &LogsConfig{Type: "foo"})
	source2 := NewLogSource("two", &LogsConfig{Type: "foo"})
	source3 := NewLogSource("three", &LogsConfig{Type: "foo"})

	go func() { sources.AddSource(source1) }()

	streamA := sources.GetAddedForType("foo")
	assert.NotNil(t, streamA)
	sa1 := <-streamA
	assert.Equal(t, sa1, source1)

	go func() { sources.AddSource(source2) }()
	sa2 := <-streamA
	assert.Equal(t, sa2, source2)

	streamB := sources.GetAddedForType("foo")
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

func TestGetRemovedForType(t *testing.T) {
	sources := NewLogSources()
	source := NewLogSource("foo", &LogsConfig{Type: "foo"})

	streamA := sources.GetRemovedForType("foo")
	assert.NotNil(t, streamA)
	assert.Equal(t, 0, len(streamA))

	streamB := sources.GetRemovedForType("foo")
	assert.NotNil(t, streamB)
	assert.Equal(t, 0, len(streamB))

	// removing a nonexistent source does nothing
	sources.RemoveSource(source)
	assert.Equal(t, 0, len(streamA))
	assert.Equal(t, 0, len(streamB))

	sources.AddSource(source)
	go func() { sources.RemoveSource(source) }()
	sa := <-streamA
	sb := <-streamB
	assert.Equal(t, sa, source)
	assert.Equal(t, sb, source)
}
