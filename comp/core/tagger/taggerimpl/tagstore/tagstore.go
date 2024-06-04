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

	"github.com/benbjohnson/clock"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/subscriber"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

	store     map[string]EntityTags
	telemetry map[string]map[string]float64

	subscriber *subscriber.Subscriber

	clock clock.Clock

	telemetryStore *telemetry.Store
}

// NewTagStore creates new TagStore.
func NewTagStore(telemetryStore *telemetry.Store) *TagStore {
	return newTagStoreWithClock(clock.New(), telemetryStore)
}

func newTagStoreWithClock(clock clock.Clock, telemetryStore *telemetry.Store) *TagStore {
	return &TagStore{
		telemetry:      make(map[string]map[string]float64),
		store:          make(map[string]EntityTags),
		subscriber:     subscriber.NewSubscriber(telemetryStore),
		clock:          clock,
		telemetryStore: telemetryStore,
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
func (s *TagStore) ProcessTagInfo(tagInfos []*types.TagInfo) {
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
				storedTags.setSourceExpiration(info.Source, s.clock.Now().Add(deletedTTL))
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
			tags := storedTags.tagsForSource(info.Source)
			if tags != nil && reflect.DeepEqual(tags, newSt) {
				continue
			}
		} else {
			eventType = types.EventTypeAdded
			storedTags = newEntityTags(info.Entity, info.Source)
			s.store[info.Entity] = storedTags
		}

		s.telemetryStore.UpdatedEntities.Inc()
		storedTags.setTagsForSource(info.Source, newSt)

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

	for entityName, entityTags := range s.store {
		prefix, _ := containers.SplitEntityName(entityName)

		for _, source := range entityTags.sources() {
			if _, ok := s.telemetry[prefix]; !ok {
				s.telemetry[prefix] = make(map[string]float64)
			}

			s.telemetry[prefix][source]++
		}
	}

	for prefix, sources := range s.telemetry {
		for source, storedEntities := range sources {
			s.telemetryStore.StoredEntities.Set(storedEntities, source, prefix)
			s.telemetry[prefix][source] = 0
		}
	}
}

// Subscribe returns a channel that receives a slice of events whenever an entity is
// added, modified or deleted. It can send an initial burst of events only to the new
// subscriber, without notifying all of the others.
func (s *TagStore) Subscribe(cardinality types.TagCardinality) chan []types.EntityEvent {
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
		changed := storedTags.deleteExpired(now)

		if !changed && !storedTags.shouldRemove() {
			continue
		}

		if storedTags.shouldRemove() {
			s.telemetryStore.PrunedEntities.Inc()
			delete(s.store, entity)
			events = append(events, types.EntityEvent{
				EventType: types.EventTypeDeleted,
				Entity:    storedTags.toEntity(),
			})
		} else {
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
func (s *TagStore) LookupHashed(entity string, cardinality types.TagCardinality) tagset.HashedTags {
	s.RLock()
	defer s.RUnlock()
	storedTags, present := s.store[entity]

	if !present {
		return tagset.HashedTags{}
	}
	return storedTags.getHashedTags(cardinality)
}

// Lookup gets tags from the store and returns them concatenated in a string slice.
func (s *TagStore) Lookup(entity string, cardinality types.TagCardinality) []string {
	return s.LookupHashed(entity, cardinality).Get()
}

// LookupStandard returns the standard tags recorded for a given entity
func (s *TagStore) LookupStandard(entityID string) ([]string, error) {
	storedTags, err := s.getEntityTags(entityID)
	if err != nil {
		return nil, err
	}

	return storedTags.getStandard(), nil
}

// List returns full list of entities and their tags per source in an API format.
func (s *TagStore) List() types.TaggerListResponse {
	r := types.TaggerListResponse{
		Entities: make(map[string]types.TaggerListEntity),
	}

	s.RLock()
	defer s.RUnlock()

	for entityID, et := range s.store {
		r.Entities[entityID] = types.TaggerListEntity{
			Tags: et.tagsBySource(),
		}
	}

	return r
}

// GetEntity returns the entity corresponding to the specified id and an error
func (s *TagStore) GetEntity(entityID string) (*types.Entity, error) {
	tags, err := s.getEntityTags(entityID)
	if err != nil {
		return nil, err
	}

	entity := tags.toEntity()
	return &entity, nil
}

func (s *TagStore) getEntityTags(entityID string) (EntityTags, error) {
	s.RLock()
	defer s.RUnlock()

	storedTags, present := s.store[entityID]
	if !present {
		return nil, ErrNotFound
	}

	return storedTags, nil
}
