package tagger

import (
	"fmt"
	"sync"

	//log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
)

// entityTags holds the tag information for a given entity
type entityTags struct {
	sync.RWMutex
	lowCardTags  map[string][]string
	highCardTags map[string][]string
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

	// Start fresh
	s.toDelete = make(map[string]struct{})

	return nil
}

// lookup gets tags from the store and returns them concatenated in a []string
// array. It returns the source names in the second []string to allow the
// client to trigger manual lookups on missing sources.
func (s *tagStore) lookup(entity string, highCard bool) ([]string, []string, error) {
	s.storeMutex.RLock()
	storedTags, present := s.store[entity]
	s.storeMutex.RUnlock()

	if present == false {
		//return nil, nil, fmt.Errorf("entity not in memory store")
		return nil, nil, nil
	}

	var arrays [][]string
	var sources []string

	// TODO: the concatenated array could be pre-computed
	// in entityTags on first read and invalidated on write
	storedTags.RLock()
	for source, tags := range storedTags.lowCardTags {
		arrays = append(arrays, tags)
		sources = append(sources, source)
	}
	if highCard {
		for _, tags := range storedTags.highCardTags {
			arrays = append(arrays, tags)
		}
	}
	tags := utils.ConcatenateTags(arrays)
	storedTags.RUnlock()

	return tags, sources, nil
}
