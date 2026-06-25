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

// eventChanCapacity bounds how many events may queue between drains. A full
// channel drops new events rather than blocking the informer's delivery
// goroutine; see enqueue.
const eventChanCapacity = 1000

// EventCollector watches Kubernetes events through a shared informer and
// buffers them for a periodic consumer to drain. The informer's reflector owns
// resource-version tracking and resync, so this type holds none of the manual
// watermarking that RunEventCollection required.
//
// An EventCollector is single-use: create a new one each time the informer is
// (re)started, because a SharedInformerFactory cannot be restarted once its
// stop channel is closed.
type EventCollector struct {
	factory informers.SharedInformerFactory
	startTS time.Time

	// events buffers collected events between drains. The informer's single
	// delivery goroutine sends; Drain receives. dropped counts events shed when
	// the channel was full, for the check to surface as telemetry.
	events  chan *v1.Event
	dropped atomic.Uint64
}

// NewEventCollector returns an EventCollector whose informer lists/watches
// events matching filter (a field selector, e.g. the value built from
// filtered_event_types).
func (c *APIClient) NewEventCollector(filter string) *EventCollector {
	factory := c.GetInformerWithOptions(nil, informers.WithTweakListOptions(func(opts *metav1.ListOptions) {
		opts.FieldSelector = filter
	}))
	return &EventCollector{
		factory: factory,
		events:  make(chan *v1.Event, eventChanCapacity),
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

// Drain returns the events buffered since the last call, leaving the channel
// empty. It never blocks: it pulls only what is currently queued.
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

// Dropped returns the cumulative count of events shed because the buffer was
// full, so the check can surface it as telemetry.
func (ec *EventCollector) Dropped() uint64 {
	return ec.dropped.Load()
}

// enqueue buffers an event surfaced by the informer, dropping anything
// shouldCollect rejects. The send is non-blocking: a full buffer drops the
// event rather than stalling the informer's delivery goroutine.
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

// shouldCollect reports whether an event the informer surfaced should be
// buffered for emission.
func (ec *EventCollector) shouldCollect(ev *v1.Event) bool {
	return ev.LastTimestamp.After(ec.startTS) || ev.EventTime.Time.After(ec.startTS)
}
