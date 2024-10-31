// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package local implements a local Tagger.
package local

import (
	"context"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	taggercommon "github.com/DataDog/datadog-agent/comp/core/tagger/common"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/collectors"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/empty"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/tagstore"
	"github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// Tagger is the entry class for entity tagging. It hold the tagger collector,
// memory store, and handles the query logic. One should use the package
// methods in comp/core/tagger to use the default Tagger instead of instantiating it
// directly.
type Tagger struct {
	sync.RWMutex

	tagStore      *tagstore.TagStore
	workloadStore workloadmeta.Component
	cfg           config.Component
	collector     *collectors.WorkloadMetaCollector

	ctx            context.Context
	cancel         context.CancelFunc
	telemetryStore *telemetry.Store
	empty.Tagger
}

// NewTagger returns an allocated tagger. You are probably looking for
// tagger.Tag() using the global instance instead of creating your own.
func NewTagger(cfg config.Component, workloadStore workloadmeta.Component, telemetryStore *telemetry.Store) *Tagger {
	return &Tagger{
		tagStore:       tagstore.NewTagStore(cfg, telemetryStore),
		workloadStore:  workloadStore,
		telemetryStore: telemetryStore,
		cfg:            cfg,
	}
}

var _ tagger.Component = NewTagger(nil, nil, nil)

// Start starts the workloadmeta collector and then it is ready for requests.
func (t *Tagger) Start(ctx context.Context) error {
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
func (t *Tagger) Stop() error {
	t.cancel()
	return nil
}

// getTags returns a read only list of tags for a given entity.
func (t *Tagger) getTags(entityID types.EntityID, cardinality types.TagCardinality) (tagset.HashedTags, error) {
	if entityID.Empty() {
		t.telemetryStore.QueriesByCardinality(cardinality).EmptyEntityID.Inc()
		return tagset.HashedTags{}, fmt.Errorf("empty entity ID")
	}

	cachedTags := t.tagStore.LookupHashedWithEntityStr(entityID, cardinality)

	t.telemetryStore.QueriesByCardinality(cardinality).Success.Inc()
	return cachedTags, nil
}

// AccumulateTagsFor appends tags for a given entity from the tagger to the TagsAccumulator
func (t *Tagger) AccumulateTagsFor(entityID types.EntityID, cardinality types.TagCardinality, tb tagset.TagsAccumulator) error {
	tags, err := t.getTags(entityID, cardinality)
	tb.AppendHashed(tags)
	return err
}

// Tag returns a copy of the tags for a given entity
func (t *Tagger) Tag(entityID types.EntityID, cardinality types.TagCardinality) ([]string, error) {
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
func (t *Tagger) LegacyTag(entity string, cardinality types.TagCardinality) ([]string, error) {
	prefix, id, err := taggercommon.ExtractPrefixAndID(entity)
	if err != nil {
		return nil, err
	}

	entityID := types.NewEntityID(prefix, id)
	return t.Tag(entityID, cardinality)
}

// Standard returns standard tags for a given entity
// It triggers a tagger fetch if the no tags are found
func (t *Tagger) Standard(entityID types.EntityID) ([]string, error) {
	if entityID.Empty() {
		return nil, fmt.Errorf("empty entity ID")
	}

	return t.tagStore.LookupStandard(entityID)
}

// GetEntity returns the entity corresponding to the specified id and an error
func (t *Tagger) GetEntity(entityID types.EntityID) (*types.Entity, error) {
	return t.tagStore.GetEntity(entityID)
}

// List the content of the tagger
func (t *Tagger) List() types.TaggerListResponse {
	return t.tagStore.List()
}

// Subscribe returns a channel that receives a slice of events whenever an entity is
// added, modified or deleted. It can send an initial burst of events only to the new
// subscriber, without notifying all of the others.
func (t *Tagger) Subscribe(subscriptionID string, filter *types.Filter) (types.Subscription, error) {
	return t.tagStore.Subscribe(subscriptionID, filter)
}

// ReplayTagger returns the replay tagger instance
// This is a no-op for the local tagger
func (t *Tagger) ReplayTagger() tagger.ReplayTagger {
	return nil
}

// GetTaggerTelemetryStore returns tagger telemetry store
func (t *Tagger) GetTaggerTelemetryStore() *telemetry.Store {
	return t.telemetryStore
}
