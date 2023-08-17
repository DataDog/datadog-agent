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

func SetGlobalStore(w Component) {
	globalStore.Set(w)
}

func GetGlobalStore() Component {
	return globalStore.Get()
}

func (m *metaStore) Set(w Component) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.wmeta = w
}

func (m *metaStore) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.wmeta = nil
}

func (m *metaStore) Get() Component {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.wmeta
}
