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

type subscription struct {
	ch   chan *LogSource
	done chan struct{}
}

// LogSources serves as the interface between Schedulers and Launchers, distributing
// notifications of added/removed LogSources to subscribed Launchers.
//
// Each subscription receives its own unbuffered channel for sources, and should
// consume from the channel quickly to avoid blocking other goroutines.
// Callers must provide a done channel and close it when they stop consuming,
// so that blocked sends can be skipped.
//
// If any sources have been added when GetAddedForType is called, then those sources
// are immediately sent to the channel.
//
// This type is threadsafe, and all of its methods can be called concurrently.
type LogSources struct {
	prepareSource []func(*LogSource) error
	accepted      map[*LogSource]bool
	mu            sync.Mutex
	sources       []*LogSource
	added         []*subscription
	addedByType   map[string][]*subscription
	removed       []*subscription
	removedByType map[string][]*subscription
}

// NewLogSources creates log sources with optional preparation hooks. Hooks run
// after configuration validation and before sources are published to launchers.
func NewLogSources(prepareSource ...func(*LogSource) error) *LogSources {
	return &LogSources{
		prepareSource: prepareSource,
		accepted:      make(map[*LogSource]bool),
		addedByType:   make(map[string][]*subscription),
		removedByType: make(map[string][]*subscription),
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
	if !s.prepare(source) {
		s.mu.Unlock()
		return
	}
	s.accepted[source] = true
	streams := s.added
	streamsForType := s.addedByType[source.Config.Type]
	s.mu.Unlock()

	for _, stream := range streams {
		select {
		case stream.ch <- source:
		case <-stream.done:
		}
	}

	for _, stream := range streamsForType {
		select {
		case stream.ch <- source:
		case <-stream.done:
		}
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
	var streamsForType []*subscription
	if source.Config != nil {
		streamsForType = s.removedByType[source.Config.Type]
	}
	sourceFound = sourceFound && s.accepted[source]
	delete(s.accepted, source)
	s.mu.Unlock()

	if sourceFound {
		for _, stream := range streams {
			select {
			case stream.ch <- source:
			case <-stream.done:
			}
		}
		for _, stream := range streamsForType {
			select {
			case stream.ch <- source:
			case <-stream.done:
			}
		}
	}
}

// SubscribeAll returns two channels carrying notifications of all added and
// removed sources, respectively.  This guarantees consistency if sources are
// added or removed concurrently.
//
// Any sources added before this call are delivered from a new goroutine.
func (s *LogSources) SubscribeAll(addedDone, removedDone chan struct{}) (chan *LogSource, chan *LogSource) {
	s.mu.Lock()
	defer s.mu.Unlock()

	added := &subscription{ch: make(chan *LogSource), done: addedDone}
	removed := &subscription{ch: make(chan *LogSource), done: removedDone}

	s.added = append(s.added, added)
	s.removed = append(s.removed, removed)

	existingSources := s.acceptedSources() // snapshot for goroutine
	go func() {
		for _, source := range existingSources {
			select {
			case added.ch <- source:
			case <-addedDone:
				return
			}
		}
	}()

	return added.ch, removed.ch
}

// SubscribeForType returns two channels carrying notifications of added and
// removed sources with the given type, respectively.  This guarantees
// consistency if sources are added or removed concurrently.
//
// Any sources added before this call are delivered from a new goroutine.
func (s *LogSources) SubscribeForType(sourceType string, addedDone, removedDone chan struct{}) (chan *LogSource, chan *LogSource) {
	s.mu.Lock()
	defer s.mu.Unlock()

	added := &subscription{ch: make(chan *LogSource), done: addedDone}
	removed := &subscription{ch: make(chan *LogSource), done: removedDone}

	if _, exists := s.addedByType[sourceType]; !exists {
		s.addedByType[sourceType] = []*subscription{}
	}
	s.addedByType[sourceType] = append(s.addedByType[sourceType], added)

	if _, exists := s.removedByType[sourceType]; !exists {
		s.removedByType[sourceType] = []*subscription{}
	}
	s.removedByType[sourceType] = append(s.removedByType[sourceType], removed)

	existingSources := s.acceptedSources() // snapshot for goroutine
	go func() {
		for _, source := range existingSources {
			if source.Config.Type == sourceType {
				select {
				case added.ch <- source:
				case <-addedDone:
					return
				}
			}
		}
	}()

	return added.ch, removed.ch
}

// GetAddedForType returns a channel carrying notifications of new sources
// with the given type.
//
// Any sources added before this call are delivered from a new goroutine.
func (s *LogSources) GetAddedForType(sourceType string, addedDone chan struct{}) chan *LogSource {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, exists := s.addedByType[sourceType]
	if !exists {
		s.addedByType[sourceType] = []*subscription{}
	}

	stream := &subscription{ch: make(chan *LogSource), done: addedDone}
	s.addedByType[sourceType] = append(s.addedByType[sourceType], stream)

	existingSources := s.acceptedSources() // snapshot for goroutine
	go func() {
		for _, source := range existingSources {
			if source.Config.Type == sourceType {
				select {
				case stream.ch <- source:
				case <-addedDone:
					return
				}
			}
		}
	}()

	return stream.ch
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

// prepare runs under mu, before any launcher can observe the source. Invalid
// sources remain visible in status but are never sent to current or later subscribers.
func (s *LogSources) prepare(source *LogSource) bool {
	if source.Config == nil {
		return false
	}
	err := source.Config.Validate()
	if err == nil {
		for _, prepare := range s.prepareSource {
			if err = prepare(source); err != nil {
				break
			}
		}
	}
	if err != nil {
		source.Status().Error(err)
		return false
	}
	return true
}

// acceptedSources requires mu to be held.
func (s *LogSources) acceptedSources() []*LogSource {
	accepted := make([]*LogSource, 0, len(s.sources))
	for _, source := range s.sources {
		if s.accepted[source] {
			accepted = append(accepted, source)
		}
	}
	return accepted
}
