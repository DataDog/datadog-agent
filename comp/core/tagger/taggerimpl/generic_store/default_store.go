// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package genericstore

import "github.com/DataDog/datadog-agent/comp/core/tagger/types"

type defaulObjectStore[T any] map[types.EntityID]T

func newDefaultObjectStore[T any]() types.ObjectStore[T] {
	return make(defaulObjectStore[T])
}

// Get implements ObjectStore#Get
func (os defaulObjectStore[T]) Get(entityID types.EntityID) (object T, found bool) {
	obj, found := os[entityID]
	return obj, found
}

// Set implements ObjectStore#Set
func (os defaulObjectStore[T]) Set(entityID types.EntityID, object T) {
	os[entityID] = object
}

// Unset implements ObjectStore#Unset
func (os defaulObjectStore[T]) Unset(entityID types.EntityID) {
	delete(os, entityID)
}

// Size implements ObjectStore#Size
func (os defaulObjectStore[T]) Size() int {
	return len(os)
}

// ListObjects implements ObjectStore#ListObjects
func (os defaulObjectStore[T]) ListObjects() []T {
	objects := make([]T, 0)

	for _, object := range os {
		objects = append(objects, object)
	}

	return objects
}

// ForEach implements ObjectStore#ForEach
func (os defaulObjectStore[T]) ForEach(apply types.ApplyFunc[T]) {
	for id, object := range os {
		apply(id, object)
	}
}
