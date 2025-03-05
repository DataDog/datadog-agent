// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package taggerimpl

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tagstore"
	"github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// FakeTagger is a fake implementation of the tagger interface
type FakeTagger struct {
	tagger   tagger.Component
	tagStore *tagstore.TagStore
}

type MockRequires struct {
	Config       config.Component
	WorkloadMeta workloadmeta.Component
	Log          logdef.Component
	Telemetry    coretelemetry.Component
}

// MockProvides is a struct containing the mock and the endpoint
type MockProvides struct {
	Comp taggermock.Mock
}

// New instantiates a new fake tagger
func New(req MockRequires) MockProvides {
	tagStore := tagstore.NewTagStore(nil)
	localTagger, err := newLocalTagger(req.Config, req.WorkloadMeta, req.Log, req.Telemetry, tagStore)
	if err != nil {
		log.Errorf("Failed to create local tagger: %v", err)
	}

	return MockProvides{
		Comp: &FakeTagger{
			tagger:   localTagger,
			tagStore: tagStore,
		},
	}
}

// SetTags allows to set tags in tagStore for a given source, entity
func (f *FakeTagger) SetTags(entityID types.EntityID, source string, low, orch, high, std []string) {
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
func (f *FakeTagger) SetGlobalTags(low, orch, high, std []string) {
	f.SetTags(types.GetGlobalEntityID(), "static", low, orch, high, std)
}

// LoadState loads the state for the tagger from the supplied map.
func (f *FakeTagger) LoadState(state []types.Entity) {
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
func (f *FakeTagger) GetTagStore() *tagstore.TagStore {
	return f.tagStore
}

// Tagger methods

// Start calls tagger.Start().
func (f *FakeTagger) Start(ctx context.Context) error {
	return f.tagger.Start(ctx)
}

// Stop calls tagger.Stop().
func (f *FakeTagger) Stop() error {
	return f.tagger.Stop()
}

// GetTaggerTelemetryStore calls tagger.GetTaggerTelemetryStore().
func (f *FakeTagger) GetTaggerTelemetryStore() *telemetry.Store {
	return f.tagger.GetTaggerTelemetryStore()
}

// LegacyTag calls tagger.LegacyTag().
func (f *FakeTagger) LegacyTag(entity string, cardinality types.TagCardinality) ([]string, error) {
	return f.tagger.LegacyTag(entity, cardinality)
}

// Tag calls tagger.Tag().
func (f *FakeTagger) Tag(entityID types.EntityID, cardinality types.TagCardinality) ([]string, error) {
	return f.tagger.Tag(entityID, cardinality)
}

// GenerateContainerIDFromOriginInfo calls tagger.GenerateContainerIDFromOriginInfo().
func (f *FakeTagger) GenerateContainerIDFromOriginInfo(originInfo origindetection.OriginInfo) (string, error) {
	return f.tagger.GenerateContainerIDFromOriginInfo(originInfo)
}

// AccumulateTagsFor calls tagger.AccumulateTagsFor().
func (f *FakeTagger) AccumulateTagsFor(entityID types.EntityID, cardinality types.TagCardinality, tb tagset.TagsAccumulator) error {
	return f.tagger.AccumulateTagsFor(entityID, cardinality, tb)
}

// Standard calls tagger.Standard().
func (f *FakeTagger) Standard(entityID types.EntityID) ([]string, error) {
	return f.tagger.Standard(entityID)
}

// List calls tagger.List().
func (f *FakeTagger) List() types.TaggerListResponse {
	return f.tagger.List()
}

// GetEntity calls tagger.GetEntity().
func (f *FakeTagger) GetEntity(entityID types.EntityID) (*types.Entity, error) {
	return f.tagger.GetEntity(entityID)
}

// Subscribe calls tagger.Subscribe().
func (f *FakeTagger) Subscribe(subscriptionID string, filter *types.Filter) (types.Subscription, error) {
	return f.tagger.Subscribe(subscriptionID, filter)
}

// GetEntityHash calls tagger.GetEntityHash().
func (f *FakeTagger) GetEntityHash(entityID types.EntityID, cardinality types.TagCardinality) string {
	return f.tagger.GetEntityHash(entityID, cardinality)
}

// AgentTags calls tagger.AgentTags().
func (f *FakeTagger) AgentTags(cardinality types.TagCardinality) ([]string, error) {
	return f.tagger.AgentTags(cardinality)
}

// GlobalTags calls tagger.GlobalTags().
func (f *FakeTagger) GlobalTags(cardinality types.TagCardinality) ([]string, error) {
	return f.tagger.GlobalTags(cardinality)
}

// EnrichTags calls tagger.EnrichTags().
func (f *FakeTagger) EnrichTags(tb tagset.TagsAccumulator, originInfo taggertypes.OriginInfo) {
	f.tagger.EnrichTags(tb, originInfo)
}

// ChecksCardinality calls tagger.ChecksCardinality().
func (f *FakeTagger) ChecksCardinality() types.TagCardinality {
	return f.tagger.ChecksCardinality()
}
