// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"sync"
)

// LogSources stores a list of log sources.
type LogSources struct {
	sources []*LogSource
	mu      sync.Mutex
}

// NewLogSources creates a new log sources.
func NewLogSources(sources []*LogSource) *LogSources {
	return &LogSources{
		sources: sources,
	}
}

// AddSource adds a new source.
func (s *LogSources) AddSource(source *LogSource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sources = append(s.sources, source)
}

// RemoveSource removes a source.
func (s *LogSources) RemoveSource(source *LogSource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, src := range s.sources {
		if src == source {
			s.sources = append(s.sources[:i], s.sources[i+1:]...)
			break
		}
	}
}

// GetSources returns all the sources currently held.
func (s *LogSources) GetSources() []*LogSource {
	return s.sources
}

// GetValidSources returns the sources which config is valid.
func (s *LogSources) GetValidSources() []*LogSource {
	return s.getSources(func(source *LogSource) bool {
		return source.Config != nil && source.Config.Validate() == nil
	})
}

// GetSourcesWithType returns the sources which config type matches the provided type.
func (s *LogSources) GetSourcesWithType(sourceType string) []*LogSource {
	return s.getSources(func(source *LogSource) bool {
		return source.Config != nil && source.Config.Type == sourceType
	})
}

// GetValidSourcesWithType returns the sources which config is valid
// and matches the provided type.
func (s *LogSources) GetValidSourcesWithType(sourceType string) []*LogSource {
	return s.getSources(func(source *LogSource) bool {
		return source.Config != nil && source.Config.Validate() == nil && source.Config.Type == sourceType
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
