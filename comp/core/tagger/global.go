// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagger

import (
	"fmt"
	"sync"

	tagger_api "github.com/DataDog/datadog-agent/comp/core/tagger/api"
	"github.com/DataDog/datadog-agent/comp/core/tagger/collectors"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

var (
	// globalTagger is the global tagger instance backing the global Tag functions
	globalTagger *TaggerClient

	// initOnce ensures that the global tagger is only initialized once.  It is reset every
	// time the default tagger is set.
	initOnce sync.Once
)

// SetGlobalStore sets the global taggerClient instance
func SetGlobalTaggerClient(t *TaggerClient) {
	initOnce.Do(func() {
		globalTagger = t
	})
}

// ResetGlobalTaggerClient resets the global taggerClient instance
func ResetGlobalTaggerClient(t *TaggerClient) {
	initOnce = sync.Once{}
	SetGlobalTaggerClient(t)
}

func GetEntity(entityID string) (*types.Entity, error) {
	if globalTagger == nil {
		return nil, fmt.Errorf("a global tagger must be set before calling GetEntity")
	}
	return globalTagger.GetEntity(entityID)
}

func Tag(entity string, cardinality collectors.TagCardinality) ([]string, error) {
	if globalTagger == nil {
		return nil, fmt.Errorf("a global tagger must be set before calling Tag")
	}
	return globalTagger.Tag(entity, cardinality)
}

func AccumulateTagsFor(entity string, cardinality collectors.TagCardinality, tb tagset.TagsAccumulator) error {
	if globalTagger == nil {
		return fmt.Errorf("a global tagger must be set before calling AccumulateTagsFor")
	}
	return globalTagger.AccumulateTagsFor(entity, cardinality, tb)
}

func GetEntityHash(entity string, cardinality collectors.TagCardinality) string {
	if globalTagger != nil {
		return globalTagger.GetEntityHash(entity, cardinality)
	}
	return ""
}

func StandardTags(entity string) ([]string, error) {
	if globalTagger == nil {
		return nil, fmt.Errorf("a global tagger must be set before calling StandardTags")
	}
	return globalTagger.Standard(entity)
}

func AgentTags(cardinality collectors.TagCardinality) ([]string, error) {
	if globalTagger == nil {
		return nil, fmt.Errorf("a global tagger must be set before calling AgentTags")
	}
	return globalTagger.AgentTags(cardinality)
}

func GlobalTags(cardinality collectors.TagCardinality) ([]string, error) {
	if globalTagger == nil {
		return nil, fmt.Errorf("a global tagger must be set before calling GlobalTags")
	}
	return globalTagger.GlobalTags(cardinality)
}

// List the content of the defaulTagger
func List(cardinality collectors.TagCardinality) tagger_api.TaggerListResponse {
	if globalTagger != nil {
		return globalTagger.List(cardinality)
	}
	return tagger_api.TaggerListResponse{}
}

// GetTaggerInstance returns the global Tagger instance
func GetTaggerInstance() Component {
	return globalTagger
}

func SetNewCaptureTagger() {
	if globalTagger != nil {
		globalTagger.SetNewCaptureTagger()
	}
}

func ResetCaptureTagger() {
	if globalTagger != nil {
		globalTagger.ResetCaptureTagger()
	}
}

func EnrichTags(tb tagset.TagsAccumulator, udsOrigin string, clientOrigin string, cardinalityName string) {
	if globalTagger != nil {
		globalTagger.EnrichTags(tb, udsOrigin, clientOrigin, cardinalityName)
	}
}
