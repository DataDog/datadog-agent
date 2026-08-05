// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot_test

import (
	"context"
	"slices"
	"sync"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// delayedWLM wraps a workloadmeta.Component and introduces a fixed delay before
// delivering each event bundle to subscribers.
type delayedWLM struct {
	workloadmeta.Component
	ctx   context.Context
	delay time.Duration

	mu       sync.Mutex
	channels map[chan workloadmeta.EventBundle]delayedSubscription
}

type delayedSubscription struct {
	realCh chan workloadmeta.EventBundle
	done   chan struct{}
}

func newDelayedWLM(ctx context.Context, component workloadmeta.Component, delay time.Duration) *delayedWLM {
	return &delayedWLM{
		Component: component,
		ctx:       ctx,
		delay:     delay,
		channels:  make(map[chan workloadmeta.EventBundle]delayedSubscription),
	}
}

// Subscribe returns a channel that receives the same event bundles as the real
// WLM channel but delayed by d.delay.
func (d *delayedWLM) Subscribe(name string, priority workloadmeta.SubscriberPriority, filter *workloadmeta.Filter) chan workloadmeta.EventBundle {
	realCh := d.Component.Subscribe(name, priority, filter)
	wrappedCh := make(chan workloadmeta.EventBundle, 100)
	type delayedEvents struct {
		events    []workloadmeta.Event
		deliverAt time.Time
	}
	intermediate := make(chan delayedEvents, 100)
	done := make(chan struct{})

	d.mu.Lock()
	d.channels[wrappedCh] = delayedSubscription{realCh: realCh, done: done}
	d.mu.Unlock()

	// Producer: reads from realCh, acknowledges immediately, and stamps each
	// bundle with a delivery time before queuing it to intermediate.
	go func() {
		defer close(intermediate)
		for {
			select {
			case bundle, ok := <-realCh:
				if !ok {
					return
				}
				// Clone events before acknowledging: Acknowledge() signals the WLM
				// that it may recycle the bundle, so the slice must be copied first.
				events := slices.Clone(bundle.Events)
				bundle.Acknowledge()
				intermediate <- delayedEvents{events: events, deliverAt: time.Now().Add(d.delay)}
			case <-done:
				return
			case <-d.ctx.Done():
				return
			}
		}
	}()

	// Consumer: sleeps until each bundle's deliverAt before forwarding to wrappedCh,
	// preserving delivery order while applying the configured delay.
	go func() {
		for delayed := range intermediate {
			select {
			case <-time.After(time.Until(delayed.deliverAt)):
				wrappedCh <- workloadmeta.EventBundle{
					Ch:     make(chan struct{}),
					Events: delayed.events,
				}
			case <-d.ctx.Done():
				return
			}
		}
	}()

	return wrappedCh
}

// Unsubscribe stops the producer/consumer goroutines and unsubscribes from the real WLM.
func (d *delayedWLM) Unsubscribe(ch chan workloadmeta.EventBundle) {
	d.mu.Lock()
	sub, ok := d.channels[ch]
	if ok {
		delete(d.channels, ch)
	}
	d.mu.Unlock()

	if ok {
		close(sub.done)
		d.Component.Unsubscribe(sub.realCh)
	}
}
