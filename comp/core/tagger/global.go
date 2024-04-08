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
	"github.com/DataDog/datadog-agent/comp/core/tagger/replay"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

var (
	// globalTagger is the global tagger instance backing the global Tag functions
	// // TODO(components) (tagger): globalTagger is a legacy global variable but still in use, to be eliminated
	globalTagger *TaggerClient

	// initOnce ensures that the global tagger is only initialized once.  It is reset every
	// time the default tagger is set.
	initOnce sync.Once
)

// SetGlobalTaggerClient sets the global taggerClient instance
func SetGlobalTaggerClient(t *TaggerClient) {
	initOnce.Do(func() {
		globalTagger = t
	})
}

// UnlockGlobalTaggerClient releases the initOnce lock on the global tagger. For testing only.
func UnlockGlobalTaggerClient() {
	initOnce = sync.Once{}
}

// GetEntity returns the hash for the provided entity id.
func GetEntity(entityID string) (*types.Entity, error) {
	if globalTagger == nil {
		return nil, fmt.Errorf("a global tagger must be set before calling GetEntity")
	}
	return globalTagger.GetEntity(entityID)
}

// Tag is an interface function that queries taggerclient singleton
func Tag(entity string, cardinality collectors.TagCardinality) ([]string, error) {
	if globalTagger == nil {
		return nil, fmt.Errorf("a global tagger must be set before calling Tag")
	}
	return globalTagger.Tag(entity, cardinality)
}

// AccumulateTagsFor is an interface function that queries taggerclient singleton
func AccumulateTagsFor(entity string, cardinality collectors.TagCardinality, tb tagset.TagsAccumulator) error {
	if globalTagger == nil {
		return fmt.Errorf("a global tagger must be set before calling AccumulateTagsFor")
	}
	return globalTagger.AccumulateTagsFor(entity, cardinality, tb)
}

// GetEntityHash is an interface function that queries taggerclient singleton
func GetEntityHash(entity string, cardinality collectors.TagCardinality) string {
	if globalTagger != nil {
		return globalTagger.GetEntityHash(entity, cardinality)
	}
	return ""
}

// StandardTags is an interface function that queries taggerclient singleton
func StandardTags(entity string) ([]string, error) {
	if globalTagger == nil {
		return nil, fmt.Errorf("a global tagger must be set before calling StandardTags")
	}
	return globalTagger.Standard(entity)
}

// AgentTags is an interface function that queries taggerclient singleton
func AgentTags(cardinality collectors.TagCardinality) ([]string, error) {
	if globalTagger == nil {
		return nil, fmt.Errorf("a global tagger must be set before calling AgentTags")
	}
	return globalTagger.AgentTags(cardinality)
}

// GlobalTags is an interface function that queries taggerclient singleton
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

// SetNewCaptureTagger will set capture tagger in global tagger instance by using provided capture tagger
func SetNewCaptureTagger(newCaptureTagger *replay.Tagger) {
	if globalTagger != nil {
		globalTagger.SetNewCaptureTagger(newCaptureTagger)
	}
}

// ResetCaptureTagger will reset capture tagger
func ResetCaptureTagger() {
	if globalTagger != nil {
		globalTagger.ResetCaptureTagger()
	}
}

// EnrichTags is an interface function that queries taggerclient singleton
func EnrichTags(tb tagset.TagsAccumulator, originInfo taggertypes.OriginInfo) {
	if globalTagger != nil {
		globalTagger.EnrichTags(tb, originInfo)
	}
}
