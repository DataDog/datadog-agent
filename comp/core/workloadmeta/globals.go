// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"sync"
)

type metaStore struct {
	mu    sync.RWMutex
	wmeta Component
}

var globalStore metaStore

// SetGlobalStore sets the global workloadmeta instance
func SetGlobalStore(w Component) {
	globalStore.Set(w)
}

// GetGlobalStore returns the global workloadmeta instance
func GetGlobalStore() Component {
	return globalStore.Get()
}

// Set sets the workloadmeta component instance in the global `metaStore` in a threadsafe manner
func (m *metaStore) Set(w Component) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.wmeta = w
}

// Reset resets the workloadmeta component instance in the global `metaStore` to nil in a threadsafe manner
func (m *metaStore) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.wmeta = nil
}

// Get grabs the workloadmeta component instance in the global `metaStore` variable.
func (m *metaStore) Get() Component {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.wmeta
}
