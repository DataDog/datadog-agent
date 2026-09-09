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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func makeCollector(bufferSize int) *EventCollector {
	return &EventCollector{
		events:       make(chan *v1.Event, bufferSize),
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

func TestFilterEventListAfterResourceVersion(t *testing.T) {
	t.Run("checkpoint filters each page and preserves pagination", func(t *testing.T) {
		events := &v1.EventList{
			ListMeta: metav1.ListMeta{
				ResourceVersion: "100",
				Continue:        "next-page",
			},
			Items: []v1.Event{
				{ObjectMeta: metav1.ObjectMeta{Name: "old", ResourceVersion: "9"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "newer", ResourceVersion: "11"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "equal", ResourceVersion: "10"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "newest", ResourceVersion: "20"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "invalid", ResourceVersion: "invalid"}},
			},
		}

		filterEventListAfterResourceVersion(events, 10)

		assert.Equal(t, []v1.Event{
			{ObjectMeta: metav1.ObjectMeta{Name: "newer", ResourceVersion: "11"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "newest", ResourceVersion: "20"}},
		}, events.Items)
		assert.Equal(t, "100", events.ResourceVersion)
		assert.Equal(t, "next-page", events.Continue)
	})

	t.Run("no checkpoint skips the retained snapshot but preserves pagination", func(t *testing.T) {
		remaining := int64(50)
		events := &v1.EventList{
			ListMeta: metav1.ListMeta{
				ResourceVersion:    "100",
				Continue:           "next-page",
				RemainingItemCount: &remaining,
			},
			Items: []v1.Event{
				{ObjectMeta: metav1.ObjectMeta{Name: "retained", ResourceVersion: "99"}},
			},
		}

		filterEventListAfterResourceVersion(events, 0)

		assert.Nil(t, events.Items)
		assert.Equal(t, "100", events.ResourceVersion)
		assert.Equal(t, "next-page", events.Continue)
		assert.Equal(t, &remaining, events.RemainingItemCount)
	})
}
