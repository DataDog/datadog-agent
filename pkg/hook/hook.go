// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hook

import (
	"context"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

const subsystem = "hooks"

var (
	hooksGauge           = telemetry.NewGauge(subsystem, "hooks_gauge", []string{"hook_name"}, "The number of hooks")
	dropperCounter       = telemetry.NewCounter(subsystem, "drops_counter", []string{"hook_name", "consumer_name"}, "The number of payloads dropped on the consumer side")
	hooksSubscribedGauge = telemetry.NewGauge(subsystem, "subscribed_callbacks_gauge", []string{"hook_name"}, "The number of callbacks subscribed to the hook")
)

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

	// Subscribe registers callback as a consumer of this hook.
	// name must be unique among active subscribers on this hook.
	// The callback is invoked from a dedicated goroutine; it must return
	// promptly and must not retain the payload beyond its own return.
	// The returned function unsubscribes the consumer and terminates its goroutine.
	Subscribe(consumerName string, callback func(payload T)) (unsubscribe func())
}

// noopHook is a Hook implementation that discards all published payloads and ignores subscriptions.
type noopHook[T any] struct{}

func (noopHook[T]) Name() string                                       { return "noop" }
func (noopHook[T]) Publish(_ string, _ T)                              {}
func (noopHook[T]) Subscribe(_ string, _ func(T)) (unsubscribe func()) { return func() {} }

// NewNoopHook returns a Hook that silently discards all published payloads.
// Use this in tests or in code paths where hook observation is optional.
func NewNoopHook[T any]() Hook[T] { return noopHook[T]{} }

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
	ch        chan T
	done      chan struct{}
	dropLabel []string // == []string{hookName, consumerName}
}

type hook[T any] struct {
	ctx       context.Context
	name      string
	mu        sync.RWMutex
	consumers map[string]consumer[T]
}

func (h *hook[T]) Name() string {
	return h.name
}

// Publish delivers payload to every subscriber's buffered channel.
// If a subscriber's channel is full the payload is dropped for that subscriber only.
// The hot path (channel has space) allocates 0 bytes.
func (h *hook[T]) Publish(_ string, payload T) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, c := range h.consumers {
		select {
		case c.ch <- payload:
		default:
			dropperCounter.Inc(c.dropLabel...)
		}
	}
}

// Subscribe subscribes to the hook and calls callback with each payload.
// name must be unique per consumer.
// The returned unsubscribe function stops delivery and terminates the consumer goroutine.
func (h *hook[T]) Subscribe(name string, callback func(payload T)) (unsubscribe func()) {
	c := consumer[T]{
		ch:        make(chan T, 100),
		done:      make(chan struct{}),
		dropLabel: []string{h.name, name},
	}
	hooksSubscribedGauge.Inc(h.name)

	h.mu.Lock()
	h.consumers[name] = c
	h.mu.Unlock()

	go func() {
		for {
			select {
			case <-h.ctx.Done():
				return
			case <-c.done:
				return
			case payload := <-c.ch:
				callback(payload)
			}
		}
	}()

	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.consumers[name]; ok {
			delete(h.consumers, name)
			close(c.done)
			hooksSubscribedGauge.Dec(h.name)
		}
	}
}
