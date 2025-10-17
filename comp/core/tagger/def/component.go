// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tagger provides the tagger interface for the Datadog Agent
package tagger

import (
	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// team: container-platform

// Component is the component type.
type Component interface {
	Tag(entityID types.EntityID, cardinality types.TagCardinality) ([]string, error)
	GenerateContainerIDFromOriginInfo(originInfo origindetection.OriginInfo) (string, error)
	AccumulateTagsFor(entityID types.EntityID, cardinality types.TagCardinality, tb tagset.TagsAccumulator) error
	Standard(entityID types.EntityID) ([]string, error)
	List() types.TaggerListResponse
	GetEntity(entityID types.EntityID) (*types.Entity, error)
	// subscriptionID is used for logging and debugging purposes
	Subscribe(subscriptionID string, filter *types.Filter) (types.Subscription, error)
	GetEntityHash(entityID types.EntityID, cardinality types.TagCardinality) string
	AgentTags(cardinality types.TagCardinality) ([]string, error)
	GlobalTags(cardinality types.TagCardinality) ([]string, error)
	EnrichTags(tb tagset.TagsAccumulator, originInfo taggertypes.OriginInfo)
}
