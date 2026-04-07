// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hook

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

const subsystem = "hooks"

var (
	hooksGauge           = telemetry.NewGauge(subsystem, "hooks_gauge", []string{"hook_name"}, "The number of hooks")
	dropperCounter       = telemetry.NewCounter(subsystem, "drops_counter", []string{"hook_name", "consumer_name"}, "The number of payloads dropped on the consumer side")
	hooksSubscribedGauge = telemetry.NewGauge(subsystem, "subscribed_callbacks_gauge", []string{"hook_name"}, "The number of callbacks subscribed to the hook")
)

// Option is a functional option for [Hook.Subscribe].
type Option[T any] func(*consumer[T])

// WithBufferSize sets the subscriber's channel capacity.
// The default is 100.  Use a larger value for consumers that can temporarily
// fall behind producers (e.g. a Unix-socket writer) and must absorb bursts
// without dropping payloads.
func WithBufferSize[T any](n int) Option[T] {
	return func(c *consumer[T]) {
		c.bufferSize = n
	}
}

// WithRecycle configures pool-based recycling for a subscriber.
//
// clone is called by Publish to create a private copy of the payload for
// this subscriber before it is enqueued.  Each subscriber therefore owns its
// own copy of the data and can safely use it after other subscribers have
// finished with theirs.
//
// recycle is called by the subscriber goroutine immediately after the
// callback returns, so the copy can be returned to a pool.
//
// Together, clone+recycle eliminate heap allocations on the steady-state
// delivery path: the same memory is reused across calls with no GC pressure.
//
// Example ([]MetricSampleSnapshot with a sync.Pool):
//
//	pool := sync.Pool{New: func() any {
//	    return make([]hook.MetricSampleSnapshot, 0, 32)
//	}}
//	unsub := h.Subscribe("observer", process,
//	    hook.WithRecycle(
//	        func(src []hook.MetricSampleSnapshot) []hook.MetricSampleSnapshot {
//	            dst := pool.Get().([]hook.MetricSampleSnapshot)
//	            return append(dst[:0], src...)
//	        },
//	        func(b []hook.MetricSampleSnapshot) { pool.Put(b[:0]) },
//	    ),
//	)
func WithRecycle[T any](clone func(T) T, recycle func(T)) Option[T] {
	return func(c *consumer[T]) {
		c.clone = clone
		c.recycle = recycle
	}
}

// Hook is a named, typed publish/subscribe channel.
//
// Producers call Publish to broadcast a payload to all registered subscribers.
// Consumers call Subscribe to register a callback that runs in its own
// goroutine.  Multiple producers and consumers are supported concurrently.
type Hook[T any] interface {
	// Name returns the hook's identifier as supplied to NewHook.
	Name() string

	// Publish delivers payload to every subscriber that currently has space in
	// its buffer.  Subscribers whose buffer is full silently drop this payload.
	// Publish never blocks regardless of consumer state.
	//
	// producerName identifies the call site for telemetry purposes only and
	// does not affect routing.
	Publish(producerName string, payload T)

	// HasSubscribers reports whether at least one consumer is currently subscribed.
	// Producers can use this to skip payload construction when no one is listening:
	//
	//	if h.HasSubscribers() {
	//	    h.Publish("producer", expensiveSnapshot())
	//	}
	HasSubscribers() bool

	// Subscribe registers callback as a consumer of this hook.
	// name must be unique among active subscribers on this hook; panics if already in use.
	// The callback is invoked from a dedicated goroutine; it must return
	// promptly and must not retain the payload beyond its own return.
	// The returned function unsubscribes the consumer and terminates its goroutine.
	Subscribe(consumerName string, callback func(payload T), opts ...Option[T]) (unsubscribe func())
}

// NewHook creates a new Hook that fans out published payloads to all subscribers.
// Publish is synchronous: it delivers to each subscriber's buffered channel inline,
// dropping the payload for any subscriber whose channel is full.
func NewHook[T any](name string) Hook[T] {
	hooksGauge.Inc(name)
	return &hook[T]{
		name:      name,
		consumers: make(map[string]consumer[T]),
		ctx:       context.Background(),
	}
}

// consumer holds everything the hook needs per-subscriber.
// dropLabel is pre-allocated at subscribe time so Publish's hot path
// spreads an existing slice rather than building a new one on each call.
type consumer[T any] struct {
	ch         chan T
	done       chan struct{}
	dropLabel  []string  // == []string{hookName, consumerName}
	bufferSize int       // channel capacity; 0 means use defaultBufferSize
	clone      func(T) T // optional: creates a private copy before channel send
	recycle    func(T)   // optional: returns the copy to a pool after callback
}

type hook[T any] struct {
	ctx             context.Context
	name            string
	mu              sync.RWMutex
	consumers       map[string]consumer[T]
	subscriberCount atomic.Int32
}

func (h *hook[T]) Name() string {
	return h.name
}

func (h *hook[T]) HasSubscribers() bool {
	return h.subscriberCount.Load() > 0
}

// Publish delivers payload to every subscriber's buffered channel.
// If a subscriber has a clone function, a private copy is made before
// enqueuing so each subscriber owns independent data.
// If a subscriber's channel is full the payload (or its clone) is dropped
// for that subscriber only, and its recycle function is called if set.
// Returns immediately with no lock when there are no subscribers.
func (h *hook[T]) Publish(_ string, payload T) {
	if h.subscriberCount.Load() == 0 {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, c := range h.consumers {
		item := payload
		if c.clone != nil {
			item = c.clone(payload)
		}
		select {
		case c.ch <- item:
		default:
			if c.recycle != nil {
				c.recycle(item)
			}
			dropperCounter.Inc(c.dropLabel...)
		}
	}
}

// Subscribe subscribes to the hook and calls callback with each payload.
// name must be unique among active subscribers on this hook; panics if already in use.
// The returned unsubscribe function stops delivery and terminates the consumer goroutine.
const defaultBufferSize = 100

func (h *hook[T]) Subscribe(name string, callback func(payload T), opts ...Option[T]) (unsubscribe func()) {
	c := consumer[T]{
		done:      make(chan struct{}),
		dropLabel: []string{h.name, name},
	}
	for _, opt := range opts {
		opt(&c)
	}
	if c.bufferSize == 0 {
		c.bufferSize = defaultBufferSize
	}
	c.ch = make(chan T, c.bufferSize)

	h.mu.Lock()
	if _, exists := h.consumers[name]; exists {
		h.mu.Unlock()
		panic(fmt.Sprintf("hook %q: consumer %q is already subscribed", h.name, name))
	}
	h.consumers[name] = c
	h.subscriberCount.Add(1)
	h.mu.Unlock()

	hooksSubscribedGauge.Inc(h.name)

	go func() {
		for {
			select {
			case <-h.ctx.Done():
				return
			case <-c.done:
				return
			case payload := <-c.ch:
				callback(payload)
				if c.recycle != nil {
					c.recycle(payload)
				}
			}
		}
	}()

	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.consumers[name]; ok {
			delete(h.consumers, name)
			h.subscriberCount.Add(-1)
			close(c.done)
			hooksSubscribedGauge.Dec(h.name)
		}
	}
}
