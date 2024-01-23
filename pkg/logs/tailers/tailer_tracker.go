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
	panic("not called")
}

// All returns all active tailers.
func (t *TailerTracker) All() []Tailer {
	panic("not called")
}

// AnyTailerContainer is a type erased tailer container. This is used as a proxy for
// typed tailer containers to mix and match collections of tailers of any underlying type.
type AnyTailerContainer interface {
	Tailers() []Tailer
}

// TailerContainer is a container for a concrete tailer type.
type TailerContainer[T Tailer] struct {
	sync.RWMutex
	tailers map[string]T
}

// NewTailerContainer creates a new TailerContainer instance.
func NewTailerContainer[T Tailer]() *TailerContainer[T] {
	panic("not called")
}

// Get returns a tailer with the provided id if it exists.
func (t *TailerContainer[T]) Get(id string) (T, bool) {
	panic("not called")
}

// Contains returns true if the key exists.
func (t *TailerContainer[T]) Contains(id string) bool {
	panic("not called")
}

// Add adds a new tailer to the container.
func (t *TailerContainer[T]) Add(tailer T) {
	panic("not called")
}

// Remove removes a tailer from the container.
func (t *TailerContainer[T]) Remove(tailer T) {
	panic("not called")
}

// All returns a slice of all tailers in the container.
func (t *TailerContainer[T]) All() []T {
	panic("not called")
}

// Count returns the number of tailers in the container.
func (t *TailerContainer[T]) Count() int {
	panic("not called")
}

// Tailers returns a slice of all tailers in the container without their concrete types.
func (t *TailerContainer[T]) Tailers() []Tailer {
	panic("not called")
}
