package subscriber

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/telemetry"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
)

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
	// this buffer size is an educated guess, as we know the rate of
	// updates, but not how fast these can be streamed out yet. it most
	// likely should be configurable.
	bufferSize := 100

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
	defer telemetry.Subscribers.Dec()

	delete(s.subscribers, ch)
	close(ch)
}

// Notify sends a slice of EntityEvents to all registered subscribers at their
// chosen cardinality.
func (s *Subscriber) Notify(events []types.EntityEvent) {
	if len(events) == 0 {
		return
	}

	s.RLock()
	defer s.RUnlock()

	for ch, cardinality := range s.subscribers {
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
