// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package genericstore

import (
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

// ObjectStore is a generic store that can store objects indexed by keys.
type ObjectStore[T any] struct {
	data map[types.EntityIDPrefix]map[string]T
	size int
}

// NewObjectStore creates a new ObjectStore.
func NewObjectStore[T any]() *ObjectStore[T] {
	return &ObjectStore[T]{
		data: make(map[types.EntityIDPrefix]map[string]T),
		size: 0,
	}
}

// Get returns the object with the specified entity ID if it exists in the store.
func (os *ObjectStore[T]) Get(entityID types.EntityID) (object T, found bool) {
	submap, found := os.data[entityID.GetPrefix()]
	if !found {
		return
	}

	object, found = submap[entityID.GetID()]
	return
}

// Set stores the provided object under the specified entity ID.
func (os *ObjectStore[T]) Set(entityID types.EntityID, object T) {
	prefix := entityID.GetPrefix()
	id := entityID.GetID()
	if submap, found := os.data[prefix]; found {
		if _, exists := submap[id]; !exists {
			os.size++
		}
		submap[id] = object
	} else {
		os.data[prefix] = map[string]T{id: object}
		os.size++
	}
}

// Unset removes the object associated with the specified entity ID.
func (os *ObjectStore[T]) Unset(entityID types.EntityID) {
	prefix := entityID.GetPrefix()
	id := entityID.GetID()
	// TODO: prune
	if submap, found := os.data[prefix]; found {
		if _, exists := submap[id]; exists {
			delete(submap, id)
			os.size--
		}
	}
}

// Size returns the total number of objects in the store.
func (os *ObjectStore[T]) Size() int {
	return os.size
}

// ListObjects returns every object in the store matching the provided filter.
func (os *ObjectStore[T]) ListObjects(filter *types.Filter) []T {
	objects := make([]T, 0, os.Size())

	for prefix := range filter.GetPrefixes() {
		idToObjects := os.data[prefix]
		for _, object := range idToObjects {
			objects = append(objects, object)
		}
	}

	return objects
}

// ForEach applies the provided function to each object in the store matching the filter.
func (os *ObjectStore[T]) ForEach(filter *types.Filter, apply types.ApplyFunc[T]) {
	for prefix := range filter.GetPrefixes() {
		idToObjects := os.data[prefix]
		for id, object := range idToObjects {
			apply(types.NewEntityID(prefix, id), object)
		}
	}
}
