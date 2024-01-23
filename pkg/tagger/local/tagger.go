// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package local implements a local Tagger.
package local

import (
	"context"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/tagset"

	tagger_api "github.com/DataDog/datadog-agent/pkg/tagger/api"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/tagstore"
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
	panic("not called")
}

// Init goes through a catalog and tries to detect which are relevant
// for this host. It then starts the collection logic and is ready for
// requests.
func (t *Tagger) Init(ctx context.Context) error {
	panic("not called")
}

// Stop queues a shutdown of Tagger
func (t *Tagger) Stop() error {
	panic("not called")
}

// getTags returns a read only list of tags for a given entity.
func (t *Tagger) getTags(entity string, cardinality collectors.TagCardinality) (tagset.HashedTags, error) {
	panic("not called")
}

// AccumulateTagsFor appends tags for a given entity from the tagger to the TagsAccumulator
func (t *Tagger) AccumulateTagsFor(entity string, cardinality collectors.TagCardinality, tb tagset.TagsAccumulator) error {
	panic("not called")
}

// Tag returns a copy of the tags for a given entity
func (t *Tagger) Tag(entity string, cardinality collectors.TagCardinality) ([]string, error) {
	panic("not called")
}

// Standard returns standard tags for a given entity
// It triggers a tagger fetch if the no tags are found
func (t *Tagger) Standard(entity string) ([]string, error) {
	panic("not called")
}

// GetEntity returns the entity corresponding to the specified id and an error
func (t *Tagger) GetEntity(entityID string) (*types.Entity, error) {
	panic("not called")
}

// List the content of the tagger
//
//nolint:revive // TODO(CINT) Fix revive linter
func (t *Tagger) List(cardinality collectors.TagCardinality) tagger_api.TaggerListResponse {
	panic("not called")
}

// Subscribe returns a channel that receives a slice of events whenever an entity is
// added, modified or deleted. It can send an initial burst of events only to the new
// subscriber, without notifying all of the others.
func (t *Tagger) Subscribe(cardinality collectors.TagCardinality) chan []types.EntityEvent {
	panic("not called")
}

// Unsubscribe ends a subscription to entity events and closes its channel.
func (t *Tagger) Unsubscribe(ch chan []types.EntityEvent) {
	panic("not called")
}
