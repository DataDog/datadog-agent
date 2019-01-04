// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package config

import (
	"sync"
)

// LogSources stores a list of log sources.
type LogSources struct {
	mu            sync.Mutex
	sources       []*LogSource
	addedByType   map[string]chan *LogSource
	removedByType map[string]chan *LogSource
}

// NewLogSources creates a new log sources.
func NewLogSources() *LogSources {
	return &LogSources{
		addedByType:   make(map[string]chan *LogSource),
		removedByType: make(map[string]chan *LogSource),
	}
}

// AddSource adds a new source.
func (s *LogSources) AddSource(source *LogSource) {
	s.mu.Lock()
	s.sources = append(s.sources, source)
	if source.Config == nil || source.Config.Validate() != nil {
		s.mu.Unlock()
		return
	}
	stream, exists := s.addedByType[source.Config.Type]
	s.mu.Unlock()

	if exists {
		stream <- source
	}
}

// RemoveSource removes a source.
func (s *LogSources) RemoveSource(source *LogSource) {
	s.mu.Lock()
	var sourceFound bool
	for i, src := range s.sources {
		if src == source {
			s.sources = append(s.sources[:i], s.sources[i+1:]...)
			sourceFound = true
			break
		}
	}
	stream, streamExists := s.removedByType[source.Config.Type]
	s.mu.Unlock()

	if sourceFound && streamExists {
		stream <- source
	}
}

// GetAddedForType returns the new added sources matching the provided type.
func (s *LogSources) GetAddedForType(sourceType string) chan *LogSource {
	s.mu.Lock()
	defer s.mu.Unlock()

	stream, exists := s.addedByType[sourceType]
	if !exists {
		stream = make(chan *LogSource)
		s.addedByType[sourceType] = stream
	}
	return stream
}

// GetRemovedForType returns the new removed sources matching the provided type.
func (s *LogSources) GetRemovedForType(sourceType string) chan *LogSource {
	s.mu.Lock()
	defer s.mu.Unlock()

	stream, exists := s.removedByType[sourceType]
	if !exists {
		stream = make(chan *LogSource)
		s.removedByType[sourceType] = stream
	}
	return stream
}

// GetSources returns all the sources currently held.
func (s *LogSources) GetSources() []*LogSource {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.sources
}
