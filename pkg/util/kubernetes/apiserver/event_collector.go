// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"context"
	"time"

	"go.uber.org/atomic"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EventCollector watches Kubernetes events through a client-go Reflector and
// buffers them for a periodic consumer to drain.
type EventCollector struct {
	client  kubernetes.Interface
	filter  string
	startTS time.Time

	events  chan *v1.Event
	dropped *atomic.Uint64
}

// NewEventCollector returns an EventCollector whose Reflector lists/watches
// events matching filter (a field selector). Requires that the bufferSize be greater than 0.
func (c *APIClient) NewEventCollector(filter string, bufferSize int) *EventCollector {
	if bufferSize <= 0 {
		log.Errorf("Event collection buffer size must be greater than 0, got %d", bufferSize)
		return nil
	}
	return &EventCollector{
		client:  c.InformerCl,
		filter:  filter,
		events:  make(chan *v1.Event, bufferSize),
		dropped: atomic.NewUint64(0),
	}
}

// Start builds the events Reflector and runs it until stopCh is closed. The
// Reflector lists then watches events matching the field selector, forwarding
// them to the buffer through eventReflectorStore.
func (ec *EventCollector) Start(stopCh <-chan struct{}) error {
	ec.startTS = time.Now()

	lw := &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = ec.filter
			return ec.client.CoreV1().Events(metav1.NamespaceAll).List(context.TODO(), opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = ec.filter
			return ec.client.CoreV1().Events(metav1.NamespaceAll).Watch(context.TODO(), opts)
		},
	}

	reflector := cache.NewReflector(lw, &v1.Event{}, &eventReflectorStore{enqueue: ec.enqueue}, 0)
	go reflector.Run(stopCh)

	return nil
}

// Drain returns the events buffered since the last call.
func (ec *EventCollector) Drain() []*v1.Event {
	var drained []*v1.Event
	for {
		select {
		case ev := <-ec.events:
			drained = append(drained, ev)
		default:
			return drained
		}
	}
}

// Dropped returns the number of events dropped due to the buffer being full.
func (ec *EventCollector) Dropped() uint64 {
	return ec.dropped.Load()
}

// enqueue adds an event to the buffer. If the event's lastTimestamp and EventTime are before the startTS, it is not collected.
// If the buffer is full, the event is dropped.
func (ec *EventCollector) enqueue(obj interface{}) {
	ev, ok := obj.(*v1.Event)
	if !ok {
		log.Errorf("Expected *v1.Event, got %T; skipping", obj)
		return
	}

	if !ec.shouldCollect(ev) {
		return
	}

	select {
	case ec.events <- ev:
	default:
		ec.dropped.Add(1)
	}
}

// shouldCollect reports whether an event should be collected.
func (ec *EventCollector) shouldCollect(ev *v1.Event) bool {
	return ev.LastTimestamp.After(ec.startTS) || ev.EventTime.Time.After(ec.startTS)
}
