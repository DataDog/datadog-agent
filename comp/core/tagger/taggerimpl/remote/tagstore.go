// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remote

import (
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	genericstore "github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/generic_store"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/subscriber"
	"github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

const remoteSource = "remote"

type tagStore struct {
	mutex     sync.RWMutex
	store     types.ObjectStore[*types.Entity]
	telemetry map[string]float64
	cfg       config.Component

	subscriptionManager subscriber.SubscriptionManager
	telemetryStore      *telemetry.Store
}

func newTagStore(cfg config.Component, telemetryStore *telemetry.Store) *tagStore {
	return &tagStore{
		store:               genericstore.NewObjectStore[*types.Entity](),
		telemetry:           make(map[string]float64),
		cfg:                 cfg,
		subscriptionManager: subscriber.NewSubscriptionManager(telemetryStore),
		telemetryStore:      telemetryStore,
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

		switch event.EventType {
		case types.EventTypeAdded:
			s.telemetryStore.UpdatedEntities.Inc()
			s.store.Set(event.Entity.ID, &entity)

		case types.EventTypeModified:
			s.telemetryStore.UpdatedEntities.Inc()
			s.store.Set(event.Entity.ID, &entity)

		case types.EventTypeDeleted:
			s.store.Unset(event.Entity.ID)
		}
	}

	s.notifySubscribers(events)

	return nil
}

func (s *tagStore) getEntity(entityID types.EntityID) *types.Entity {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if entity, present := s.store.Get(entityID); present {
		return entity
	}

	return nil
}

func (s *tagStore) listEntities() []*types.Entity {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.store.ListObjects(types.NewMatchAllFilter())
}

func (s *tagStore) collectTelemetry() {
	// our telemetry package does not seem to have a way to reset a Gauge,
	// so we need to keep track of all the labels we use, and re-set them
	// to zero after we're done to ensure a new run of collectTelemetry
	// will not forget to clear them if they disappear.

	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.store.ForEach(nil, func(_ types.EntityID, e *types.Entity) { s.telemetry[string(e.ID.GetPrefix())]++ })

	for prefix, storedEntities := range s.telemetry {
		s.telemetryStore.StoredEntities.Set(storedEntities, remoteSource, prefix)
		s.telemetry[prefix] = 0
	}
}

func (s *tagStore) subscribe(subscriptionID string, filter *types.Filter) (types.Subscription, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	events := make([]types.EntityEvent, 0, s.store.Size())

	s.store.ForEach(nil, func(_ types.EntityID, e *types.Entity) {
		events = append(events, types.EntityEvent{
			EventType: types.EventTypeAdded,
			Entity:    *e,
		})
	})

	return s.subscriptionManager.Subscribe(subscriptionID, filter, events)
}

func (s *tagStore) notifySubscribers(events []types.EntityEvent) {
	s.subscriptionManager.Notify(events)
}

// reset clears the local store, preparing it to be re-initialized from a fresh
// stream coming from the remote source. It also notifies all subscribers that
// entities have been deleted before re-adding them. In practice, a remote
// tagger is not expected to have subscribers though.
// NOTE: caller must ensure that it holds s.mutex's lock, as this func does not
// do it on its own.
func (s *tagStore) reset() {
	if s.store.Size() == 0 {
		return
	}

	events := make([]types.EntityEvent, 0, s.store.Size())

	s.store.ForEach(nil, func(_ types.EntityID, e *types.Entity) {
		events = append(events, types.EntityEvent{
			EventType: types.EventTypeDeleted,
			Entity:    types.Entity{ID: e.ID},
		})
	})

	s.notifySubscribers(events)

	s.store = genericstore.NewObjectStore[*types.Entity]()
}
