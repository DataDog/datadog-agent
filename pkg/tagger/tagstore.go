package tagger

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"hash/fnv"

	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
)

// entityTags holds the tag information for a given entity
type entityTags struct {
	sync.RWMutex
	lowCardTags   map[string][]string
	highCardTags  map[string][]string
	cacheValid    bool
	cachedSource  []string
	cachedAll     []string // Low + high
	cachedLow     []string // Sub-slice of cachedAll
	freshnessHash string
	outdatedTags  bool
}

// tagStore stores entity tags in memory and handles search and collation.
// Queries should go through the Tagger for cache-miss handling
type tagStore struct {
	storeMutex    sync.RWMutex
	store         map[string]*entityTags
	toDeleteMutex sync.RWMutex
	toDelete      map[string]struct{} // set emulation
}

func newTagStore() *tagStore {
	return &tagStore{
		store:    make(map[string]*entityTags),
		toDelete: make(map[string]struct{}),
	}
}

func (s *tagStore) processTagInfo(info *collectors.TagInfo) error {
	if info == nil {
		return fmt.Errorf("skipping nil message")
	}
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
	storedTags.outdatedTags = false
	storedTags.Unlock()

	if exist == false {
		s.storeMutex.Lock()
		s.store[info.Entity] = storedTags
		s.storeMutex.Unlock()
	}

	digestInfo := digest(info)

	storedTags.Lock()
	defer storedTags.Unlock()
	if storedTags.freshnessHash == "" || storedTags.freshnessHash != digestInfo {
		log.Infof("freshHash is %s and digestInfo %s", storedTags.freshnessHash, digestInfo) // REMOVE
		log.Infof("highcards is %s and low is %s", info.HighCardTags, info.LowCardTags)      // REMOVE
		storedTags.freshnessHash = digestInfo
		storedTags.outdatedTags = true
		log.Infof("NEW freshHash is %s and digestInfo %s", storedTags.freshnessHash, digestInfo) // REMOVE
	}

	return nil
}

func digest(info *collectors.TagInfo) string {
	log.Infof("digesting %s containing low %s and high %s", info.Entity, info.LowCardTags, info.HighCardTags)
	h := fnv.New64()
	for _, i := range info.HighCardTags {
		h.Write([]byte(i))
	}
	for _, i := range info.LowCardTags {
		h.Write([]byte(i))
	}
	return strconv.FormatUint(h.Sum64(), 16)
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

type tagPriority struct {
	tag        string                       // full tag
	priority   collectors.CollectorPriority // collector priority
	isHighCard bool                         // is the tag high cardinality
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
	var sources []string
	tagPrioMapper := make(map[string][]tagPriority)

	for source, tags := range e.lowCardTags {
		sources = append(sources, source)
		insertWithPriority(tagPrioMapper, tags, source, false)
	}

	for source, tags := range e.highCardTags {
		insertWithPriority(tagPrioMapper, tags, source, true)
	}

	lowCardTags := []string{}
	highCardTags := []string{}
	for _, tags := range tagPrioMapper {
		for i := 0; i < len(tags); i++ {
			insert := true
			for j := 0; j < len(tags); j++ {
				// if we find a duplicate tag with higher priority we do not insert the tag
				if i != j && tags[i].priority < tags[j].priority {
					insert = false
					break
				}
			}
			if insert {
				if tags[i].isHighCard {
					highCardTags = append(highCardTags, tags[i].tag)
				} else {
					lowCardTags = append(lowCardTags, tags[i].tag)
				}
			}
		}
	}

	tags := append(lowCardTags, highCardTags...)

	// Write cache
	e.RUnlock()
	e.Lock()
	e.cacheValid = true
	e.cachedSource = sources
	e.cachedAll = tags
	e.cachedLow = e.cachedAll[:len(lowCardTags)]
	e.Unlock()

	if highCard {
		return tags, sources
	}
	return lowCardTags, sources
}

func insertWithPriority(tagPrioMapper map[string][]tagPriority, tags []string, source string, isHighCard bool) {
	priority, found := collectors.CollectorPriorities[source]
	if !found {
		log.Warnf("Tagger: %s collector has no defined priority, assuming low", source)
		priority = collectors.NodeRuntime
	}

	for _, t := range tags {
		tagName := strings.Split(t, ":")[0]
		tagPrioMapper[tagName] = append(tagPrioMapper[tagName], tagPriority{
			tag:        t,
			priority:   priority,
			isHighCard: isHighCard,
		})
	}
}
