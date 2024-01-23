// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package local

import (
	"context"
	"strconv"
	"sync"

	tagger_api "github.com/DataDog/datadog-agent/pkg/tagger/api"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/tagstore"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
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
	panic("not called")
}

// SetTagsFromInfo allows to set tags from list of TagInfo
func (f *FakeTagger) SetTagsFromInfo(tags []*collectors.TagInfo) {
	panic("not called")
}

// SetError allows to set an error to be returned when `Tag` or `AccumulateTagsFor` is called
// for this entity and cardinality
func (f *FakeTagger) SetError(entity string, cardinality collectors.TagCardinality, err error) {
	panic("not called")
}

// Tagger interface

// Init not implemented in fake tagger
func (f *FakeTagger) Init(context.Context) error {
	panic("not called")
}

// Stop not implemented in fake tagger
func (f *FakeTagger) Stop() error {
	panic("not called")
}

// Tag fake implementation
func (f *FakeTagger) Tag(entity string, cardinality collectors.TagCardinality) ([]string, error) {
	tags := f.store.Lookup(entity, cardinality)

	key := f.getKey(entity, cardinality)
	if err := f.errors[key]; err != nil {
		return nil, err
	}

	return tags, nil
}

// AccumulateTagsFor fake implementation
func (f *FakeTagger) AccumulateTagsFor(entity string, cardinality collectors.TagCardinality, tb tagset.TagsAccumulator) error {
	tags, err := f.Tag(entity, cardinality)
	if err != nil {
		return err
	}

	tb.Append(tags...)
	return nil
}

// Standard fake implementation
func (f *FakeTagger) Standard(entity string) ([]string, error) {
	panic("not called")
}

// GetEntity returns faked entity corresponding to the specified id and an error
func (f *FakeTagger) GetEntity(entityID string) (*types.Entity, error) {
	panic("not called")
}

// List fake implementation
//
//nolint:revive // TODO(CINT) Fix revive linter
func (f *FakeTagger) List(cardinality collectors.TagCardinality) tagger_api.TaggerListResponse {
	panic("not called")
}

// Subscribe fake implementation
func (f *FakeTagger) Subscribe(cardinality collectors.TagCardinality) chan []types.EntityEvent {
	panic("not called")
}

// Unsubscribe fake implementation
func (f *FakeTagger) Unsubscribe(ch chan []types.EntityEvent) {
	panic("not called")
}

// Fake internals
func (f *FakeTagger) getKey(entity string, cardinality collectors.TagCardinality) string {
	return entity + strconv.FormatInt(int64(cardinality), 10)
}
