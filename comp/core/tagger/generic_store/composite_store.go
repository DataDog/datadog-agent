// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package genericstore

import (
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

type compositeObjectStore[T any] struct {
	data map[types.EntityIDPrefix]map[string]T
	size int
}

func newCompositeObjectStore[T any]() types.ObjectStore[T] {
	return &compositeObjectStore[T]{
		data: make(map[types.EntityIDPrefix]map[string]T),
		size: 0,
	}
}

// Get implements ObjectStore#Get
func (os *compositeObjectStore[T]) Get(entityID types.EntityID) (object T, found bool) {
	submap, found := os.data[entityID.GetPrefix()]
	if !found {
		return
	}

	object, found = submap[entityID.GetID()]
	return
}

// Set implements ObjectStore#Set
func (os *compositeObjectStore[T]) Set(entityID types.EntityID, object T) {
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

// Unset implements ObjectStore#Unset
func (os *compositeObjectStore[T]) Unset(entityID types.EntityID) {
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

// Size implements ObjectStore#Size
func (os *compositeObjectStore[T]) Size() int {
	return os.size
}

// ListObjects implements ObjectStore#ListObjects
func (os *compositeObjectStore[T]) ListObjects(filter *types.Filter) []T {
	objects := make([]T, 0, os.Size())

	for prefix := range filter.GetPrefixes() {
		idToObjects := os.data[prefix]
		for _, object := range idToObjects {
			objects = append(objects, object)
		}
	}

	return objects
}

// ForEach implements ObjectStore#ForEach
func (os *compositeObjectStore[T]) ForEach(filter *types.Filter, apply types.ApplyFunc[T]) {
	for prefix := range filter.GetPrefixes() {
		idToObjects := os.data[prefix]
		for id, object := range idToObjects {
			apply(types.NewEntityID(prefix, id), object)
		}
	}
}
