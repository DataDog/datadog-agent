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

type subscriberChannelContent struct {
	batches        int
	totalEvents    int
	eventsByPrefix map[types.EntityIDPrefix]int
	eventsByType   map[string]int
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

	sm.Lock()

	if _, found := sm.subscribers[id]; found {
		sm.Unlock()
		return nil, fmt.Errorf("duplicate subscription id error: subscription id %s is already in use", id)
	}

	subscriber := &Subscriber{
		filter:  *filter,
		id:      id,
		ch:      ch,
		manager: sm,
	}

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

// unsubscribe ends a subscription to entity events and closes its channel.
// This method is not thread-safe.
func (sm *subscriptionManager) unsubscribe(subscriptionID string) {
	sub, found := sm.subscribers[subscriptionID]
	if !found {
		log.Debugf("subscriber with %q is already unsubscribed", subscriptionID)
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

// Unsubscribe is a thread-safe implementation of unsubscribe
func (sm *subscriptionManager) Unsubscribe(subscriptionID string) {
	sm.Lock()
	defer sm.Unlock()
	sm.unsubscribe(subscriptionID)
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
					channelContent := inspectChannel(subscriber.ch)
					log.Errorf("subscriber with id %q has a channel full (%d events in %d batches: prefixes=%v, types=%v), canceling subscription",
						subscriber.id, channelContent.totalEvents, channelContent.batches,
						channelContent.eventsByPrefix, channelContent.eventsByType)
					sm.unsubscribe(subscriber.id)
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

func inspectChannel(ch chan []types.EntityEvent) subscriberChannelContent {
	batches := 0
	totalEvents := 0
	eventsByPrefix := make(map[types.EntityIDPrefix]int)
	eventsByType := make(map[string]int)

	// The subscriber may still be reading from the channel while we drain it.
	// That's OK, because after calling this function, we unsubscribe the
	// subscriber to force a reconnect. Note that the drained contents may not
	// exactly match what was queued when the overflow was detected, but should
	// be good enough for debugging.
	for {
		select {
		case batch := <-ch:
			batches++
			for _, event := range batch {
				totalEvents++
				eventsByPrefix[event.Entity.ID.GetPrefix()]++

				// EventType is an int, convert to string for easier debugging
				switch event.EventType {
				case types.EventTypeAdded:
					eventsByType["Added"]++
				case types.EventTypeModified:
					eventsByType["Modified"]++
				case types.EventTypeDeleted:
					eventsByType["Deleted"]++
				}
			}
		default:
			// Channel emptied
			return subscriberChannelContent{
				batches:        batches,
				totalEvents:    totalEvents,
				eventsByPrefix: eventsByPrefix,
				eventsByType:   eventsByType,
			}
		}
	}
}
