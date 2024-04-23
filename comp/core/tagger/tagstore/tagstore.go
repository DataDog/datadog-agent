// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tagstore implements the TagStore which is the component of the Tagger
// responsible for storing the tags in memory.
package tagstore

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"time"

	tagger_api "github.com/DataDog/datadog-agent/comp/core/tagger/api"
	"github.com/DataDog/datadog-agent/comp/core/tagger/collectors"
	"github.com/DataDog/datadog-agent/comp/core/tagger/subscriber"
	"github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/benbjohnson/clock"
)

const (
	deletedTTL = 5 * time.Minute
)

// ErrNotFound is returned when entity id is not found in the store.
var ErrNotFound = errors.New("entity not found")

// TagStore stores entity tags in memory and handles search and collation.
// Queries should go through the Tagger for cache-miss handling
type TagStore struct {
	sync.RWMutex

	store     map[string]*EntityTags
	telemetry map[string]map[string]float64

	subscriber *subscriber.Subscriber

	clock clock.Clock
}

// NewTagStore creates new TagStore.
func NewTagStore() *TagStore {
	return newTagStoreWithClock(clock.New())
}

func newTagStoreWithClock(clock clock.Clock) *TagStore {
	return &TagStore{
		telemetry:  make(map[string]map[string]float64),
		store:      make(map[string]*EntityTags),
		subscriber: subscriber.NewSubscriber(),
		clock:      clock,
	}
}

// Run performs background maintenance for TagStore.
func (s *TagStore) Run(ctx context.Context) {
	pruneTicker := time.NewTicker(1 * time.Minute)
	telemetryTicker := time.NewTicker(1 * time.Minute)
	health := health.RegisterLiveness("tagger-store")

	for {
		select {
		case <-telemetryTicker.C:
			s.collectTelemetry()

		case <-pruneTicker.C:
			s.Prune()

		case <-health.C:

		case <-ctx.Done():
			pruneTicker.Stop()
			telemetryTicker.Stop()

			return
		}
	}
}

// ProcessTagInfo updates tagger store with tags fetched by collectors.
func (s *TagStore) ProcessTagInfo(tagInfos []*collectors.TagInfo) {
	events := []types.EntityEvent{}

	s.Lock()
	defer s.Unlock()

	for _, info := range tagInfos {
		if info == nil {
			log.Tracef("ProcessTagInfo err: skipping nil message")
			continue
		}
		if info.Entity == "" {
			log.Tracef("ProcessTagInfo err: empty entity name, skipping message")
			continue
		}
		if info.Source == "" {
			log.Tracef("ProcessTagInfo err: empty source name, skipping message")
			continue
		}

		storedTags, exist := s.store[info.Entity]

		if info.DeleteEntity {
			if exist {
				st, ok := storedTags.sourceTags[info.Source]
				if ok {
					st.expiryDate = s.clock.Now().Add(deletedTTL)
					storedTags.sourceTags[info.Source] = st
				}
			}

			continue
		}

		newSt := sourceTags{
			lowCardTags:          info.LowCardTags,
			orchestratorCardTags: info.OrchestratorCardTags,
			highCardTags:         info.HighCardTags,
			standardTags:         info.StandardTags,
			expiryDate:           info.ExpiryDate,
		}

		eventType := types.EventTypeModified
		if exist {
			st, ok := storedTags.sourceTags[info.Source]
			if ok && reflect.DeepEqual(st, newSt) {
				continue
			}
		} else {
			eventType = types.EventTypeAdded
			storedTags = newEntityTags(info.Entity)
			s.store[info.Entity] = storedTags
		}

		telemetry.UpdatedEntities.Inc()
		storedTags.cacheValid = false
		storedTags.sourceTags[info.Source] = newSt

		events = append(events, types.EntityEvent{
			EventType: eventType,
			Entity:    storedTags.toEntity(),
		})
	}

	if len(events) > 0 {
		s.notifySubscribers(events)
	}
}

func (s *TagStore) collectTelemetry() {
	// our telemetry package does not seem to have a way to reset a Gauge,
	// so we need to keep track of all the labels we use, and re-set them
	// to zero after we're done to ensure a new run of collectTelemetry
	// will not forget to clear them if they disappear.

	s.Lock()
	defer s.Unlock()

	for _, entityTags := range s.store {
		prefix, _ := containers.SplitEntityName(entityTags.entityID)

		for source := range entityTags.sourceTags {
			if _, ok := s.telemetry[prefix]; !ok {
				s.telemetry[prefix] = make(map[string]float64)
			}

			s.telemetry[prefix][source]++
		}
	}

	for prefix, sources := range s.telemetry {
		for source, storedEntities := range sources {
			telemetry.StoredEntities.Set(storedEntities, source, prefix)
			s.telemetry[prefix][source] = 0
		}
	}
}

// Subscribe returns a channel that receives a slice of events whenever an entity is
// added, modified or deleted. It can send an initial burst of events only to the new
// subscriber, without notifying all of the others.
func (s *TagStore) Subscribe(cardinality collectors.TagCardinality) chan []types.EntityEvent {
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

// Unsubscribe ends a subscription to entity events and closes its channel.
func (s *TagStore) Unsubscribe(ch chan []types.EntityEvent) {
	s.subscriber.Unsubscribe(ch)
}

func (s *TagStore) notifySubscribers(events []types.EntityEvent) {
	s.subscriber.Notify(events)
}

// Prune deletes tags for entities that have been marked as deleted. This is to
// be called regularly from the user class.
func (s *TagStore) Prune() {
	s.Lock()
	defer s.Unlock()

	now := s.clock.Now()
	events := []types.EntityEvent{}

	for entity, storedTags := range s.store {
		changed := false

		// remove any sourceTags that have expired
		for source, st := range storedTags.sourceTags {
			if st.isExpired(now) {
				delete(storedTags.sourceTags, source)
				changed = true
			}
		}

		// remove all sourceTags only if they're all empty
		if storedTags.shouldRemove() {
			storedTags.sourceTags = nil
			changed = true
		}

		if !changed {
			continue
		}

		if len(storedTags.sourceTags) == 0 {
			telemetry.PrunedEntities.Inc()
			delete(s.store, entity)
			events = append(events, types.EntityEvent{
				EventType: types.EventTypeDeleted,
				Entity:    storedTags.toEntity(),
			})
		} else {
			storedTags.cacheValid = false
			events = append(events, types.EntityEvent{
				EventType: types.EventTypeModified,
				Entity:    storedTags.toEntity(),
			})
		}
	}

	if len(events) > 0 {
		s.notifySubscribers(events)
	}
}

// LookupHashed gets tags from the store and returns them as a HashedTags instance.
func (s *TagStore) LookupHashed(entity string, cardinality collectors.TagCardinality) tagset.HashedTags {
	s.RLock()
	defer s.RUnlock()
	storedTags, present := s.store[entity]

	if !present {
		return tagset.HashedTags{}
	}
	return storedTags.getHashedTags(cardinality)
}

// Lookup gets tags from the store and returns them concatenated in a string slice.
func (s *TagStore) Lookup(entity string, cardinality collectors.TagCardinality) []string {
	return s.LookupHashed(entity, cardinality).Get()
}

// LookupStandard returns the standard tags recorded for a given entity
func (s *TagStore) LookupStandard(entityID string) ([]string, error) {
	storedTags, err := s.GetEntityTags(entityID)
	if err != nil {
		return nil, err
	}

	return storedTags.getStandard(), nil
}

// GetEntityTags returns the EntityTags for a given entity
func (s *TagStore) GetEntityTags(entityID string) (*EntityTags, error) {
	s.RLock()
	defer s.RUnlock()

	storedTags, present := s.store[entityID]
	if !present {
		return nil, ErrNotFound
	}

	return storedTags, nil
}

// List returns full list of entities and their tags per source in an API format.
func (s *TagStore) List() tagger_api.TaggerListResponse {
	r := tagger_api.TaggerListResponse{
		Entities: make(map[string]tagger_api.TaggerListEntity),
	}

	s.RLock()
	defer s.RUnlock()

	for entityID, et := range s.store {
		entity := tagger_api.TaggerListEntity{
			Tags: make(map[string][]string),
		}

		for source, sourceTags := range et.sourceTags {
			tags := append([]string(nil), sourceTags.lowCardTags...)
			tags = append(tags, sourceTags.orchestratorCardTags...)
			tags = append(tags, sourceTags.highCardTags...)
			entity.Tags[source] = tags
		}

		r.Entities[entityID] = entity
	}

	return r
}

// GetEntity returns the entity corresponding to the specified id and an error
func (s *TagStore) GetEntity(entityID string) (*types.Entity, error) {
	tags, err := s.GetEntityTags(entityID)
	if err != nil {
		return nil, err
	}

	entity := tags.toEntity()
	return &entity, nil
}
