// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagstore

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/api/response"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/subscriber"
	"github.com/DataDog/datadog-agent/pkg/tagger/telemetry"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	tagInfoBufferSize = 50
	deletedTTL        = 5 * time.Minute
)

// ErrNotFound is returned when entity id is not found in the store.
var ErrNotFound = errors.New("entity not found")

// EntityTags holds the tag information for a given entity. It is not
// thread-safe, so should not be shared outside of the store. Usage inside the
// store is safe since it relies on a global lock.
type EntityTags struct {
	entityID           string
	sourceTags         map[string]sourceTags
	cacheValid         bool
	cachedSource       []string
	cachedAll          util.HashedTags // Low + orchestrator + high
	cachedOrchestrator util.HashedTags // Low + orchestrator (subslice of cachedAll)
	cachedLow          util.HashedTags // Sub-slice of cachedAll
}

func newEntityTags(entityID string) *EntityTags {
	return &EntityTags{
		entityID:   entityID,
		sourceTags: make(map[string]sourceTags),
		cacheValid: true,
	}
}

// sourceTags holds the tags for a given entity collected from a single source,
// grouped by their cardinality.
type sourceTags struct {
	lowCardTags          []string
	orchestratorCardTags []string
	highCardTags         []string
	standardTags         []string
	expiryDate           time.Time
}

// TagStore stores entity tags in memory and handles search and collation.
// Queries should go through the Tagger for cache-miss handling
type TagStore struct {
	sync.RWMutex

	store     map[string]*EntityTags
	telemetry map[string]map[string]float64
	InfoIn    chan []*collectors.TagInfo

	subscriber *subscriber.Subscriber

	clock clock
}

// NewTagStore creates new TagStore.
func NewTagStore() *TagStore {
	return &TagStore{
		telemetry:  make(map[string]map[string]float64),
		store:      make(map[string]*EntityTags),
		InfoIn:     make(chan []*collectors.TagInfo, tagInfoBufferSize),
		subscriber: subscriber.NewSubscriber(),
		clock:      realClock{},
	}
}

// Run performs background maintenance for TagStore.
func (s *TagStore) Run(ctx context.Context) {
	pruneTicker := time.NewTicker(1 * time.Minute)
	telemetryTicker := time.NewTicker(1 * time.Minute)
	health := health.RegisterLiveness("tagger-store")

	for {
		select {
		case msg := <-s.InfoIn:
			s.ProcessTagInfo(msg)

		case <-telemetryTicker.C:
			s.collectTelemetry()

		case <-pruneTicker.C:
			s.Prune()

		case <-health.C:

		case <-ctx.Done():
			pruneTicker.Stop()
			telemetryTicker.Stop()

			return
		}
	}
}

// ProcessTagInfo updates tagger store with tags fetched by collectors.
func (s *TagStore) ProcessTagInfo(tagInfos []*collectors.TagInfo) {
	events := []types.EntityEvent{}

	s.Lock()
	defer s.Unlock()

	for _, info := range tagInfos {
		if info == nil {
			log.Tracef("ProcessTagInfo err: skipping nil message")
			continue
		}
		if info.Entity == "" {
			log.Tracef("ProcessTagInfo err: empty entity name, skipping message")
			continue
		}
		if info.Source == "" {
			log.Tracef("ProcessTagInfo err: empty source name, skipping message")
			continue
		}

		storedTags, exist := s.store[info.Entity]

		if info.DeleteEntity {
			if exist {
				st, ok := storedTags.sourceTags[info.Source]
				if ok {
					st.expiryDate = s.clock.Now().Add(deletedTTL)
					storedTags.sourceTags[info.Source] = st
				}
			}

			continue
		}

		eventType := types.EventTypeModified
		if !exist {
			eventType = types.EventTypeAdded
			storedTags = newEntityTags(info.Entity)
			s.store[info.Entity] = storedTags
		}

		// TODO: check if real change

		_, found := storedTags.sourceTags[info.Source]
		if found && info.CacheMiss {
			log.Tracef("ProcessTagInfo err: try to overwrite an existing entry with and empty cache-miss entry, info.Source: %s, info.Entity: %s", info.Source, info.Entity)
			continue
		}

		telemetry.UpdatedEntities.Inc()
		updateStoredTags(storedTags, info)

		events = append(events, types.EntityEvent{
			EventType: eventType,
			Entity:    storedTags.toEntity(),
		})
	}

	if len(events) > 0 {
		s.notifySubscribers(events)
	}
}

func updateStoredTags(storedTags *EntityTags, info *collectors.TagInfo) {
	storedTags.cacheValid = false
	storedTags.sourceTags[info.Source] = sourceTags{
		lowCardTags:          info.LowCardTags,
		orchestratorCardTags: info.OrchestratorCardTags,
		highCardTags:         info.HighCardTags,
		standardTags:         info.StandardTags,
		expiryDate:           info.ExpiryDate,
	}
}

func (s *TagStore) collectTelemetry() {
	// our telemetry package does not seem to have a way to reset a Gauge,
	// so we need to keep track of all the labels we use, and re-set them
	// to zero after we're done to ensure a new run of collectTelemetry
	// will not forget to clear them if they disappear.

	s.Lock()
	defer s.Unlock()

	for _, entityTags := range s.store {
		prefix, _ := containers.SplitEntityName(entityTags.entityID)

		for source := range entityTags.sourceTags {
			if _, ok := s.telemetry[prefix]; !ok {
				s.telemetry[prefix] = make(map[string]float64)
			}

			s.telemetry[prefix][source]++
		}
	}

	for prefix, sources := range s.telemetry {
		for source, storedEntities := range sources {
			telemetry.StoredEntities.Set(storedEntities, source, prefix)
			s.telemetry[prefix][source] = 0
		}
	}
}

// Subscribe returns a list of existing entities in the store, alongside a
// channel that receives events whenever an entity is added, modified or
// deleted.
func (s *TagStore) Subscribe(cardinality collectors.TagCardinality) chan []types.EntityEvent {
	s.RLock()
	defer s.RUnlock()

	events := make([]types.EntityEvent, 0, len(s.store))
	for _, storedTags := range s.store {
		events = append(events, types.EntityEvent{
			EventType: types.EventTypeAdded,
			Entity:    storedTags.toEntity(),
		})
	}

	return s.subscriber.Subscribe(cardinality, events)
}

// Unsubscribe ends a subscription to entity events and closes its channel.
func (s *TagStore) Unsubscribe(ch chan []types.EntityEvent) {
	s.subscriber.Unsubscribe(ch)
}

func (s *TagStore) notifySubscribers(events []types.EntityEvent) {
	s.subscriber.Notify(events)
}

// Prune deletes tags for entities that are deleted or with empty entries.
// This is to be called regularly from the user class.
func (s *TagStore) Prune() {
	s.Lock()
	defer s.Unlock()

	now := s.clock.Now()
	events := []types.EntityEvent{}

	for entity, storedTags := range s.store {
		changed := false

		// remove any sourceTags that have expired
		for source, st := range storedTags.sourceTags {
			if st.isExpired(now) {
				delete(storedTags.sourceTags, source)
				changed = true
			}
		}

		// remove all sourceTags only if they're all empty
		if storedTags.shouldRemove() {
			storedTags.sourceTags = nil
			changed = true
		}

		if !changed {
			continue
		}

		if len(storedTags.sourceTags) == 0 {
			telemetry.PrunedEntities.Inc()
			delete(s.store, entity)
			events = append(events, types.EntityEvent{
				EventType: types.EventTypeDeleted,
				Entity:    storedTags.toEntity(),
			})
		} else {
			storedTags.cacheValid = false
			events = append(events, types.EntityEvent{
				EventType: types.EventTypeModified,
				Entity:    storedTags.toEntity(),
			})
		}
	}

	if len(events) > 0 {
		s.notifySubscribers(events)
	}
}

// LookupHashed gets tags from the store and returns them as a TagsBuilder instance. It
// returns the source names in the second slice to allow the client to trigger manual
// lookups on missing sources.
func (s *TagStore) LookupHashed(entity string, cardinality collectors.TagCardinality) (*util.HashedTags, []string) {
	s.RLock()
	defer s.RUnlock()
	storedTags, present := s.store[entity]

	if present == false {
		return nil, nil
	}
	return storedTags.getBuilder(cardinality)
}

// Lookup gets tags from the store and returns them concatenated in a string slice. It
// returns the source names in the second slice to allow the client to trigger manual
// lookups on missing sources.
func (s *TagStore) Lookup(entity string, cardinality collectors.TagCardinality) ([]string, []string) {
	tags, sources := s.LookupHashed(entity, cardinality)
	if tags == nil {
		return nil, sources
	}
	return tags.Get(), sources
}

// LookupStandard returns the standard tags recorded for a given entity
func (s *TagStore) LookupStandard(entityID string) ([]string, error) {

	storedTags, err := s.GetEntityTags(entityID)
	if err != nil {
		return nil, err
	}

	return storedTags.getStandard(), nil
}

// GetEntityTags returns the standard tags recorded for a given entity
func (s *TagStore) GetEntityTags(entityID string) (*EntityTags, error) {
	s.RLock()
	defer s.RUnlock()

	storedTags, present := s.store[entityID]
	if present == false {
		return nil, ErrNotFound
	}

	return storedTags, nil
}

func (e *EntityTags) getStandard() []string {
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

func (e *EntityTags) get(cardinality collectors.TagCardinality) ([]string, []string) {
	tags, sources := e.getBuilder(cardinality)
	return tags.Get(), sources
}

func (e *EntityTags) getBuilder(cardinality collectors.TagCardinality) (*util.HashedTags, []string) {
	e.computeCache()

	if cardinality == collectors.HighCardinality {
		return &e.cachedAll, e.cachedSource
	} else if cardinality == collectors.OrchestratorCardinality {
		return &e.cachedOrchestrator, e.cachedSource
	}
	return &e.cachedLow, e.cachedSource
}

func (e *EntityTags) toEntity() types.Entity {
	e.computeCache()

	cachedAll := e.cachedAll.Get()
	cachedOrchestrator := e.cachedOrchestrator.Get()
	cachedLow := e.cachedLow.Get()

	return types.Entity{
		ID:                          e.entityID,
		StandardTags:                e.getStandard(),
		HighCardinalityTags:         cachedAll[len(cachedOrchestrator):],
		OrchestratorCardinalityTags: cachedOrchestrator[len(cachedLow):],
		LowCardinalityTags:          cachedLow,
	}
}

// List returns full list of entities and their tags per source in an API format.
func (s *TagStore) List() response.TaggerListResponse {
	r := response.TaggerListResponse{
		Entities: make(map[string]response.TaggerListEntity),
	}

	s.RLock()
	defer s.RUnlock()

	for entityID, et := range s.store {
		entity := response.TaggerListEntity{
			Tags: make(map[string][]string),
		}

		for source, sourceTags := range et.sourceTags {
			tags := append([]string(nil), sourceTags.lowCardTags...)
			tags = append(tags, sourceTags.orchestratorCardTags...)
			tags = append(tags, sourceTags.highCardTags...)
			entity.Tags[source] = tags
		}

		r.Entities[entityID] = entity
	}

	return r
}

func (e *EntityTags) computeCache() {
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

	cached := util.NewHashedTagsFromSlice(tags)

	// Write cache
	e.cacheValid = true
	e.cachedSource = sources
	e.cachedAll = cached
	e.cachedLow = cached.Slice(0, len(lowCardTags))
	e.cachedOrchestrator = cached.Slice(0, len(lowCardTags)+len(orchestratorCardTags))
}

func (e *EntityTags) shouldRemove() bool {
	for _, tags := range e.sourceTags {
		if !tags.expiryDate.IsZero() || !tags.isEmpty() {
			return false
		}
	}

	return true
}

func (st *sourceTags) isEmpty() bool {
	return len(st.lowCardTags) == 0 && len(st.orchestratorCardTags) == 0 && len(st.highCardTags) == 0 && len(st.standardTags) == 0
}

func (st *sourceTags) isExpired(t time.Time) bool {
	if st.expiryDate.IsZero() {
		return false
	}

	return st.expiryDate.Before(t)
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

// GetEntity returns the entity corresponding to the specified id and an error
func (s *TagStore) GetEntity(entityID string) (*types.Entity, error) {
	tags, err := s.GetEntityTags(entityID)
	if err != nil {
		return nil, err
	}

	entity := tags.toEntity()
	return &entity, nil
}
