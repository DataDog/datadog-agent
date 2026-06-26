// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the configstream component
package mock

import (
	"sync"
	"sync/atomic"
	"testing"

	configstream "github.com/DataDog/datadog-agent/comp/core/configstream/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// Component is a mock for the configstream component.
type Component struct {
	m           sync.Mutex
	subscribers map[string]chan *pb.ConfigEvent
	closed      atomic.Bool

	// SubscribedC is a channel that receives the request of any new subscriber.
	// Tests can use this to verify that a component has subscribed.
	SubscribedC chan *pb.ConfigStreamRequest

	// UnsubscribedC is a channel that is signaled when a client unsubscribes.
	UnsubscribedC chan struct{}
}

// Mock returns a new mock component.
func Mock(t *testing.T) configstream.Component {
	m := &Component{
		subscribers:   make(map[string]chan *pb.ConfigEvent),
		SubscribedC:   make(chan *pb.ConfigStreamRequest, 1),
		UnsubscribedC: make(chan struct{}, 1),
	}

	t.Cleanup(func() {
		m.Close()
	})

	return m
}

// Subscribe implements the component interface.
func (mock *Component) Subscribe(req *pb.ConfigStreamRequest) (<-chan *pb.ConfigEvent, func()) {
	ch := make(chan *pb.ConfigEvent, 100)

	mock.m.Lock()
	mock.subscribers[req.Name] = ch
	mock.m.Unlock()

	// Send outside the lock: SubscribedC has capacity 1 and a blocking send
	// while holding the lock would deadlock any concurrent Subscribe/Close call.
	if !mock.closed.Load() {
		mock.SubscribedC <- req
	}

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			mock.m.Lock()
			_, ok := mock.subscribers[req.Name]
			delete(mock.subscribers, req.Name)
			mock.m.Unlock()

			if ok {
				close(ch)
				if !mock.closed.Load() {
					mock.UnsubscribedC <- struct{}{}
				}
			}
		})
	}

	return ch, unsubscribe
}

// SendEvent is a helper for tests to simulate broadcasting an event.
func (mock *Component) SendEvent(event *pb.ConfigEvent) {
	mock.m.Lock()
	defer mock.m.Unlock()

	for _, ch := range mock.subscribers {
		select {
		case ch <- event:
		default:
			// Don't block if the subscriber's channel is full
		}
	}
}

// Close cleans up the mock's resources.
func (mock *Component) Close() {
	mock.closed.Store(true)

	mock.m.Lock()
	subs := mock.subscribers
	mock.subscribers = make(map[string]chan *pb.ConfigEvent)
	mock.m.Unlock()

	// Close channels that were not already closed by their unsubscribe function.
	for _, ch := range subs {
		close(ch)
	}
	close(mock.SubscribedC)
	close(mock.UnsubscribedC)
}
