package dogstatsd

import (
	"github.com/hashicorp/golang-lru"
)

type mapperCacheResult struct {
	Name    string
	Matched bool
	Tags    []string
}

type mapperCache struct {
	cache *lru.Cache
}

// newMapperCache creates a new mapperCache
func newMapperCache(size int) (*mapperCache, error) {
	cache, err := lru.New(size)
	if err != nil {
		return &mapperCache{}, err
	}
	return &mapperCache{cache: cache}, nil
}

// get returns:
// - a mapperCacheResult if found, otherwise nil
// - a boolean indicating if a match has been found
func (m *mapperCache) get(metricName string) (*mapperCacheResult, bool) {
	if result, ok := m.cache.Get(metricName); ok {
		return result.(*mapperCacheResult), true
	}
	return nil, false
}

// addMatch adds mapperCacheResult to cache with metric name as key
func (m *mapperCache) addMatch(metricName string, mappedName string, tags []string) {
	m.cache.Add(metricName, &mapperCacheResult{Name: mappedName, Matched: true, Tags: tags})
}

// addMiss adds mapperCacheResult with Matched:false to cache with metric name as key
func (m *mapperCache) addMiss(metricName string) {
	m.cache.Add(metricName, &mapperCacheResult{Matched: false})
}
