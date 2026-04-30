// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot_test

import (
	"sync"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// delayedWLM wraps a workloadmeta.Component and introduces a fixed delay before
// delivering each event bundle to subscribers.
type delayedWLM struct {
	workloadmeta.Component
	delay time.Duration

	mu       sync.Mutex
	channels map[chan workloadmeta.EventBundle]delayedSubscription
}

type delayedSubscription struct {
	realCh chan workloadmeta.EventBundle
	done   chan struct{}
}

func newDelayedWLM(component workloadmeta.Component, delay time.Duration) *delayedWLM {
	return &delayedWLM{
		Component: component,
		delay:     delay,
		channels:  make(map[chan workloadmeta.EventBundle]delayedSubscription),
	}
}

// Subscribe returns a channel that receives the same event bundles as the real
// WLM channel but delayed by d.delay.
func (d *delayedWLM) Subscribe(name string, priority workloadmeta.SubscriberPriority, filter *workloadmeta.Filter) chan workloadmeta.EventBundle {
	realCh := d.Component.Subscribe(name, priority, filter)
	wrappedCh := make(chan workloadmeta.EventBundle, 100)
	done := make(chan struct{})

	d.mu.Lock()
	d.channels[wrappedCh] = delayedSubscription{realCh: realCh, done: done}
	d.mu.Unlock()

	go func() {
		defer close(wrappedCh)
		for {
			select {
			case bundle, ok := <-realCh:
				if !ok {
					return
				}
				time.Sleep(d.delay)
				select {
				case wrappedCh <- bundle:
				case <-done:
					return
				}
			case <-done:
				return
			}
		}
	}()

	return wrappedCh
}

// Unsubscribe stops the forwarding goroutine and unsubscribes from the real WLM.
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
