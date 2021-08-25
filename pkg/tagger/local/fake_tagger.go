// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package local

import (
	"strconv"
	"sync"

	"github.com/DataDog/datadog-agent/cmd/agent/api/response"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util"
)

// FakeTagger implements the Tagger interface
type FakeTagger struct {
	errors map[string]error
	store  *tagStore
	sync.RWMutex
}

// NewFakeTagger returns a new fake Tagger
func NewFakeTagger() *FakeTagger {
	return &FakeTagger{
		errors: make(map[string]error),
		store:  newTagStore(),
	}
}

// FakeTagger specific interface

// SetTags allows to set tags in store for a given source, entity
func (f *FakeTagger) SetTags(entity, source string, low, orch, high, std []string) {
	f.store.processTagInfo([]*collectors.TagInfo{
		{
			Source:               source,
			Entity:               entity,
			LowCardTags:          low,
			OrchestratorCardTags: orch,
			HighCardTags:         high,
			StandardTags:         std,
		},
	})
}

// SetTagsFromInfo allows to set tags from list of TagInfo
func (f *FakeTagger) SetTagsFromInfo(tags []*collectors.TagInfo) {
	f.store.processTagInfo(tags)
}

// SetError allows to set an error to be returned when `Tag` or `TagBuilder` is called
// for this entity and cardinality
func (f *FakeTagger) SetError(entity string, cardinality collectors.TagCardinality, err error) {
	f.Lock()
	defer f.Unlock()

	f.errors[f.getKey(entity, cardinality)] = err
}

// Tagger interface

// Init not implemented in fake tagger
func (f *FakeTagger) Init() error {
	return nil
}

// Stop not implemented in fake tagger
func (f *FakeTagger) Stop() error {
	return nil
}

// Tag fake implementation
func (f *FakeTagger) Tag(entity string, cardinality collectors.TagCardinality) ([]string, error) {
	tags, _ := f.store.lookup(entity, cardinality)

	key := f.getKey(entity, cardinality)
	if err := f.errors[key]; err != nil {
		return nil, err
	}

	return tags, nil
}

// TagBuilder fake implementation
func (f *FakeTagger) TagBuilder(entity string, cardinality collectors.TagCardinality, tb *util.TagsBuilder) error {
	tags, err := f.Tag(entity, cardinality)
	if err != nil {
		return err
	}

	tb.Append(tags...)
	return nil
}

// Standard fake implementation
func (f *FakeTagger) Standard(entity string) ([]string, error) {
	return f.store.lookupStandard(entity)
}

// GetEntity returns faked entity corresponding to the specified id and an error
func (f *FakeTagger) GetEntity(entityID string) (*types.Entity, error) {

	tags, err := f.store.getEntityTags(entityID)
	if err != nil {
		return nil, err
	}

	entity := tags.toEntity()
	return &entity, nil
}

// List fake implementation
func (f *FakeTagger) List(cardinality collectors.TagCardinality) response.TaggerListResponse {
	r := response.TaggerListResponse{
		Entities: make(map[string]response.TaggerListEntity),
	}

	f.store.RLock()
	defer f.store.RUnlock()

	for entityID, et := range f.store.store {
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

// Subscribe fake implementation
func (f *FakeTagger) Subscribe(cardinality collectors.TagCardinality) chan []types.EntityEvent {
	return f.store.subscribe(cardinality)
}

// Unsubscribe fake implementation
func (f *FakeTagger) Unsubscribe(ch chan []types.EntityEvent) {
	f.store.unsubscribe(ch)
}

// Fake internals
func (f *FakeTagger) getKey(entity string, cardinality collectors.TagCardinality) string {
	return entity + strconv.FormatInt(int64(cardinality), 10)
}
