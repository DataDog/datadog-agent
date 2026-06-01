// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listeners

import "sync"

// StaticConfigIndex is a refcounted set of integration names that currently
// have at least one scheduled non-template (static) *check* config — a config
// with Instances. Logs-only static configs are intentionally excluded since
// they do not configure a check and must not suppress dynamic check templates
// of the same integration. It is shared between the autodiscovery config
// manager (writer) and listeners that need to deduplicate against static
// configs (reader). Reads are safe to perform while the writer holds its own
// mutex; the index has its own RWMutex and must not be locked by any caller.
//
// A nil pointer is safe to use: Has always returns false, and Add/Remove are
// no-ops.
type StaticConfigIndex struct {
	mu     sync.RWMutex
	counts map[string]int
}

// NewStaticConfigIndex returns an empty index.
func NewStaticConfigIndex() *StaticConfigIndex {
	return &StaticConfigIndex{counts: map[string]int{}}
}

// Add increments the refcount for the given integration name. It returns true
// if the name transitioned from absent to present so callers can trigger
// reconciliation only on the meaningful edge.
func (i *StaticConfigIndex) Add(name string) bool {
	if i == nil {
		return false
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.counts[name]++
	return i.counts[name] == 1
}

// Remove decrements the refcount for the given integration name. It returns
// true if the name transitioned from present to absent. Remove on an absent
// name is a no-op and returns false.
func (i *StaticConfigIndex) Remove(name string) bool {
	if i == nil {
		return false
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.counts[name] == 0 {
		return false
	}
	i.counts[name]--
	if i.counts[name] == 0 {
		delete(i.counts, name)
		return true
	}
	return false
}

// Has reports whether at least one static config with the given integration
// name is currently scheduled.
func (i *StaticConfigIndex) Has(name string) bool {
	if i == nil {
		return false
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.counts[name] > 0
}
