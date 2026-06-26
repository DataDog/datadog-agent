// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func makeCollector(bufferSize int, startTS time.Time) *EventCollector {
	return &EventCollector{
		events:  make(chan *v1.Event, bufferSize),
		startTS: startTS,
		dropped: atomic.NewUint64(0),
	}
}

// TestShouldCollect verifies collection is gated on at least one timestamp being after startTS.
func TestShouldCollect(t *testing.T) {
	now := time.Now()
	future := metav1.NewTime(now.Add(time.Second))
	past := metav1.NewTime(now.Add(-time.Second))
	ec := makeCollector(1, now)

	for _, tc := range []struct {
		name string
		ev   *v1.Event
		want bool
	}{
		{
			name: "LastTimestamp after startTS, EventTime zero: collect",
			ev:   &v1.Event{LastTimestamp: future},
			want: true,
		},
		{
			name: "EventTime after startTS, LastTimestamp zero: collect",
			ev:   &v1.Event{EventTime: metav1.MicroTime(future)},
			want: true,
		},
		{
			name: "both timestamps before startTS: drop",
			ev:   &v1.Event{LastTimestamp: past, EventTime: metav1.MicroTime(past)},
			want: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ec.shouldCollect(tc.ev))
		})
	}
}

// TestDrain verifies Drain returns all buffered events and leaves the channel empty.
func TestDrain(t *testing.T) {
	startTS := time.Now().Add(-time.Second)

	t.Run("empty channel: nil", func(t *testing.T) {
		ec := makeCollector(10, startTS)
		assert.Nil(t, ec.Drain())
	})

	t.Run("one event: returns slice of length one", func(t *testing.T) {
		ec := makeCollector(10, startTS)
		ev := &v1.Event{Reason: "OOM"}
		ec.events <- ev
		assert.Equal(t, []*v1.Event{ev}, ec.Drain())
		assert.Nil(t, ec.Drain())
	})

	t.Run("multiple events: returns all and clears", func(t *testing.T) {
		ec := makeCollector(10, startTS)
		ev1 := &v1.Event{Reason: "OOM"}
		ev2 := &v1.Event{Reason: "Evicted"}
		ec.events <- ev1
		ec.events <- ev2

		assert.Equal(t, []*v1.Event{ev1, ev2}, ec.Drain())
		assert.Nil(t, ec.Drain())
	})
}

// TestEnqueue verifies enqueue filters by type and timestamp, buffers qualifying events, and counts drops.
func TestEnqueue(t *testing.T) {
	startTS := time.Now().Add(-time.Second)
	recent := metav1.NewTime(time.Now())
	ev := &v1.Event{Reason: "OOM", LastTimestamp: recent}

	t.Run("non-Event obj: skipped, no drop counted", func(t *testing.T) {
		ec := makeCollector(10, startTS)
		ec.enqueue("not an event")
		assert.Equal(t, uint64(0), ec.Dropped())
		assert.Nil(t, ec.Drain())
	})

	t.Run("shouldCollect false: skipped, no drop counted", func(t *testing.T) {
		ec := makeCollector(10, startTS)
		stale := &v1.Event{Reason: "OOM", LastTimestamp: metav1.NewTime(startTS.Add(-time.Minute))}
		ec.enqueue(stale)
		assert.Equal(t, uint64(0), ec.Dropped())
		assert.Nil(t, ec.Drain())
	})

	t.Run("buffer not full: event buffered, Dropped stays zero", func(t *testing.T) {
		ec := makeCollector(1, startTS)
		ec.enqueue(ev)
		assert.Equal(t, uint64(0), ec.Dropped())
		assert.Len(t, ec.Drain(), 1)
	})

	t.Run("buffer full: event dropped, Dropped incremented", func(t *testing.T) {
		ec := makeCollector(1, startTS)
		ec.events <- ev // fill the buffer
		ec.enqueue(ev)
		assert.Equal(t, uint64(1), ec.Dropped())
	})
}
