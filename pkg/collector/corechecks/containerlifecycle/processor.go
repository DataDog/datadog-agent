// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/agent-payload/v5/contlcycle"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	types "github.com/DataDog/datadog-agent/pkg/containerlifecycle"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"google.golang.org/protobuf/proto"
)

type processor struct {
	sender          sender.Sender
	handlers        []Handler
	podsQueue       *queue
	containersQueue *queue
	tasksQueue      *queue
}

func newProcessor(sender sender.Sender, chunkSize int, store workloadmeta.Component) *processor {
	return &processor{
		sender: sender,
		handlers: []Handler{
			NewContainerTerminationHandler(store),
			&PodTerminationHandler{},
			&TaskTerminationHandler{},
		},
		podsQueue:       newQueue(chunkSize),
		containersQueue: newQueue(chunkSize),
		tasksQueue:      newQueue(chunkSize),
	}
}

// start spawns a go routine to consume event queues
func (p *processor) start(ctx context.Context, pollInterval time.Duration) {
	go p.processQueues(ctx, pollInterval)
}

// processEvents handles workloadmeta events, supports pods and container unset events.
func (p *processor) processEvents(evBundle workloadmeta.EventBundle) {
	evBundle.Acknowledge()

	log.Debugf("container_lifecycle processor: processing %d events", len(evBundle.Events))

	for _, event := range evBundle.Events {
		entityID := event.Entity.GetID()
		handled := false
		for _, h := range p.handlers {
			if h.CanHandle(event) {
				handled = true
				les, err := h.Handle(event)
				if err != nil {
					log.Debugf("Handler '%s' failed to handle event %q: %v", h.String(), entityID.ID, err)
					continue
				}

				log.Debugf("Handler '%s' produced %d lifecycle event(s) for %s/%s (type=%v)", h.String(), len(les), entityID.Kind, entityID.ID, event.Type)
				for _, le := range les {
					if err := p.enqueue(le); err != nil {
						log.Debugf("Couldn't enqueue lifecycle event: %+v: %v", le, err)
					} else {
						log.Debugf("Enqueued lifecycle event kind=%s for %s/%s", le.ObjectKind, entityID.Kind, entityID.ID)
					}
				}
				// Handlers don't need to be mutually exclusive, so we don't break here
			}
		}
		if !handled {
			log.Debugf("container_lifecycle processor: no handler claimed event %s/%s (type=%v)", entityID.Kind, entityID.ID, event.Type)
		}
	}
}

func (p *processor) enqueue(le LifecycleEvent) error {
	switch le.ObjectKind {
	case types.ObjectKindContainer:
		return p.containersQueue.add(le)
	case types.ObjectKindPod:
		return p.podsQueue.add(le)
	case types.ObjectKindTask:
		return p.tasksQueue.add(le)
	default:
		return fmt.Errorf("unknown object kind %q", le.ObjectKind)
	}
}

// processQueues consumes the data available in the queues
func (p *processor) processQueues(ctx context.Context, pollInterval time.Duration) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.flush()
		case <-ctx.Done():
			p.flush()
			return
		}
	}
}

// flush forwards all queued events to the aggregator
func (p *processor) flush() {
	p.flushContainers()
	p.flushPods()
	p.flushTasks()
}

// flushContainers forwards queued container events to the aggregator
func (p *processor) flushContainers() {
	msgs := p.containersQueue.flush()
	if len(msgs) > 0 {
		log.Debugf("container_lifecycle: flushing %d container payload(s)", len(msgs))
		p.containerLifecycleEvent(msgs)

		for eventType, eventCount := range eventCountByType(msgs) {
			emittedEvents.Add(float64(eventCount), eventType, types.ObjectKindContainer)
		}
	} else {
		log.Debugf("container_lifecycle: no container payloads to flush")
	}
}

// flushPods forwards queued pod events to the aggregator
func (p *processor) flushPods() {
	msgs := p.podsQueue.flush()
	if len(msgs) > 0 {
		log.Debugf("container_lifecycle: flushing %d pod payload(s)", len(msgs))
		p.containerLifecycleEvent(msgs)

		for eventType, eventCount := range eventCountByType(msgs) {
			emittedEvents.Add(float64(eventCount), eventType, types.ObjectKindPod)
		}
	} else {
		log.Debugf("container_lifecycle: no pod payloads to flush")
	}
}

// flushTasks forwards queued task events to the aggregator
func (p *processor) flushTasks() {
	msgs := p.tasksQueue.flush()
	if len(msgs) > 0 {
		log.Debugf("container_lifecycle: flushing %d task payload(s)", len(msgs))
		p.containerLifecycleEvent(msgs)

		for eventType, eventCount := range eventCountByType(msgs) {
			emittedEvents.Add(float64(eventCount), eventType, types.ObjectKindTask)
		}
	} else {
		log.Debugf("container_lifecycle: no task payloads to flush")
	}
}

func (p *processor) containerLifecycleEvent(msgs []*contlcycle.EventsPayload) {
	for _, msg := range msgs {
		encoded, err := proto.Marshal(msg)
		if err != nil {
			log.Errorf("Unable to encode message: %+v", err)
			continue
		}

		log.Debugf("container_lifecycle: emitting EventPlatformEvent with %d events (encoded %d bytes)", len(msg.Events), len(encoded))
		p.sender.EventPlatformEvent(encoded, eventplatform.EventTypeContainerLifecycle)
	}
}

func eventCountByType(eventPayloads []*contlcycle.EventsPayload) map[string]int {
	res := make(map[string]int)

	for _, payload := range eventPayloads {
		for _, ev := range payload.Events {
			eventType := strings.ToLower(ev.GetEventType().String())
			res[eventType]++
		}
	}

	return res
}
