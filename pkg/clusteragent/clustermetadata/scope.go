// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustermetadata

import (
	"sync"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// RingScope adapts a RingController to the workloadmeta pod-watch
// interfaces: it is the single object crossing from the ring package into
// the kubeapiserver collector's dependencies.
type RingScope struct {
	controller *RingController

	mu          sync.Mutex
	subscribers map[int]func()
	nextID      int
}

var (
	_ workloadmeta.PodWatchScope    = (*RingScope)(nil)
	_ workloadmeta.NodeSyncReporter = (*RingScope)(nil)
)

// NewRingScope returns the scope adapter for the controller. The controller
// supports one owned-set callback, held by this adapter forever.
func NewRingScope(controller *RingController) *RingScope {
	scope := &RingScope{
		controller:  controller,
		subscribers: make(map[int]func()),
	}
	controller.SubscribeOwnedNodes(func(prev, next []string) {
		scope.notify()
	})
	return scope
}

// Nodes implements workloadmeta.PodWatchScope.
func (s *RingScope) Nodes() []string {
	return s.controller.State().MyNodes
}

// Subscribe implements workloadmeta.PodWatchScope: the current state is
// delivered immediately.
func (s *RingScope) Subscribe(fn func()) func() {
	s.mu.Lock()
	id := s.nextID
	s.nextID++
	s.subscribers[id] = fn
	s.mu.Unlock()

	fn()
	return func() {
		s.mu.Lock()
		delete(s.subscribers, id)
		s.mu.Unlock()
	}
}

// NodeSynced implements workloadmeta.NodeSyncReporter.
func (s *RingScope) NodeSynced(node string, synced bool) {
	s.controller.SetNodeSynced(node, synced)
}

func (s *RingScope) notify() {
	s.mu.Lock()
	fns := make([]func(), 0, len(s.subscribers))
	for _, fn := range s.subscribers {
		fns = append(fns, fn)
	}
	s.mu.Unlock()

	for _, fn := range fns {
		fn()
	}
}
