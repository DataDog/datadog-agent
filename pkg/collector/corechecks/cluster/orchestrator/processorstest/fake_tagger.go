// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

//nolint:revive
package processorstest

import (
	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// FakeTagger is a minimal tagger stub for testing BeforeCacheCheck handlers.
// It returns tags only for entity IDs that match the configured map entries,
// ensuring tests validate that the correct entity ID is built.
//
//nolint:revive
type FakeTagger struct {
	TagsByEntityID map[types.EntityID][]string
}

// NewFakeTagger creates a FakeTagger that returns tags keyed by entity ID.
//
//nolint:revive
func NewFakeTagger(tagsByEntityID map[types.EntityID][]string) *FakeTagger {
	return &FakeTagger{TagsByEntityID: tagsByEntityID}
}

// NewEmptyFakeTagger creates a FakeTagger with no configured tags.
//
//nolint:revive
func NewEmptyFakeTagger() *FakeTagger {
	return &FakeTagger{TagsByEntityID: map[types.EntityID][]string{}}
}

// Tag returns the configured tags for the given entity ID.
//
//nolint:revive
func (f *FakeTagger) Tag(entityID types.EntityID, cardinality types.TagCardinality) ([]string, error) {
	return f.TagsByEntityID[entityID], nil
}

// TagWithCompleteness returns nil tags with complete status.
//
//nolint:revive
func (f *FakeTagger) TagWithCompleteness(entityID types.EntityID, cardinality types.TagCardinality) ([]string, bool, error) {
	return nil, true, nil
}

// GenerateContainerIDFromOriginInfo returns an empty string.
//
//nolint:revive
func (f *FakeTagger) GenerateContainerIDFromOriginInfo(originInfo origindetection.OriginInfo) (string, error) {
	return "", nil
}

// Standard returns nil tags.
//
//nolint:revive
func (f *FakeTagger) Standard(entityID types.EntityID) ([]string, error) {
	return nil, nil
}

// List returns an empty response.
//
//nolint:revive
func (f *FakeTagger) List() types.TaggerListResponse {
	return types.TaggerListResponse{}
}

// GetEntity returns nil.
//
//nolint:revive
func (f *FakeTagger) GetEntity(entityID types.EntityID) (*types.Entity, error) {
	return nil, nil
}

// Subscribe returns nil.
//
//nolint:revive
func (f *FakeTagger) Subscribe(subscriptionID string, filter *types.Filter) (types.Subscription, error) {
	return nil, nil
}

// GetEntityHash returns an empty string.
//
//nolint:revive
func (f *FakeTagger) GetEntityHash(entityID types.EntityID, cardinality types.TagCardinality) string {
	return ""
}

// AgentTags returns nil tags.
//
//nolint:revive
func (f *FakeTagger) AgentTags(cardinality types.TagCardinality) ([]string, error) {
	return nil, nil
}

// GlobalTags returns nil tags.
//
//nolint:revive
func (f *FakeTagger) GlobalTags(cardinality types.TagCardinality) ([]string, error) {
	return nil, nil
}

// EnrichTags is a no-op.
//
//nolint:revive
func (f *FakeTagger) EnrichTags(tb tagset.TagsAccumulator, originInfo taggertypes.OriginInfo) {}
