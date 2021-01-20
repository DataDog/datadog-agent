// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package local

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/subscriber"
	"github.com/DataDog/datadog-agent/pkg/tagger/telemetry"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var errNotFound = errors.New("entity not found")

// entityTags holds the tag information for a given entity. It is not
// thread-safe, so should not be shared outside of the store. Usage inside the
// store is safe since it relies on a global lock.
type entityTags struct {
	entityID           string
	sourceTags         map[string]sourceTags
	cacheValid         bool
	cachedSource       []string
	cachedAll          []string // Low + orchestrator + high
	cachedOrchestrator []string // Low + orchestrator (subslice of cachedAll)
	cachedLow          []string // Sub-slice of cachedAll
	toDelete           map[string]struct{}
}

func newEntityTags(entityID string) *entityTags {
	return &entityTags{
		entityID:   entityID,
		sourceTags: make(map[string]sourceTags),
		toDelete:   make(map[string]struct{}),
	}
}

// sourceTags holds the tags for a given entity collected from a single source,
// grouped by their cardinality.
type sourceTags struct {
	lowCardTags          []string
	orchestratorCardTags []string
	highCardTags         []string
	standardTags         []string
}

// tagStore stores entity tags in memory and handles search and collation.
// Queries should go through the Tagger for cache-miss handling
type tagStore struct {
	sync.RWMutex

	store    map[string]*entityTags
	toDelete map[string]struct{} // set emulation

	subscriber *subscriber.Subscriber
}

func newTagStore() *tagStore {
	return &tagStore{
		store:      make(map[string]*entityTags),
		toDelete:   make(map[string]struct{}),
		subscriber: subscriber.NewSubscriber(),
	}
}

func (s *tagStore) processTagInfo(tagInfos []*collectors.TagInfo) {
	events := []types.EntityEvent{}

	s.Lock()
	defer s.Unlock()

	for _, info := range tagInfos {
		if info == nil {
			log.Tracef("processTagInfo err: skipping nil message")
			continue
		}
		if info.Entity == "" {
			log.Tracef("processTagInfo err: empty entity name, skipping message")
			continue
		}
		if info.Source == "" {
			log.Tracef("processTagInfo err: empty source name, skipping message")
			continue
		}

		storedTags, exist := s.store[info.Entity]

		if info.DeleteEntity {
			if exist {
				s.toDelete[info.Entity] = struct{}{}
				storedTags.toDelete[info.Source] = struct{}{}
			}

			continue
		}

		eventType := types.EventTypeModified
		if !exist {
			eventType = types.EventTypeAdded
			storedTags = newEntityTags(info.Entity)
			s.store[info.Entity] = storedTags
		}

		// TODO: check if real change

		telemetry.UpdatedEntities.Inc()

		err := updateStoredTags(storedTags, info)
		if err != nil {
			log.Tracef("processTagInfo err: %v", err)
			continue
		}

		events = append(events, types.EntityEvent{
			EventType: eventType,
			Entity:    storedTags.toEntity(),
		})
	}

	if len(events) > 0 {
		s.notifySubscribers(events)
	}
}

func updateStoredTags(storedTags *entityTags, info *collectors.TagInfo) error {
	_, found := storedTags.sourceTags[info.Source]
	if found && info.CacheMiss {
		// check if the source tags is already present for this entry
		return fmt.Errorf("try to overwrite an existing entry with and empty cache-miss entry, info.Source: %s, info.Entity: %s", info.Source, info.Entity)
	}

	if !found {
		prefix, _ := containers.SplitEntityName(info.Entity)
		telemetry.StoredEntities.Inc(info.Source, prefix)
	}

	storedTags.cacheValid = false
	storedTags.sourceTags[info.Source] = sourceTags{
		lowCardTags:          info.LowCardTags,
		orchestratorCardTags: info.OrchestratorCardTags,
		highCardTags:         info.HighCardTags,
		standardTags:         info.StandardTags,
	}

	return nil
}

func (s *tagStore) subscribe(cardinality collectors.TagCardinality) chan []types.EntityEvent {
	s.RLock()
	defer s.RUnlock()

	events := make([]types.EntityEvent, 0, len(s.store))
	for _, storedTags := range s.store {
		events = append(events, types.EntityEvent{
			EventType: types.EventTypeAdded,
			Entity:    storedTags.toEntity(),
		})
	}

	return s.subscriber.Subscribe(cardinality, events)
}

func (s *tagStore) unsubscribe(ch chan []types.EntityEvent) {
	s.subscriber.Unsubscribe(ch)
}

func (s *tagStore) notifySubscribers(events []types.EntityEvent) {
	s.subscriber.Notify(events)
}

// prune will lock the store and delete tags for the entity previously
// passed as delete. This is to be called regularly from the user class.
func (s *tagStore) prune() error {
	s.Lock()
	defer s.Unlock()

	if len(s.toDelete) == 0 {
		return nil
	}

	events := []types.EntityEvent{}

	for entity := range s.toDelete {
		storedTags, ok := s.store[entity]
		if !ok {
			continue
		}

		prefix, _ := containers.SplitEntityName(entity)

		for source := range storedTags.toDelete {
			if _, ok := storedTags.sourceTags[source]; !ok {
				continue
			}

			delete(storedTags.sourceTags, source)
			telemetry.StoredEntities.Dec(source, prefix)
		}

		if len(storedTags.sourceTags) == 0 {
			delete(s.store, entity)
			events = append(events, types.EntityEvent{
				EventType: types.EventTypeDeleted,
				Entity:    storedTags.toEntity(),
			})
		} else {
			storedTags.cacheValid = false
			storedTags.toDelete = make(map[string]struct{})
			events = append(events, types.EntityEvent{
				EventType: types.EventTypeModified,
				Entity:    storedTags.toEntity(),
			})
		}
	}

	log.Debugf("pruned %d removed entities, %d remaining", len(s.toDelete), len(s.store))

	// Start fresh
	s.toDelete = make(map[string]struct{})

	if len(events) > 0 {
		s.notifySubscribers(events)
	}

	return nil
}

// lookup gets tags from the store and returns them concatenated in a string
// slice. It returns the source names in the second slice to allow the
// client to trigger manual lookups on missing sources.
func (s *tagStore) lookup(entity string, cardinality collectors.TagCardinality) ([]string, []string) {
	s.RLock()
	defer s.RUnlock()
	storedTags, present := s.store[entity]

	if present == false {
		return nil, nil
	}
	return storedTags.get(cardinality)
}

// lookupStandard returns the standard tags recorded for a given entity
func (s *tagStore) lookupStandard(entity string) ([]string, error) {
	s.RLock()
	defer s.RUnlock()

	storedTags, present := s.store[entity]
	if present == false {
		return nil, errNotFound
	}

	return storedTags.getStandard(), nil
}

func (e *entityTags) getStandard() []string {
	tags := []string{}
	for _, t := range e.sourceTags {
		tags = append(tags, t.standardTags...)
	}
	return tags
}

type tagPriority struct {
	tag         string                       // full tag
	priority    collectors.CollectorPriority // collector priority
	cardinality collectors.TagCardinality    // cardinality level of the tag (low, orchestrator, high)
}

func (e *entityTags) get(cardinality collectors.TagCardinality) ([]string, []string) {
	e.computeCache()

	if cardinality == collectors.HighCardinality {
		return e.cachedAll, e.cachedSource
	} else if cardinality == collectors.OrchestratorCardinality {
		return e.cachedOrchestrator, e.cachedSource
	}
	return e.cachedLow, e.cachedSource
}

func (e *entityTags) toEntity() types.Entity {
	e.computeCache()

	return types.Entity{
		ID:                          e.entityID,
		StandardTags:                e.getStandard(),
		HighCardinalityTags:         e.cachedAll[len(e.cachedOrchestrator):],
		OrchestratorCardinalityTags: e.cachedOrchestrator[len(e.cachedLow):],
		LowCardinalityTags:          e.cachedLow,
	}
}

func (e *entityTags) computeCache() {
	if e.cacheValid {
		return
	}

	var sources []string
	tagPrioMapper := make(map[string][]tagPriority)

	for source, tags := range e.sourceTags {
		sources = append(sources, source)
		insertWithPriority(tagPrioMapper, tags.lowCardTags, source, collectors.LowCardinality)
		insertWithPriority(tagPrioMapper, tags.orchestratorCardTags, source, collectors.OrchestratorCardinality)
		insertWithPriority(tagPrioMapper, tags.highCardTags, source, collectors.HighCardinality)
	}

	var lowCardTags []string
	var orchestratorCardTags []string
	var highCardTags []string
	for _, tags := range tagPrioMapper {
		for i := 0; i < len(tags); i++ {
			insert := true
			for j := 0; j < len(tags); j++ {
				// if we find a duplicate tag with higher priority we do not insert the tag
				if i != j && tags[i].priority < tags[j].priority {
					insert = false
					break
				}
			}
			if !insert {
				continue
			}
			if tags[i].cardinality == collectors.HighCardinality {
				highCardTags = append(highCardTags, tags[i].tag)
				continue
			} else if tags[i].cardinality == collectors.OrchestratorCardinality {
				orchestratorCardTags = append(orchestratorCardTags, tags[i].tag)
				continue
			}
			lowCardTags = append(lowCardTags, tags[i].tag)
		}
	}

	tags := append(lowCardTags, orchestratorCardTags...)
	tags = append(tags, highCardTags...)

	// Write cache
	e.cacheValid = true
	e.cachedSource = sources
	e.cachedAll = tags
	e.cachedLow = e.cachedAll[:len(lowCardTags)]
	e.cachedOrchestrator = e.cachedAll[:len(lowCardTags)+len(orchestratorCardTags)]
}

func insertWithPriority(tagPrioMapper map[string][]tagPriority, tags []string, source string, cardinality collectors.TagCardinality) {
	priority, found := collectors.CollectorPriorities[source]
	if !found {
		log.Warnf("Tagger: %s collector has no defined priority, assuming low", source)
		priority = collectors.NodeRuntime
	}

	for _, t := range tags {
		tagName := strings.Split(t, ":")[0]
		tagPrioMapper[tagName] = append(tagPrioMapper[tagName], tagPriority{
			tag:         t,
			priority:    priority,
			cardinality: cardinality,
		})
	}
}
