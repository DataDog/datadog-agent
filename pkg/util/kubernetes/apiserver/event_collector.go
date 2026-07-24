// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"context"
	"strconv"
	"sync"

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

	// lastRV is the relist-dedup watermark, seeded from the persisted checkpoint on start.
	lastRV *atomic.Uint64
	// maxDrainedRV is the highest resourceVersion delivered, persisted as the restart checkpoint.
	maxDrainedRV *atomic.Uint64

	events  chan *v1.Event
	dropped *atomic.Uint64

	// unbounded, when set, routes enqueue/Drain through buf/bufMu instead of events,
	// trading the fixed capacity (and its drops) for unbounded memory growth.
	unbounded bool
	bufMu     sync.Mutex
	buf       []*v1.Event
}

// NewEventCollector returns an EventCollector whose Reflector lists/watches events
// matching filter (a field selector). If unbounded is true, bufferSize is ignored and
// the buffer is never full: events are never dropped, but memory is unbounded. Otherwise
// bufferSize must be greater than 0.
func (c *APIClient) NewEventCollector(filter string, bufferSize int, unbounded bool) *EventCollector {
	if !unbounded && bufferSize <= 0 {
		log.Errorf("Event collection buffer size must be greater than 0, got %d", bufferSize)
		return nil
	}
	ec := &EventCollector{
		client:       c.InformerCl,
		filter:       filter,
		lastRV:       atomic.NewUint64(0),
		maxDrainedRV: atomic.NewUint64(0),
		dropped:      atomic.NewUint64(0),
		unbounded:    unbounded,
	}
	if !unbounded {
		ec.events = make(chan *v1.Event, bufferSize)
	}
	return ec
}

// SetCheckpoint seeds the relist-dedup watermark from a persisted resourceVersion
// so the initial list after a restart forwards only events created since it,
// rather than re-listing the whole retained backlog. Call before Start.
func (ec *EventCollector) SetCheckpoint(rv string) {
	parsed := parseResourceVersion(rv)
	ec.lastRV.Store(parsed)
	ec.maxDrainedRV.Store(parsed)
}

// Checkpoint returns the highest delivered resourceVersion, for persisting so a
// restart or leader handoff resumes from here instead of from scratch.
func (ec *EventCollector) Checkpoint() string {
	return strconv.FormatUint(ec.maxDrainedRV.Load(), 10)
}

// Start builds the events Reflector and runs it until stopCh is closed. The
// Reflector lists then watches events matching the field selector, forwarding
// them to the buffer through eventReflectorStore.
func (ec *EventCollector) Start(stopCh <-chan struct{}) error {
	lw := &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = ec.filter
			return ec.client.CoreV1().Events(metav1.NamespaceAll).List(ctx, opts)
		},
		WatchFuncWithContext: func(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = ec.filter
			return ec.client.CoreV1().Events(metav1.NamespaceAll).Watch(ctx, opts)
		},
	}

	store := &eventReflectorStore{enqueue: ec.enqueue, watermark: ec.lastRV}
	reflector := cache.NewReflector(noWatchListLW{lw}, &v1.Event{}, store, 0)
	go reflector.Run(stopCh)

	return nil
}

// Drain returns the events buffered since the last call, advancing the
// delivered-resourceVersion checkpoint.
func (ec *EventCollector) Drain() []*v1.Event {
	drained := ec.drain()
	for _, ev := range drained {
		if rv := parseResourceVersion(ev.ResourceVersion); rv > ec.maxDrainedRV.Load() {
			ec.maxDrainedRV.Store(rv)
		}
	}
	return drained
}

// drain empties the buffer without touching maxDrainedRV.
func (ec *EventCollector) drain() []*v1.Event {
	if ec.unbounded {
		ec.bufMu.Lock()
		defer ec.bufMu.Unlock()
		drained := ec.buf
		ec.buf = nil
		return drained
	}

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

// noWatchListLW opts the events Reflector out of WatchList, forcing a
// deterministic List+Watch initial sync instead of a bookmark-terminated stream.
type noWatchListLW struct {
	*cache.ListWatch
}

// IsWatchListSemanticsUnSupported is read structurally by client-go's reflector.
func (noWatchListLW) IsWatchListSemanticsUnSupported() bool { return true }

// enqueue buffers an event. If unbounded, it always succeeds; otherwise it drops
// the event (and counts the drop) if the buffer is full.
func (ec *EventCollector) enqueue(ev *v1.Event) {
	if ec.unbounded {
		ec.bufMu.Lock()
		ec.buf = append(ec.buf, ev)
		ec.bufMu.Unlock()
		return
	}

	select {
	case ec.events <- ev:
	default:
		ec.dropped.Add(1)
	}
}
