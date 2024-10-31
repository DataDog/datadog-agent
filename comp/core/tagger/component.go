// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tagger implements the Tagger component. The Tagger is the central
// source of truth for client-side entity tagging. It subscribes to workloadmeta
// to get updates for all the entity kinds (containers, kubernetes pods,
// kubernetes nodes, etc.) and extracts the tags for each of them. Tags are then
// stored in memory (by the TagStore) and can be queried by the tagger.Tag()
// method.

// Package tagger provides the tagger component for the Datadog Agent
package tagger

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// team: container-platform

// ReplayTagger interface represent the tagger use for replaying dogstatsd events.
type ReplayTagger interface {
	Component

	// LoadState loads the state of the replay tagger from a list of entities.
	LoadState(state []types.Entity)
}

// Component is the component type.
type Component interface {
	Start(ctx context.Context) error
	Stop() error
	ReplayTagger() ReplayTagger
	GetTaggerTelemetryStore() *telemetry.Store
	// LegacyTag has the same behaviour as the Tag method, but it receives the entity id as a string and parses it.
	// If possible, avoid using this function, and use the Tag method instead.
	// This function exists in order not to break backward compatibility with rtloader and python
	// integrations using the tagger
	LegacyTag(entity string, cardinality types.TagCardinality) ([]string, error)
	Tag(entityID types.EntityID, cardinality types.TagCardinality) ([]string, error)
	AccumulateTagsFor(entityID types.EntityID, cardinality types.TagCardinality, tb tagset.TagsAccumulator) error
	Standard(entityID types.EntityID) ([]string, error)
	List() types.TaggerListResponse
	GetEntity(entityID types.EntityID) (*types.Entity, error)
	// subscriptionID is used for logging and debugging purposes
	Subscribe(subscriptionID string, filter *types.Filter) (types.Subscription, error)
	GetEntityHash(entityID types.EntityID, cardinality types.TagCardinality) string
	AgentTags(cardinality types.TagCardinality) ([]string, error)
	GlobalTags(cardinality types.TagCardinality) ([]string, error)
	SetNewCaptureTagger(newCaptureTagger Component)
	ResetCaptureTagger()
	EnrichTags(tb tagset.TagsAccumulator, originInfo taggertypes.OriginInfo)
	ChecksCardinality() types.TagCardinality
	DogstatsdCardinality() types.TagCardinality
}
