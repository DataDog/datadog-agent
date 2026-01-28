// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sources

import (
	"slices"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LogSources serves as the interface between Schedulers and Launchers, distributing
// notifications of added/removed LogSources to subscribed Launchers.
//
// Each subscription receives its own unbuffered channel for sources, and should
// consume from the channel quickly to avoid blocking other goroutines.  There is
// no means to unsubscribe.
//
// If any sources have been added when GetAddedForType is called, then those sources
// are immediately sent to the channel.
//
// This type is threadsafe, and all of its methods can be called concurrently.
type LogSources struct {
	mu            sync.Mutex
	sources       []*LogSource
	added         []chan *LogSource
	addedByType   map[string][]chan *LogSource
	removed       []chan *LogSource
	removedByType map[string][]chan *LogSource
}

// NewLogSources creates a new log sources.
func NewLogSources() *LogSources {
	return &LogSources{
		addedByType:   make(map[string][]chan *LogSource),
		removedByType: make(map[string][]chan *LogSource),
	}
}

// AddSource adds a new source.
//
// All of the subscribers registered for this source's type (src.Config.Type) will be
// notified.
func (s *LogSources) AddSource(source *LogSource) {
	log.Tracef("Adding %s", source.Dump(false))
	s.mu.Lock()
	s.sources = append(s.sources, source)
	if source.Config == nil || source.Config.Validate() != nil {
		s.mu.Unlock()
		return
	}
	streams := s.added
	streamsForType := s.addedByType[source.Config.Type]
	s.mu.Unlock()

	for _, stream := range streams {
		stream <- source
	}

	for _, stream := range streamsForType {
		stream <- source
	}
}

// RemoveSource removes a source.
//
// All of the subscribers registered for this source's type (src.Config.Type) will be
// notified of its removal.
func (s *LogSources) RemoveSource(source *LogSource) {
	log.Tracef("Removing %s", source.Dump(false))
	s.mu.Lock()
	var sourceFound bool
	for i, src := range s.sources {
		if src == source {
			s.sources = slices.Delete(s.sources, i, i+1)
			sourceFound = true
			break
		}
	}
	streams := s.removed
	streamsForType := s.removedByType[source.Config.Type]
	s.mu.Unlock()

	if sourceFound {
		for _, stream := range streams {
			stream <- source
		}
		for _, stream := range streamsForType {
			stream <- source
		}
	}
}

// SubscribeAll returns two channels carrying notifications of all added and
// removed sources, respectively.  This guarantees consistency if sources are
// added or removed concurrently.
//
// Any sources added before this call are delivered from a new goroutine.
func (s *LogSources) SubscribeAll() (added chan *LogSource, removed chan *LogSource) {
	s.mu.Lock()
	defer s.mu.Unlock()

	added = make(chan *LogSource)
	removed = make(chan *LogSource)

	s.added = append(s.added, added)
	s.removed = append(s.removed, removed)

	existingSources := slices.Clone(s.sources) // clone for goroutine
	go func() {
		for _, source := range existingSources {
			added <- source
		}
	}()

	return
}

// SubscribeForType returns two channels carrying notifications of added and
// removed sources with the given type, respectively.  This guarantees
// consistency if sources are added or removed concurrently.
//
// Any sources added before this call are delivered from a new goroutine.
func (s *LogSources) SubscribeForType(sourceType string) (added chan *LogSource, removed chan *LogSource) {
	s.mu.Lock()
	defer s.mu.Unlock()

	added = make(chan *LogSource)
	removed = make(chan *LogSource)

	if _, exists := s.addedByType[sourceType]; !exists {
		s.addedByType[sourceType] = []chan *LogSource{}
	}
	s.addedByType[sourceType] = append(s.addedByType[sourceType], added)

	if _, exists := s.removedByType[sourceType]; !exists {
		s.removedByType[sourceType] = []chan *LogSource{}
	}
	s.removedByType[sourceType] = append(s.removedByType[sourceType], removed)

	existingSources := slices.Clone(s.sources) // clone for goroutine
	go func() {
		for _, source := range existingSources {
			if source.Config.Type == sourceType {
				added <- source
			}
		}
	}()

	return
}

// GetAddedForType returns a channel carrying notifications of new sources
// with the given type.
//
// Any sources added before this call are delivered from a new goroutine.
func (s *LogSources) GetAddedForType(sourceType string) chan *LogSource {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, exists := s.addedByType[sourceType]
	if !exists {
		s.addedByType[sourceType] = []chan *LogSource{}
	}

	stream := make(chan *LogSource)
	s.addedByType[sourceType] = append(s.addedByType[sourceType], stream)

	existingSources := slices.Clone(s.sources) // clone for goroutine
	go func() {
		for _, source := range existingSources {
			if source.Config.Type == sourceType {
				stream <- source
			}
		}
	}()

	return stream
}

// GetSources returns all the sources currently held.  The result is copied and
// will not be modified after it is returned.  However, the copy in the LogSources
// instance may change in that time (changing indexes or adding/removing entries).
func (s *LogSources) GetSources() []*LogSource {
	s.mu.Lock()
	defer s.mu.Unlock()

	clone := slices.Clone(s.sources)
	return clone
}
