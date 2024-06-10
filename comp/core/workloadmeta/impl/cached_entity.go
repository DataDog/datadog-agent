// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package workloadmeta

import (
	"reflect"
	"sort"

	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// cachedEntity stores each source of an entity, alongside a cached version
// with all of the sources merged into one. It is not thread-safe, as its only
// meant to be used internally by workloadmeta.Store, and is protected by
// a `Store.storeMut` lock.
type cachedEntity struct {
	cached        wmdef.Entity
	sources       map[wmdef.Source]wmdef.Entity
	sortedSources []string
}

func newCachedEntity() *cachedEntity {
	return &cachedEntity{
		sources: make(map[wmdef.Source]wmdef.Entity),
	}
}

func (e *cachedEntity) unset(source wmdef.Source) bool {
	if _, found := e.sources[source]; found {
		delete(e.sources, source)
		e.computeCache()
		return true
	}

	return false
}

func (e *cachedEntity) set(source wmdef.Source, entity wmdef.Entity) (found, changed bool) {
	old, found := e.sources[source]

	if found && reflect.DeepEqual(old, entity) {
		return true, false
	}

	e.sources[source] = entity
	e.computeCache()

	return found, true
}

func (e *cachedEntity) get(source wmdef.Source) wmdef.Entity {
	if source == wmdef.SourceAll {
		return e.cached
	}

	return e.sources[source]
}

// computeCache merges the entities in e.sources into one and caches the result
// in e.cached. Priority is established by the string representation of the
// source in alphabetical order, and data is considered missing if it's a zero
// value. Conflicts are not expected (entities should represent the same data),
// so the sorting is to ensure deterministic behavior more than anything.
func (e *cachedEntity) computeCache() {
	sources := make([]string, 0, len(e.sources))
	for source := range e.sources {
		sources = append(sources, string(source))
	}

	sort.Strings(sources)

	e.sortedSources = sources

	var merged wmdef.Entity
	for _, source := range e.sortedSources {
		if e, ok := e.sources[wmdef.Source(source)]; ok {
			if merged == nil {
				merged = e.DeepCopy()
			} else {
				err := merged.Merge(e)
				if err != nil {
					log.Errorf("Cannot merge %+v into %+v: %s", merged, e, err)
				}
			}
		}
	}

	e.cached = merged
}

func (e *cachedEntity) copy() *cachedEntity {
	newEntity := newCachedEntity()

	newEntity.cached = e.cached.DeepCopy()

	copy(newEntity.sortedSources, e.sortedSources)

	for source, entity := range e.sources {
		newEntity.sources[source] = entity
	}

	return newEntity
}
