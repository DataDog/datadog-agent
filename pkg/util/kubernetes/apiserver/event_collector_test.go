// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
)

func makeCollector(bufferSize int) *EventCollector {
	return &EventCollector{
		events:       make(chan *v1.Event, bufferSize),
		dropped:      atomic.NewUint64(0),
		lastRV:       atomic.NewUint64(0),
		maxDrainedRV: atomic.NewUint64(0),
	}
}

func makeUnboundedCollector() *EventCollector {
	return &EventCollector{
		unbounded:    true,
		dropped:      atomic.NewUint64(0),
		lastRV:       atomic.NewUint64(0),
		maxDrainedRV: atomic.NewUint64(0),
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
		assert.Equal(t, uint64(0), ec.DrainDropped())
		assert.Len(t, ec.Drain(), 1)
	})

	t.Run("buffer full: event dropped, Dropped incremented", func(t *testing.T) {
		ec := makeCollector(1)
		ec.events <- ev // fill the buffer
		ec.enqueue(ev)
		assert.Equal(t, uint64(1), ec.DrainDropped())
	})

	t.Run("unbounded: events beyond any bounded capacity are never dropped", func(t *testing.T) {
		ec := makeUnboundedCollector()
		for i := 0; i < 100; i++ {
			ec.enqueue(ev)
		}
		assert.Equal(t, uint64(0), ec.DrainDropped())
		assert.Len(t, ec.Drain(), 100)
		assert.Nil(t, ec.Drain())
	})
}

// TestNewEventCollector verifies bufferSize validation is skipped when unbounded is requested.
func TestNewEventCollector(t *testing.T) {
	c := &APIClient{InformerCl: fakeclientset.NewSimpleClientset()}

	t.Run("bounded, bufferSize zero: rejected", func(t *testing.T) {
		assert.Nil(t, c.NewEventCollector("", 0, false))
	})

	t.Run("bounded, bufferSize positive: accepted", func(t *testing.T) {
		assert.NotNil(t, c.NewEventCollector("", 1, false))
	})

	t.Run("unbounded, bufferSize zero: accepted, bufferSize ignored", func(t *testing.T) {
		ec := c.NewEventCollector("", 0, true)
		require.NotNil(t, ec)
		assert.True(t, ec.unbounded)
	})
}

// TestCheckpoint verifies the checkpoint tracks the highest delivered resourceVersion and round-trips through SetCheckpoint.
func TestCheckpoint(t *testing.T) {
	rvEvent := func(rv string) *v1.Event {
		return &v1.Event{ObjectMeta: metav1.ObjectMeta{ResourceVersion: rv}}
	}

	t.Run("checkpoint is the highest drained resourceVersion", func(t *testing.T) {
		ec := makeCollector(10)
		ec.events <- rvEvent("5")
		ec.events <- rvEvent("9")
		ec.Drain()
		assert.Equal(t, "9", ec.Checkpoint())
	})

	t.Run("SetCheckpoint seeds both lastRV and maxDrainedRV", func(t *testing.T) {
		ec := makeCollector(10)
		ec.SetCheckpoint("42")
		assert.Equal(t, uint64(42), ec.lastRV.Load())
		assert.Equal(t, "42", ec.Checkpoint())
	})
}
