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

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	types "github.com/DataDog/datadog-agent/pkg/containerlifecycle"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"google.golang.org/protobuf/proto"
)

type processor struct {
	sender          sender.Sender
	podsQueue       *queue
	containersQueue *queue
	tasksQueue      *queue
	store           workloadmeta.Component
}

func newProcessor(sender sender.Sender, chunkSize int, store workloadmeta.Component) *processor {
	return &processor{
		sender:          sender,
		podsQueue:       newQueue(chunkSize),
		containersQueue: newQueue(chunkSize),
		tasksQueue:      newQueue(chunkSize),
		store:           store,
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
			pod, ok := event.Entity.(*workloadmeta.KubernetesPod)
			if !ok {
				log.Debugf("Expected workloadmeta.KubernetesPod got %T, skipping", event.Entity)
				continue
			}

			err := p.processPod(pod)
			if err != nil {
				log.Debugf("Couldn't process pod %q: %v", event.Entity.GetID().ID, err)
			}
		case workloadmeta.KindECSTask:
			task, ok := event.Entity.(*workloadmeta.ECSTask)
			if !ok {
				log.Debugf("Expected workloadmeta.ECSTask got %T, skipping", event.Entity)
				continue
			}

			err := p.processTask(task)
			if err != nil {
				log.Debugf("Couldn't process task %q: %v", event.Entity.GetID().ID, err)
			}
		default:
			log.Tracef("Cannot handle event for entity %q with kind %q", entityID.ID, entityID.Kind)
		}
	}
}

// processContainer enqueue container events
func (p *processor) processContainer(container *workloadmeta.Container, sources []workloadmeta.Source) error {
	ctr := &contlcycle.ContainerEvent{
		ContainerID: container.ID,
	}

	if len(sources) > 0 {
		ctr.Source = string(sources[0])
	}

	if !container.State.FinishedAt.IsZero() {
		ts := container.State.FinishedAt.Unix()
		ctr.OptionalExitTimestamp = &contlcycle.ContainerEvent_ExitTimestamp{ExitTimestamp: ts}
	}

	if container.State.ExitCode != nil {
		code := int32(*container.State.ExitCode)
		ctr.OptionalExitCode = &contlcycle.ContainerEvent_ExitCode{ExitCode: code}
	}

	// Because the container processor is triggered off of runtime events, and the
	// container runtime would have no knowledge surrounding what owns the container,
	// we need to query the workloadmeta store to get this information.
	if c, err := p.store.GetContainer(container.ID); err == nil {
		if c.Owner != nil {
			switch c.Owner.Kind {
			case workloadmeta.KindKubernetesPod:
				if ownerKind, err := kindToModel(types.ObjectKindPod); err == nil {
					ctr.Owner = &contlcycle.ContainerEvent_Owner{OwnerType: ownerKind, OwnerUID: c.Owner.ID}
				}
			case workloadmeta.KindECSTask:
				if ownerKind, err := kindToModel(types.ObjectKindTask); err == nil {
					ctr.Owner = &contlcycle.ContainerEvent_Owner{OwnerType: ownerKind, OwnerUID: c.Owner.ID}
				}
			default:
				log.Tracef("Cannot handle owner for container %q with type %q", container.ID, c.Owner.Kind)
			}
		}
	}

	le := LifecycleEvent{
		ObjectKind: types.ObjectKindContainer,
		ProtoEvent: &contlcycle.Event{
			EventType:  contlcycle.Event_Delete,
			TypedEvent: &contlcycle.Event_Container{Container: ctr},
		},
	}
	return p.containersQueue.add(le)
}

// processPod enqueue pod events
func (p *processor) processPod(pod *workloadmeta.KubernetesPod) error {
	podEvent := &contlcycle.PodEvent{
		PodUID: pod.GetID().ID,
		Source: string(workloadmeta.SourceNodeOrchestrator),
	}

	if !pod.FinishedAt.IsZero() {
		ts := pod.FinishedAt.Unix()
		podEvent.ExitTimestamp = &ts
	}

	le := LifecycleEvent{
		ObjectKind: types.ObjectKindPod,
		ProtoEvent: &contlcycle.Event{
			EventType:  contlcycle.Event_Delete,
			TypedEvent: &contlcycle.Event_Pod{Pod: podEvent},
		},
	}
	return p.podsQueue.add(le)
}

func (p *processor) processTask(task *workloadmeta.ECSTask) error {
	source := string(workloadmeta.SourceNodeOrchestrator)
	if task.LaunchType == workloadmeta.ECSLaunchTypeFargate {
		source = string(workloadmeta.SourceRuntime)
	}

	// we don't have exit timestamp for tasks in the response of metadata v1 api, so we use the current timestamp
	ts := time.Now().Unix()
	taskEvent := &contlcycle.TaskEvent{
		TaskARN:       task.GetID().ID,
		Source:        source,
		ExitTimestamp: &ts,
	}

	le := LifecycleEvent{
		ObjectKind: types.ObjectKindTask,
		ProtoEvent: &contlcycle.Event{
			EventType:  contlcycle.Event_Delete,
			TypedEvent: &contlcycle.Event_Task{Task: taskEvent},
		},
	}
	return p.tasksQueue.add(le)
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
