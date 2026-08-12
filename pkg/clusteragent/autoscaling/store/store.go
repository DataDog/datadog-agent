// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package store provides an in-memory key/value store with per-item locking and observer notifications.
package store

import (
	"slices"
	"sync"
)

// SenderID identifies the component that triggered a store change.
type SenderID string

// ObserverFunc is called after the per-item lock has been released, so observers may
// safely re-enter the store.
type ObserverFunc func(key string, sender SenderID)

// Observer watches changes in a Store. A nil func is ignored at registration.
type Observer struct {
	SetFunc    ObserverFunc // called after a value is created or updated
	DeleteFunc ObserverFunc // called after a value is removed
}

// Observable is implemented by every Store.
type Observable interface {
	RegisterObserver(observer Observer)
}

// ItemAction is the verdict a ProcessAll processor returns for an item.
type ItemAction int

const (
	// KeepItem leaves the stored value as-is and notifies no observer.
	KeepItem ItemAction = iota
	// SetItem persists the returned value and notifies set observers.
	SetItem
	// DeleteItem removes the key and notifies delete observers.
	DeleteItem
)

// keyLock is a per-key mutex plus a reference count of goroutines holding or waiting
// on it, used to remove idle lock entries safely.
type keyLock struct {
	mu   sync.Mutex
	refs int
}

// Store is an in-memory key/value store with per-item locking and observer support.
//
// Peek takes only the store lock and never wait on a held item. Get and ProcessAll
// take one item's lock at a time; observers run after that lock is released.
//
// Stored values must be treated as immutable. The store returns shallow copies of T;
// replace reference-typed fields rather than mutating their backing data.
type Store[T any] struct {
	// mu guards data, the per-key lock registry, and each entry's ref count. It is never
	// held across callbacks or while an item lock is held.
	mu    sync.RWMutex
	data  map[string]T
	locks map[string]*keyLock

	observersMu     sync.RWMutex
	setObservers    []ObserverFunc
	deleteObservers []ObserverFunc
}

// NewStore creates an empty Store.
func NewStore[T any]() *Store[T] {
	return &Store[T]{
		data:  make(map[string]T),
		locks: make(map[string]*keyLock),
	}
}

// RegisterObserver registers a synchronous observer. Nil funcs are ignored.
func (s *Store[T]) RegisterObserver(observer Observer) {
	s.observersMu.Lock()
	defer s.observersMu.Unlock()

	if observer.SetFunc != nil {
		s.setObservers = append(s.setObservers, observer.SetFunc)
	}
	if observer.DeleteFunc != nil {
		s.deleteObservers = append(s.deleteObservers, observer.DeleteFunc)
	}
}

// Peek returns the value stored under id without taking the per-item lock.
// To read-modify-write atomically, use Get.
func (s *Store[T]) Peek(id string) (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	v, ok := s.data[id]
	return v, ok
}

// List returns shallow copies of values matched by filter; a nil filter returns all
// values. The filter runs without any lock held, so it may safely read the store.
// Results are in no particular order.
func (s *Store[T]) List(filter func(T) bool) []T {
	s.mu.RLock()
	snapshot := make([]T, 0, len(s.data))
	for _, v := range s.data {
		snapshot = append(snapshot, v)
	}
	s.mu.RUnlock()

	if filter == nil {
		return snapshot
	}

	// Filter outside the lock, reusing the snapshot's backing array. DeleteFunc also
	// zeroes the dropped slots, so filtered-out values aren't kept alive.
	return slices.DeleteFunc(snapshot, func(v T) bool { return !filter(v) })
}

// Count returns the number of items in the store.
func (s *Store[T]) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.data)
}

// Get acquires id's lock and returns a handle plus whether a value already existed.
// The handle is always non-nil and must be ended with Upsert, Delete, or Release.
// Deferring Release is safe because terminal calls make it a no-op:
//
//	item, found := store.Get(id)
//	defer item.Release()
//	if !found { return }
//	v := item.Value()
//	// ... modify a copy, then write it back ...
//	item.Upsert(v, sender) // or item.Delete(sender)
func (s *Store[T]) Get(id string) (*LockedItem[T], bool) {
	kl := s.acquire(id)

	s.mu.RLock()
	v, found := s.data[id]
	s.mu.RUnlock()

	return &LockedItem[T]{store: s, key: id, kl: kl, value: v}, found
}

// ProcessAll runs process against every item present at the start of the pass, one
// item lock at a time. The processor returns the new value with an ItemAction.
//
// The pass is not atomic: items may appear or disappear mid-pass, and ProcessAll
// blocks on any key a concurrent caller is holding.
func (s *Store[T]) ProcessAll(sender SenderID, process func(id string, value T) (T, ItemAction)) {
	s.mu.RLock()
	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	s.mu.RUnlock()

	for _, key := range keys {
		switch action := s.processItem(key, process); action {
		case SetItem:
			s.notifySet(key, sender)
		case DeleteItem:
			s.notifyDelete(key, sender)
		}
	}
}

// processItem returns the observer notification to send after releasing the item lock.
func (s *Store[T]) processItem(key string, process func(id string, value T) (T, ItemAction)) ItemAction {
	kl := s.acquire(key)
	defer s.release(key, kl)

	s.mu.RLock()
	v, ok := s.data[key]
	s.mu.RUnlock()
	if !ok {
		// Vanished between the snapshot and acquiring the lock.
		return KeepItem
	}

	newValue, action := process(key, v)
	switch action {
	case SetItem:
		s.storeValue(key, newValue)
		return SetItem
	case DeleteItem:
		if existed := s.removeValue(key); existed {
			return DeleteItem
		}
		return KeepItem
	default: // KeepItem
		return KeepItem
	}
}

// --- per-key lock registry ---

// acquire increments the refcount before blocking, so waiting goroutines keep the
// lock entry alive.
func (s *Store[T]) acquire(key string) *keyLock {
	s.mu.Lock()
	kl := s.locks[key]
	if kl == nil {
		kl = &keyLock{}
		s.locks[key] = kl
	}
	kl.refs++
	s.mu.Unlock()

	kl.mu.Lock()
	return kl
}

// release removes the registry entry once no goroutine holds or waits on it.
func (s *Store[T]) release(key string, kl *keyLock) {
	kl.mu.Unlock()

	s.mu.Lock()
	kl.refs--
	if kl.refs == 0 {
		delete(s.locks, key)
	}
	s.mu.Unlock()
}

// --- value map mutations (callers hold the key's lock) ---

func (s *Store[T]) storeValue(key string, value T) {
	s.mu.Lock()
	s.data[key] = value
	s.mu.Unlock()
}

func (s *Store[T]) removeValue(key string) bool {
	s.mu.Lock()
	_, existed := s.data[key]
	delete(s.data, key)
	s.mu.Unlock()
	return existed
}

// --- observer notification (always after the item lock is released) ---

func (s *Store[T]) notifySet(key string, sender SenderID) {
	s.notify(&s.setObservers, key, sender)
}

func (s *Store[T]) notifyDelete(key string, sender SenderID) {
	s.notify(&s.deleteObservers, key, sender)
}

// notify invokes observers without holding observersMu.
func (s *Store[T]) notify(observers *[]ObserverFunc, key string, sender SenderID) {
	s.observersMu.RLock()
	fns := *observers
	s.observersMu.RUnlock()

	for _, f := range fns {
		f(key, sender)
	}
}
