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

// eventReflectorStore is the cache.Store the events Reflector writes into. It
// keeps no objects: Add/Update/Replace forward events through enqueue and the
// read methods are inert. The watermark, shared with the owning EventCollector,
// is the highest resourceVersion forwarded; it dedups relists (where Replace
// re-delivers the full live set) and is persisted for restart recovery.
type eventReflectorStore struct {
	enqueue   func(*v1.Event)
	watermark *atomic.Uint64
	seeded    bool
}

// forwardIfNew forwards an object if it is an *v1.Event whose resourceVersion
// exceeds the watermark, advancing the mark. The Reflector serializes all store
// calls (resync is disabled), so the Load-then-Store needs no lock: one writer.
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

// Replace handles the initial list and every relist. On a cold start, we set the watermark to the list's resourceVersion.
// Otherwise on a relist, or the first list after a restart, we only forward events past the watermark.
func (s *eventReflectorStore) Replace(list []interface{}, resourceVersion string) error {
	if s.seeded || s.watermark.Load() > 0 {
		for _, obj := range list {
			s.forwardIfNew(obj)
		}
	}
	s.seeded = true
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
