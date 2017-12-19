package tagger

import (
	"fmt"
	"sync"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
)

// entityTags holds the tag information for a given entity
type entityTags struct {
	sync.RWMutex
	lowCardTags  map[string][]string
	highCardTags map[string][]string
	cacheValid   bool
	cachedSource []string
	cachedAll    []string // Low + high
	cachedLow    []string // Sub-slice of cachedAll
}

// tagStore stores entity tags in memory and handles search and collation.
// Queries should go through the Tagger for cache-miss handling
type tagStore struct {
	storeMutex    sync.RWMutex
	store         map[string]*entityTags
	toDeleteMutex sync.RWMutex
	toDelete      map[string]struct{} // set emulation
}

func newTagStore() (*tagStore, error) {
	t := &tagStore{
		store:    make(map[string]*entityTags),
		toDelete: make(map[string]struct{}),
	}
	return t, nil
}

func (s *tagStore) processTagInfo(info *collectors.TagInfo) error {
	if info.Entity == "" {
		return fmt.Errorf("empty entity name, skipping message")
	}
	if info.Source == "" {
		return fmt.Errorf("empty source name, skipping message")
	}
	if info.DeleteEntity {
		s.toDeleteMutex.Lock()
		s.toDelete[info.Entity] = struct{}{}
		s.toDeleteMutex.Unlock()
		return nil
	}

	// TODO: check if real change
	s.storeMutex.RLock()
	storedTags, exist := s.store[info.Entity]
	s.storeMutex.RUnlock()
	if exist == false {
		storedTags = &entityTags{
			lowCardTags:  make(map[string][]string),
			highCardTags: make(map[string][]string),
		}
	}

	storedTags.Lock()
	storedTags.lowCardTags[info.Source] = info.LowCardTags
	storedTags.highCardTags[info.Source] = info.HighCardTags
	storedTags.cacheValid = false
	storedTags.Unlock()

	if exist == false {
		s.storeMutex.Lock()
		s.store[info.Entity] = storedTags
		s.storeMutex.Unlock()
	}

	return nil
}

// prune will lock the store and delete tags for the entity previously
// passed as delete. This is to be called regularly from the user class.
func (s *tagStore) prune() error {
	s.toDeleteMutex.Lock()
	defer s.toDeleteMutex.Unlock()

	if len(s.toDelete) == 0 {
		return nil
	}

	s.storeMutex.Lock()
	for entity := range s.toDelete {
		delete(s.store, entity)
	}
	s.storeMutex.Unlock()

	log.Debugf("pruned %d removed entites, %d remaining", len(s.toDelete), len(s.store))

	// Start fresh
	s.toDelete = make(map[string]struct{})

	return nil
}

// lookup gets tags from the store and returns them concatenated in a []string
// array. It returns the source names in the second []string to allow the
// client to trigger manual lookups on missing sources.
func (s *tagStore) lookup(entity string, highCard bool) ([]string, []string) {
	s.storeMutex.RLock()
	storedTags, present := s.store[entity]
	s.storeMutex.RUnlock()

	if present == false {
		return nil, nil
	}
	return storedTags.get(highCard)
}

func (e *entityTags) get(highCard bool) ([]string, []string) {
	e.RLock()

	// Cache hit
	if e.cacheValid {
		defer e.RUnlock()
		if highCard {
			return e.cachedAll, e.cachedSource
		}
		return e.cachedLow, e.cachedSource
	}

	// Cache miss
	var arrays [][]string
	var sources []string
	lowCardCount := 0

	for source, tags := range e.lowCardTags {
		arrays = append(arrays, tags)
		lowCardCount += len(tags)
		sources = append(sources, source)
	}
	for _, tags := range e.highCardTags {
		arrays = append(arrays, tags)
	}
	tags := utils.ConcatenateTags(arrays)

	// Write cache
	e.RUnlock()
	e.Lock()
	e.cacheValid = true
	e.cachedSource = sources
	e.cachedAll = tags
	e.cachedLow = e.cachedAll[:lowCardCount]
	e.Unlock()

	if highCard {
		return tags, sources
	}
	return tags[:lowCardCount], sources
}
