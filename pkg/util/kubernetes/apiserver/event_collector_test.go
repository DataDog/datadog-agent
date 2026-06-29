// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"
	v1 "k8s.io/api/core/v1"
)

func makeCollector(bufferSize int) *EventCollector {
	return &EventCollector{
		events:  make(chan *v1.Event, bufferSize),
		dropped: atomic.NewUint64(0),
	}
}

// TestDrain verifies Drain returns all buffered events and leaves the channel empty.
func TestDrain(t *testing.T) {
	t.Run("empty channel: nil", func(t *testing.T) {
		ec := makeCollector(10)
		assert.Nil(t, ec.Drain())
	})

	t.Run("one event: returns slice of length one", func(t *testing.T) {
		ec := makeCollector(10)
		ev := &v1.Event{Reason: "OOM"}
		ec.events <- ev
		assert.Equal(t, []*v1.Event{ev}, ec.Drain())
		assert.Nil(t, ec.Drain())
	})

	t.Run("multiple events: returns all and clears", func(t *testing.T) {
		ec := makeCollector(10)
		ev1 := &v1.Event{Reason: "OOM"}
		ev2 := &v1.Event{Reason: "Evicted"}
		ec.events <- ev1
		ec.events <- ev2

		assert.Equal(t, []*v1.Event{ev1, ev2}, ec.Drain())
		assert.Nil(t, ec.Drain())
	})
}

// TestEnqueue verifies enqueue buffers events and counts drops when the buffer is full.
func TestEnqueue(t *testing.T) {
	ev := &v1.Event{Reason: "OOM"}

	t.Run("buffer not full: event buffered, Dropped stays zero", func(t *testing.T) {
		ec := makeCollector(1)
		ec.enqueue(ev)
		assert.Equal(t, uint64(0), ec.Dropped())
		assert.Len(t, ec.Drain(), 1)
	})

	t.Run("buffer full: event dropped, Dropped incremented", func(t *testing.T) {
		ec := makeCollector(1)
		ec.events <- ev // fill the buffer
		ec.enqueue(ev)
		assert.Equal(t, uint64(1), ec.Dropped())
	})
}
