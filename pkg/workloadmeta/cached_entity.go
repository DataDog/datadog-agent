package workloadmeta

import (
	"sort"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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

func (s *cachedEntity) unset(source Source) bool {
	if _, found := s.sources[source]; found {
		delete(s.sources, source)
		s.computeCache()
		return true
	}

	return false
}

func (s *cachedEntity) set(source Source, entity Entity) bool {
	_, found := s.sources[source]

	s.sources[source] = entity
	s.computeCache()

	return found
}

func (s *cachedEntity) get(source Source) Entity {
	if source == "" {
		return s.cached
	}

	return s.sources[source]
}

func (s *cachedEntity) computeCache() {
	var sources []string
	for source := range s.sources {
		sources = append(sources, string(source))
	}

	// sort sources for deterministic merging
	sort.Strings(sources)

	s.sortedSources = sources

	var merged Entity
	for _, source := range s.sortedSources {
		if e, ok := s.sources[Source(source)]; ok {
			if merged == nil {
				merged = e.DeepCopy()
			} else {
				err := merged.Merge(e)
				if err != nil {
					log.Errorf("cannot merge %+v into %+v: %s", merged, e, err)
				}
			}
		}
	}

	s.cached = merged
}
