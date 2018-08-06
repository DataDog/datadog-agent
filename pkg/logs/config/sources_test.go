// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddSource(t *testing.T) {
	sources := NewEmptyLogSources()
	assert.Equal(t, 0, len(sources.GetSources()))
	sources.AddSource(NewLogSource("foo", nil))
	assert.Equal(t, 1, len(sources.GetSources()))
	sources.AddSource(NewLogSource("bar", nil))
	assert.Equal(t, 2, len(sources.GetSources()))
}

func TestRemoveSource(t *testing.T) {
	source1 := NewLogSource("foo", nil)
	source2 := NewLogSource("bar", nil)
	sources := NewLogSources([]*LogSource{source1, source2})
	assert.Equal(t, 2, len(sources.GetSources()))
	sources.RemoveSource(source1)
	assert.Equal(t, 1, len(sources.GetSources()))
	assert.Equal(t, source2, sources.GetSources()[0])
	sources.RemoveSource(source2)
	assert.Equal(t, 0, len(sources.GetSources()))
}

func TestGetSources(t *testing.T) {
	sources := NewEmptyLogSources()
	assert.Equal(t, 0, len(sources.GetSources()))
	sources.AddSource(NewLogSource("", nil))
	assert.Equal(t, 1, len(sources.GetSources()))
}

func TestGetValidSources(t *testing.T) {
	source1 := NewLogSource("foo", &LogsConfig{Type: FileType})
	source2 := NewLogSource("bar", &LogsConfig{Type: DockerType})
	sources := NewLogSources([]*LogSource{source1, source2})
	assert.Equal(t, 1, len(sources.GetValidSources()))
}

func TestGetSourcesWithType(t *testing.T) {
	source1 := NewLogSource("foo", nil)
	source2 := NewLogSource("bar", &LogsConfig{})
	source3 := NewLogSource("baz", &LogsConfig{Type: FileType, Path: "foo"})
	source4 := NewLogSource("qux", &LogsConfig{Type: FileType})
	sources := NewLogSources([]*LogSource{source1, source2, source3, source4})
	assert.Equal(t, 2, len(sources.GetSourcesWithType(FileType)))
}

func TestGetValidSourcesWithType(t *testing.T) {
	source1 := NewLogSource("foo", nil)
	source2 := NewLogSource("bar", &LogsConfig{})
	source3 := NewLogSource("baz", &LogsConfig{Type: FileType, Path: "foo"})
	source4 := NewLogSource("qux", &LogsConfig{Type: FileType})
	sources := NewLogSources([]*LogSource{source1, source2, source3, source4})
	assert.Equal(t, 1, len(sources.GetValidSourcesWithType(FileType)))
}

// NewEmptyLogSources creates a new log sources with no initial entries.
func NewEmptyLogSources() *LogSources {
	return NewLogSources(make([]*LogSource, 0))
}
