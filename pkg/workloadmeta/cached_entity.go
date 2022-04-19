package workloadmeta

import (
	"sort"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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
	return &cachedEntity{
		sources: make(map[Source]Entity),
	}
}

func (e *cachedEntity) unset(source Source) bool {
	if _, found := e.sources[source]; found {
		delete(e.sources, source)
		e.computeCache()
		return true
	}

	return false
}

func (e *cachedEntity) set(source Source, entity Entity) bool {
	_, found := e.sources[source]

	e.sources[source] = entity
	e.computeCache()

	return found
}

func (e *cachedEntity) get(source Source) Entity {
	if source == SourceAll {
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
	var sources []string
	for source := range e.sources {
		sources = append(sources, string(source))
	}

	sort.Strings(sources)

	e.sortedSources = sources

	var merged Entity
	for _, source := range e.sortedSources {
		if e, ok := e.sources[Source(source)]; ok {
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
