// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package genericstore

import (
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

// CompositeObjectStore is a generic store that can store objects indexed by keys.
type CompositeObjectStore[T any] struct {
	data map[types.EntityIDPrefix]map[string]T
	size int
}

// NewCompositeObjectStore creates a new CompositeObjectStore.
func NewCompositeObjectStore[T any]() *CompositeObjectStore[T] {
	return &CompositeObjectStore[T]{
		data: make(map[types.EntityIDPrefix]map[string]T),
		size: 0,
	}
}

func (os *CompositeObjectStore[T]) Get(entityID types.EntityID) (object T, found bool) {
	submap, found := os.data[entityID.GetPrefix()]
	if !found {
		return
	}

	object, found = submap[entityID.GetID()]
	return
}

func (os *CompositeObjectStore[T]) Set(entityID types.EntityID, object T) {
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

func (os *CompositeObjectStore[T]) Unset(entityID types.EntityID) {
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

func (os *CompositeObjectStore[T]) Size() int {
	return os.size
}

func (os *CompositeObjectStore[T]) ListObjects(filter *types.Filter) []T {
	objects := make([]T, 0, os.Size())

	for prefix := range filter.GetPrefixes() {
		idToObjects := os.data[prefix]
		for _, object := range idToObjects {
			objects = append(objects, object)
		}
	}

	return objects
}

func (os *CompositeObjectStore[T]) ForEach(filter *types.Filter, apply types.ApplyFunc[T]) {
	for prefix := range filter.GetPrefixes() {
		idToObjects := os.data[prefix]
		for id, object := range idToObjects {
			apply(types.NewEntityID(prefix, id), object)
		}
	}
}
