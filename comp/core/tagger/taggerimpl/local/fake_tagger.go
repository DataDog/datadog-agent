// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package local

import (
	"context"
	"strconv"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/collectors"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/empty"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/tagstore"
	"github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// FakeTagger implements the Tagger interface
type FakeTagger struct {
	errors         map[string]error
	store          *tagstore.TagStore
	telemetryStore *telemetry.Store
	sync.RWMutex
	empty.Tagger
}

// NewFakeTagger returns a new fake Tagger
func NewFakeTagger(cfg config.Component, telemetryStore *telemetry.Store) *FakeTagger {
	return &FakeTagger{
		errors:         make(map[string]error),
		store:          tagstore.NewTagStore(cfg, telemetryStore),
		telemetryStore: telemetryStore,
	}
}

// FakeTagger specific interface

// SetTags allows to set tags in store for a given source, entity
func (f *FakeTagger) SetTags(entityID string, source string, low, orch, high, std []string) {
	id, _ := types.NewEntityIDFromString(entityID)
	f.store.ProcessTagInfo([]*types.TagInfo{
		{
			Source:               source,
			EntityID:             id,
			LowCardTags:          low,
			OrchestratorCardTags: orch,
			HighCardTags:         high,
			StandardTags:         std,
		},
	})
}

// SetGlobalTags allows to set tags in store for the global entity
func (f *FakeTagger) SetGlobalTags(low, orch, high, std []string) {
	f.SetTags(collectors.GlobalEntityID.String(), "static", low, orch, high, std)
}

// SetTagsFromInfo allows to set tags from list of TagInfo
func (f *FakeTagger) SetTagsFromInfo(tags []*types.TagInfo) {
	f.store.ProcessTagInfo(tags)
}

// SetError allows to set an error to be returned when `Tag` or `AccumulateTagsFor` is called
// for this entity and cardinality
func (f *FakeTagger) SetError(entityID types.EntityID, cardinality types.TagCardinality, err error) {
	f.Lock()
	defer f.Unlock()

	f.errors[f.getKey(entityID, cardinality)] = err
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

// ReplayTagger returns the replay tagger instance
// This is a no-op for the fake tagger
func (f *FakeTagger) ReplayTagger() tagger.ReplayTagger {
	return nil
}

// GetTaggerTelemetryStore returns tagger telemetry store
func (f *FakeTagger) GetTaggerTelemetryStore() *telemetry.Store {
	return f.telemetryStore
}

// Tag fake implementation
func (f *FakeTagger) Tag(entityID string, cardinality types.TagCardinality) ([]string, error) {
	id, _ := types.NewEntityIDFromString(entityID)
	tags := f.store.Lookup(id, cardinality)

	key := f.getKey(id, cardinality)
	if err := f.errors[key]; err != nil {
		return nil, err
	}

	return tags, nil
}

// GlobalTags fake implementation
func (f *FakeTagger) GlobalTags(cardinality types.TagCardinality) ([]string, error) {
	return f.Tag(collectors.GlobalEntityID.String(), cardinality)
}

// AccumulateTagsFor fake implementation
func (f *FakeTagger) AccumulateTagsFor(entityID string, cardinality types.TagCardinality, tb tagset.TagsAccumulator) error {
	tags, err := f.Tag(entityID, cardinality)
	if err != nil {
		return err
	}

	tb.Append(tags...)
	return nil
}

// Standard fake implementation
func (f *FakeTagger) Standard(entityID string) ([]string, error) {
	id, err := types.NewEntityIDFromString(entityID)
	if err != nil {
		return nil, err
	}
	return f.store.LookupStandard(id)
}

// GetEntity returns faked entity corresponding to the specified id and an error
func (f *FakeTagger) GetEntity(entityID string) (*types.Entity, error) {
	id, err := types.NewEntityIDFromString(entityID)
	if err != nil {
		return nil, err
	}
	return f.store.GetEntity(id)
}

// List fake implementation
func (f *FakeTagger) List() types.TaggerListResponse {
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
func (f *FakeTagger) getKey(entity types.EntityID, cardinality types.TagCardinality) string {
	return entity.String() + strconv.FormatInt(int64(cardinality), 10)
}
