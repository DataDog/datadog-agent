// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"strconv"

	v1 "k8s.io/api/core/v1"
)

// eventReflectorStore is a fake cache.Store that forwards events via a callback.
type eventReflectorStore struct {
	enqueue   func(interface{})
	highestRV uint64
	seeded    bool
}

// forwardIfNew forwards an object if it is castable as an *v1.Event, and only if its resourceVersion > s.highestRV.
func (s *eventReflectorStore) forwardIfNew(obj interface{}) {
	ev, ok := obj.(*v1.Event)
	if !ok {
		return
	}
	rv := parseResourceVersion(ev.ResourceVersion)
	if rv <= s.highestRV {
		return
	}
	s.highestRV = rv
	s.enqueue(ev)
}

// Add forwards a newly watched event.
func (s *eventReflectorStore) Add(obj interface{}) error { s.forwardIfNew(obj); return nil }

// Update forwards a modified event.
func (s *eventReflectorStore) Update(obj interface{}) error { s.forwardIfNew(obj); return nil }

// Delete does nothing.
func (s *eventReflectorStore) Delete(_ interface{}) error { return nil }

// Replace seeds the highestRV from the list's resourceVersion and forwards all events.
func (s *eventReflectorStore) Replace(list []interface{}, resourceVersion string) error {
	if s.seeded {
		for _, obj := range list {
			s.forwardIfNew(obj)
		}
	}
	s.seeded = true
	if listRV := parseResourceVersion(resourceVersion); listRV > s.highestRV {
		s.highestRV = listRV
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
