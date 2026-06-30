// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"context"
	"strconv"

	"go.uber.org/atomic"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EventCollector watches Kubernetes events through a Reflector and
// buffers them for a periodic consumer to drain.
type EventCollector struct {
	client kubernetes.Interface
	filter string

	// lastRV is the highest resourceVersion forwarded so far
	lastRV *atomic.Uint64

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
		lastRV:  atomic.NewUint64(0),
		events:  make(chan *v1.Event, bufferSize),
		dropped: atomic.NewUint64(0),
	}
}

// SetResourceVersion sets the highest forwarded resourceVersion.
func (ec *EventCollector) SetResourceVersion(rv string) {
	ec.lastRV.Store(parseResourceVersion(rv))
}

// ResourceVersion returns the highest forwarded resourceVersion.
func (ec *EventCollector) ResourceVersion() string {
	return strconv.FormatUint(ec.lastRV.Load(), 10)
}

// Start builds the events Reflector and runs it until stopCh is closed. The
// Reflector lists then watches events matching the field selector, forwarding
// them to the buffer through eventReflectorStore.
func (ec *EventCollector) Start(stopCh <-chan struct{}) error {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-stopCh
		cancel()
	}()

	lw := &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = ec.filter
			return ec.client.CoreV1().Events(metav1.NamespaceAll).List(ctx, opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = ec.filter
			return ec.client.CoreV1().Events(metav1.NamespaceAll).Watch(ctx, opts)
		},
	}

	store := &eventReflectorStore{enqueue: ec.enqueue, watermark: ec.lastRV}
	reflector := cache.NewReflector(lw, &v1.Event{}, store, 0)
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

// DrainDropped returns the number of events dropped since the last call, resetting the counter.
func (ec *EventCollector) DrainDropped() uint64 {
	return ec.dropped.Swap(0)
}

// enqueue buffers an event, dropping it (and counting the drop) if the buffer is full.
func (ec *EventCollector) enqueue(ev *v1.Event) {
	select {
	case ec.events <- ev:
	default:
		ec.dropped.Add(1)
	}
}
