// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagstore

import (
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// EntityTags holds the tag information for a given entity. It is not
// thread-safe, so should not be shared outside of the store. Usage inside the
// store is safe since it relies on a global lock.
type EntityTags struct {
	entityID           string
	sourceTags         map[string]sourceTags
	cacheValid         bool
	cachedAll          tagset.HashedTags // Low + orchestrator + high
	cachedOrchestrator tagset.HashedTags // Low + orchestrator (subslice of cachedAll)
	cachedLow          tagset.HashedTags // Sub-slice of cachedAll
}

func newEntityTags(entityID string) *EntityTags {
	panic("not called")
}

func (e *EntityTags) getStandard() []string {
	panic("not called")
}

func (e *EntityTags) get(cardinality collectors.TagCardinality) []string {
	panic("not called")
}

func (e *EntityTags) getHashedTags(cardinality collectors.TagCardinality) tagset.HashedTags {
	panic("not called")
}

func (e *EntityTags) toEntity() types.Entity {
	panic("not called")
}

func (e *EntityTags) computeCache() {
	panic("not called")
}

func (e *EntityTags) shouldRemove() bool {
	panic("not called")
}
