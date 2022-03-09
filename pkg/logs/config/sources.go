// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"sync"
)

// LogSources serves as the interface between Schedulers and Launchers, distributing
// notifications of added/removed LogSources to subscribed Launchers.
//
// If more than one Launcher subscribes to the same type, the sources will be
// distributed randomly to the Launchers.  This is generally undesirable, and the
// caller should ensure at most one subscriber for each type.
//
// This type is threadsafe, and all of its methods can be called concurrently.
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
//
// Any subscribers registered for this source's type (src.Config.Type) will be
// notified.
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
//
// Any subscribers registered for this source's type (src.Config.Type) will be
// notified of its removal.
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

// GetAddedForType returns a channel carrying notifications of new sources
// with the given type.
//
// Any sources added before this call are not included.
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

// GetRemovedForType returns a channel carrying notifications of removed sources
// with the given type.
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

// GetSources returns all the sources currently held.  The result is copied and
// will not be modified after it is returned.  However, the copy in the LogSources
// instance may change in that time (changing indexes or adding/removing entries).
func (s *LogSources) GetSources() []*LogSource {
	s.mu.Lock()
	defer s.mu.Unlock()

	clone := append([]*LogSource{}, s.sources...)
	return clone
}
