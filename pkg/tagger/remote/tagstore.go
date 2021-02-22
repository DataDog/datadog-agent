// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remote

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/subscriber"
	"github.com/DataDog/datadog-agent/pkg/tagger/telemetry"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

const remoteSource = "remote"

type tagStore struct {
	mutex sync.RWMutex
	store map[string]*types.Entity

	subscriber *subscriber.Subscriber
}

func newTagStore() *tagStore {
	return &tagStore{
		store:      make(map[string]*types.Entity),
		subscriber: subscriber.NewSubscriber(),
	}
}

func (s *tagStore) processEvents(events []types.EntityEvent, replace bool) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if replace {
		s.reset()
	}

	for _, event := range events {
		entity := event.Entity
		prefix, _ := containers.SplitEntityName(entity.ID)

		switch event.EventType {
		case types.EventTypeAdded:
			telemetry.StoredEntities.Inc(remoteSource, prefix)
			telemetry.UpdatedEntities.Inc()
			s.store[event.Entity.ID] = &entity

		case types.EventTypeModified:
			telemetry.UpdatedEntities.Inc()
			s.store[event.Entity.ID] = &entity

		case types.EventTypeDeleted:
			telemetry.StoredEntities.Dec(remoteSource, prefix)
			delete(s.store, event.Entity.ID)
		}
	}

	s.notifySubscribers(events)

	return nil
}

func (s *tagStore) getEntity(entityID string) (*types.Entity, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.store[entityID], nil
}

func (s *tagStore) listEntities() []*types.Entity {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	entities := make([]*types.Entity, 0, len(s.store))

	for _, e := range s.store {
		entities = append(entities, e)
	}

	return entities
}

func (s *tagStore) subscribe(cardinality collectors.TagCardinality) chan []types.EntityEvent {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	events := make([]types.EntityEvent, 0, len(s.store))

	for _, e := range s.store {
		events = append(events, types.EntityEvent{
			EventType: types.EventTypeAdded,
			Entity:    *e,
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

// reset clears the local store, preparing it to be re-initialized from a fresh
// stream coming from the remote source. It also notifies all subscribers that
// entities have been deleted before re-adding them. In practice, a remote
// tagger is not expected to have subscribers though.
// NOTE: caller must ensure that it holds s.mutex's lock, as this func does not
// do it on its own.
func (s *tagStore) reset() {
	if len(s.store) == 0 {
		return
	}

	events := make([]types.EntityEvent, 0, len(s.store))

	for _, e := range s.store {
		prefix, _ := containers.SplitEntityName(e.ID)
		telemetry.StoredEntities.Dec(remoteSource, prefix)

		events = append(events, types.EntityEvent{
			EventType: types.EventTypeDeleted,
			Entity:    types.Entity{ID: e.ID},
		})
	}

	s.notifySubscribers(events)

	s.store = make(map[string]*types.Entity)
}
