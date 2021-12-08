// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import (
	"errors"
	"sync"

	model "github.com/DataDog/agent-payload/v5/contlcycle"
)

type queue struct {
	chunkSize       int
	data            []model.EventsPaylaod
	payloadTemplate model.EventsPaylaod
	sync.RWMutex
}

func newQueue(chunkSize int, template model.EventsPaylaod) *queue {
	return &queue{
		chunkSize:       chunkSize,
		payloadTemplate: template,
		data:            []model.EventsPaylaod{},
	}
}

func (q *queue) dump() []model.EventsPaylaod {
	q.RLock()
	defer q.RUnlock()

	return q.data
}

func (q *queue) reset() {
	q.Lock()
	defer q.Unlock()

	q.data = []model.EventsPaylaod{}
}

func (q *queue) add(ev *event) {
	if q.isEmpty() || q.isLastPayloadFull() {
		payload := q.buildPayload(ev)
		q.addPayload(payload)

		return
	}

	event := buildEvent(ev)
	q.addEvent(event)
}

func (q *queue) addPayload(payload model.EventsPaylaod) {
	q.Lock()
	defer q.Unlock()

	q.data = append(q.data, payload)
}

func (q *queue) addEvent(event *model.Event) {
	q.Lock()
	defer q.Unlock()

	lenQueue := len(q.data)
	q.data[lenQueue-1].Events = append(q.data[lenQueue-1].Events, event)
}

func (q *queue) isEmpty() bool {
	q.RLock()
	defer q.RUnlock()

	return len(q.data) == 0
}

func (q *queue) lastPayload() (model.EventsPaylaod, error) {
	if q.isEmpty() {
		return model.EventsPaylaod{}, errors.New("empty queue")
	}

	q.RLock()
	defer q.RUnlock()

	return q.data[len(q.data)-1], nil
}

func (q *queue) isLastPayloadFull() bool {
	lastElem, err := q.lastPayload()
	if err != nil {
		return false
	}

	return len(lastElem.Events) >= q.chunkSize
}

func (q *queue) buildPayload(ev *event) model.EventsPaylaod {
	collectorEvent := q.payloadTemplate
	collectorEvent.EventType = ev.eventType
	collectorEvent.ObjectKind = ev.objectKind
	collectorEvent.Events = []*model.Event{buildEvent(ev)}

	return collectorEvent
}

func buildEvent(ev *event) *model.Event {
	return &model.Event{
		ObjectID:      ev.objectID,
		Source:        ev.source,
		ExitCode:      ev.exitCode,
		ExitTimestamp: ev.exitTS,
	}
}
