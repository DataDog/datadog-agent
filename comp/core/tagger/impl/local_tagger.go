// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package taggerimpl

import (
	"context"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger/collectors"
	taggercommon "github.com/DataDog/datadog-agent/comp/core/tagger/common"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tagstore"
	"github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// Tagger is the entry class for entity tagging. It hold the tagger collector,
// memory store, and handles the query logic. One should use the package
// methods in comp/core/tagger to use the default Tagger instead of instantiating it
// directly.
type localTagger struct {
	sync.RWMutex

	tagStore      *tagstore.TagStore
	workloadStore workloadmeta.Component
	cfg           config.Component
	collector     *collectors.WorkloadMetaCollector

	ctx            context.Context
	cancel         context.CancelFunc
	telemetryStore *telemetry.Store
}

func newLocalTagger(cfg config.Component, wmeta workloadmeta.Component, telemetryStore *telemetry.Store) (tagger.Component, error) {
	return &localTagger{
		tagStore:       tagstore.NewTagStore(telemetryStore),
		workloadStore:  wmeta,
		telemetryStore: telemetryStore,
		cfg:            cfg,
	}, nil
}

// Start starts the workloadmeta collector and then it is ready for requests.
func (t *localTagger) Start(ctx context.Context) error {
	t.ctx, t.cancel = context.WithCancel(ctx)

	t.collector = collectors.NewWorkloadMetaCollector(
		t.ctx,
		t.cfg,
		t.workloadStore,
		t.tagStore,
	)

	go t.tagStore.Run(t.ctx)
	go t.collector.Run(t.ctx)

	return nil
}

// Stop queues a shutdown of Tagger
func (t *localTagger) Stop() error {
	t.cancel()
	return nil
}

// getTags returns a read only list of tags for a given entity.
func (t *localTagger) getTags(entityID types.EntityID, cardinality types.TagCardinality) (tagset.HashedTags, error) {
	if entityID.Empty() {
		t.telemetryStore.QueriesByCardinality(cardinality).EmptyEntityID.Inc()
		return tagset.HashedTags{}, fmt.Errorf("empty entity ID")
	}

	cachedTags := t.tagStore.LookupHashedWithEntityStr(entityID, cardinality)

	t.telemetryStore.QueriesByCardinality(cardinality).Success.Inc()
	return cachedTags, nil
}

// AccumulateTagsFor appends tags for a given entity from the tagger to the TagsAccumulator
func (t *localTagger) AccumulateTagsFor(entityID types.EntityID, cardinality types.TagCardinality, tb tagset.TagsAccumulator) error {
	tags, err := t.getTags(entityID, cardinality)
	tb.AppendHashed(tags)
	return err
}

// Tag returns a copy of the tags for a given entity
func (t *localTagger) Tag(entityID types.EntityID, cardinality types.TagCardinality) ([]string, error) {
	tags, err := t.getTags(entityID, cardinality)
	if err != nil {
		return nil, err
	}
	return tags.Copy(), nil
}

// LegacyTag has the same behaviour as the Tag method, but it receives the entity id as a string and parses it.
// If possible, avoid using this function, and use the Tag method instead.
// This function exists in order not to break backward compatibility with rtloader and python
// integrations using the tagger
func (t *localTagger) LegacyTag(entity string, cardinality types.TagCardinality) ([]string, error) {
	prefix, id, err := taggercommon.ExtractPrefixAndID(entity)
	if err != nil {
		return nil, err
	}

	entityID := types.NewEntityID(prefix, id)
	return t.Tag(entityID, cardinality)
}

// Standard returns standard tags for a given entity
// It triggers a tagger fetch if the no tags are found
func (t *localTagger) Standard(entityID types.EntityID) ([]string, error) {
	if entityID.Empty() {
		return nil, fmt.Errorf("empty entity ID")
	}

	return t.tagStore.LookupStandard(entityID)
}

// GetEntity returns the entity corresponding to the specified id and an error
func (t *localTagger) GetEntity(entityID types.EntityID) (*types.Entity, error) {
	return t.tagStore.GetEntity(entityID)
}

// List the content of the tagger
func (t *localTagger) List() types.TaggerListResponse {
	return t.tagStore.List()
}

// Subscribe returns a channel that receives a slice of events whenever an entity is
// added, modified or deleted. It can send an initial burst of events only to the new
// subscriber, without notifying all of the others.
func (t *localTagger) Subscribe(subscriptionID string, filter *types.Filter) (types.Subscription, error) {
	return t.tagStore.Subscribe(subscriptionID, filter)
}

// ReplayTagger returns the replay tagger instance
// This is a no-op for the local tagger
func (t *localTagger) ReplayTagger() tagger.ReplayTagger {
	return nil
}

// GetTaggerTelemetryStore returns tagger telemetry store
func (t *localTagger) GetTaggerTelemetryStore() *telemetry.Store {
	return t.telemetryStore
}

func (t *localTagger) GetEntityHash(types.EntityID, types.TagCardinality) string {
	return ""
}

func (t *localTagger) AgentTags(types.TagCardinality) ([]string, error) {
	return []string{}, nil
}

func (t *localTagger) GlobalTags(types.TagCardinality) ([]string, error) {
	return []string{}, nil
}

func (t *localTagger) SetNewCaptureTagger(tagger.Component) {}

func (t *localTagger) ResetCaptureTagger() {}

func (t *localTagger) EnrichTags(tagset.TagsAccumulator, taggertypes.OriginInfo) {}

func (t *localTagger) ChecksCardinality() types.TagCardinality {
	return types.LowCardinality
}

func (t *localTagger) DogstatsdCardinality() types.TagCardinality {
	return types.LowCardinality
}
