// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tagger implements the Tagger component. The Tagger is the central
// source of truth for client-side entity tagging. It runs Collectors that
// detect entities and collect their tags. Tags are then stored in memory (by
// the TagStore) and can be queried by the tagger.Tag() method. Calling once
// tagger.Init() after the config package is ready is needed to enable
// collection.
package tagger

import (
	"context"

	tagger_api "github.com/DataDog/datadog-agent/pkg/tagger/api"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// Tagger is an interface for transparent access to both localTagger and
// remoteTagger.
type Tagger interface {
	Init(context.Context) error
	Stop() error

	Tag(entity string, cardinality collectors.TagCardinality) ([]string, error)
	AccumulateTagsFor(entity string, cardinality collectors.TagCardinality, tb tagset.TagsAccumulator) error
	Standard(entity string) ([]string, error)
	List(cardinality collectors.TagCardinality) tagger_api.TaggerListResponse
	GetEntity(entityID string) (*types.Entity, error)

	Subscribe(cardinality collectors.TagCardinality) chan []types.EntityEvent
	Unsubscribe(ch chan []types.EntityEvent)
}
