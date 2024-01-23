// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package workloadmeta

// cachedEntity stores each source of an entity, alongside a cached version
// with all of the sources merged into one. It is not thread-safe, as its only
// meant to be used internally by workloadmeta.Store, and is protected by
// a `Store.storeMut` lock.
type cachedEntity struct {
	cached        Entity
	sources       map[Source]Entity
	sortedSources []string
}

func newCachedEntity() *cachedEntity {
	panic("not called")
}

func (e *cachedEntity) unset(source Source) bool {
	panic("not called")
}

func (e *cachedEntity) set(source Source, entity Entity) (found, changed bool) {
	panic("not called")
}

func (e *cachedEntity) get(source Source) Entity {
	panic("not called")
}

// computeCache merges the entities in e.sources into one and caches the result
// in e.cached. Priority is established by the string representation of the
// source in alphabetical order, and data is considered missing if it's a zero
// value. Conflicts are not expected (entities should represent the same data),
// so the sorting is to ensure deterministic behavior more than anything.
func (e *cachedEntity) computeCache() {
	panic("not called")
}

func (e *cachedEntity) copy() *cachedEntity {
	panic("not called")
}
