// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LogSources stores a list of log sources.
type LogSources struct {
	sources map[string]*LogSource
	lock    *sync.Mutex
}

// NewLogSources creates a new log sources.
func NewLogSources(sources []*LogSource) *LogSources {
	sourceMap := make(map[string]*LogSource)
	for _, source := range sources {
		sourceMap[source.Name] = source
	}
	return &LogSources{
		sources: sourceMap,
		lock:    &sync.Mutex{},
	}
}

// AddSource adds a new source.
func (s *LogSources) AddSource(source *LogSource) {
	s.lock.Lock()
	if _, exists := s.sources[source.Name]; exists {
		log.Warnf("source %s already exists, updating it")
	}
	s.sources[source.Name] = source
	s.lock.Unlock()
}

// RemoveSource removes a source.
func (s *LogSources) RemoveSource(source *LogSource) {
	s.lock.Lock()
	delete(s.sources, source.Name)
	s.lock.Unlock()
}

// GetSources returns all the sources currently held.
func (s *LogSources) GetSources() []*LogSource {
	s.lock.Lock()
	defer s.lock.Unlock()
	sources := make([]*LogSource, 0, len(s.sources))
	for _, source := range s.sources {
		sources = append(sources, source)
	}
	return sources
}

// GetValidSources returns the sources which status is not in error.
func (s *LogSources) GetValidSources() []*LogSource {
	return s.getSources(func(source *LogSource) bool {
		return !source.Status.IsError()
	})
}

// GetValidSourcesWithType returns the sources which status is not in error,
// and the config type matches the provided type.
func (s *LogSources) GetValidSourcesWithType(sourceType string) []*LogSource {
	return s.getSources(func(source *LogSource) bool {
		return !source.Status.IsError() && source.Config != nil && source.Config.Type == sourceType
	})
}

// getSources returns all the sources matching the provided filter.
func (s *LogSources) getSources(filter func(*LogSource) bool) []*LogSource {
	sources := make([]*LogSource, 0)
	for _, source := range s.sources {
		if filter(source) {
			sources = append(sources, source)
		}
	}
	return sources
}
