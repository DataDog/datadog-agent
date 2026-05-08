// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package stats holds the system-probe module statistics map.
package stats

import (
	"maps"
	"sync"
)

var (
	mu sync.Mutex
	m  = make(map[string]any)
)

// Get returns a snapshot of the current stats.
func Get() map[string]any {
	mu.Lock()
	defer mu.Unlock()
	return maps.Clone(m)
}

// Set stores a single stat by key.
func Set(key string, value any) {
	mu.Lock()
	defer mu.Unlock()
	m[key] = value
}

// Reset clears all stats.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	m = make(map[string]any)
}
