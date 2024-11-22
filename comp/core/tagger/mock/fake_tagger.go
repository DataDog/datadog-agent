// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mock

import (
	"context"
	"strconv"

	taggercommon "github.com/DataDog/datadog-agent/comp/core/tagger/common"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tagstore"
	"github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// Mock implements mock-specific methods for the tagger component.
type Mock interface {
	tagger.Component

	// SetTags allows to set tags in the mock fake tagger
	SetTags(entityID types.EntityID, source string, low, orch, high, std []string)

	// SetGlobalTags allows to set tags in store for the global entity
	SetGlobalTags(low, orch, high, std []string)
}

// FakeTagger is a fake implementation of the tagger interface
type FakeTagger struct {
	errors map[string]error
	store  *tagstore.TagStore
}

// Provides is a struct containing the mock and the endpoint
type Provides struct {
	Comp Mock
}

// New instantiates a new fake tagger
func New() Provides {
	return Provides{
		Comp: &FakeTagger{
			errors: make(map[string]error),
			store:  tagstore.NewTagStore(nil),
		},
	}
}

// SetTags allows to set tags in store for a given source, entity
func (f *FakeTagger) SetTags(entityID types.EntityID, source string, low, orch, high, std []string) {
	f.store.ProcessTagInfo([]*types.TagInfo{
		{
			Source:               source,
			EntityID:             entityID,
			LowCardTags:          low,
			OrchestratorCardTags: orch,
			HighCardTags:         high,
			StandardTags:         std,
		},
	})
}

// SetGlobalTags allows to set tags in store for the global entity
func (f *FakeTagger) SetGlobalTags(low, orch, high, std []string) {
	f.SetTags(taggercommon.GetGlobalEntityID(), "static", low, orch, high, std)
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
// The fake tagger returns nil as it doesn't use telemetry
func (f *FakeTagger) GetTaggerTelemetryStore() *telemetry.Store {
	return nil
}

// Tag fake implementation
func (f *FakeTagger) Tag(entityID types.EntityID, cardinality types.TagCardinality) ([]string, error) {
	tags := f.store.Lookup(entityID, cardinality)

	key := f.getKey(entityID, cardinality)
	if err := f.errors[key]; err != nil {
		return nil, err
	}

	return tags, nil
}

// LegacyTag has the same behaviour as the Tag method, but it receives the entity id as a string and parses it.
// If possible, avoid using this function, and use the Tag method instead.
// This function exists in order not to break backward compatibility with rtloader and python
// integrations using the tagger
func (f *FakeTagger) LegacyTag(entity string, cardinality types.TagCardinality) ([]string, error) {
	prefix, id, err := taggercommon.ExtractPrefixAndID(entity)
	if err != nil {
		return nil, err
	}

	entityID := types.NewEntityID(prefix, id)
	return f.Tag(entityID, cardinality)
}

// GlobalTags fake implementation
func (f *FakeTagger) GlobalTags(cardinality types.TagCardinality) ([]string, error) {
	return f.Tag(taggercommon.GetGlobalEntityID(), cardinality)
}

// AccumulateTagsFor fake implementation
func (f *FakeTagger) AccumulateTagsFor(entityID types.EntityID, cardinality types.TagCardinality, tb tagset.TagsAccumulator) error {
	tags, err := f.Tag(entityID, cardinality)
	if err != nil {
		return err
	}

	tb.Append(tags...)
	return nil
}

// Standard fake implementation
func (f *FakeTagger) Standard(entityID types.EntityID) ([]string, error) {
	return f.store.LookupStandard(entityID)
}

// GetEntity returns faked entity corresponding to the specified id and an error
func (f *FakeTagger) GetEntity(entityID types.EntityID) (*types.Entity, error) {
	return f.store.GetEntity(entityID)
}

// List fake implementation
func (f *FakeTagger) List() types.TaggerListResponse {
	return f.store.List()
}

// Subscribe fake implementation
func (f *FakeTagger) Subscribe(subscriptionID string, filter *types.Filter) (types.Subscription, error) {
	return f.store.Subscribe(subscriptionID, filter)
}

// Fake internals
func (f *FakeTagger) getKey(entity types.EntityID, cardinality types.TagCardinality) string {
	return entity.String() + strconv.FormatInt(int64(cardinality), 10)
}

// GetEntityHash noop
func (f *FakeTagger) GetEntityHash(types.EntityID, types.TagCardinality) string {
	return ""
}

// AgentTags noop
func (f *FakeTagger) AgentTags(types.TagCardinality) ([]string, error) {
	return []string{}, nil
}

// SetNewCaptureTagger noop
func (f *FakeTagger) SetNewCaptureTagger(tagger.Component) {}

// ResetCaptureTagger noop
func (f *FakeTagger) ResetCaptureTagger() {}

// EnrichTags noop
func (f *FakeTagger) EnrichTags(tagset.TagsAccumulator, taggertypes.OriginInfo) {}

// ChecksCardinality noop
func (f *FakeTagger) ChecksCardinality() types.TagCardinality {
	return types.LowCardinality
}

// DogstatsdCardinality noop
func (f *FakeTagger) DogstatsdCardinality() types.TagCardinality {
	return types.LowCardinality
}
