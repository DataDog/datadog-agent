// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package networkconfigmanagementimpl

import "sync"

// Map is a wrapper around sync.Map with type enforcement.
type Map[T any] struct {
	m sync.Map
}

// NewMap generates a new empty Map.
func NewMap[T any]() *Map[T] {
	return &Map[T]{sync.Map{}}
}

// Load returns the value stored in the map for a key, or the zero value if no
// value is present.
// The ok result indicates whether value was found in the map.
func (m *Map[T]) Load(key string) (value T, ok bool) {
	v, ok := m.m.Load(key)
	if !ok {
		return
	}
	return v.(T), true
}

// Store sets the value for a key.
func (m *Map[T]) Store(key string, value T) {
	m.m.Store(key, value)
}

// LoadOrStore returns the existing value for the key if present.
// Otherwise, it stores and returns the given value.
// The loaded result is true if the value was loaded, false if stored.
func (m *Map[T]) LoadOrStore(key string, value T) (actual T, loaded bool) {
	v, loaded := m.m.LoadOrStore(key, value)
	return v.(T), loaded
}

// LoadAndDelete deletes the value for a key, returning the previous value if any.
// The loaded result reports whether the key was present.
func (m *Map[T]) LoadAndDelete(key string) (value T, loaded bool) {
	v, loaded := m.m.LoadAndDelete(key)
	if !loaded {
		return
	}
	return v.(T), true
}

// Delete deletes the value for a key.
// If the key is not in the map, Delete does nothing.
func (m *Map[T]) Delete(key string) {
	m.m.Delete(key)
}

// Swap swaps the value for a key and returns the previous value if any.
// The loaded result reports whether the key was present. If it wasn't,
// the returned value will be the zero value.
func (m *Map[T]) Swap(key string, value T) (previous T, loaded bool) {
	v, loaded := m.m.Swap(key, value)
	if !loaded {
		return
	}
	return v.(T), true
}

// CompareAndSwap swaps the old and new values for key
// if the value stored in the map is equal to old.
// The old value must be of a comparable type.
func (m *Map[T]) CompareAndSwap(key string, old, new T) bool {
	return m.m.CompareAndSwap(key, old, new)
}

// CompareAndDelete deletes the entry for key if its value is equal to old.
// The old value must be of a comparable type.
//
// If there is no current value for key in the map, CompareAndDelete
// returns false (even if the old value is the nil interface value).
func (m *Map[T]) CompareAndDelete(key string, old T) bool {
	return m.m.CompareAndDelete(key, old)
}

// Range calls f sequentially for each key and value present in the map.
// If f returns false, range stops the iteration.
//
// Range does not necessarily correspond to any consistent snapshot of the Map's
// contents: no key will be visited more than once, but if the value for any key
// is stored or deleted concurrently (including by f), Range may reflect any
// mapping for that key from any point during the Range call. Range does not
// block other methods on the receiver; even f itself may call any method on m.
//
// Range may be O(N) with the number of elements in the map even if f returns
// false after a constant number of calls.
func (m *Map[T]) Range(f func(key string, value T) bool) {
	m.m.Range(func(key, value any) bool {
		return f(key.(string), value.(T))
	})
}

// Clear deletes all the entries, resulting in an empty Map.
func (m *Map[T]) Clear() {
	m.m.Clear()
}
