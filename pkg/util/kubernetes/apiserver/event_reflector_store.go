// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"strconv"

	"go.uber.org/atomic"
	v1 "k8s.io/api/core/v1"
)

// eventReflectorStore is a cache.Store that retains nothing: it forwards events
// through enqueue. The watermark (highest resourceVersion forwarded) dedups
// relists within the collector's lifetime.
type eventReflectorStore struct {
	enqueue   func(*v1.Event)
	watermark *atomic.Uint64
}

// forwardIfNew forwards an *v1.Event whose resourceVersion exceeds the watermark,
// advancing it. The Reflector serializes store calls, so no lock is needed.
func (s *eventReflectorStore) forwardIfNew(obj interface{}) {
	ev, ok := obj.(*v1.Event)
	if !ok {
		return
	}
	rv := parseResourceVersion(ev.ResourceVersion)
	if rv <= s.watermark.Load() {
		return
	}
	s.watermark.Store(rv)
	s.enqueue(ev)
}

// Add forwards a newly watched event.
func (s *eventReflectorStore) Add(obj interface{}) error { s.forwardIfNew(obj); return nil }

// Update forwards a modified event; handled like Add.
func (s *eventReflectorStore) Update(obj interface{}) error { s.forwardIfNew(obj); return nil }

// Delete drops a removed event. Removals are never forwarded to Datadog.
func (s *eventReflectorStore) Delete(_ interface{}) error { return nil }

// Replace handles the initial list and every relist. Lists are unordered, so we
// forward against a fixed threshold and advance the watermark after the loop.
func (s *eventReflectorStore) Replace(list []interface{}, resourceVersion string) error {
	threshold := s.watermark.Load()
	for _, obj := range list {
		ev, ok := obj.(*v1.Event)
		if !ok {
			continue
		}
		if parseResourceVersion(ev.ResourceVersion) > threshold {
			s.enqueue(ev)
		}
	}
	if listRV := parseResourceVersion(resourceVersion); listRV > s.watermark.Load() {
		s.watermark.Store(listRV)
	}
	return nil
}

// eventReflectorStore.List is not implemented.
func (s *eventReflectorStore) List() []interface{} { return nil }

// eventReflectorStore.ListKeys is not implemented.
func (s *eventReflectorStore) ListKeys() []string { return nil }

// eventReflectorStore.Get is not implemented.
func (s *eventReflectorStore) Get(_ interface{}) (interface{}, bool, error) { return nil, false, nil }

// eventReflectorStore.GetByKey is not implemented.
func (s *eventReflectorStore) GetByKey(_ string) (interface{}, bool, error) { return nil, false, nil }

// eventReflectorStore.Resync is not implemented.
func (s *eventReflectorStore) Resync() error { return nil }

// parseResourceVersion parses a Kubernetes resourceVersion. Unparseable values (e.g. empty) are 0.
func parseResourceVersion(rv string) uint64 {
	n, err := strconv.ParseUint(rv, 10, 64)
	if err != nil {
		return 0
	}
	return n
}
