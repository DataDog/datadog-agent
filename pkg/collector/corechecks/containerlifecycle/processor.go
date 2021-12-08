// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import (
	"time"

	model "github.com/DataDog/agent-payload/v5/contlcycle"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	types "github.com/DataDog/datadog-agent/pkg/containerlifecycle"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

type event struct {
	objectKind string
	objectID   string
	eventType  string
	source     string
	exitCode   int32
	exitTS     int64
}

type processor struct {
	sender          aggregator.Sender
	podsQueue       *queue
	containersQueue *queue
}

func newProcessor(sender aggregator.Sender, chunkSize int) *processor {
	template := model.EventsPaylaod{Version: types.PayloadV1}

	return &processor{
		sender:          sender,
		podsQueue:       newQueue(chunkSize, template),
		containersQueue: newQueue(chunkSize, template),
	}
}

// start spawns a go routine to consume event queues
func (p *processor) start(stop chan struct{}, pollInterval time.Duration) {
	go p.processQueues(stop, pollInterval)
}

// processEvents handles workloadmeta events, supports pods and container unset events.
func (p *processor) processEvents(evBundle workloadmeta.EventBundle) {
	close(evBundle.Ch)

	log.Tracef("Processing %d events", len(evBundle.Events))

	for _, event := range evBundle.Events {
		entityID := event.Entity.GetID()

		switch event.Type {
		case workloadmeta.EventTypeUnset:
			switch entityID.Kind {
			case workloadmeta.KindContainer:
				container, ok := event.Entity.(*workloadmeta.Container)
				if !ok {
					log.Debugf("Expected workloadmeta.Container got %T, skipping", event.Entity)
					continue
				}

				p.processContainer(container, event.Sources)
			case workloadmeta.KindKubernetesPod:
				p.processPod(event.Entity)
			case workloadmeta.KindECSTask: // not supported
			default:
				log.Debugf("Cannot handle event for entity %q with kind %q", entityID.ID, entityID.Kind)
			}

		case workloadmeta.EventTypeSet: // not supported
		default:
			log.Debugf("Cannot handle event of type %d", event.Type)
		}
	}
}

// processContainer enqueue container events
func (p *processor) processContainer(container *workloadmeta.Container, sources []workloadmeta.Source) {
	event := &event{
		objectKind: types.ObjectKindContainer,
		eventType:  types.EventNameDelete,
	}

	if len(sources) > 0 {
		event.source = string(sources[0])
	}

	if !container.State.FinishedAt.IsZero() {
		event.exitTS = container.State.FinishedAt.Unix()
	}

	if container.State.ExitCode != nil {
		event.exitCode = int32(*container.State.ExitCode)
	} else {
		event.exitCode = -1
	}

	p.containersQueue.add(event)
}

// processPod enqueue pod events
func (p *processor) processPod(pod workloadmeta.Entity) {
	event := &event{
		objectKind: types.ObjectKindPod,
		eventType:  types.EventNameDelete,
		objectID:   pod.GetID().ID,
		source:     string(workloadmeta.SourceKubelet),
	}

	p.podsQueue.add(event)
}

// processQueues consumes the data available in the queues
func (p *processor) processQueues(stop chan struct{}, pollInterval time.Duration) {
	ticker := time.NewTicker(pollInterval)

	for {
		select {
		case <-ticker.C:
			if !p.containersQueue.isEmpty() {
				msgs := p.containersQueue.dump()
				p.containersQueue.reset()
				p.sender.ContainerLifecycleEvent(msgs)
			}

			if !p.podsQueue.isEmpty() {
				msgs := p.podsQueue.dump()
				p.podsQueue.reset()
				p.sender.ContainerLifecycleEvent(msgs)
			}
		case <-stop:
			return
		}
	}
}
