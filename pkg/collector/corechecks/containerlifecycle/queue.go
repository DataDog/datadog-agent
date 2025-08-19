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
	data      []*model.EventsPayload
	sync.RWMutex
}

// newQueue returns a new *queue.
func newQueue(chunkSize int) *queue {
	return &queue{
		chunkSize: chunkSize,
		data:      []*model.EventsPayload{},
	}
}

// flush returns and resets the queue content. Returns nil if the queue is empty.
// flush is thread-safe.
func (q *queue) flush() []*model.EventsPayload {
	q.Lock()
	defer q.Unlock()

	if q.isEmpty() {
		return nil
	}

	data := q.data

	// Reset the data in the queue.
	q.data = []*model.EventsPayload{}

	return data
}

// add enqueues a new event.
// add is thread-safe.
func (q *queue) add(ev event) error {
	q.Lock()
	defer q.Unlock()

	if q.isEmpty() || q.isLastPayloadFull() {
		return q.addPayload(ev)
	}

	return q.addEvent(ev)
}

// addPayload enqueues the event in a new payload entry in the queue.
// To be used if the queue is empty or if the last payload entry in the queue is full.
// addPayload is not thread-safe, the caller must lock the queue.
func (q *queue) addPayload(ev event) error {
	payload, err := ev.toPayloadModel()
	if err != nil {
		return err
	}

	q.data = append(q.data, payload)

	return nil
}

// addEvent enqueues the event in last payload entry.
// To be used if the queue is not empty and the last payload entry is not full.
// addEvent is not thread-safe, the caller must lock the queue.
func (q *queue) addEvent(ev event) error {
	if q.isEmpty() {
		return errors.New("cannot add event to an empty queue")
	}

	event, err := ev.toEventModel()
	if err != nil {
		return err
	}

	lenQueue := len(q.data)
	q.data[lenQueue-1].Events = append(q.data[lenQueue-1].Events, event)

	return nil
}

// isEmpty returns whether the queue is empty.
// isEmpty is not thread-safe, the caller must lock the queue.
func (q *queue) isEmpty() bool {
	return len(q.data) == 0
}

// lastPayload returns the last payload entry in the queue.
// lastPayload is not thread-safe, the caller must lock the queue.
func (q *queue) lastPayload() (*model.EventsPayload, error) {
	if q.isEmpty() {
		return nil, errors.New("empty queue")
	}

	return q.data[len(q.data)-1], nil
}

// isLastPayloadFull returns whether the last payload entry
// is full compared to the configured chunk size.
// isLastPayloadFull is not thread-safe, the caller must lock the queue.
func (q *queue) isLastPayloadFull() bool {
	lastElem, err := q.lastPayload()
	if err != nil {
		return false
	}

	return len(lastElem.Events) >= q.chunkSize
}
