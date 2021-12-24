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
	chunkSize int
	data      []model.EventsPayload
	sync.RWMutex
}

func newQueue(chunkSize int) *queue {
	return &queue{
		chunkSize: chunkSize,
		data:      []model.EventsPayload{},
	}
}

func (q *queue) dump() []model.EventsPayload {
	q.RLock()
	defer q.RUnlock()

	return q.data
}

func (q *queue) reset() {
	q.Lock()
	defer q.Unlock()

	q.data = []model.EventsPayload{}
}

func (q *queue) add(ev event) error {
	if q.isEmpty() || q.isLastPayloadFull() {
		payload, err := ev.toPayloadModel()
		if err != nil {
			return err
		}

		q.addPayload(payload)
		return nil
	}

	event, err := ev.toEventModel()
	if err != nil {
		return err
	}

	q.addEvent(event)

	return nil
}

func (q *queue) addPayload(payload model.EventsPayload) {
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

func (q *queue) lastPayload() (model.EventsPayload, error) {
	if q.isEmpty() {
		return model.EventsPayload{}, errors.New("empty queue")
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
