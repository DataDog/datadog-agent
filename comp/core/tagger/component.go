// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tagger implements the Tagger component. The Tagger is the central
// source of truth for client-side entity tagging. It runs Collectors that
// detect entities and collect their tags. Tags are then stored in memory (by
// the TagStore) and can be queried by the tagger.Tag() method.

// Package tagger provides the tagger component for the Datadog Agent
package tagger

import (
	tagger_api "github.com/DataDog/datadog-agent/comp/core/tagger/api"
	"github.com/DataDog/datadog-agent/comp/core/tagger/collectors"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// team: container-integrations

// Component is the component type.
type Component interface {
	Tag(entity string, cardinality collectors.TagCardinality) ([]string, error)
	AccumulateTagsFor(entity string, cardinality collectors.TagCardinality, tb tagset.TagsAccumulator) error
	Standard(entity string) ([]string, error)
	List(cardinality collectors.TagCardinality) tagger_api.TaggerListResponse
	GetEntity(entityID string) (*types.Entity, error)
	Subscribe(cardinality collectors.TagCardinality) chan []types.EntityEvent
	Unsubscribe(ch chan []types.EntityEvent)
}
