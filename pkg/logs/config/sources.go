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
	mu           sync.Mutex
	sources      []*LogSource
	streamByType map[string]chan *LogSource
}

// NewLogSources creates a new log sources.
func NewLogSources() *LogSources {
	return &LogSources{
		streamByType: make(map[string]chan *LogSource),
	}
}

// AddSource adds a new source.
func (s *LogSources) AddSource(source *LogSource) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sources = append(s.sources, source)
	if source.Config == nil || source.Config.Validate() != nil {
		return
	}
	stream := s.GetSourceStreamForType(source.Config.Type)
	go func() {
		stream <- source
	}()
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

// GetSourceStreamForType returns the stream of valid sources matching the provided type.
func (s *LogSources) GetSourceStreamForType(sourceType string) chan *LogSource {
	stream, exists := s.streamByType[sourceType]
	if !exists {
		stream = make(chan *LogSource)
		s.streamByType[sourceType] = stream
	}
	return stream
}

// GetSources returns all the sources currently held.
func (s *LogSources) GetSources() []*LogSource {
	return s.sources
}
