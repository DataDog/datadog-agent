package tagger

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// entityTags holds the tag information for a given entity
type entityTags struct {
	sync.RWMutex
	entityID           string
	sourceTags         map[string]sourceTags
	cacheValid         bool
	cachedSource       []string
	cachedAll          []string // Low + orchestrator + high
	cachedOrchestrator []string // Low + orchestrator (subslice of cachedAll)
	cachedLow          []string // Sub-slice of cachedAll
	tagsHash           string
	toDelete           map[string]struct{}
}

func newEntityTags(entityID string) *entityTags {
	return &entityTags{
		entityID:   entityID,
		sourceTags: make(map[string]sourceTags),
		toDelete:   make(map[string]struct{}),
	}
}

// sourceTags holds the tags for a given entity collected from a single source,
// grouped by their cardinality.
type sourceTags struct {
	lowCardTags          []string
	orchestratorCardTags []string
	highCardTags         []string
	standardTags         []string
}

type entityEvent struct {
	eventType  EventType
	storedTags *entityTags
}

// tagStore stores entity tags in memory and handles search and collation.
// Queries should go through the Tagger for cache-miss handling
type tagStore struct {
	sync.RWMutex

	store    map[string]*entityTags
	toDelete map[string]struct{} // set emulation

	subscribersMutex sync.RWMutex
	subscribers      map[chan []EntityEvent]collectors.TagCardinality
}

func newTagStore() *tagStore {
	return &tagStore{
		store:       make(map[string]*entityTags),
		toDelete:    make(map[string]struct{}),
		subscribers: make(map[chan []EntityEvent]collectors.TagCardinality),
	}
}

func (s *tagStore) processTagInfo(tagInfos []*collectors.TagInfo) {
	events := []entityEvent{}

	s.Lock()
	defer s.Unlock()

	for _, info := range tagInfos {
		if info == nil {
			log.Tracef("processTagInfo err: skipping nil message")
			continue
		}
		if info.Entity == "" {
			log.Tracef("processTagInfo err: empty entity name, skipping message")
			continue
		}
		if info.Source == "" {
			log.Tracef("processTagInfo err: empty source name, skipping message")
			continue
		}

		storedTags, exist := s.store[info.Entity]

		if info.DeleteEntity {
			if exist {
				s.toDelete[info.Entity] = struct{}{}

				storedTags.Lock()
				storedTags.toDelete[info.Source] = struct{}{}
				storedTags.Unlock()
			}

			continue
		}

		eventType := EventTypeModified
		if !exist {
			eventType = EventTypeAdded
			storedTags = newEntityTags(info.Entity)
			s.store[info.Entity] = storedTags
		}

		// TODO: check if real change

		updatedEntities.Inc()

		err := updateStoredTags(storedTags, info)
		if err != nil {
			log.Tracef("processTagInfo err: %v", err)
			continue
		}

		events = append(events, entityEvent{
			eventType:  eventType,
			storedTags: storedTags,
		})
	}

	s.notifySubscribers(events)
}

func updateStoredTags(storedTags *entityTags, info *collectors.TagInfo) error {
	storedTags.Lock()
	defer storedTags.Unlock()

	_, found := storedTags.sourceTags[info.Source]
	if found && info.CacheMiss {
		// check if the source tags is already present for this entry
		return fmt.Errorf("try to overwrite an existing entry with and empty cache-miss entry, info.Source: %s, info.Entity: %s", info.Source, info.Entity)
	}

	if !found {
		prefix, _ := containers.SplitEntityName(info.Entity)
		storedEntities.Inc(info.Source, prefix)
	}

	storedTags.cacheValid = false
	storedTags.sourceTags[info.Source] = sourceTags{
		lowCardTags:          info.LowCardTags,
		orchestratorCardTags: info.OrchestratorCardTags,
		highCardTags:         info.HighCardTags,
		standardTags:         info.StandardTags,
	}

	return nil
}

// Entity is an entity ID + tags.
type Entity struct {
	ID                          string
	Hash                        string
	HighCardinalityTags         []string
	OrchestratorCardinalityTags []string
	LowCardinalityTags          []string
	StandardTags                []string
}

// EventType is a type of event, triggered when an entity is added, modified or
// deleted.
type EventType int

const (
	// EventTypeAdded means an entity was added.
	EventTypeAdded EventType = iota
	// EventTypeModified means an entity was modified.
	EventTypeModified
	// EventTypeDeleted means an entity was deleted.
	EventTypeDeleted
)

// EntityEvent is an event generated when an entity is added, modified or
// deleted. It contains the event type and the new entity.
type EntityEvent struct {
	EventType EventType
	Entity    Entity
}

// subscribe returns a channel that receives a slice of events whenever an entity is
// added, modified or deleted.
func (s *tagStore) subscribe(cardinality collectors.TagCardinality) chan []EntityEvent {
	// this buffer size is an educated guess, as we know the rate of
	// updates, but not how fast these can be streamed out yet. it most
	// likely should be configurable.
	bufferSize := 100

	// this is a `ch []EntityEvent` instead of a `ch EntityEvent` to
	// improve throughput, as bursts of events are as likely to occur as
	// isolated events, especially at startup or with the kubelet
	// collector, since it's a collector that periodically pulls changes.
	ch := make(chan []EntityEvent, bufferSize)

	s.RLock()
	defer s.RUnlock()
	events := make([]EntityEvent, 0, len(s.store))
	for _, storedTags := range s.store {
		events = append(events, EntityEvent{
			EventType: EventTypeAdded,
			Entity:    storedTags.toEntity(cardinality),
		})
	}

	s.subscribersMutex.Lock()
	defer s.subscribersMutex.Unlock()
	s.subscribers[ch] = cardinality

	ch <- events

	return ch
}

// unsubscribe ends a subscription to entity events and closes its channel.
func (s *tagStore) unsubscribe(ch chan []EntityEvent) {
	s.subscribersMutex.Lock()
	defer s.subscribersMutex.Unlock()

	delete(s.subscribers, ch)
	close(ch)
}

// notifySubscribers sends a slice of EntityEvents of a certain type for the
// passed entities all registered subscribers.
func (s *tagStore) notifySubscribers(events []entityEvent) {
	s.subscribersMutex.RLock()
	defer s.subscribersMutex.RUnlock()

	// NOTE: we need to add some telemetry on the amount of subscribers and
	// notifications being sent, and at which cardinality

	for ch, cardinality := range s.subscribers {
		subscriberEvents := make([]EntityEvent, 0, len(events))

		for _, event := range events {
			var entity Entity

			if event.eventType != EventTypeDeleted {
				entity = event.storedTags.toEntity(cardinality)
			} else {
				entity = Entity{ID: event.storedTags.entityID}
			}

			subscriberEvents = append(subscriberEvents, EntityEvent{
				EventType: event.eventType,
				Entity:    entity,
			})
		}

		ch <- subscriberEvents
	}
}

func computeTagsHash(tags []string) string {
	hash := ""
	if len(tags) > 0 {
		// do not sort original slice
		tags = copyArray(tags)
		h := fnv.New64()
		sort.Strings(tags)
		for _, i := range tags {
			h.Write([]byte(i)) //nolint:errcheck
		}
		hash = strconv.FormatUint(h.Sum64(), 16)
	}
	return hash
}

// prune will lock the store and delete tags for the entity previously
// passed as delete. This is to be called regularly from the user class.
func (s *tagStore) prune() error {
	s.Lock()
	defer s.Unlock()

	if len(s.toDelete) == 0 {
		return nil
	}

	events := []entityEvent{}

	for entity := range s.toDelete {
		storedTags, ok := s.store[entity]
		if !ok {
			continue
		}

		storedTags.Lock()

		prefix, _ := containers.SplitEntityName(entity)

		for source := range storedTags.toDelete {
			if _, ok := storedTags.sourceTags[source]; !ok {
				continue
			}

			delete(storedTags.sourceTags, source)
			storedEntities.Dec(source, prefix)
		}

		if len(storedTags.sourceTags) == 0 {
			delete(s.store, entity)
			events = append(events, entityEvent{
				eventType:  EventTypeDeleted,
				storedTags: storedTags,
			})
		} else {
			storedTags.cacheValid = false
			storedTags.toDelete = make(map[string]struct{})
			events = append(events, entityEvent{
				eventType:  EventTypeModified,
				storedTags: storedTags,
			})
		}

		storedTags.Unlock()
	}

	log.Debugf("pruned %d removed entities, %d remaining", len(s.toDelete), len(s.store))

	// Start fresh
	s.toDelete = make(map[string]struct{})

	s.notifySubscribers(events)

	return nil
}

// lookup gets tags from the store and returns them concatenated in a string
// slice. It returns the source names in the second slice to allow the
// client to trigger manual lookups on missing sources, the last string
// is the tags hash to have a snapshot digest of all the tags.
func (s *tagStore) lookup(entity string, cardinality collectors.TagCardinality) ([]string, []string, string) {
	s.RLock()
	defer s.RUnlock()
	storedTags, present := s.store[entity]

	if present == false {
		return nil, nil, ""
	}
	return storedTags.get(cardinality)
}

// lookupStandard returns the standard tags recorded for a given entity
func (s *tagStore) lookupStandard(entity string) ([]string, error) {
	s.RLock()
	defer s.RUnlock()
	storedTags, present := s.store[entity]
	if present == false {
		return nil, fmt.Errorf("entity %s not found", entity)
	}
	return storedTags.getStandard(), nil
}

func (e *entityTags) getStandard() []string {
	e.RLock()
	defer e.RUnlock()
	tags := []string{}
	for _, t := range e.sourceTags {
		tags = append(tags, t.standardTags...)
	}
	return tags
}

type tagPriority struct {
	tag         string                       // full tag
	priority    collectors.CollectorPriority // collector priority
	cardinality collectors.TagCardinality    // cardinality level of the tag (low, orchestrator, high)
}

func (e *entityTags) get(cardinality collectors.TagCardinality) ([]string, []string, string) {
	e.computeCache()

	e.Lock()
	defer e.Unlock()

	if cardinality == collectors.HighCardinality {
		return e.cachedAll, e.cachedSource, e.tagsHash
	} else if cardinality == collectors.OrchestratorCardinality {
		return e.cachedOrchestrator, e.cachedSource, e.tagsHash
	}
	return e.cachedLow, e.cachedSource, e.tagsHash
}

func (e *entityTags) toEntity(cardinality collectors.TagCardinality) Entity {
	e.computeCache()

	standardTags := e.getStandard()

	e.RLock()
	defer e.RUnlock()

	entity := Entity{
		ID:           e.entityID,
		Hash:         e.tagsHash,
		StandardTags: standardTags,
	}

	switch cardinality {
	case collectors.HighCardinality:
		entity.HighCardinalityTags = e.cachedAll[len(e.cachedOrchestrator):]
		fallthrough
	case collectors.OrchestratorCardinality:
		entity.OrchestratorCardinalityTags = e.cachedOrchestrator[len(e.cachedLow):]
		fallthrough
	case collectors.LowCardinality:
		entity.LowCardinalityTags = e.cachedLow
	}

	return entity
}

func (e *entityTags) computeCache() {
	e.Lock()
	defer e.Unlock()

	if e.cacheValid {
		return
	}

	var sources []string
	tagPrioMapper := make(map[string][]tagPriority)

	for source, tags := range e.sourceTags {
		sources = append(sources, source)
		insertWithPriority(tagPrioMapper, tags.lowCardTags, source, collectors.LowCardinality)
		insertWithPriority(tagPrioMapper, tags.orchestratorCardTags, source, collectors.OrchestratorCardinality)
		insertWithPriority(tagPrioMapper, tags.highCardTags, source, collectors.HighCardinality)
	}

	var lowCardTags []string
	var orchestratorCardTags []string
	var highCardTags []string
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
			if !insert {
				continue
			}
			if tags[i].cardinality == collectors.HighCardinality {
				highCardTags = append(highCardTags, tags[i].tag)
				continue
			} else if tags[i].cardinality == collectors.OrchestratorCardinality {
				orchestratorCardTags = append(orchestratorCardTags, tags[i].tag)
				continue
			}
			lowCardTags = append(lowCardTags, tags[i].tag)
		}
	}

	tags := append(lowCardTags, orchestratorCardTags...)
	tags = append(tags, highCardTags...)

	// Write cache
	e.cacheValid = true
	e.cachedSource = sources
	e.cachedAll = tags
	e.cachedLow = e.cachedAll[:len(lowCardTags)]
	e.cachedOrchestrator = e.cachedAll[:len(lowCardTags)+len(orchestratorCardTags)]
	e.tagsHash = computeTagsHash(e.cachedAll)
}

func insertWithPriority(tagPrioMapper map[string][]tagPriority, tags []string, source string, cardinality collectors.TagCardinality) {
	priority, found := collectors.CollectorPriorities[source]
	if !found {
		log.Warnf("Tagger: %s collector has no defined priority, assuming low", source)
		priority = collectors.NodeRuntime
	}

	for _, t := range tags {
		tagName := strings.Split(t, ":")[0]
		tagPrioMapper[tagName] = append(tagPrioMapper[tagName], tagPriority{
			tag:         t,
			priority:    priority,
			cardinality: cardinality,
		})
	}
}
