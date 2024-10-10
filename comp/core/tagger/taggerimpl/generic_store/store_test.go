// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package genericstore

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

func TestObjectStore_GetSet(t *testing.T) {
	store := NewObjectStore[any]()

	id := types.NewEntityID("prefix", "id")
	// getting a non existent item
	obj, found := store.Get(id)
	assert.Nil(t, obj)
	assert.Falsef(t, found, "item should not be found in store")

	// set item
	store.Set(id, struct{}{})

	// getting item
	obj, found = store.Get(id)
	assert.NotNil(t, obj)
	assert.Truef(t, found, "item should be found in store")

	// unsetting item
	store.Unset(id)

	// getting a non existent item
	obj, found = store.Get(id)
	assert.Nil(t, obj)
	assert.Falsef(t, found, "item should not be found in store")
}

func TestObjectStore_GetWithEntityIDStr(t *testing.T) {
	store := NewObjectStore[any]()

	id := types.NewEntityID("prefix", "id")
	idStr := id.String()
	// getting a non-existent item
	obj, found := store.GetWithEntityIDStr(idStr)
	assert.Nil(t, obj)
	assert.Falsef(t, found, "item should not be found in store")

	// set item
	store.Set(id, struct{}{})

	// getting item
	obj, found = store.GetWithEntityIDStr(idStr)
	assert.NotNil(t, obj)
	assert.Truef(t, found, "item should be found in store")

	// unsetting item
	store.Unset(id)

	// getting a non-existent item
	obj, found = store.GetWithEntityIDStr(idStr)
	assert.Nil(t, obj)
	assert.Falsef(t, found, "item should not be found in store")
}

func TestObjectStore_Size(t *testing.T) {
	store := NewObjectStore[any]()

	// store should be empty
	assert.Equalf(t, store.Size(), 0, "store should be empty")

	// add item to store
	id := types.NewEntityID("prefix", "id")
	store.Set(id, struct{}{})

	// store size should be 1
	assert.Equalf(t, 1, store.Size(), "store should contain 1 item")

	// unset item
	store.Unset(id)

	// store should be empty
	assert.Equalf(t, 0, store.Size(), "store should be empty")
}

func TestObjectStore_ListObjects(t *testing.T) {
	store := NewObjectStore[any]()

	// build some filter
	fb := types.NewFilterBuilder()
	fb.Include(types.EntityIDPrefix("prefix1"), types.EntityIDPrefix("prefix2"))
	filter := fb.Build(types.HighCardinality)

	// list should return empty
	list := store.ListObjects(filter)
	assert.Equalf(t, len(list), 0, "ListObjects should return an empty list")

	// add some items
	ids := []string{"prefix1://id1", "prefix2://id2", "prefix3://id3", "prefix4://id4"}
	for _, id := range ids {
		entityID, _ := types.NewEntityIDFromString(id)
		store.Set(entityID, id)
	}

	// list should return empty
	list = store.ListObjects(filter)
	expectedListing := []any{"prefix1://id1", "prefix2://id2"}
	assert.ElementsMatch(t, expectedListing, list)
}

func TestObjectStore_ForEach(t *testing.T) {
	store := NewObjectStore[any]()

	// add some items
	ids := []string{"prefix1://id1", "prefix2://id2", "prefix3://id3", "prefix4://id4"}
	for _, id := range ids {
		entityID, _ := types.NewEntityIDFromString(id)
		store.Set(entityID, struct{}{})
	}

	accumulator := []string{}

	// build some filter
	fb := types.NewFilterBuilder()
	fb.Include(types.EntityIDPrefix("prefix1"), types.EntityIDPrefix("prefix2"))
	filter := fb.Build(types.HighCardinality)

	store.ForEach(filter, func(id types.EntityID, _ any) { accumulator = append(accumulator, id.String()) })

	// list should return empty
	assert.ElementsMatch(t, accumulator, []string{"prefix1://id1", "prefix2://id2"})
}
