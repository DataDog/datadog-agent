// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

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

	sources.AddSource(source)

	stream := sources.GetAddedForType("foo")
	assert.NotNil(t, stream)
	assert.Equal(t, 0, len(stream))

	go func() { sources.AddSource(source) }()
	s := <-stream
	assert.Equal(t, s, source)
}

func TestGetRemovedForType(t *testing.T) {
	sources := NewLogSources()
	source := NewLogSource("foo", &LogsConfig{Type: "foo"})

	sources.RemoveSource(source)

	stream := sources.GetRemovedForType("foo")
	assert.NotNil(t, stream)
	assert.Equal(t, 0, len(stream))

	sources.RemoveSource(source)
	assert.Equal(t, 0, len(stream))

	sources.AddSource(source)
	go func() { sources.RemoveSource(source) }()
	s := <-stream
	assert.Equal(t, s, source)
}
