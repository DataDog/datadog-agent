// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package subscriber

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/telemetry"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const bufferSize = 100

// Subscriber allows processes to subscribe to entity events generated from a
// tagger.
type Subscriber struct {
	sync.RWMutex
	subscribers map[chan []types.EntityEvent]collectors.TagCardinality
}

// NewSubscriber returns a new subscriber.
func NewSubscriber() *Subscriber {
	return &Subscriber{
		subscribers: make(map[chan []types.EntityEvent]collectors.TagCardinality),
	}
}

// Subscribe returns a channel that receives a slice of events whenever an
// entity is added, modified or deleted. It can send an initial burst of events
// only to the new subscriber, without notifying all of the others.
func (s *Subscriber) Subscribe(cardinality collectors.TagCardinality, events []types.EntityEvent) chan []types.EntityEvent {
	// this is a `ch []EntityEvent` instead of a `ch EntityEvent` to
	// improve throughput, as bursts of events are as likely to occur as
	// isolated events, especially at startup or with collectors that
	// periodically pull changes.
	ch := make(chan []types.EntityEvent, bufferSize)

	s.Lock()
	s.subscribers[ch] = cardinality
	telemetry.Subscribers.Inc()
	s.Unlock()

	if events != nil && len(events) > 0 {
		notify(ch, events, cardinality)
	}

	return ch
}

// Unsubscribe ends a subscription to entity events and closes its channel.
func (s *Subscriber) Unsubscribe(ch chan []types.EntityEvent) {
	s.Lock()
	defer s.Unlock()

	s.unsubscribe(ch)
}

// unsubscribe ends a subscription to entity events and closes its channel. It
// is not thread-safe, and callers should take care of synchronization.
func (s *Subscriber) unsubscribe(ch chan []types.EntityEvent) {
	if _, ok := s.subscribers[ch]; ok {
		telemetry.Subscribers.Dec()
		delete(s.subscribers, ch)
		close(ch)
	}
}

// Notify sends a slice of EntityEvents to all registered subscribers at their
// chosen cardinality.
func (s *Subscriber) Notify(events []types.EntityEvent) {
	if len(events) == 0 {
		return
	}

	s.Lock()
	defer s.Unlock()

	for ch, cardinality := range s.subscribers {
		if len(ch) >= bufferSize {
			log.Info("channel full, canceling subscription")
			s.unsubscribe(ch)
			continue
		}

		notify(ch, events, cardinality)
	}
}

// notify sends a slice of EntityEvents to a channel at a chosen cardinality.
func notify(ch chan []types.EntityEvent, events []types.EntityEvent, cardinality collectors.TagCardinality) {
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

	telemetry.Sends.Inc()
	telemetry.Events.Add(float64(len(events)), collectors.TagCardinalityToString(cardinality))

	ch <- subscriberEvents
}
