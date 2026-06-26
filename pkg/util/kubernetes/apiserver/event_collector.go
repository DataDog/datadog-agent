// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"sync/atomic"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EventCollector watches Kubernetes events through a shared informer and
// buffers them for a periodic consumer to drain.
type EventCollector struct {
	factory informers.SharedInformerFactory
	startTS time.Time

	events  chan *v1.Event
	dropped atomic.Uint64
}

// NewEventCollector returns an EventCollector whose informer lists/watches
// events matching filter (a field selector). Requires that the bufferSize be greater than 0.
func (c *APIClient) NewEventCollector(filter string, bufferSize int) *EventCollector {
	if bufferSize <= 0 {
		log.Errorf("Event collection buffer size must be greater than 0, got %d", bufferSize)
		return nil
	}
	factory := c.GetInformerWithOptions(nil, informers.WithTweakListOptions(func(opts *metav1.ListOptions) {
		opts.FieldSelector = filter
	}))
	return &EventCollector{
		factory: factory,
		events:  make(chan *v1.Event, bufferSize),
	}
}

// Start registers the events informer and its handlers, starts it, and blocks
// until its cache has synced. The informer runs until stopCh is closed.
func (ec *EventCollector) Start(stopCh <-chan struct{}) error {
	ec.startTS = time.Now()

	eventsInformer := ec.factory.Core().V1().Events()
	if _, err := eventsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    ec.enqueue,
		UpdateFunc: func(_, newObj interface{}) { ec.enqueue(newObj) },
	}); err != nil {
		return err
	}

	go eventsInformer.Informer().Run(stopCh)

	return SyncInformers(map[InformerName]cache.SharedInformer{
		"kubernetes-events": eventsInformer.Informer(),
	}, 0)
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
