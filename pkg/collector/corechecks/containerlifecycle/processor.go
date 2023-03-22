// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import (
	"context"
	"strings"
	"time"

	"github.com/DataDog/agent-payload/v5/contlcycle"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	types "github.com/DataDog/datadog-agent/pkg/containerlifecycle"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	"google.golang.org/protobuf/proto"
)

type processor struct {
	sender          aggregator.Sender
	podsQueue       *queue
	containersQueue *queue
}

func newProcessor(sender aggregator.Sender, chunkSize int) *processor {
	return &processor{
		sender:          sender,
		podsQueue:       newQueue(chunkSize),
		containersQueue: newQueue(chunkSize),
	}
}

// start spawns a go routine to consume event queues
func (p *processor) start(ctx context.Context, pollInterval time.Duration) {
	go p.processQueues(ctx, pollInterval)
}

// processEvents handles workloadmeta events, supports pods and container unset events.
func (p *processor) processEvents(evBundle workloadmeta.EventBundle) {
	close(evBundle.Ch)

	log.Tracef("Processing %d events", len(evBundle.Events))

	for _, event := range evBundle.Events {
		entityID := event.Entity.GetID()
		log.Debugf("Received deletion event for kind %q - ID %q", entityID.Kind, entityID.ID)

		switch entityID.Kind {
		case workloadmeta.KindContainer:
			container, ok := event.Entity.(*workloadmeta.Container)
			if !ok {
				log.Debugf("Expected workloadmeta.Container got %T, skipping", event.Entity)
				continue
			}

			err := p.processContainer(container, []workloadmeta.Source{workloadmeta.SourceRuntime})
			if err != nil {
				log.Debugf("Couldn't process container %q: %v", container.ID, err)
			}
		case workloadmeta.KindKubernetesPod:
			err := p.processPod(event.Entity)
			if err != nil {
				log.Debugf("Couldn't process pod %q: %v", event.Entity.GetID().ID, err)
			}
		case workloadmeta.KindECSTask: // not supported
		default:
			log.Tracef("Cannot handle event for entity %q with kind %q", entityID.ID, entityID.Kind)
		}
	}
}

// processContainer enqueue container events
func (p *processor) processContainer(container *workloadmeta.Container, sources []workloadmeta.Source) error {
	event := newEvent()
	event.withObjectKind(types.ObjectKindContainer)
	event.withEventType(types.EventNameDelete)
	event.withObjectID(container.ID)

	if len(sources) > 0 {
		event.withSource(string(sources[0]))
	}

	if !container.State.FinishedAt.IsZero() {
		ts := container.State.FinishedAt.Unix()
		event.withContainerExitTimestamp(&ts)
	}

	if container.State.ExitCode != nil {
		code := int32(*container.State.ExitCode)
		event.withContainerExitCode(&code)
	}

	return p.containersQueue.add(event)
}

// processPod enqueue pod events
func (p *processor) processPod(pod workloadmeta.Entity) error {
	event := newEvent()
	event.withObjectKind(types.ObjectKindPod)
	event.withEventType(types.EventNameDelete)
	event.withObjectID(pod.GetID().ID)
	event.withSource(string(workloadmeta.SourceNodeOrchestrator))

	return p.podsQueue.add(event)
}

// processQueues consumes the data available in the queues
func (p *processor) processQueues(ctx context.Context, pollInterval time.Duration) {
	ticker := time.NewTicker(pollInterval)

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

func (p *processor) containerLifecycleEvent(msgs []*contlcycle.EventsPayload) {
	for _, msg := range msgs {
		encoded, err := proto.Marshal(msg)
		if err != nil {
			log.Errorf("Unable to encode message: %+v", err)
			continue
		}

		p.sender.EventPlatformEvent(encoded, epforwarder.EventTypeContainerLifecycle)
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
