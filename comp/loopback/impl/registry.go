// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package loopbackimpl

import (
	"slices"
	"strings"
	"sync"

	"github.com/twmb/murmur3"
)

// contextEntry holds the canonical name and sorted tags for a context key.
type contextEntry struct {
	name string
	tags []string // sorted, owned copy
}

// contextRegistry is a concurrent-safe mapping between context key ↔ name+tags.
// It is populated on the write path and queried on the Flush path.
type contextRegistry struct {
	mu     sync.RWMutex
	byKey  map[uint64]contextEntry
	byName map[string][]uint64 // name → context keys
}

func newContextRegistry() *contextRegistry {
	return &contextRegistry{
		byKey:  make(map[uint64]contextEntry),
		byName: make(map[string][]uint64),
	}
}

// register computes a synthetic key for a sample whose ContextKey=0
// (check sampler / no-aggregation pipeline) and registers the mapping.
// If the name+tags combination is already registered it returns the existing key.
func (r *contextRegistry) register(name string, tags []string) uint64 {
	sorted := sortedTagsCopy(tags)
	key := syntheticKey(name, sorted)
	r.registerWithKey(key, name, tags)
	return key
}

// registerWithKey stores the mapping for a known context key.
// Uses double-checked locking: the fast path (already registered) takes RLock only.
func (r *contextRegistry) registerWithKey(key uint64, name string, tags []string) {
	r.mu.RLock()
	_, exists := r.byKey[key]
	r.mu.RUnlock()
	if exists {
		return
	}
	sorted := sortedTagsCopy(tags)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists = r.byKey[key]; exists {
		return
	}
	r.byKey[key] = contextEntry{name: name, tags: sorted}
	r.byName[name] = append(r.byName[name], key)
}

// registerEntryLocked stores an entry without lock acquisition.
// Must be called with r.mu held for writing. Used during catalog load at startup.
func (r *contextRegistry) registerEntryLocked(key uint64, name string, tags []string) {
	if _, exists := r.byKey[key]; exists {
		return
	}
	sorted := sortedTagsCopy(tags)
	r.byKey[key] = contextEntry{name: name, tags: sorted}
	r.byName[name] = append(r.byName[name], key)
}

// lookupKeys returns all context keys associated with the given name.
// If tags is non-nil, only keys whose registered tags contain all requested tags
// are returned (subset match). Returns nil when nothing matches.
func (r *contextRegistry) lookupKeys(name string, tags []string) []uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := r.byName[name]
	if len(keys) == 0 {
		return nil
	}
	if tags == nil {
		out := make([]uint64, len(keys))
		copy(out, keys)
		return out
	}
	var out []uint64
	for _, k := range keys {
		entry := r.byKey[k]
		if tagsSubset(tags, entry.tags) {
			out = append(out, k)
		}
	}
	return out
}

// getEntry returns the name and tags for a context key.
func (r *contextRegistry) getEntry(key uint64) (name string, tags []string, ok bool) {
	r.mu.RLock()
	e, ok := r.byKey[key]
	r.mu.RUnlock()
	return e.name, e.tags, ok
}

// allEntries returns a snapshot of all registered entries. Used for catalog compaction.
func (r *contextRegistry) allEntries() map[uint64]contextEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[uint64]contextEntry, len(r.byKey))
	for k, v := range r.byKey {
		out[k] = v
	}
	return out
}

// syntheticKey computes a stable murmur3 key for samples without a pre-assigned
// context key (ContextKey=0 pipelines). tags must already be sorted.
func syntheticKey(name string, sortedTags []string) uint64 {
	return murmur3.Sum64([]byte(name + "|" + strings.Join(sortedTags, ",")))
}

func sortedTagsCopy(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, len(tags))
	copy(out, tags)
	slices.Sort(out)
	return out
}

// tagsSubset reports whether every tag in requested is present in registered.
func tagsSubset(requested, registered []string) bool {
	for _, req := range requested {
		if !slices.Contains(registered, req) {
			return false
		}
	}
	return true
}
