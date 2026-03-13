// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package taggerimpl

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tagstore"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// fakeTagger is a fake implementation of the tagger interface
type fakeTagger struct {
	tagger   tagger.Component
	tagStore *tagstore.TagStore
}

// MockRequires is a struct containing the required components for the mock.
type MockRequires struct {
	Config       config.Component
	WorkloadMeta workloadmeta.Component
	Log          logdef.Component
	Telemetry    coretelemetry.Component
}

// MockProvides is a struct containing the mock.
type MockProvides struct {
	Comp taggermock.Mock
}

// NewMock instantiates a new fakeTagger.
func NewMock(req MockRequires) MockProvides {
	tagStore := tagstore.NewTagStore(nil)
	localTagger, err := newLocalTagger(req.Config, req.WorkloadMeta, req.Log, req.Telemetry, tagStore)
	if err != nil {
		log.Errorf("Failed to create local tagger: %v", err)
	}

	return MockProvides{
		Comp: &fakeTagger{
			tagger:   localTagger,
			tagStore: tagStore,
		},
	}
}

// SetTags allows to set tags in tagStore for a given source, entity
func (f *fakeTagger) SetTags(entityID types.EntityID, source string, low, orch, high, std []string) {
	f.tagStore.ProcessTagInfo([]*types.TagInfo{
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

// SetGlobalTags allows to set tags in tagStore for the global entity
func (f *fakeTagger) SetGlobalTags(low, orch, high, std []string) {
	f.SetTags(types.GetGlobalEntityID(), "static", low, orch, high, std)
}

// LoadState loads the state for the tagger from the supplied map.
func (f *fakeTagger) LoadState(state []types.Entity) {
	if state == nil {
		return
	}

	for _, entity := range state {
		f.tagStore.ProcessTagInfo([]*types.TagInfo{{
			Source:               "replay",
			EntityID:             entity.ID,
			HighCardTags:         entity.HighCardinalityTags,
			OrchestratorCardTags: entity.OrchestratorCardinalityTags,
			LowCardTags:          entity.LowCardinalityTags,
			StandardTags:         entity.StandardTags,
			ExpiryDate:           time.Time{},
		}})
	}

	log.Debugf("Loaded %v elements into tag tagStore", len(state))
}

// GetTagStore returns the tag store.
func (f *fakeTagger) GetTagStore() *tagstore.TagStore {
	return f.tagStore
}

// Tagger methods

// Tag calls tagger.Tag().
func (f *fakeTagger) Tag(entityID types.EntityID, cardinality types.TagCardinality) ([]string, error) {
	return f.tagger.Tag(entityID, cardinality)
}

// TagWithCompleteness calls tagger.TagWithCompleteness().
func (f *fakeTagger) TagWithCompleteness(entityID types.EntityID, cardinality types.TagCardinality) ([]string, bool, error) {
	return f.tagger.TagWithCompleteness(entityID, cardinality)
}

// GenerateContainerIDFromOriginInfo calls tagger.GenerateContainerIDFromOriginInfo().
func (f *fakeTagger) GenerateContainerIDFromOriginInfo(originInfo origindetection.OriginInfo) (string, error) {
	return f.tagger.GenerateContainerIDFromOriginInfo(originInfo)
}

// Standard calls tagger.Standard().
func (f *fakeTagger) Standard(entityID types.EntityID) ([]string, error) {
	return f.tagger.Standard(entityID)
}

// List calls tagger.List().
func (f *fakeTagger) List() types.TaggerListResponse {
	return f.tagger.List()
}

// GetEntity calls tagger.GetEntity().
func (f *fakeTagger) GetEntity(entityID types.EntityID) (*types.Entity, error) {
	return f.tagger.GetEntity(entityID)
}

// Subscribe calls tagger.Subscribe().
func (f *fakeTagger) Subscribe(subscriptionID string, filter *types.Filter) (types.Subscription, error) {
	return f.tagger.Subscribe(subscriptionID, filter)
}

// GetEntityHash calls tagger.GetEntityHash().
func (f *fakeTagger) GetEntityHash(entityID types.EntityID, cardinality types.TagCardinality) string {
	return f.tagger.GetEntityHash(entityID, cardinality)
}

// AgentTags calls tagger.AgentTags().
func (f *fakeTagger) AgentTags(cardinality types.TagCardinality) ([]string, error) {
	return f.tagger.AgentTags(cardinality)
}

// GlobalTags calls tagger.GlobalTags().
func (f *fakeTagger) GlobalTags(cardinality types.TagCardinality) ([]string, error) {
	return f.tagger.GlobalTags(cardinality)
}

// EnrichTags calls tagger.EnrichTags().
func (f *fakeTagger) EnrichTags(tb tagset.TagsAccumulator, originInfo taggertypes.OriginInfo) {
	f.tagger.EnrichTags(tb, originInfo)
}
