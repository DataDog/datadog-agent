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
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
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

	log.Tracef("Processing %d events", len(evBundle.Events))

	for _, event := range evBundle.Events {
		for _, h := range p.handlers {
			if h.CanHandle(event) {
				les, err := h.Handle(event)
				if err != nil {
					log.Errorf("Handler '%s' failed to handle event %q: %v", h.String(), event.Entity.GetID().ID, err)
					continue
				}

				for _, le := range les {
					err := p.enqueue(le)
					if err != nil {
						log.Errorf("Couldn't enqueue lifecycle event: %+v: %v", le, err)
					}
				}
			}
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
		p.containerLifecycleEvent(msgs)

		for eventType, eventCount := range eventCountByType(msgs) {
			emittedEvents.Add(float64(eventCount), eventType, types.ObjectKindContainer)
		}
	}
}

// flushPods forwards queued pod events to the aggregator
func (p *processor) flushPods() {
	msgs := p.podsQueue.flush()
	if len(msgs) > 0 {
		p.containerLifecycleEvent(msgs)

		for eventType, eventCount := range eventCountByType(msgs) {
			emittedEvents.Add(float64(eventCount), eventType, types.ObjectKindPod)
		}
	}
}

// flushTasks forwards queued task events to the aggregator
func (p *processor) flushTasks() {
	msgs := p.tasksQueue.flush()
	if len(msgs) > 0 {
		p.containerLifecycleEvent(msgs)

		for eventType, eventCount := range eventCountByType(msgs) {
			emittedEvents.Add(float64(eventCount), eventType, types.ObjectKindTask)
		}
	}
}

func (p *processor) containerLifecycleEvent(msgs []*contlcycle.EventsPayload) {
	for _, msg := range msgs {
		encoded, err := proto.Marshal(msg)
		if err != nil {
			log.Errorf("Unable to encode message: %+v", err)
			continue
		}

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
