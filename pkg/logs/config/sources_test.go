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
	sources := NewLogSources()
	registerConsumer(sources, "boo")
	assert.Equal(t, 0, len(sources.GetSources()))
	sources.AddSource(NewLogSource("foo", &LogsConfig{Type: "boo"}))
	assert.Equal(t, 1, len(sources.GetSources()))
	sources.AddSource(NewLogSource("bar", &LogsConfig{Type: "boo"}))
	assert.Equal(t, 2, len(sources.GetSources()))
}

func TestRemoveSource(t *testing.T) {
	sources := NewLogSources()
	registerConsumer(sources, "boo")
	source1 := NewLogSource("foo", &LogsConfig{Type: "boo"})
	sources.AddSource(source1)
	source2 := NewLogSource("bar", &LogsConfig{Type: "boo"})
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
	registerConsumer(sources, "boo")
	assert.Equal(t, 0, len(sources.GetSources()))
	sources.AddSource(NewLogSource("", &LogsConfig{Type: "boo"}))
	assert.Equal(t, 1, len(sources.GetSources()))
}

func registerConsumer(sources *LogSources, sourceType string) {
	go func() {
		sources := sources.GetSourceStreamForType(sourceType)
		for range sources {
			// ensure that another component is consuming the channel to prevent
			// the producer to get stuck.
		}
	}()
}
