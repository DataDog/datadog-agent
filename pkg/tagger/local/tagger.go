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

	"github.com/DataDog/datadog-agent/comp/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/tagset"

	tagger_api "github.com/DataDog/datadog-agent/pkg/tagger/api"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/tagstore"
	"github.com/DataDog/datadog-agent/pkg/tagger/telemetry"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
)

// Tagger is the entry class for entity tagging. It hold the tagger collector,
// memory store, and handles the query logic. One should use the package
// methods in pkg/tagger to use the default Tagger instead of instantiating it
// directly.
type Tagger struct {
	sync.RWMutex

	tagStore      *tagstore.TagStore
	workloadStore workloadmeta.Component
	collector     *collectors.WorkloadMetaCollector

	ctx    context.Context
	cancel context.CancelFunc
}

// NewTagger returns an allocated tagger. You still have to run Init() once the
// config package is ready. You are probably looking for tagger.Tag() using
// the global instance instead of creating your own.
func NewTagger(workloadStore workloadmeta.Component) *Tagger {
	return &Tagger{
		tagStore:      tagstore.NewTagStore(),
		workloadStore: workloadStore,
	}
}

// Init goes through a catalog and tries to detect which are relevant
// for this host. It then starts the collection logic and is ready for
// requests.
func (t *Tagger) Init(ctx context.Context) error {
	t.ctx, t.cancel = context.WithCancel(ctx)

	t.collector = collectors.NewWorkloadMetaCollector(
		t.ctx,
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
func (t *Tagger) getTags(entity string, cardinality collectors.TagCardinality) (tagset.HashedTags, error) {
	if entity == "" {
		telemetry.QueriesByCardinality(cardinality).EmptyEntityID.Inc()
		return tagset.HashedTags{}, fmt.Errorf("empty entity ID")
	}

	cachedTags := t.tagStore.LookupHashed(entity, cardinality)

	telemetry.QueriesByCardinality(cardinality).Success.Inc()
	return cachedTags, nil
}

// AccumulateTagsFor appends tags for a given entity from the tagger to the TagsAccumulator
func (t *Tagger) AccumulateTagsFor(entity string, cardinality collectors.TagCardinality, tb tagset.TagsAccumulator) error {
	tags, err := t.getTags(entity, cardinality)
	tb.AppendHashed(tags)
	return err
}

// Tag returns a copy of the tags for a given entity
func (t *Tagger) Tag(entity string, cardinality collectors.TagCardinality) ([]string, error) {
	tags, err := t.getTags(entity, cardinality)
	if err != nil {
		return nil, err
	}
	return tags.Copy(), nil
}

// Standard returns standard tags for a given entity
// It triggers a tagger fetch if the no tags are found
func (t *Tagger) Standard(entity string) ([]string, error) {
	if entity == "" {
		return nil, fmt.Errorf("empty entity ID")
	}

	return t.tagStore.LookupStandard(entity)
}

// GetEntity returns the entity corresponding to the specified id and an error
func (t *Tagger) GetEntity(entityID string) (*types.Entity, error) {
	return t.tagStore.GetEntity(entityID)
}

// List the content of the tagger
func (t *Tagger) List(cardinality collectors.TagCardinality) tagger_api.TaggerListResponse {
	return t.tagStore.List()
}

// Subscribe returns a channel that receives a slice of events whenever an entity is
// added, modified or deleted. It can send an initial burst of events only to the new
// subscriber, without notifying all of the others.
func (t *Tagger) Subscribe(cardinality collectors.TagCardinality) chan []types.EntityEvent {
	return t.tagStore.Subscribe(cardinality)
}

// Unsubscribe ends a subscription to entity events and closes its channel.
func (t *Tagger) Unsubscribe(ch chan []types.EntityEvent) {
	t.tagStore.Unsubscribe(ch)
}
