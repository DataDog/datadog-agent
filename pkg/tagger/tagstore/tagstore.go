// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tagstore implements the TagStore which is the component of the Tagger
// responsible for storing the tags in memory.
package tagstore

import (
	"context"
	"errors"
	"sync"
	"time"

	tagger_api "github.com/DataDog/datadog-agent/pkg/tagger/api"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/subscriber"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"

	"github.com/benbjohnson/clock"
)

const (
	deletedTTL = 5 * time.Minute
)

// ErrNotFound is returned when entity id is not found in the store.
var ErrNotFound = errors.New("entity not found")

// TagStore stores entity tags in memory and handles search and collation.
// Queries should go through the Tagger for cache-miss handling
type TagStore struct {
	sync.RWMutex

	store     map[string]*EntityTags
	telemetry map[string]map[string]float64

	subscriber *subscriber.Subscriber

	clock clock.Clock
}

// NewTagStore creates new TagStore.
func NewTagStore() *TagStore {
	return newTagStoreWithClock(clock.New())
}

func newTagStoreWithClock(clock clock.Clock) *TagStore {
	return &TagStore{
		telemetry:  make(map[string]map[string]float64),
		store:      make(map[string]*EntityTags),
		subscriber: subscriber.NewSubscriber(),
		clock:      clock,
	}
}

// Run performs background maintenance for TagStore.
func (s *TagStore) Run(ctx context.Context) {
	panic("not called")
}

// ProcessTagInfo updates tagger store with tags fetched by collectors.
func (s *TagStore) ProcessTagInfo(tagInfos []*collectors.TagInfo) {
	panic("not called")
}

func (s *TagStore) collectTelemetry() {
	panic("not called")
}

// Subscribe returns a channel that receives a slice of events whenever an entity is
// added, modified or deleted. It can send an initial burst of events only to the new
// subscriber, without notifying all of the others.
func (s *TagStore) Subscribe(cardinality collectors.TagCardinality) chan []types.EntityEvent {
	panic("not called")
}

// Unsubscribe ends a subscription to entity events and closes its channel.
func (s *TagStore) Unsubscribe(ch chan []types.EntityEvent) {
	panic("not called")
}

func (s *TagStore) notifySubscribers(events []types.EntityEvent) {
	panic("not called")
}

// Prune deletes tags for entities that have been marked as deleted. This is to
// be called regularly from the user class.
func (s *TagStore) Prune() {
	panic("not called")
}

// LookupHashed gets tags from the store and returns them as a HashedTags instance. It
// returns the source names in the second slice to allow the client to trigger manual
// lookups on missing sources.
func (s *TagStore) LookupHashed(entity string, cardinality collectors.TagCardinality) tagset.HashedTags {
	s.RLock()
	defer s.RUnlock()
	storedTags, present := s.store[entity]

	if !present {
		return tagset.HashedTags{}
	}
	return storedTags.getHashedTags(cardinality)
}

// Lookup gets tags from the store and returns them concatenated in a string slice. It
// returns the source names in the second slice to allow the client to trigger manual
// lookups on missing sources.
func (s *TagStore) Lookup(entity string, cardinality collectors.TagCardinality) []string {
	return s.LookupHashed(entity, cardinality).Get()
}

// LookupStandard returns the standard tags recorded for a given entity
func (s *TagStore) LookupStandard(entityID string) ([]string, error) {
	panic("not called")
}

// GetEntityTags returns the EntityTags for a given entity
func (s *TagStore) GetEntityTags(entityID string) (*EntityTags, error) {
	panic("not called")
}

// List returns full list of entities and their tags per source in an API format.
func (s *TagStore) List() tagger_api.TaggerListResponse {
	panic("not called")
}

// GetEntity returns the entity corresponding to the specified id and an error
func (s *TagStore) GetEntity(entityID string) (*types.Entity, error) {
	panic("not called")
}
