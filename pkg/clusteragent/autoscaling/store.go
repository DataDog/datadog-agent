// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoscaling

import (
	"sync"
)

const (
	setOperation storeOperation = iota
	deleteOperation
)

// ObserverFunc represents observer functions of the store
type ObserverFunc func(string, string)

// Observer allows to define functions to watch changes in Store
type Observer struct {
	SetFunc    ObserverFunc
	DeleteFunc ObserverFunc
}

// Observable is an interface type for all stores
type Observable interface {
	RegisterObserver(observer Observer)
}

// Store is a simple in-memory store with observer support
type Store[T any] struct {
	store         map[string]T
	lock          sync.RWMutex
	observers     map[storeOperation][]ObserverFunc
	observersLock sync.RWMutex
}

type storeOperation int

// NewStore creates a new NewStore
func NewStore[T any]() *Store[T] {
	return &Store[T]{
		store: make(map[string]T),
		observers: map[storeOperation][]ObserverFunc{
			setOperation:    make([]ObserverFunc, 0),
			deleteOperation: make([]ObserverFunc, 0),
		},
	}
}

// RegisterObserver registers an observer that will be notified when changes happen in the store
// Current implementation does not scale beyond a handful of observers.
// Calls are made synchronously to each observer for each operation.
// The store guarantees that any lock has been released before calling observers.
func (s *Store[T]) RegisterObserver(observer Observer) {
	s.observersLock.Lock()
	defer s.observersLock.Unlock()

	addObserver := func(operationType storeOperation, observerFunc ObserverFunc) {
		if observerFunc != nil {
			s.observers[operationType] = append(s.observers[operationType], observerFunc)
		}
	}

	addObserver(setOperation, observer.SetFunc)
	addObserver(deleteOperation, observer.DeleteFunc)
}

// Get returns object for given id, returns nil if absent
func (s *Store[T]) Get(id string) (T, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	res, ok := s.store[id]
	return res, ok
}

// GetAll returns a copy of all store values
func (s *Store[T]) GetAll() []T {
	return s.GetFiltered(func(T) bool { return true })
}

// GetFiltered returns a copy of all store values matched by the `filter` function
func (s *Store[T]) GetFiltered(filter func(T) bool) []T {
	s.lock.RLock()
	defer s.lock.RUnlock()

	objects := make([]T, 0, len(s.store))
	for _, object := range s.store {
		if filter(object) {
			objects = append(objects, object)
		}
	}

	return objects
}

// Update updates all objects in the store with the result of the `updator` function.
// Updator func is expected to return the new object and a boolean indicating if the object has changed.
// The object is updated only if boolean is true, observers are notified only for updated objects after all objects have been updated.
func (s *Store[T]) Update(updator func(T) (T, bool), sender string) {
	var changedIDs []string
	s.lock.Lock()
	for id, object := range s.store {
		newObject, changed := updator(object)
		if changed {
			s.store[id] = newObject
			changedIDs = append(changedIDs, id)
		}
	}
	s.lock.Unlock()

	// Notifying must be done after releasing the lock
	for _, id := range changedIDs {
		s.notify(setOperation, id, sender)
	}
}

// Count returns number of elements in store
func (s *Store[T]) Count() int {
	s.lock.RLock()
	defer s.lock.RUnlock()

	return len(s.store)
}

// Set object for id
func (s *Store[T]) Set(id string, obj T, sender string) {
	s.lock.Lock()
	s.store[id] = obj
	s.lock.Unlock()

	s.notify(setOperation, id, sender)
}

// Delete object corresponding to id if present
func (s *Store[T]) Delete(id, sender string) {
	s.lock.Lock()
	_, exists := s.store[id]
	delete(s.store, id)
	s.lock.Unlock()

	if exists {
		s.notify(deleteOperation, id, sender)
	}
}

// LockRead allows to get an item and leave the store in a locked state to allow safe Read -> Operation -> Write sequences
// Still locks if the key does not exist as you may want to prevent a concurrent Write.
// It's not very efficient to lock the whole store but it's probably enough for our use case.
func (s *Store[T]) LockRead(id string, lockOnMissing bool) (T, bool) {
	s.lock.Lock()

	res, ok := s.store[id]
	if !ok {
		if !lockOnMissing {
			s.lock.Unlock()
		}
	}

	return res, ok
}

// Unlock allows to unlock after a read that do not require any modification to the internal object
func (s *Store[T]) Unlock(string) {
	s.lock.Unlock()
}

// UnlockSet sets the new object value and releases the lock (previously acquired by `LockRead`)
func (s *Store[T]) UnlockSet(id string, obj T, sender string) {
	s.store[id] = obj
	s.lock.Unlock()

	s.notify(setOperation, id, sender)
}

// UnlockDelete deletes an object and releases the lock (previously acquired by `LockRead`)
func (s *Store[T]) UnlockDelete(id, sender string) {
	_, exists := s.store[id]

	delete(s.store, id)
	s.lock.Unlock()

	if exists {
		s.notify(deleteOperation, id, sender)
	}
}

// It's a very simple implementation of a notify process, but it's enough in our case as we aim at only 1 or 2 observers
func (s *Store[T]) notify(operationType storeOperation, key, sender string) {
	s.observersLock.RLock()
	defer s.observersLock.RUnlock()

	for _, observer := range s.observers[operationType] {
		observer(key, sender)
	}
}
