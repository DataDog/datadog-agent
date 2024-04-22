// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package local

import (
	"context"
	"strconv"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/tagstore"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// FakeTagger implements the Tagger interface
type FakeTagger struct {
	errors map[string]error
	store  *tagstore.TagStore
	sync.RWMutex
}

// NewFakeTagger returns a new fake Tagger
func NewFakeTagger() *FakeTagger {
	return &FakeTagger{
		errors: make(map[string]error),
		store:  tagstore.NewTagStore(),
	}
}

// FakeTagger specific interface

// SetTags allows to set tags in store for a given source, entity
func (f *FakeTagger) SetTags(entity, source string, low, orch, high, std []string) {
	f.store.ProcessTagInfo([]*types.TagInfo{
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
func (f *FakeTagger) SetTagsFromInfo(tags []*types.TagInfo) {
	f.store.ProcessTagInfo(tags)
}

// SetError allows to set an error to be returned when `Tag` or `AccumulateTagsFor` is called
// for this entity and cardinality
func (f *FakeTagger) SetError(entity string, cardinality types.TagCardinality, err error) {
	f.Lock()
	defer f.Unlock()

	f.errors[f.getKey(entity, cardinality)] = err
}

// Tagger interface

// Start not implemented in fake tagger
func (f *FakeTagger) Start(_ context.Context) error {
	return nil
}

// Stop not implemented in fake tagger
func (f *FakeTagger) Stop() error {
	return nil
}

// Tag fake implementation
func (f *FakeTagger) Tag(entity string, cardinality types.TagCardinality) ([]string, error) {
	tags := f.store.Lookup(entity, cardinality)

	key := f.getKey(entity, cardinality)
	if err := f.errors[key]; err != nil {
		return nil, err
	}

	return tags, nil
}

// AccumulateTagsFor fake implementation
func (f *FakeTagger) AccumulateTagsFor(entity string, cardinality types.TagCardinality, tb tagset.TagsAccumulator) error {
	tags, err := f.Tag(entity, cardinality)
	if err != nil {
		return err
	}

	tb.Append(tags...)
	return nil
}

// Standard fake implementation
func (f *FakeTagger) Standard(entity string) ([]string, error) {
	return f.store.LookupStandard(entity)
}

// GetEntity returns faked entity corresponding to the specified id and an error
func (f *FakeTagger) GetEntity(entityID string) (*types.Entity, error) {
	return f.store.GetEntity(entityID)
}

// List fake implementation
func (f *FakeTagger) List(types.TagCardinality) types.TaggerListResponse {
	return f.store.List()
}

// Subscribe fake implementation
func (f *FakeTagger) Subscribe(cardinality types.TagCardinality) chan []types.EntityEvent {
	return f.store.Subscribe(cardinality)
}

// Unsubscribe fake implementation
func (f *FakeTagger) Unsubscribe(ch chan []types.EntityEvent) {
	f.store.Unsubscribe(ch)
}

// Fake internals
func (f *FakeTagger) getKey(entity string, cardinality types.TagCardinality) string {
	return entity + strconv.FormatInt(int64(cardinality), 10)
}

// GetEntityHash returns the hash for the tags associated with the given entity
// Returns an empty string if the tags lookup fails
func (f *FakeTagger) GetEntityHash(string, types.TagCardinality) string {
	return ""
}

// AgentTags returns the agent tags
// It relies on the container provider utils to get the Agent container ID
func (f *FakeTagger) AgentTags(types.TagCardinality) ([]string, error) {
	return []string{}, nil
}

// GlobalTags queries global tags that should apply to all data coming from the
// agent.
func (f *FakeTagger) GlobalTags(types.TagCardinality) ([]string, error) {
	return []string{}, nil
}

// SetNewCaptureTagger sets the tagger to be used when replaying a capture
func (f *FakeTagger) SetNewCaptureTagger(tagger.Component) {}

// ResetCaptureTagger resets the capture tagger to nil
func (f *FakeTagger) ResetCaptureTagger() {}

// EnrichTags extends a tag list with origin detection tags
func (f *FakeTagger) EnrichTags(tagset.TagsAccumulator, taggertypes.OriginInfo) {}
