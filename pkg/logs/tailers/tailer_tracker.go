// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package tailers

import "sync"

// TailerTracker keeps track of all the active tailers in the agent.
type TailerTracker struct {
	sync.RWMutex
	containers []AnyTailerContainer
}

// NewTailerTracker creates a new TailerTracker instance.
func NewTailerTracker() *TailerTracker {
	return &TailerTracker{}
}

// Add a tailer container to the tracker.
func (t *TailerTracker) Add(container AnyTailerContainer) {
	t.Lock()
	defer t.Unlock()
	t.containers = append(t.containers, container)
}

// All returns all active tailers.
func (t *TailerTracker) All() []Tailer {
	t.RLock()
	defer t.RUnlock()
	tailers := []Tailer{}
	for _, container := range t.containers {
		tailers = append(tailers, container.Tailers()...)
	}
	return tailers
}

// AnyTailerContainer is a type erased tailer container. This is used as a proxy for
// typed tailer containers to mix and match collections of tailers of any underlying type.
type AnyTailerContainer interface {
	Tailers() []Tailer
}

// TailerContainer is a container for a concrete tailer type.
//
// Multiple tailers may share the same GetID() (for instance, when two log
// configurations accidentally point at the same file path). To make sure each
// tailer instance remains observable through the status endpoint, the
// container stores all tailers sharing an ID under a slice keyed by that ID.
// Get/Contains return the first tailer registered for an ID, and Remove
// removes the specific instance provided by the caller.
type TailerContainer[T Tailer] struct {
	sync.RWMutex
	tailers map[string][]T
}

// NewTailerContainer creates a new TailerContainer instance.
func NewTailerContainer[T Tailer]() *TailerContainer[T] {
	return &TailerContainer[T]{
		tailers: make(map[string][]T),
	}
}

// Get returns a tailer with the provided id if it exists. If multiple tailers
// share the same id, the first one added is returned.
func (t *TailerContainer[T]) Get(id string) (T, bool) {
	t.RLock()
	defer t.RUnlock()
	tailers, ok := t.tailers[id]
	if !ok || len(tailers) == 0 {
		var zero T
		return zero, false
	}
	return tailers[0], true
}

// Contains returns true if at least one tailer with the provided id exists.
func (t *TailerContainer[T]) Contains(id string) bool {
	t.RLock()
	defer t.RUnlock()
	tailers, ok := t.tailers[id]
	return ok && len(tailers) > 0
}

// Add adds a new tailer to the container. If another tailer with the same ID
// is already present, both are retained so that both remain visible in the
// agent status.
func (t *TailerContainer[T]) Add(tailer T) {
	t.Lock()
	defer t.Unlock()
	id := tailer.GetID()
	t.tailers[id] = append(t.tailers[id], tailer)
}

// Remove removes the given tailer instance from the container. Only the
// matching instance is removed; other tailers sharing the same ID are kept.
func (t *TailerContainer[T]) Remove(tailer T) {
	t.Lock()
	defer t.Unlock()
	id := tailer.GetID()
	tailers, ok := t.tailers[id]
	if !ok {
		return
	}
	// Tailer is an interface type, so its type set is not statically
	// comparable; compare instance identity via the empty interface, which
	// uses the underlying dynamic type's == operator at runtime (pointer
	// equality for the concrete tailer pointer types this container holds).
	target := any(tailer)
	for i, existing := range tailers {
		if any(existing) == target {
			tailers = append(tailers[:i], tailers[i+1:]...)
			break
		}
	}
	if len(tailers) == 0 {
		delete(t.tailers, id)
	} else {
		t.tailers[id] = tailers
	}
}

// All returns a slice of all tailers in the container.
func (t *TailerContainer[T]) All() []T {
	t.RLock()
	defer t.RUnlock()
	tailers := make([]T, 0, len(t.tailers))
	for _, bucket := range t.tailers {
		tailers = append(tailers, bucket...)
	}
	return tailers
}

// Count returns the number of tailers in the container.
func (t *TailerContainer[T]) Count() int {
	t.RLock()
	defer t.RUnlock()
	count := 0
	for _, bucket := range t.tailers {
		count += len(bucket)
	}
	return count
}

// Tailers returns a slice of all tailers in the container without their concrete types.
func (t *TailerContainer[T]) Tailers() []Tailer {
	t.RLock()
	defer t.RUnlock()
	tailers := make([]Tailer, 0, len(t.tailers))
	for _, bucket := range t.tailers {
		for _, tailer := range bucket {
			tailers = append(tailers, tailer)
		}
	}
	return tailers
}
