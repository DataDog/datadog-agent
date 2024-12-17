// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package subscriber

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SubscriptionManager allows processes to subscribe to entity events generated from a
// tagger.
type SubscriptionManager interface {
	Subscribe(id string, filter *types.Filter, events []types.EntityEvent) (types.Subscription, error)
	Unsubscribe(subscriptionID string)
	Notify(events []types.EntityEvent)
}

// subscriptionManager implements SubscriptionManager
type subscriptionManager struct {
	sync.RWMutex
	subscribers    map[string]*Subscriber
	prefixToSub    map[types.EntityIDPrefix][]*Subscriber
	telemetryStore *telemetry.Store
}

// NewSubscriptionManager creates and returns a new subscription manager
func NewSubscriptionManager(telemetryStore *telemetry.Store) SubscriptionManager {
	return &subscriptionManager{
		subscribers:    make(map[string]*Subscriber),
		prefixToSub:    make(map[types.EntityIDPrefix][]*Subscriber),
		telemetryStore: telemetryStore,
	}
}

// Subscribe returns a channel that receives a slice of events whenever an
// entity is added, modified or deleted. It can send an initial burst of events
// only to the new subscriber, without notifying all of the others.
func (sm *subscriptionManager) Subscribe(id string, filter *types.Filter, events []types.EntityEvent) (types.Subscription, error) {
	// this is a `ch []EntityEvent` instead of a `ch EntityEvent` to
	// improve throughput, as bursts of events are as likely to occur as
	// isolated events, especially at startup or with collectors that
	// periodically pull changes.
	ch := make(chan []types.EntityEvent, bufferSize)

	if _, found := sm.subscribers[id]; found {
		return nil, fmt.Errorf("duplicate subscription id error: subscription id %s is already in use", id)
	}

	subscriber := &Subscriber{
		filter:  *filter,
		id:      id,
		ch:      ch,
		manager: sm,
	}

	sm.Lock()
	sm.subscribers[id] = subscriber
	sm.telemetryStore.Subscribers.Inc()

	for prefix := range subscriber.filter.GetPrefixes() {
		sm.prefixToSub[prefix] = append(sm.prefixToSub[prefix], subscriber)
	}

	sm.Unlock()

	if len(events) > 0 {
		sm.notify(ch, events, subscriber.filter.GetCardinality())
	}

	return subscriber, nil
}

// unsubscribe ends a subscription to entity events and closes its channel. It
// is not thread-safe, and callers should take care of synchronization.
func (sm *subscriptionManager) Unsubscribe(subscriptionID string) {
	sm.Lock()
	defer sm.Unlock()

	sub, found := sm.subscribers[subscriptionID]
	if !found {
		log.Debugf("subscriber with %q is already unsubscribed", sub.id)
		return
	}

	for prefix := range sub.filter.GetPrefixes() {
		currentPrefixSubscribers, found := sm.prefixToSub[prefix]
		if !found {
			// no subscribers for this prefix
			continue
		}

		newPrefixSubscribers := make([]*Subscriber, 0, len(currentPrefixSubscribers))

		for _, prefixSubscriber := range currentPrefixSubscribers {
			if prefixSubscriber.id != sub.id {
				newPrefixSubscribers = append(newPrefixSubscribers, prefixSubscriber)
			}
		}

		sm.prefixToSub[prefix] = newPrefixSubscribers
	}

	delete(sm.subscribers, sub.id)
	close(sub.ch)
	sm.telemetryStore.Subscribers.Dec()
}

// Notify sends a slice of EntityEvents to all registered subscribers at their
// chosen cardinality.
func (sm *subscriptionManager) Notify(events []types.EntityEvent) {
	if len(events) == 0 {
		return
	}

	sm.Lock()
	defer sm.Unlock()

	subIDToEvents := map[string][]types.EntityEvent{}

	for _, event := range events {
		entityID := event.Entity.ID

		prefix := entityID.GetPrefix()
		if subscribers, found := sm.prefixToSub[prefix]; found {
			for _, subscriber := range subscribers {

				if len(subscriber.ch) >= bufferSize {
					log.Info("channel full, canceling subscription")
					sm.Unsubscribe(subscriber.id)
					continue
				}
				subIDToEvents[subscriber.id] = append(subIDToEvents[subscriber.id], event)
			}
		}
	}

	for subID, events := range subIDToEvents {
		subscriber := sm.subscribers[subID]
		sm.notify(subscriber.ch, events, subscriber.filter.GetCardinality())
	}
}

// notify sends a slice of EntityEvents to a channel at a chosen cardinality.
func (sm *subscriptionManager) notify(ch chan []types.EntityEvent, events []types.EntityEvent, cardinality types.TagCardinality) {
	subscriberEvents := make([]types.EntityEvent, 0, len(events))

	for _, event := range events {
		var entity types.Entity

		if event.EventType == types.EventTypeDeleted {
			entity = types.Entity{ID: event.Entity.ID}
		} else {
			entity = event.Entity.Copy(cardinality)
		}

		subscriberEvents = append(subscriberEvents, types.EntityEvent{
			EventType: event.EventType,
			Entity:    entity,
		})
	}

	sm.telemetryStore.Sends.Inc()
	sm.telemetryStore.Events.Add(float64(len(events)), types.TagCardinalityToString(cardinality))

	ch <- subscriberEvents
}
