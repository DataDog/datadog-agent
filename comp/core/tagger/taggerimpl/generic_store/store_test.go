// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package genericstore

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestNewObjectStore(t *testing.T) {
	test := func(t *testing.T, isComposite bool) {
		cfg := configmock.New(t)
		cfg.SetWithoutSource("tagger.tagstore_use_composite_entity_id", isComposite)

		store := NewObjectStore[any](cfg)

		// assert correct store type is initialised
		if isComposite {
			_, ok := store.(*compositeObjectStore[any])
			assert.Truef(t, ok, "Should have initialised a composite object store")
		} else {
			_, ok := store.(defaulObjectStore[any])
			assert.Truef(t, ok, "Should have initialised a default object store")
		}
	}

	// default store
	test(t, false)

	// composite composite
	test(t, true)
}

func TestObjectStore_GetSet(t *testing.T) {
	test := func(t *testing.T, isComposite bool) {
		cfg := configmock.New(t)
		cfg.SetWithoutSource("tagger.tagstore_use_composite_entity_id", isComposite)

		store := NewObjectStore[any](cfg)

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

	// default store
	test(t, false)

	// composite store
	test(t, true)
}

func TestObjectStore_Size(t *testing.T) {

	test := func(t *testing.T, isComposite bool) {
		// initialise store
		cfg := configmock.New(t)
		cfg.SetWithoutSource("tagger.tagstore_use_composite_entity_id", isComposite)
		store := NewObjectStore[any](cfg)

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

	// default store
	test(t, false)

	// composite store
	test(t, true)
}

func TestObjectStore_ListObjects(t *testing.T) {
	test := func(t *testing.T, isComposite bool) {
		// initialise store
		cfg := configmock.New(t)
		cfg.SetWithoutSource("tagger.tagstore_use_composite_entity_id", isComposite)
		store := NewObjectStore[any](cfg)

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

	// default store
	test(t, false)

	// composite store
	test(t, true)
}

func TestObjectStore_ForEach(t *testing.T) {
	test := func(t *testing.T, isComposite bool) {
		// initialise store
		cfg := configmock.New(t)
		cfg.SetWithoutSource("tagger.tagstore_use_composite_entity_id", isComposite)
		store := NewObjectStore[any](cfg)

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

	// default store
	test(t, false)

	// composite store
	test(t, true)
}
